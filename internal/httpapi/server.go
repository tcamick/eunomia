// Package httpapi serves the web app and its JSON API. Handlers read; the
// only writes (from E2 and E3a) are the owner's visits and reading marks
// (docs/go-conventions.md, Security). Nothing here authenticates: the
// Tailscale Ingress and a NetworkPolicy decide who reaches the server.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/store"
)

// StatusReader lists job outcomes for the status page; *store.Store
// implements it.
type StatusReader interface {
	Jobs(ctx context.Context) ([]store.JobStatus, error)
}

// Options configure the app's handler.
type Options struct {
	Store      StatusReader
	UI         fs.FS
	Version    string
	Now        func() time.Time
	Log        zerolog.Logger
	Registerer prometheus.Registerer
	// Roster serves the roster pages; nil leaves their routes out.
	Roster RosterReader
	// PortraitDir is where the portraits job writes; empty serves none.
	PortraitDir string
	// Jobs are the registered job names; the status page lists only these.
	// Nil lists every recorded job.
	Jobs []string
}

// contentSecurityPolicy keeps every fetch the page makes on this origin
// (docs/web-conventions.md, Security). Inline style attributes stay allowed
// because Svelte's style: directive sets them.
const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
	"script-src 'self'; connect-src 'self'; font-src 'self'; manifest-src 'self'; object-src 'none'; " +
	"base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// companionHeaders go with the CSP on every response: no MIME sniffing, no
// Referer that would tell a publisher the app's tailnet name, no framing, no
// cross-origin window or resource access, and no camera, microphone or
// location.
var companionHeaders = map[string]string{
	"X-Content-Type-Options":       "nosniff",
	"Referrer-Policy":              "no-referrer",
	"X-Frame-Options":              "DENY",
	"Cross-Origin-Opener-Policy":   "same-origin",
	"Cross-Origin-Resource-Policy": "same-origin",
	"Permissions-Policy":           "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
}

// Labels of eunomia_http_requests_total that are not a route pattern.
const (
	routeUnmatched = "unmatched"
	codeAborted    = "aborted"
)

// Status is the /api/status response.
type Status struct {
	Version string      `json:"version"`
	Now     time.Time   `json:"now"`
	Jobs    []JobStatus `json:"jobs"`
	// JobsError says why Jobs is empty when the history could not be read.
	JobsError string `json:"jobsError,omitempty"`
}

// JobStatus is one job on the status page.
type JobStatus struct {
	Name       string    `json:"name"`
	LastRun    time.Time `json:"lastRun"`
	LastOK     time.Time `json:"lastOk,omitzero"`
	OK         bool      `json:"ok"`
	DurationMs int64     `json:"durationMs"`
	Message    string    `json:"message,omitempty"`
}

// New returns the app's handler: the API, the health check and the embedded
// web app, wrapped in the security headers, request logging and metrics.
func New(o Options) (http.Handler, error) {
	if o.Store == nil || o.UI == nil || o.Now == nil || o.Registerer == nil {
		return nil, errors.New("httpapi: Store, UI, Now and Registerer are required")
	}
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "eunomia_http_requests_total",
		Help: "HTTP requests to the app. route is a registered route pattern or unmatched; code is the status code or aborted.",
	}, []string{"route", "code"})
	if err := o.Registerer.Register(requests); err != nil {
		return nil, err
	}
	a := &api{roster: o.Roster, portraitDir: o.PortraitDir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /api/status", statusHandler(o))
	if o.Roster != nil {
		mux.HandleFunc("GET /api/congress", a.congressHandler)
		mux.HandleFunc("GET /api/members", a.membersHandler)
		mux.HandleFunc("GET /api/people/{id}", a.personHandler)
		mux.HandleFunc("GET /api/committees", a.committeesHandler)
		mux.HandleFunc("GET /api/committees/{id}", a.committeeHandler)
		mux.HandleFunc("GET /api/search", a.searchHandler)
		mux.HandleFunc("GET /api/states", a.statesHandler)
		mux.HandleFunc("GET /api/states/{code}", a.stateHandler)
		mux.HandleFunc("GET /portraits/{name}", a.portraitHandler)
	}
	// Every pattern names a method, so the mux never reports a request path
	// as the matched pattern, and the route label stays bounded.
	mux.Handle("GET /", spaHandler(o.UI))
	return observe(o.Log, o.Now, requests, mux), nil
}

// api holds what the data handlers read.
type api struct {
	roster      RosterReader
	portraitDir string
}

// Internal returns the handler for the internal port: /metrics and a health
// check. Only Prometheus reaches it (k8s/app/network.yaml).
func Internal(metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics)
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func statusHandler(o Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := Status{Version: o.Version, Now: o.Now().UTC(), Jobs: []JobStatus{}}
		jobs, err := o.Store.Jobs(r.Context())
		if err != nil {
			zerolog.Ctx(r.Context()).Warn().Err(err).Msg("Could not read job history")
			st.JobsError = "the job history could not be read"
		}
		for _, j := range jobs {
			if o.Jobs != nil && !slices.Contains(o.Jobs, j.Name) {
				continue // a job since removed; its last run is history, not status
			}
			st.Jobs = append(st.Jobs, JobStatus{
				Name: j.Name, LastRun: j.LastRun, LastOK: j.LastOK, OK: j.OK,
				DurationMs: j.LastDuration.Milliseconds(), Message: j.Message,
			})
		}
		writeJSON(w, st)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// spaHandler serves files from ui and answers an extension-less path with
// index.html, so the app's routes deep-link. An unknown asset, a directory
// and an unknown /api/ path stay 404, so a broken reference is seen.
func spaHandler(ui fs.FS) http.Handler {
	files := http.FileServer(http.FS(ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if p == "api" || strings.HasPrefix(p, "api/") {
			http.NotFound(w, r)
			return
		}
		if info, err := fs.Stat(ui, p); err == nil {
			if info.IsDir() {
				http.NotFound(w, r)
				return
			}
			switch {
			case path.Ext(p) == ".webmanifest":
				w.Header().Set("Content-Type", "application/manifest+json")
			case path.Ext(p) == ".html", p == "sw.js":
				// The shell and the service worker must be revalidated so a
				// new build is picked up.
				w.Header().Set("Cache-Control", "no-cache")
			case strings.HasPrefix(p, "assets/"):
				// Vite names these by content hash.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		if path.Ext(p) == "" {
			if _, err := fs.Stat(ui, "index.html"); err == nil {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				w.Header().Set("Cache-Control", "no-cache")
				files.ServeHTTP(w, r2)
				return
			}
		}
		http.NotFound(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code  int
	wrote bool
}

// WriteHeader records the first status code before delegating.
func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.code, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write records that the response has started (an implicit 200).
func (s *statusRecorder) Write(p []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(p)
}

// Unwrap lets http.ResponseController reach the connection.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// observe sets the security headers, puts a request-scoped logger in the
// context, counts the request by route and code, and turns a handler panic
// into a logged, counted 500.
func observe(base zerolog.Logger, now func() time.Time, requests *prometheus.CounterVec, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestID()
		log := base.With().Str("requestId", id).Str("method", r.Method).Str("path", r.URL.Path).Logger()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := now()
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		for k, v := range companionHeaders {
			w.Header().Set(k, v)
		}
		req := r.WithContext(log.WithContext(r.Context()))
		defer func() {
			code := strconv.Itoa(rec.code)
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler { //nolint:errorlint // the sentinel is compared by identity, as net/http does
					code = codeAborted
				} else {
					log.Error().Interface("panic", p).Msg("Recovered handler panic")
					if !rec.wrote {
						http.Error(rec, "internal error", http.StatusInternalServerError)
					}
					code = "500"
				}
			}
			route := req.Pattern
			if route == "" {
				route = routeUnmatched
			}
			requests.WithLabelValues(route, code).Inc()
			log.Debug().Int("status", rec.code).Int64("durationMs", now().Sub(start).Milliseconds()).Msg("Served request")
			if code == codeAborted {
				panic(http.ErrAbortHandler)
			}
		}()
		mux.ServeHTTP(rec, req)
	})
}

// requestID returns 8 random bytes in hex.
func requestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
