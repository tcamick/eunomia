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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		mu    sync.Mutex
		paths []string
	)
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
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
