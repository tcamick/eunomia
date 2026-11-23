package jobs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/store"
)

// fakeRecorder keeps what it was asked to record.
type fakeRecorder struct {
	mu      sync.Mutex
	results []store.JobResult
	err     error
}

func (f *fakeRecorder) RecordJob(_ context.Context, r store.JobResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	return f.err
}

// failing is a job that returns err.
type failing struct{ err error }

func (failing) Name() string                { return "failing" }
func (f failing) Run(context.Context) error { return f.err }

// steppingClock advances by step on each call, so a run has a duration.
func steppingClock(start time.Time, step time.Duration) func() time.Time {
	var mu sync.Mutex
	t := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now := t
		t = t.Add(step)
		return now
	}
}

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

func newRunner(t *testing.T, rec Recorder, js ...Job) (*Runner, *prometheus.Registry) {
	t.Helper()
	registry, err := NewRegistry(js...)
	require.NoError(t, err, "calling NewRegistry")
	reg := prometheus.NewRegistry()
	r, err := NewRunner(registry, rec, reg, steppingClock(t0, 2*time.Second))
	require.NoError(t, err, "calling NewRunner")
	return r, reg
}

func TestRunner_RecordsASuccess(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	r, _ := newRunner(t, rec, Noop{})
	require.NoError(t, r.Run(context.Background(), "noop"), "calling r.Run")
	require.Len(t, rec.results, 1)
	got := rec.results[0]
	assert.Equal(t, store.JobResult{Name: "noop", RunAt: t0, Duration: 2 * time.Second, OK: true}, got)
	assert.InDelta(t, 1, testutil.ToFloat64(r.ok), 0)
	assert.InDelta(t, 2, testutil.ToFloat64(r.duration), 0)
}

func TestRunner_RecordsAFailureAndReturnsIt(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	cause := errors.New("source said 503")
	r, _ := newRunner(t, rec, failing{err: cause})
	err := r.Run(context.Background(), "failing")
	require.ErrorIs(t, err, ErrJobFailed)
	require.ErrorIs(t, err, cause)
	require.Len(t, rec.results, 1, "a failure is recorded too")
	assert.False(t, rec.results[0].OK)
	assert.Equal(t, "source said 503", rec.results[0].Message)
	assert.InDelta(t, 0, testutil.ToFloat64(r.ok), 0)
}

func TestRunner_ReportsARecordingFailure(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{err: errors.New("disk full")}
	r, _ := newRunner(t, rec, Noop{})
	require.ErrorIs(t, r.Run(context.Background(), "noop"), ErrRecord)
}

func TestRunner_UnknownJobRecordsNothing(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	r, _ := newRunner(t, rec, Noop{})
	require.ErrorIs(t, r.Run(context.Background(), "nope"), ErrUnknownJob)
	assert.Empty(t, rec.results, "an unregistered name is never recorded, so the job label stays bounded")
}

func TestNoop_SecondRunChangesOnlyTheTimes(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	r, _ := newRunner(t, rec, Noop{})
	ctx := context.Background()
	require.NoError(t, r.Run(ctx, "noop"), "calling r.Run")
	require.NoError(t, r.Run(ctx, "noop"), "calling r.Run")
	require.Len(t, rec.results, 2)
	a, b := rec.results[0], rec.results[1]
	a.RunAt, b.RunAt = time.Time{}, time.Time{}
	assert.Equal(t, a, b, "two runs differ only in when they ran")
}

func TestNewRegistry_RefusesADuplicateName(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Noop{}, Noop{})
	require.Error(t, err, "expected NewRegistry to fail")
}

// pushed is what a fake Pushgateway received.
type pushed struct {
	method, path, body string
}

func fakeGateway(t *testing.T) (*httptest.Server, func() pushed) {
	t.Helper()
	var (
		mu  sync.Mutex
		got pushed
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = pushed{method: r.Method, path: r.URL.Path, body: string(b)}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, func() pushed { mu.Lock(); defer mu.Unlock(); return got }
}

func TestPush_SendsTheRunsGaugesGroupedByBatch(t *testing.T) {
	t.Parallel()
	gw, got := fakeGateway(t)
	r, reg := newRunner(t, &fakeRecorder{}, Noop{})
	// A family with a job label, and one from outside the app, must not go.
	withJob := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "eunomia_job_test", Help: "h"}, []string{"job"})
	withJob.WithLabelValues("noop").Set(1)
	reg.MustRegister(withJob, collectors.NewGoCollector())
	require.NoError(t, r.Run(context.Background(), "noop"), "calling r.Run")

	require.NoError(t, Push(context.Background(), gw.URL, "noop", reg, gw.Client()), "calling Push")
	p := got()
	assert.Equal(t, http.MethodPut, p.method, "PUT replaces the batch's last run")
	assert.Equal(t, "/metrics/job/eunomia-batch/batch/noop", p.path)
	assert.Contains(t, p.body, "eunomia_batch_last_run_ok")
	assert.Contains(t, p.body, "eunomia_batch_duration_seconds")
	assert.NotContains(t, p.body, "eunomia_job_test", "a family with a grouping label is left out")
	assert.NotContains(t, p.body, "go_goroutines", "the exited pod's runtime is left out")
}

func TestPush_ReportsAGatewayFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, reg := newRunner(t, &fakeRecorder{}, Noop{})
	err := Push(context.Background(), srv.URL, "noop", reg, srv.Client())
	require.Error(t, err, "expected Push to fail")
}

// fakeHistory returns fixed statuses or an error.
type fakeHistory struct {
	statuses []store.JobStatus
	err      error
}

func (f fakeHistory) Jobs(context.Context) ([]store.JobStatus, error) { return f.statuses, f.err }

func TestHistoryCollector_ExportsRegisteredJobsOnly(t *testing.T) {
	t.Parallel()
	registry, err := NewRegistry(Noop{})
	require.NoError(t, err, "calling NewRegistry")
	c := NewHistoryCollector(fakeHistory{statuses: []store.JobStatus{
		{Name: "noop", LastRun: t0, LastOK: t0, OK: true, LastDuration: 1500 * time.Millisecond},
		{Name: "retired", LastRun: t0, OK: false},
	}}, registry)

	want := `
# HELP eunomia_job_history_up 1 if the job history could be read at this scrape, else 0.
# TYPE eunomia_job_history_up gauge
eunomia_job_history_up 1
# HELP eunomia_job_last_duration_seconds Duration of the job's last run. job is a registered job name.
# TYPE eunomia_job_last_duration_seconds gauge
eunomia_job_last_duration_seconds{job="noop"} 1.5
# HELP eunomia_job_last_run_ok 1 if the job's last run succeeded, else 0. job is a registered job name.
# TYPE eunomia_job_last_run_ok gauge
eunomia_job_last_run_ok{job="noop"} 1
`
	require.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(want),
		"eunomia_job_history_up", "eunomia_job_last_duration_seconds", "eunomia_job_last_run_ok"),
		"a row for an unregistered job is not exported")
	n := testutil.CollectAndCount(c, "eunomia_job_last_success_timestamp_seconds")
	assert.Equal(t, 1, n)
}

func TestHistoryCollector_AnUnreadableHistoryIsReportedNotHidden(t *testing.T) {
	t.Parallel()
	registry, err := NewRegistry(Noop{})
	require.NoError(t, err, "calling NewRegistry")
	c := NewHistoryCollector(fakeHistory{err: errors.New("database is locked")}, registry)
	want := `
# HELP eunomia_job_history_up 1 if the job history could be read at this scrape, else 0.
# TYPE eunomia_job_history_up gauge
eunomia_job_history_up 0
`
	require.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(want), "eunomia_job_history_up"), "calling testutil.CollectAndCompare")
}
