package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/version"
)

// fakeClock is the client's clock and sleep: Wait records each duration and
// moves the clock forward by it, so nothing sleeps.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Wait(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waits = append(f.waits, d)
	if d > 0 {
		f.now = f.now.Add(d)
	}
	return ctx.Err()
}

func (f *fakeClock) Waits() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.waits...)
}

// newClient returns a client for the test server srv with the source "test",
// a fake clock, no jitter, and its own metrics registry.
func newClient(t *testing.T, srv *httptest.Server, mod func(*Options)) (*Client, *fakeClock, *prometheus.Registry) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	reg := prometheus.NewRegistry()
	o := Options{
		Sources:     []string{"test"},
		Transport:   srv.Client().Transport,
		Now:         clock.Now,
		Wait:        clock.Wait,
		Jitter:      func(time.Duration) time.Duration { return 0 },
		Registerer:  reg,
		Pace:        time.Second,
		BaseBackoff: 2 * time.Second,
		MaxBackoff:  time.Minute,
	}
	if mod != nil {
		mod(&o)
	}
	c, err := New(o)
	require.NoError(t, err, "building the client")
	return c, clock, reg
}

func TestGet_SendsOnlyTheGenericUserAgent(t *testing.T) {
	t.Parallel()
	var got http.Header
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, _, _ := newClient(t, srv, nil)

	resp, err := c.Get(context.Background(), "test", srv.URL+"/feed", Validators{})
	require.NoError(t, err, "fetching from the test server")
	assert.Equal(t, "ok", string(resp.Body))
	assert.Equal(t, "eunomia/"+version.Version, got.Get("User-Agent"), "the program and its version, nothing more")
	assert.Equal(t, got.Get("User-Agent"), UserAgent())
	for _, h := range []string{"From", "Cookie", "Authorization", "Referer"} {
		assert.Empty(t, got.Get(h), "no %s header may identify the owner", h)
	}
}

func TestGet_ConditionalRequests(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Sun, 22 Nov 2026 08:00:00 GMT")
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()
	c, _, reg := newClient(t, srv, nil)
	ctx := context.Background()

	first, err := c.Get(ctx, "test", srv.URL, Validators{})
	require.NoError(t, err, "calling c.Get")
	assert.False(t, first.NotModified)
	assert.Equal(t, Validators{ETag: `"v1"`, LastModified: "Sun, 22 Nov 2026 08:00:00 GMT"}, first.Validators)

	second, err := c.Get(ctx, "test", srv.URL, first.Validators)
	require.NoError(t, err, "calling c.Get")
	assert.True(t, second.NotModified, "unchanged source answers 304")
	assert.Empty(t, second.Body)
	assert.Equal(t, first.Validators, second.Validators, "a 304 keeps the validators that were sent")

	assert.InDelta(t, 1, testutil.ToFloat64(c.requests.WithLabelValues("test", resultOK)), 0)
	assert.InDelta(t, 1, testutil.ToFloat64(c.requests.WithLabelValues("test", resultNotModified)), 0)
	n, err := testutil.GatherAndCount(reg, "eunomia_fetch_duration_seconds")
	require.NoError(t, err, "calling testutil.GatherAndCount")
	assert.Equal(t, 1, n, "one histogram series, for the one source")
}

func TestGet_Retries(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		statuses   []int
		retryAfter string
		wantErr    bool
		wantCalls  int
		wantWaits  []time.Duration // backoff waits only; pacing waits are filtered out
	}{
		"a 503 then success retries once after the base backoff": {
			statuses: []int{503, 200}, wantCalls: 2, wantWaits: []time.Duration{2 * time.Second},
		},
		"a 429 with Retry-After waits what the source asked": {
			statuses: []int{429, 200}, retryAfter: "7", wantCalls: 2, wantWaits: []time.Duration{7 * time.Second},
		},
		"backoff doubles and gives up after four attempts": {
			statuses: []int{500, 500, 500, 500}, wantErr: true, wantCalls: 4,
			wantWaits: []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second},
		},
		"a 404 is not retried": {
			statuses: []int{404}, wantErr: true, wantCalls: 1,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.statuses[i])
			}))
			defer srv.Close()
			// A pace of zero after the default is applied would still be 1s;
			// a pace far shorter than any backoff keeps the two apart.
			c, clock, _ := newClient(t, srv, func(o *Options) { o.Pace = time.Millisecond })

			_, err := c.Get(context.Background(), "test", srv.URL, Validators{})
			if tc.wantErr {
				require.Error(t, err, "expected c.Get to fail")
			} else {
				require.NoError(t, err, "calling c.Get")
			}
			assert.Equal(t, tc.wantCalls, int(calls.Load()), "attempts made")
			var backoffs []time.Duration
			for _, w := range clock.Waits() {
				if w > time.Millisecond {
					backoffs = append(backoffs, w)
				}
			}
			assert.Equal(t, tc.wantWaits, backoffs)
		})
	}
}

func TestGet_StatusErrorCarriesTheCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c, _, _ := newClient(t, srv, nil)
	_, err := c.Get(context.Background(), "test", srv.URL, Validators{})
	var se *StatusError
	require.ErrorAs(t, err, &se, "fetching from a source that answers with an error status")
	assert.Equal(t, http.StatusForbidden, se.Code)
}

func TestGet_RefusesABodyOverTheLimit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 101)))
	}))
	defer srv.Close()
	c, _, _ := newClient(t, srv, func(o *Options) { o.MaxBytes = 100 })
	_, err := c.Get(context.Background(), "test", srv.URL, Validators{})
	require.ErrorIs(t, err, ErrTooLarge, "fetching a body over MaxBytes")
	assert.InDelta(t, 1, testutil.ToFloat64(c.requests.WithLabelValues("test", resultTooLarge)), 0)
}

func TestGet_RefusesWhatIsNotHTTPS(t *testing.T) {
	t.Parallel()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plain"))
	}))
	defer plain.Close()
	redirecting := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer redirecting.Close()
	c, _, _ := newClient(t, redirecting, nil)
	ctx := context.Background()

	_, err := c.Get(ctx, "test", plain.URL, Validators{})
	require.ErrorIs(t, err, ErrNotHTTPS, "an http URL is refused before any request")

	_, err = c.Get(ctx, "test", redirecting.URL, Validators{})
	require.ErrorIs(t, err, ErrNotHTTPS, "a redirect to http is refused")
	assert.InDelta(t, 1, testutil.ToFloat64(c.requests.WithLabelValues("test", resultTransportError)), 0,
		"a refused redirect is not retried")
}

func TestGet_RefusesAnUnknownSource(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request should be made for an unknown source")
	}))
	defer srv.Close()
	c, _, reg := newClient(t, srv, nil)
	_, err := c.Get(context.Background(), "https://example.org/whatever", srv.URL, Validators{})
	require.ErrorIs(t, err, ErrUnknownSource, "fetching under a source name that is not configured")
	n, err := testutil.GatherAndCount(reg, "eunomia_fetch_requests_total")
	require.NoError(t, err, "calling testutil.GatherAndCount")
	assert.Zero(t, n, "the source label never takes a value outside the configured list")
}

func TestGet_PacesRequestsToOneHost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, clock, _ := newClient(t, srv, func(o *Options) { o.Pace = 3 * time.Second })
	ctx := context.Background()
	for range 3 {
		_, err := c.Get(ctx, "test", srv.URL, Validators{})
		require.NoError(t, err, "calling c.Get")
	}
	// The fake clock does not move between requests, so the second and
	// third each wait out the pace from the slot before.
	assert.Equal(t, []time.Duration{0, 3 * time.Second, 3 * time.Second}, clock.Waits())
}

func TestGet_StopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	// The pacing wait before the first attempt passes; the job is stopped
	// during the backoff after the 503.
	c, _, _ := newClient(t, srv, func(o *Options) {
		o.Wait = func(ctx context.Context, d time.Duration) error {
			if d >= o.BaseBackoff {
				cancel()
			}
			return ctx.Err()
		}
	})
	_, err := c.Get(ctx, "test", srv.URL, Validators{})
	require.ErrorIs(t, err, context.Canceled, "the caller's cancellation is reported")
	assert.Equal(t, int32(1), calls.Load(), "one attempt, then the backoff was cut short")
}

func TestGet_ATransportErrorDoesNotPrintTheQuery(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	c, _, _ := newClient(t, srv, func(o *Options) { o.MaxAttempts = 1 })
	addr := srv.URL
	srv.Close() // nothing listens there now, so the request fails in transport

	_, err := c.Get(context.Background(), "test", addr+"/v3/bill?api_key=SECRET-KEY&format=json", Validators{})
	require.Error(t, err, "a request to a closed server fails")
	assert.NotContains(t, err.Error(), "SECRET-KEY", "an API key in the query never reaches a log or the status page")
	assert.NotContains(t, err.Error(), "api_key")
	assert.Contains(t, err.Error(), "/v3/bill", "the path is kept, so the failure still says what was asked")
}

func TestGet_RateLimitIsCountedApart(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c, _, _ := newClient(t, srv, func(o *Options) { o.MaxAttempts = 1 })
	_, err := c.Get(context.Background(), "test", srv.URL, Validators{})
	require.Error(t, err, "a 429 with no retries left fails")
	assert.InDelta(t, 1, testutil.ToFloat64(c.requests.WithLabelValues("test", resultRateLimited)), 0,
		"being rate-limited is not shown as a source being down")
	assert.InDelta(t, 0, testutil.ToFloat64(c.requests.WithLabelValues("test", resultServerError)), 0)
}

func TestNew_RequiresSourcesAndARegisterer(t *testing.T) {
	t.Parallel()
	_, err := New(Options{Registerer: prometheus.NewRegistry()})
	require.Error(t, err, "a client with no sources is refused")
	_, err = New(Options{Sources: []string{"x"}})
	require.Error(t, err, "a client with nowhere to put its metrics is refused")
}
