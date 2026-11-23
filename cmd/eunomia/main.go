// Command eunomia is the one binary: the server and every batch job.
//
//	eunomia serve [flags]          serve the app, its API and /metrics
//	eunomia job [flags] [name]     run one job and exit; no name lists the jobs
//	eunomia version                print the build version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	curated "github.com/tcamick/eunomia/config"
	"github.com/tcamick/eunomia/internal/config"
	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/httpapi"
	"github.com/tcamick/eunomia/internal/jobs"
	"github.com/tcamick/eunomia/internal/legislators"
	"github.com/tcamick/eunomia/internal/logging"
	"github.com/tcamick/eunomia/internal/news"
	"github.com/tcamick/eunomia/internal/portraits"
	"github.com/tcamick/eunomia/internal/roster"
	"github.com/tcamick/eunomia/internal/store"
	"github.com/tcamick/eunomia/internal/version"
	"github.com/tcamick/eunomia/web"
)

// Exit codes: 2 for a usage or configuration problem, 1 for a failure.
const (
	exitFail  = 1
	exitUsage = 2
)

const usage = `usage:
  eunomia serve [flags]        serve the app, its API and /metrics
  eunomia job [flags] [name]   run one job and exit; no name lists the jobs
  eunomia version              print the build version
Run "eunomia <command> -h" for the flags.`

// dbFile is the SQLite file's name inside the data directory.
const dbFile = "eunomia.sqlite"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version.Version)
		return 0
	case "serve", "job":
	case "-h", "-help", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
		return exitUsage
	}

	fs := flag.NewFlagSet("eunomia "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := config.Bind(fs, getenv)
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "working directory: %v\n", err)
		return exitFail
	}
	if err := cfg.Validate(cwd); err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return exitUsage
	}
	log := logging.New(stderr, cfg.LogLevel, cfg.LogFormat)

	js, err := buildJobs(deps{})
	if err != nil {
		log.Error().Err(err).Msg("Could not build the jobs")
		return exitFail
	}
	registry, err := jobs.NewRegistry(js...)
	if err != nil {
		log.Error().Err(err).Msg("Could not register jobs")
		return exitFail
	}
	if args[0] == "job" && fs.NArg() == 0 {
		for _, n := range registry.Names() {
			fmt.Fprintln(stdout, n)
		}
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx = log.WithContext(ctx)

	st, err := openStore(ctx, cfg.DataDir)
	if err != nil {
		log.Error().Err(err).Msg("Could not open the database")
		return exitFail
	}
	defer func() { _ = st.Close() }()

	if args[0] == "serve" {
		if err := serve(ctx, cfg, st, registry); err != nil {
			log.Error().Err(err).Msg("Server stopped")
			return exitFail
		}
		return 0
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "job takes one name, got %q\n", fs.Args())
		return exitUsage
	}
	return runJob(ctx, cfg, st, fs.Arg(0))
}

// openStore creates the data directory if needed and opens and migrates the
// database.
func openStore(ctx context.Context, dataDir string) (*store.Store, error) {
	// dataDir is the operator's own -data-dir, validated at startup, not
	// outside input.
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // G703: see above

		return nil, fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(ctx, filepath.Join(dataDir, dbFile), time.Now)
	if err != nil {
		return nil, err
	}
	applied, err := st.Migrate(ctx)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	if len(applied) > 0 {
		zerolog.Ctx(ctx).Info().Strs("migrations", applied).Msg("Applied migrations")
	}
	return st, nil
}

// deps are what the jobs are built from. Listing the jobs needs only their
// names, so a zero deps builds them too.
type deps struct {
	st  *store.Store
	get *fetch.Client
	// reg gets the metrics of the job named run, the one this pod runs; a
	// pushed family belongs to the job that ran (go-conventions, Metrics).
	reg     prometheus.Registerer
	run     string
	dataDir string
	// legislatorsURL and portraitsURL are the sources' directories (flags).
	legislatorsURL, portraitsURL string
}

// fetchSources are every source a job may fetch: the values of the fetch
// metric's source label, bounded by the code and config/sources.yaml.
func fetchSources(feeds []news.Feed) []string {
	out := make([]string, 0, 4+len(feeds))
	out = append(out, legislators.SourceLegislators, legislators.SourceCommittees, legislators.SourceMembership, portraits.Source)
	for _, f := range feeds {
		out = append(out, f.ID)
	}
	return out
}

// loadFeeds reads and validates config/sources.yaml (embedded); a bad list
// stops the process at startup (AGENTS.md, Structure rules).
func loadFeeds() ([]news.Feed, error) {
	feeds, err := news.LoadFeeds(curated.FS)
	if err != nil {
		return nil, fmt.Errorf("config/sources.yaml: %w", err)
	}
	return feeds, nil
}

// fetchPace is the gap between two requests to one host. The sources are
// static files on one host (GitHub Pages); half a second keeps the first
// portraits run to about five minutes.
const fetchPace = 500 * time.Millisecond

// buildJobs returns every job, in registration order.
func buildJobs(d deps) ([]jobs.Job, error) {
	if d.legislatorsURL == "" {
		d.legislatorsURL = config.DefaultLegislatorsURL
	}
	if d.portraitsURL == "" {
		d.portraitsURL = config.DefaultPortraitsURL
	}
	src := legislators.NewClient(d.get, d.legislatorsURL, legislators.MinMembers)
	var rosterReg prometheus.Registerer
	if d.run == roster.JobName {
		rosterReg = d.reg
	}
	r, err := roster.NewJob(src, d.st, d.legislatorsURL, time.Now, rosterReg)
	if err != nil {
		return nil, err
	}
	var newsReg prometheus.Registerer
	if d.run == news.JobName {
		newsReg = d.reg
	}
	feeds, err := loadFeeds()
	if err != nil {
		return nil, err
	}
	n, err := news.NewJob(feeds, d.get, d.st, time.Now, newsReg)
	if err != nil {
		return nil, err
	}
	return []jobs.Job{
		r,
		portraits.NewJob(d.get, d.st, d.portraitsURL, filepath.Join(d.dataDir, portraitDir), time.Now),
		n,
	}, nil
}

// portraitDir is the portraits' directory inside the data directory.
const portraitDir = "portraits"

// runJob runs one job, then pushes the run's gauges if a Pushgateway is set.
func runJob(ctx context.Context, cfg *config.Config, st *store.Store, name string) int {
	log := zerolog.Ctx(ctx)
	reg := prometheus.NewRegistry()
	feeds, err := loadFeeds()
	if err != nil {
		log.Error().Err(err).Msg("Could not load the feeds")
		return exitFail
	}
	client, err := fetch.New(fetch.Options{Sources: fetchSources(feeds), Pace: fetchPace, Registerer: reg})
	if err != nil {
		log.Error().Err(err).Msg("Could not build the fetch client")
		return exitFail
	}
	js, err := buildJobs(deps{st: st, get: client, reg: reg, run: name, dataDir: cfg.DataDir,
		legislatorsURL: cfg.LegislatorsURL, portraitsURL: cfg.PortraitsURL})
	if err != nil {
		log.Error().Err(err).Msg("Could not build the jobs")
		return exitFail
	}
	registry, err := jobs.NewRegistry(js...)
	if err != nil {
		log.Error().Err(err).Msg("Could not register jobs")
		return exitFail
	}
	runner, err := jobs.NewRunner(registry, st, reg, time.Now)
	if err != nil {
		log.Error().Err(err).Msg("Could not build the job runner")
		return exitFail
	}
	runErr := runner.Run(ctx, name)
	if errors.Is(runErr, jobs.ErrUnknownJob) {
		log.Error().Str("job", name).Strs("jobs", registry.Names()).Msg("Refused an unknown job")
		return exitUsage
	}
	if cfg.PushgatewayURL != "" {
		if err := jobs.Push(ctx, cfg.PushgatewayURL, name, reg, nil); err != nil {
			log.Warn().Err(err).Str("job", name).Msg("Could not push the run's metrics")
		}
	}
	switch {
	case runErr == nil:
		return 0
	case errors.Is(runErr, jobs.ErrRecord):
		log.Error().Err(runErr).Str("job", name).Msg("Could not record the job's result")
		return exitFail
	default:
		return exitFail // the runner logged the failure
	}
}

// Server timeouts: a slow or stalled client cannot hold a connection open.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 10 * time.Second
)

// serve runs the app's server and the internal metrics server until ctx ends.
func serve(ctx context.Context, cfg *config.Config, st *store.Store, registry *jobs.Registry) error {
	log := zerolog.Ctx(ctx)
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		jobs.NewHistoryCollector(st, registry),
	)
	feeds, err := loadFeeds()
	if err != nil {
		return err
	}
	app, err := httpapi.New(httpapi.Options{
		Store: st, UI: web.FS(), Version: version.Version, Now: time.Now, Log: *log, Registerer: reg,
		Roster: st, PortraitDir: filepath.Join(cfg.DataDir, portraitDir), Jobs: registry.Names(),
		News: st, Feeds: feeds,
	})
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}
	internal := httpapi.Internal(promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	servers := []*http.Server{newServer(ctx, cfg.Listen, app), newServer(ctx, cfg.InternalListen, internal)}
	errc := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("listen %s: %w", s.Addr, err)
				return
			}
			errc <- nil
		}()
	}
	log.Info().Str("listen", cfg.Listen).Str("internalListen", cfg.InternalListen).
		Str("dataDir", cfg.DataDir).Str("version", version.Version).Msg("Started server")

	var firstErr error
	select {
	case <-ctx.Done():
	case firstErr = <-errc:
	}
	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutCtx)
	}
	log.Info().Msg("Stopped server")
	return firstErr
}

// newServer builds a server with the timeouts above. Requests get the process
// context's values (the logger) but not its cancellation, so a shutdown
// drains them instead of cutting them off.
func newServer(ctx context.Context, addr string, h http.Handler) *http.Server {
	base := context.WithoutCancel(ctx)
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
}
