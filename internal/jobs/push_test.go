package jobs

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusingTransport refuses the first refusals connections, as a network
// policy not yet in force for a new pod does, then passes requests on.
type refusingTransport struct {
	mu       sync.Mutex
	refusals int
	attempts int
	next     http.RoundTripper
}

func (r *refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.attempts++
	refuse := r.attempts <= r.refusals
	r.mu.Unlock()
	if refuse {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}
	return r.next.RoundTrip(req)
}

// noNetwork stands behind a transport that should refuse every time: a
// request reaching it fails the test instead of leaving the machine.
type noNetwork struct{ t *testing.T }

func (n noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.t.Error("a request got past the refusals to the network")
	return nil, errors.New("no network in tests")
}

// waits records the pauses asked for, without sleeping.
type waits struct {
	mu  sync.Mutex
	got []time.Duration
}

func (w *waits) wait(_ context.Context, d time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.got = append(w.got, d)
	return nil
}

var testWaits = []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}

// A job that ends within moments of its pod starting can push before the
// pod's network policy is in force, and be refused (E2, 2026-11-24 UTC: the
// restore check, 60 ms, was refused twice). A refused connection is tried
// again; a gateway's answer is not.
func TestPush_RefusedConnections(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		refusals     int
		waitFails    bool
		wantAttempts int
		wantWaits    []time.Duration
		wantErr      bool
		wantLog      string // in the Info line; empty: no line at all
	}{
		"two refusals, then the push gets through, and says so": {
			refusals: 2, wantAttempts: 3, wantWaits: testWaits[:2], wantLog: `"attempts":3`,
		},
		"through at once: nothing logged": {
			refusals: 0, wantAttempts: 1,
		},
		"refused every time: one attempt, then one after each wait": {
			refusals: 100, wantAttempts: len(testWaits) + 1, wantWaits: testWaits, wantErr: true,
		},
		"a wait cut short by the deadline ends the tries": {
			refusals: 100, waitFails: true, wantAttempts: 1, wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gw, got := fakeGateway(t)
			_, reg := newRunner(t, &fakeRecorder{}, Noop{})
			var next http.RoundTripper = noNetwork{t}
			if !tc.wantErr {
				next = gw.Client().Transport
			}
			tr := &refusingTransport{refusals: tc.refusals, next: next}
			w := &waits{}
			wait := w.wait
			if tc.waitFails {
				wait = func(context.Context, time.Duration) error { return context.DeadlineExceeded }
			}

			var logged bytes.Buffer
			ctx := zerolog.New(&logged).WithContext(context.Background())

			attempts, err := pushWith(ctx, gw.URL, "noop", reg, &http.Client{Transport: tr}, testWaits, wait)

			assert.Equal(t, tc.wantAttempts, attempts, "attempts made")
			assert.Equal(t, tc.wantAttempts, tr.attempts, "attempts that reached the transport")
			assert.Equal(t, tc.wantWaits, w.got, "a pause before each new attempt")
			if tc.wantErr {
				require.ErrorIs(t, err, syscall.ECONNREFUSED, "the error says the connection was refused")
				return
			}
			require.NoError(t, err, "the push gets through")
			assert.Equal(t, http.MethodPut, got().method, "the gateway received it")
			if tc.wantLog == "" {
				assert.Empty(t, logged.String(), "a first-try push logs nothing")
				return
			}
			assert.Contains(t, logged.String(), "Pushed the run's metrics after a refused connection", "the retry is logged")
			assert.Contains(t, logged.String(), tc.wantLog, "with its attempts")
		})
	}
}

func TestPush_DoesNotRetryAGatewaysAnswer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, reg := newRunner(t, &fakeRecorder{}, Noop{})
	w := &waits{}

	attempts, err := pushWith(context.Background(), srv.URL, "noop", reg, srv.Client(), testWaits, w.wait)

	require.Error(t, err, "a 500 is a failure")
	assert.Equal(t, 1, attempts, "and is not retried")
	assert.Empty(t, w.got, "with no pause")
}

func TestSleep_ReturnsWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, sleep(ctx, time.Hour), context.Canceled, "an hour's wait ends with its context")
}

func TestPushWaits_FitInsideThePushTimeout(t *testing.T) {
	t.Parallel()
	var total time.Duration
	for _, d := range pushWaits() {
		total += d
	}
	assert.Less(t, total, pushTimeout, "every wait inside the limit")
}
