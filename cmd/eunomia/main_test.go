package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/config"
	"github.com/tcamick/eunomia/internal/legislators"
	"github.com/tcamick/eunomia/internal/news"
	"github.com/tcamick/eunomia/internal/portraits"
	"github.com/tcamick/eunomia/internal/roster"
	"github.com/tcamick/eunomia/internal/store"
)

func runCmd(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, func(k string) string { return env[k] })
	return code, out.String(), errOut.String()
}

func TestRun_Commands(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		"no command prints usage":          {args: nil, wantCode: exitUsage, wantErr: "usage:"},
		"an unknown command prints usage":  {args: []string{"nope"}, wantCode: exitUsage, wantErr: `unknown command "nope"`},
		"version prints the build version": {args: []string{"version"}, wantCode: 0, wantOut: "dev"},
		"a bad flag value names the flag":  {args: []string{"job", "-log-level=loud"}, wantCode: exitUsage, wantErr: "-log-level:"},
		"job with no name lists the jobs":  {args: []string{"job"}, wantCode: 0, wantOut: "roster"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out, errOut := runCmd(t, nil, tc.args...)
			assert.Equal(t, tc.wantCode, code, "exit code; stderr: %s", errOut)
			assert.Contains(t, out, tc.wantOut)
			assert.Contains(t, errOut, tc.wantErr)
		})
	}
}

func TestRun_JobRecordsItsRunAndPushes(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		paths  []string
		bodies [][]byte
	)
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		bodies = append(bodies, b)
		mu.Unlock()
	}))
	defer gw.Close()
	dir := t.TempDir()

	// portraits on an empty data directory has no member to fetch, so it
	// runs the whole job path (record, push) without the network.
	code, _, errOut := runCmd(t, map[string]string{"EUNOMIA_PUSHGATEWAY_URL": gw.URL}, "job", "-data-dir", dir, "portraits")
	require.Equal(t, 0, code, "stderr: %s", errOut)

	st, err := store.Open(context.Background(), filepath.Join(dir, dbFile), time.Now)
	require.NoError(t, err, "calling store.Open")
	defer func() { _ = st.Close() }()
	jobs, err := st.Jobs(context.Background())
	require.NoError(t, err, "calling st.Jobs")
	require.Len(t, jobs, 1, "the run is in the shared file")
	assert.Equal(t, "portraits", jobs[0].Name)
	assert.True(t, jobs[0].OK)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"PUT /metrics/job/eunomia-batch/batch/portraits"}, paths, "one push, at the end, grouped by the job")
	require.Len(t, bodies, 1, "one push body")
	assert.NotContains(t, string(bodies[0]), "eunomia_roster_changes_total", "the portraits push carries no roster metric")
}

func TestRun_JobTakesOneName(t *testing.T) {
	t.Parallel()
	code, _, errOut := runCmd(t, nil, "job", "-data-dir", t.TempDir(), "roster", "portraits")
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut, "job takes one name")
}

func TestRun_UnknownJobIsAUsageErrorAndRecordsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	code, _, errOut := runCmd(t, nil, "job", "-data-dir", dir, "nope")
	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut, "Refused an unknown job")

	st, err := store.Open(context.Background(), filepath.Join(dir, dbFile), time.Now)
	require.NoError(t, err, "calling store.Open")
	defer func() { _ = st.Close() }()
	jobs, err := st.Jobs(context.Background())
	require.NoError(t, err, "calling st.Jobs")
	assert.Empty(t, jobs)
}

func TestConfig_SourceDefaultsAreTheAdaptersPublishedDirectories(t *testing.T) {
	t.Parallel()
	// config keeps its defaults as literals so it stays a leaf package; this
	// holds them to the adapters' own.
	assert.Equal(t, legislators.DefaultBase, config.DefaultLegislatorsURL)
	assert.Equal(t, portraits.DefaultBase, config.DefaultPortraitsURL)
}

func TestBuildJobs_OnlyTheRunningJobRegistersItsMetrics(t *testing.T) {
	t.Parallel()
	// A pushed family belongs to the job that ran: each job's counter is
	// pushed by its own run, and by no other (go-conventions, Metrics).
	tests := map[string]struct {
		run  string
		want []string
	}{
		"a roster run registers the roster counter": {run: roster.JobName, want: []string{"eunomia_roster_changes_total"}},
		"a news run registers the news counter":     {run: news.JobName, want: []string{"eunomia_news_stories_added_total"}},
		"a portraits run registers neither":         {run: "portraits", want: []string{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reg := prometheus.NewRegistry()
			_, err := buildJobs(deps{reg: reg, run: tc.run})
			require.NoError(t, err, "building the jobs for a %s run", tc.run)
			families, err := reg.Gather()
			require.NoError(t, err, "gathering the registry")
			got := make([]string, 0, len(families))
			for _, f := range families {
				got = append(got, f.GetName())
			}
			assert.Equal(t, tc.want, got, "the families registered for a %s run", tc.run)
		})
	}
}

// The restore drill opens a backup as the server would, migrations
// included: a backup this binary cannot migrate must fail the drill.
func TestOpenVerifier_MigratesTheCopy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.sqlite")
	bare, err := store.Open(ctx, path, time.Now)
	require.NoError(t, err, "creating a database with no migrations applied")
	require.NoError(t, bare.Close(), "closing it")

	v, err := openVerifier(ctx, path)
	require.NoError(t, err, "opening it for the drill")
	t.Cleanup(func() { _ = v.Close() })
	counts, err := v.Verify(ctx)
	require.NoError(t, err, "every verified table exists: the copy was migrated")
	assert.Len(t, counts, len(store.VerifiedTables()), "every table counted")
}
