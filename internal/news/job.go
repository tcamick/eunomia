package news

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/store"
	"github.com/tcamick/eunomia/internal/tagging"
)

// JobName is the news job's registered name.
const JobName = "news"

// TagWindow is how far back mentions are recomputed on each run, so a
// change in the roster reaches recent stories (ADR 0008).
const TagWindow = 30 * 24 * time.Hour

// Store is what the news job needs of the store; *store.Store implements it.
type Store interface {
	SourceState(ctx context.Context, source string) (store.SourceState, bool, error)
	RecordSourceCheck(ctx context.Context, st store.SourceState, changed bool) error
	AddStories(ctx context.Context, feed, source string, at time.Time, items []store.NewStory) ([]int64, error)
	NamedMembers(ctx context.Context) ([]store.NamedMember, error)
	StoriesSince(ctx context.Context, since time.Time) ([]store.StoryText, error)
	ReplaceMentions(ctx context.Context, want map[int64][]store.Mention) (int, error)
}

// Getter fetches a feed; *fetch.Client implements it.
type Getter interface {
	Get(ctx context.Context, source, rawURL string, prev fetch.Validators) (*fetch.Response, error)
}

// Job is `eunomia job news`: read each feed, store what is new, and tag the
// recent stories with the members and states they name.
type Job struct {
	feeds []Feed
	get   Getter
	st    Store
	now   func() time.Time
	added *prometheus.CounterVec
}

// NewJob returns the news job. reg, if not nil, gets
// eunomia_news_stories_added_total{feed}, which the job pod pushes.
func NewJob(feeds []Feed, get Getter, st Store, now func() time.Time, reg prometheus.Registerer) (*Job, error) {
	ids := make([]string, 0, len(feeds))
	for _, f := range feeds {
		ids = append(ids, f.ID)
	}
	added := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "eunomia_news_stories_added_total",
		Help: "Stories the news job stored, by feed: " + strings.Join(ids, ", ") + ".",
	}, []string{"feed"})
	for _, id := range ids {
		added.WithLabelValues(id) // every feed is present, at zero, from the first push
	}
	if reg != nil {
		if err := reg.Register(added); err != nil {
			return nil, fmt.Errorf("register news metrics: %w", err)
		}
	}
	return &Job{feeds: feeds, get: get, st: st, now: now, added: added}, nil
}

// Name is the job's registered name.
func (*Job) Name() string { return JobName }

// Run reads every feed; one that fails is reported and the others still
// run (go-conventions, Fetching). Then it retags the recent stories.
func (j *Job) Run(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	var errs []error
	for _, f := range j.feeds {
		n, err := j.readFeed(ctx, f)
		if err != nil {
			log.Warn().Err(err).Str("feed", f.ID).Msg("Could not read a feed")
			errs = append(errs, err)
			continue
		}
		j.added.WithLabelValues(f.ID).Add(float64(n))
	}
	tagged, err := j.retag(ctx)
	if err != nil {
		errs = append(errs, err)
	}
	log.Info().Int("feeds", len(j.feeds)).Int("failed", len(errs)).Int("mentionsChanged", tagged).Msg("Read the news")
	return errors.Join(errs...)
}

// readFeed fetches one feed with the validators it last answered with, and
// stores its new items. An unchanged feed costs one request and writes no
// story.
func (j *Job) readFeed(ctx context.Context, f Feed) (int, error) {
	prev, _, err := j.st.SourceState(ctx, f.ID)
	if err != nil {
		return 0, err
	}
	resp, err := j.get.Get(ctx, f.ID, f.URL, fetch.Validators{ETag: prev.ETag, LastModified: prev.LastModified})
	if err != nil {
		return 0, err
	}
	at := j.now()
	check := store.SourceState{Source: f.ID, URL: f.URL, ETag: resp.Validators.ETag,
		LastModified: resp.Validators.LastModified, CheckedAt: at}
	if resp.NotModified {
		return 0, j.st.RecordSourceCheck(ctx, check, false)
	}
	items, err := Parse(resp.Body, f)
	if err != nil {
		return 0, err
	}
	stories := make([]store.NewStory, 0, len(items))
	for _, it := range items {
		stories = append(stories, store.NewStory{GUID: it.GUID, URL: it.URL, Title: it.Title, Summary: it.Summary,
			Published: it.Published})
	}
	added, err := j.st.AddStories(ctx, f.ID, f.URL, at, stories)
	if err != nil {
		return 0, err
	}
	return len(added), j.st.RecordSourceCheck(ctx, check, len(added) > 0)
}

// retag recomputes the mentions of the last TagWindow's stories against
// the roster as it is now, writing only the differences.
func (j *Job) retag(ctx context.Context) (int, error) {
	members, err := j.st.NamedMembers(ctx)
	if err != nil {
		return 0, err
	}
	stories, err := j.st.StoriesSince(ctx, j.now().Add(-TagWindow))
	if err != nil {
		return 0, err
	}
	m := tagging.New(members)
	want := make(map[int64][]store.Mention, len(stories))
	for _, s := range stories {
		want[s.ID] = m.Tag(s.Title + "\n" + s.Summary)
	}
	return j.st.ReplaceMentions(ctx, want)
}
