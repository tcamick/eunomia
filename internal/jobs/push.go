package jobs

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	dto "github.com/prometheus/client_model/go"
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
func Push(ctx context.Context, url, batch string, g prometheus.Gatherer, client *http.Client) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTimeout)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: pushTimeout}
	}
	err := push.New(url, PushJob).
		Grouping(labelBatch, batch).
		Gatherer(batchGatherer{g}).
		Client(client).
		PushContext(ctx)
	if err != nil {
		return fmt.Errorf("push %s metrics: %w", batch, err)
	}
	return nil
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
