package jobs

import (
	"context"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/tcamick/eunomia/internal/store"
)

// HistoryReader lists the recorded job outcomes; *store.Store implements it.
type HistoryReader interface {
	Jobs(ctx context.Context) ([]store.JobStatus, error)
}

// historyTimeout bounds the read at each scrape, so a busy file delays a
// scrape by at most this and never hangs it.
const historyTimeout = 3 * time.Second

// HistoryCollector exports each registered job's last outcome from the store
// at scrape time. The job pods are gone before Prometheus could scrape them;
// the server, which is always up, reports what they recorded.
type HistoryCollector struct {
	reader  HistoryReader
	names   []string
	lastRun *prometheus.Desc
	lastOK  *prometheus.Desc
	ok      *prometheus.Desc
	dur     *prometheus.Desc
	up      *prometheus.Desc
}

// NewHistoryCollector reports the jobs in registry from reader. A row whose
// name is not registered (a job since removed) is skipped, which keeps the
// job label to the registered names.
func NewHistoryCollector(reader HistoryReader, registry *Registry) *HistoryCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(name, help+" job is a registered job name.", []string{"job"}, nil)
	}
	return &HistoryCollector{
		reader:  reader,
		names:   registry.Names(),
		lastRun: desc("eunomia_job_last_run_timestamp_seconds", "Unix time of the job's last run."),
		lastOK: desc("eunomia_job_last_success_timestamp_seconds",
			"Unix time of the job's last successful run; absent until it has succeeded once."),
		ok:  desc("eunomia_job_last_run_ok", "1 if the job's last run succeeded, else 0."),
		dur: desc("eunomia_job_last_duration_seconds", "Duration of the job's last run."),
		up:  prometheus.NewDesc("eunomia_job_history_up", "1 if the job history could be read at this scrape, else 0.", nil, nil),
	}
}

// Describe sends the collector's descriptors.
func (c *HistoryCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.lastRun, c.lastOK, c.ok, c.dur, c.up} {
		ch <- d
	}
}

// Collect reads the history and sends one set of series per registered job
// that has run.
func (c *HistoryCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	statuses, err := c.reader.Jobs(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	for _, s := range statuses {
		if !slices.Contains(c.names, s.Name) {
			continue
		}
		ok := 0.0
		if s.OK {
			ok = 1
		}
		ch <- prometheus.MustNewConstMetric(c.lastRun, prometheus.GaugeValue, float64(s.LastRun.Unix()), s.Name)
		ch <- prometheus.MustNewConstMetric(c.ok, prometheus.GaugeValue, ok, s.Name)
		ch <- prometheus.MustNewConstMetric(c.dur, prometheus.GaugeValue, s.LastDuration.Seconds(), s.Name)
		if !s.LastOK.IsZero() {
			ch <- prometheus.MustNewConstMetric(c.lastOK, prometheus.GaugeValue, float64(s.LastOK.Unix()), s.Name)
		}
	}
}
