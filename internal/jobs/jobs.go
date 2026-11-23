// Package jobs defines the unit of batch work and the runner that records it.
// Each job runs as `eunomia job <name>` in its own pod, started by a
// Kubernetes CronJob (ADR 0003); the runner records the outcome in the store,
// where the server reads it for the status page and /metrics.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/store"
)

// Job is one named piece of work. Run must be idempotent: a second run with
// unchanged inputs writes nothing new (docs/go-conventions.md, Testing).
type Job interface {
	Name() string
	Run(ctx context.Context) error
}

// Recorder persists job outcomes; *store.Store implements it.
type Recorder interface {
	RecordJob(ctx context.Context, r store.JobResult) error
}

var (
	// ErrUnknownJob is a name that was never registered.
	ErrUnknownJob = errors.New("unknown job")
	// ErrJobFailed wraps a job's own error. The runner has logged it once,
	// at Warn, so callers only set their exit code.
	ErrJobFailed = errors.New("job failed")
	// ErrRecord wraps a failure to persist the outcome, which is unexpected:
	// a status page that silently stops moving is worse than a loud failure.
	ErrRecord = errors.New("record job result")
)

// Registry maps names to jobs. Its names are the only values the job and
// batch labels can take, since the runner resolves every name through it.
type Registry struct {
	jobs map[string]Job
}

// NewRegistry returns a registry holding js, or an error if two share a name.
func NewRegistry(js ...Job) (*Registry, error) {
	r := &Registry{jobs: map[string]Job{}}
	for _, j := range js {
		if _, dup := r.jobs[j.Name()]; dup {
			return nil, fmt.Errorf("job %q registered twice", j.Name())
		}
		r.jobs[j.Name()] = j
	}
	return r, nil
}

// Get returns the job called name or ErrUnknownJob.
func (r *Registry) Get(name string) (Job, error) {
	j, ok := r.jobs[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownJob, name)
	}
	return j, nil
}

// Names lists the registered names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.jobs))
	for n := range r.jobs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// recordTimeout bounds persisting a result after the job's own context ended.
const recordTimeout = 5 * time.Second

// Runner runs a registered job, records the outcome and sets the batch
// gauges that the push sends to the Pushgateway.
type Runner struct {
	jobs     *Registry
	recorder Recorder
	now      func() time.Time
	ok       prometheus.Gauge
	duration prometheus.Gauge
}

// NewRunner builds a runner whose gauges are registered on reg. The gauges
// carry no job label: a job pod runs one job, and the Pushgateway's grouping
// label (batch=<name>) says which.
func NewRunner(registry *Registry, recorder Recorder, reg prometheus.Registerer, now func() time.Time) (*Runner, error) {
	r := &Runner{
		jobs:     registry,
		recorder: recorder,
		now:      now,
		ok: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eunomia_batch_last_run_ok",
			Help: "1 if this batch run succeeded, 0 if it failed. Pushed once at the run's end, grouped by batch (the job's name).",
		}),
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eunomia_batch_duration_seconds",
			Help: "Wall-clock duration of this batch run. Pushed once at the run's end, grouped by batch (the job's name).",
		}),
	}
	for _, c := range []prometheus.Collector{r.ok, r.duration} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("register batch metrics: %w", err)
		}
	}
	return r, nil
}

// Run executes the job called name and records the result, even when the
// job failed or ctx was cancelled mid-run. A failure is logged here, once,
// and returned wrapped in ErrJobFailed.
func (r *Runner) Run(ctx context.Context, name string) error {
	job, err := r.jobs.Get(name)
	if err != nil {
		return err
	}
	log := zerolog.Ctx(ctx).With().Str("job", name).Logger()
	start := r.now()
	jobErr := job.Run(log.WithContext(ctx))
	elapsed := r.now().Sub(start)

	result := store.JobResult{Name: name, RunAt: start, Duration: elapsed, OK: jobErr == nil}
	r.duration.Set(elapsed.Seconds())
	if jobErr != nil {
		result.Message = jobErr.Error()
		r.ok.Set(0)
		log.Warn().Err(jobErr).Int64("durationMs", elapsed.Milliseconds()).Msg("Job failed")
	} else {
		r.ok.Set(1)
		log.Info().Int64("durationMs", elapsed.Milliseconds()).Msg("Finished job")
	}

	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	var recErr error
	if err := r.recorder.RecordJob(recCtx, result); err != nil {
		recErr = fmt.Errorf("%w: %w", ErrRecord, err)
	}
	if jobErr != nil {
		return errors.Join(fmt.Errorf("%w: %s: %w", ErrJobFailed, name, jobErr), recErr)
	}
	return recErr
}
