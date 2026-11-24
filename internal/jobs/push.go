package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
)

// PushJob is the Pushgateway's job label for every batch run; the grouping
// label batch is the job's registered name (beholder ADR 0002).
const PushJob = "eunomia-batch"

const (
	labelBatch   = "batch"
	pushTimeout  = 10 * time.Second
	pushedPrefix = "eunomia_"
)

// groupingLabels are set by the Pushgateway on every pushed series. A family
// that carries one of them itself would be refused by the client, so such a
// family is left out.
var groupingLabels = []string{"job", labelBatch}

// Push sends a batch run's eunomia_ metrics to the Pushgateway at url, with
// PUT, so the gateway holds each batch's last run and not a running total.
// batch must be a registered job name, which keeps the label bounded. It runs
// even when ctx was cancelled, since a stopped run's counts are worth having.
//
// The push has no request counter of its own, unlike every other outside
// call: it is the run's last act, so a counter would be incremented after the
// only push and lost with the pod. A failed push shows as the batch's
// push_time_seconds growing old, and in the error the caller logs.
//
// A refused connection is tried again after each of pushWaits: a job that
// ends within moments of its pod starting can push before k3s has put the
// pod's network policy in force, and be refused (E2, 2026-11-24 UTC: the restore
// check, done in 60 ms, was refused on both of its first runs; E0's noop,
// which never reached the gateway, was possibly the same). A gateway's
// answer, even an error, is final. A push that got through only after a
// refusal is logged at Info, so the cause can be confirmed.
func Push(ctx context.Context, url, batch string, g prometheus.Gatherer, client *http.Client) error {
	_, err := pushWith(ctx, url, batch, g, client, pushWaits(), sleep)
	return err
}

// pushWaits are the pauses before each new attempt: about eight seconds in
// all, inside pushTimeout.
func pushWaits() []time.Duration {
	return []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}
}

// sleep waits d, or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// pushWith is Push with its pauses and its way of waiting given, for tests.
// It returns how many attempts it made.
func pushWith(ctx context.Context, url, batch string, g prometheus.Gatherer, client *http.Client,
	waits []time.Duration, wait func(context.Context, time.Duration) error) (int, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTimeout)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: pushTimeout}
	}
	var err error
	attempts := 0
	for {
		attempts++
		err = push.New(url, PushJob).
			Grouping(labelBatch, batch).
			Gatherer(batchGatherer{g}).
			Client(client).
			PushContext(ctx)
		if err == nil {
			if attempts > 1 {
				zerolog.Ctx(ctx).Info().Str("job", batch).Int("attempts", attempts).
					Msg("Pushed the run's metrics after a refused connection")
			}
			return attempts, nil
		}
		if !errors.Is(err, syscall.ECONNREFUSED) || attempts > len(waits) {
			break
		}
		if werr := wait(ctx, waits[attempts-1]); werr != nil {
			break
		}
	}
	return attempts, fmt.Errorf("push %s metrics: %w", batch, err)
}

// batchGatherer is the part of a run's registry worth pushing: the eunomia_
// families without a grouping label. The Go runtime's and the process's own
// series describe a pod that has already exited.
type batchGatherer struct{ g prometheus.Gatherer }

// Gather returns the families worth pushing.
func (b batchGatherer) Gather() ([]*dto.MetricFamily, error) {
	mfs, err := b.g.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather for push: %w", err)
	}
	return slices.DeleteFunc(mfs, func(mf *dto.MetricFamily) bool {
		if !strings.HasPrefix(mf.GetName(), pushedPrefix) {
			return true
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if slices.Contains(groupingLabels, l.GetName()) {
					return true
				}
			}
		}
		return false
	}), nil
}
