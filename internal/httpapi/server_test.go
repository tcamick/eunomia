package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/store"
)

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

type fakeStatus struct {
	jobs []store.JobStatus
	err  error
}

func (f fakeStatus) Jobs(context.Context) ([]store.JobStatus, error) { return f.jobs, f.err }

var ui = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>Eunomia</title>")},
	"assets/app-abc1.js":   {Data: []byte("console.log(1)")},
	"manifest.webmanifest": {Data: []byte("{}")},
	"assets/fonts/x.woff2": {Data: []byte("font")},
}

func newHandler(t *testing.T, st StatusReader) (http.Handler, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	h, err := New(Options{Store: st, UI: ui, Version: "v-test", Now: func() time.Time { return t0 }, Log: zerolog.Nop(), Registerer: reg})
	require.NoError(t, err)
	return h, reg
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	return rec.Result()
}

func TestStatus_ListsJobs(t *testing.T) {
	t.Parallel()
	h, _ := newHandler(t, fakeStatus{jobs: []store.JobStatus{
		{Name: "noop", LastRun: t0, LastOK: t0, OK: true, LastDuration: 1200 * time.Millisecond},
	}})
	resp := get(t, h, "/api/status")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var st Status
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&st))
	assert.Equal(t, "v-test", st.Version)
	require.Len(t, st.Jobs, 1)
	assert.Equal(t, JobStatus{Name: "noop", LastRun: t0, LastOK: t0, OK: true, DurationMs: 1200}, st.Jobs[0])
	assert.Empty(t, st.JobsError)
}

func TestStatus_AnUnreadableHistoryIsSaidNotHidden(t *testing.T) {
	t.Parallel()
	h, _ := newHandler(t, fakeStatus{err: errors.New("database is locked")})
	resp := get(t, h, "/api/status")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var st Status
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&st))
	assert.Empty(t, st.Jobs)
	assert.NotEmpty(t, st.JobsError)
	assert.NotContains(t, st.JobsError, "locked", "the store's own error stays in the log")
}

func TestSPA_Paths(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		path      string
		wantCode  int
		wantShell bool
		wantType  string
	}{
		"the root is the shell":                     {path: "/", wantCode: 200, wantShell: true},
		"an app route deep-links to the shell":      {path: "/people/A000001", wantCode: 200, wantShell: true},
		"a built asset is served":                   {path: "/assets/app-abc1.js", wantCode: 200},
		"an unknown asset is a 404, not the shell":  {path: "/assets/missing.js", wantCode: 404},
		"an unknown API path is a 404":              {path: "/api/nope", wantCode: 404},
		"a directory is not listed":                 {path: "/assets/fonts", wantCode: 404},
		"the manifest has the type browsers expect": {path: "/manifest.webmanifest", wantCode: 200, wantType: "application/manifest+json"},
	}
	h, _ := newHandler(t, fakeStatus{})
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := get(t, h, tc.path)
			assert.Equal(t, tc.wantCode, resp.StatusCode, "GET %s", tc.path)
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, tc.wantShell, strings.Contains(string(body), "<title>Eunomia</title>"), "GET %s served the shell", tc.path)
			if tc.wantType != "" {
				assert.Equal(t, tc.wantType, resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestSecurityHeaders_OnEveryResponse(t *testing.T) {
	t.Parallel()
	h, _ := newHandler(t, fakeStatus{})
	for _, p := range []string{"/", "/api/status", "/healthz", "/assets/missing.js", "/api/nope"} {
		resp := get(t, h, p)
		assert.Equal(t, contentSecurityPolicy, resp.Header.Get("Content-Security-Policy"), "CSP on %s", p)
		assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "default-src 'self'")
		for k, v := range companionHeaders {
			assert.Equal(t, v, resp.Header.Get(k), "%s on %s", k, p)
		}
		assert.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"), "no Referer leaves for a publisher (%s)", p)
	}
}

func TestRequestsMetric_LabelsAreRoutePatternsNotPaths(t *testing.T) {
	t.Parallel()
	h, reg := newHandler(t, fakeStatus{})
	get(t, h, "/people/A000001")
	get(t, h, "/people/B000002")
	get(t, h, "/api/status")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/status", nil))

	want := `
# HELP eunomia_http_requests_total HTTP requests to the app. route is a registered route pattern or unmatched; code is the status code or aborted.
# TYPE eunomia_http_requests_total counter
eunomia_http_requests_total{code="200",route="GET /"} 2
eunomia_http_requests_total{code="200",route="GET /api/status"} 1
eunomia_http_requests_total{code="405",route="unmatched"} 1
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(want), "eunomia_http_requests_total"),
		"no path, id or person ever becomes a label value")
}

func TestObserve_APanicIsA500AndCounted(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "eunomia_http_requests_total", Help: "h"}, []string{"route", "code"})
	reg.MustRegister(requests)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	h := observe(zerolog.Nop(), func() time.Time { return t0 }, requests, mux)
	resp := get(t, h, "/boom")
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.InDelta(t, 1, testutil.ToFloat64(requests.WithLabelValues("GET /boom", "500")), 0)
}

func TestInternal_ServesMetricsAndHealth(t *testing.T) {
	t.Parallel()
	h := Internal(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("metrics")) }))
	assert.Equal(t, http.StatusOK, get(t, h, "/metrics").StatusCode)
	assert.Equal(t, http.StatusOK, get(t, h, "/healthz").StatusCode)
	assert.Equal(t, http.StatusNotFound, get(t, h, "/api/status").StatusCode, "the internal port serves no app")
}
