// Package fetch is the only way Eunomia reaches an outside service
// (docs/go-conventions.md, Fetching from public sources). Every request gets a
// timeout, a body size limit, a generic User-Agent, conditional headers, a
// pace between requests to one host, backoff on 429 and 5xx, and a counter
// and histogram by source, so no adapter can forget them.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Validators are what a source sent to let the next request ask "has it
// changed?": the ETag and Last-Modified headers.
type Validators struct {
	ETag         string
	LastModified string
}

// Response is a completed request.
type Response struct {
	// Status is the final HTTP status: 200, or 304 when NotModified.
	Status int
	// NotModified is true when the source answered 304 to the conditional
	// headers; Body is then empty and the caller keeps what it stored.
	NotModified bool
	// Body is the whole response body, at most Options.MaxBytes long.
	Body []byte
	// Validators are this response's, for the next request. On a 304 they
	// are the ones sent, unless the source sent new ones.
	Validators Validators
}

var (
	// ErrUnknownSource is a source name that was not given to New. It keeps
	// the metric's source label to the values listed when the code was
	// written.
	ErrUnknownSource = errors.New("unknown source")
	// ErrNotHTTPS is a URL, or a redirect, that is not https.
	ErrNotHTTPS = errors.New("not an https URL")
	// ErrTooLarge is a body longer than Options.MaxBytes.
	ErrTooLarge = errors.New("response too large")
)

// StatusError is a final response that was neither 200 nor 304.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string { return "unexpected status " + strconv.Itoa(e.Code) }

// Options configure a Client. Zero values take the defaults below.
type Options struct {
	// Sources are the names a request may carry; the metric label's values.
	Sources []string
	// UserAgent identifies the program and nothing else: eunomia/<version>,
	// never the owner's name, email or address (AGENTS.md).
	UserAgent string
	// Timeout bounds one attempt, connection to last byte. Default 30s.
	Timeout time.Duration
	// MaxBytes bounds a body. Default 32 MiB.
	MaxBytes int64
	// Pace is the least time between two requests to one host. Default 1s.
	Pace time.Duration
	// MaxAttempts bounds tries of one request, the first included. Default 4.
	MaxAttempts int
	// BaseBackoff is the wait after the first failed attempt, doubled after
	// each later one. Default 2s.
	BaseBackoff time.Duration
	// MaxBackoff caps one wait, including a source's Retry-After. Default 1m.
	MaxBackoff time.Duration
	// Transport sends requests; nil uses http.DefaultTransport. Tests pass
	// an httptest TLS server's transport.
	Transport http.RoundTripper
	// Now and Wait are the clock and the sleep, injected so tests neither
	// sleep nor depend on the wall clock. Wait returns early with ctx's
	// error when ctx ends.
	Now  func() time.Time
	Wait func(ctx context.Context, d time.Duration) error
	// Jitter returns a duration in [0, d) added to each backoff, so retries
	// from several jobs do not line up.
	Jitter func(d time.Duration) time.Duration
	// Registerer takes the client's metrics.
	Registerer prometheus.Registerer
}

// Client fetches from the configured sources. It is safe for concurrent use.
type Client struct {
	o        Options
	http     *http.Client
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec

	mu   sync.Mutex
	next map[string]time.Time // per host: the earliest the next request may start
}

// Results of one attempt, the values of the result label.
const (
	resultOK             = "ok"
	resultNotModified    = "not_modified"
	resultClientError    = "client_error"
	resultServerError    = "server_error"
	resultTransportError = "transport_error"
	resultTooLarge       = "too_large"
)

const maxRedirects = 5

// New returns a client for o.Sources with its metrics registered.
func New(o Options) (*Client, error) {
	if len(o.Sources) == 0 {
		return nil, errors.New("fetch: no sources")
	}
	if o.UserAgent == "" {
		return nil, errors.New("fetch: no user agent")
	}
	if o.Registerer == nil {
		return nil, errors.New("fetch: no metrics registerer")
	}
	setDefaults(&o)
	c := &Client{
		o:    o,
		next: map[string]time.Time{},
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "eunomia_fetch_requests_total",
			Help: "Requests to outside sources, one per attempt. source is a configured source name; result is ok, " +
				"not_modified, client_error, server_error, transport_error or too_large.",
		}, []string{"source", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "eunomia_fetch_duration_seconds",
			Help:    "Duration of one attempt, connection to last byte. source is a configured source name.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2.5, 8), // 50ms .. ~30s
		}, []string{"source"}),
	}
	c.http = &http.Client{
		Transport: o.Transport,
		Timeout:   o.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to %s: %w", req.URL.Redacted(), ErrNotHTTPS)
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	for _, col := range []prometheus.Collector{c.requests, c.duration} {
		if err := o.Registerer.Register(col); err != nil {
			return nil, fmt.Errorf("register fetch metrics: %w", err)
		}
	}
	return c, nil
}

func setDefaults(o *Options) {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 32 << 20
	}
	if o.Pace <= 0 {
		o.Pace = time.Second
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 4
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 2 * time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = time.Minute
	}
	if o.Transport == nil {
		o.Transport = http.DefaultTransport
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Wait == nil {
		o.Wait = sleep
	}
	if o.Jitter == nil {
		o.Jitter = func(d time.Duration) time.Duration {
			if d <= 0 {
				return 0
			}
			return rand.N(d) //nolint:gosec // spreads retries in time; nothing secret depends on it
		}
	}
}

// sleep waits d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Get fetches rawURL for source, sending prev as conditional headers. It
// retries 429, 5xx and transport errors with backoff, and returns a 304 as a
// Response with NotModified set. Any other non-200 is a *StatusError.
func (c *Client) Get(ctx context.Context, source, rawURL string, prev Validators) (*Response, error) {
	if !slices.Contains(c.o.Sources, source) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSource, source)
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("%s: %w", source, ErrNotHTTPS)
	}
	var lastErr error
	for attempt := range c.o.MaxAttempts {
		if attempt > 0 {
			if err := c.o.Wait(ctx, c.backoff(attempt, lastErr)); err != nil {
				return nil, fmt.Errorf("%s: %w", source, err)
			}
		}
		if err := c.waitTurn(ctx, u.Host); err != nil {
			return nil, fmt.Errorf("%s: %w", source, err)
		}
		resp, retryable, err := c.attempt(ctx, source, u, prev)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("%s: %w", source, lastErr)
}

// retryAfterError carries a source's Retry-After with the status that sent it.
type retryAfterError struct {
	*StatusError
	after time.Duration
}

func (e *retryAfterError) Unwrap() error { return e.StatusError }

// backoff is the wait before attempt (1-based after the first): the source's
// Retry-After when it sent one, else BaseBackoff doubled per attempt, plus
// jitter; capped at MaxBackoff.
func (c *Client) backoff(attempt int, lastErr error) time.Duration {
	var ra *retryAfterError
	if errors.As(lastErr, &ra) && ra.after > 0 {
		return min(ra.after, c.o.MaxBackoff)
	}
	d := c.o.BaseBackoff << (attempt - 1)
	if d <= 0 || d > c.o.MaxBackoff { // <= 0: the shift overflowed
		d = c.o.MaxBackoff
	}
	return min(d+c.o.Jitter(d/2), c.o.MaxBackoff)
}

// waitTurn holds the request until Pace has passed since the last one to
// host. The slot is reserved before waiting, so concurrent callers queue.
func (c *Client) waitTurn(ctx context.Context, host string) error {
	c.mu.Lock()
	now := c.o.Now()
	start := now
	if n, ok := c.next[host]; ok && n.After(now) {
		start = n
	}
	c.next[host] = start.Add(c.o.Pace)
	c.mu.Unlock()
	return c.o.Wait(ctx, start.Sub(now))
}

// attempt makes one request and reports whether a failure may be retried.
func (c *Client) attempt(ctx context.Context, source string, u *url.URL, prev Validators) (*Response, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.o.UserAgent)
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	if prev.LastModified != "" {
		req.Header.Set("If-Modified-Since", prev.LastModified)
	}
	start := c.o.Now()
	resp, err := c.http.Do(req) //nolint:gosec // G107: the URL is a configured source's, checked https above
	if err != nil {
		c.observe(source, resultTransportError, start)
		// A redirect off https is a refusal, not weather; do not retry it.
		return nil, !errors.Is(err, ErrNotHTTPS), fmt.Errorf("get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		c.observe(source, resultNotModified, start)
		return &Response{Status: resp.StatusCode, NotModified: true, Validators: merge(prev, resp.Header)}, false, nil
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, c.o.MaxBytes+1))
		if err != nil {
			c.observe(source, resultTransportError, start)
			return nil, true, fmt.Errorf("read body: %w", err)
		}
		if int64(len(body)) > c.o.MaxBytes {
			c.observe(source, resultTooLarge, start)
			return nil, false, fmt.Errorf("over %d bytes: %w", c.o.MaxBytes, ErrTooLarge)
		}
		c.observe(source, resultOK, start)
		return &Response{Status: resp.StatusCode, Body: body, Validators: merge(Validators{}, resp.Header)}, false, nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		c.observe(source, resultServerError, start)
		se := &StatusError{Code: resp.StatusCode}
		return nil, true, &retryAfterError{StatusError: se, after: retryAfter(resp.Header.Get("Retry-After"))}
	default:
		c.observe(source, resultClientError, start)
		return nil, false, &StatusError{Code: resp.StatusCode}
	}
}

func (c *Client) observe(source, result string, start time.Time) {
	c.requests.WithLabelValues(source, result).Inc()
	c.duration.WithLabelValues(source).Observe(c.o.Now().Sub(start).Seconds())
}

// merge returns prev updated with any validators in h.
func merge(prev Validators, h http.Header) Validators {
	if v := h.Get("ETag"); v != "" {
		prev.ETag = v
	}
	if v := h.Get("Last-Modified"); v != "" {
		prev.LastModified = v
	}
	return prev
}

// retryAfter reads a Retry-After given in seconds; an HTTP date or anything
// else is ignored and the client's own backoff applies.
func retryAfter(v string) time.Duration {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}
