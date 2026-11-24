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

// ErrNoItems is a feed that answered with no usable item.
var ErrNoItems = errors.New("the feed has no usable items")

// maxAhead is how far past the time of the fetch a publication date may
// be: a day covers any zone or clock skew.
const maxAhead = 24 * time.Hour

// Store is what the news job needs of the store; *store.Store implements it.
type Store interface {
	SourceState(ctx context.Context, source string) (store.SourceState, bool, error)
	RecordSourceCheck(ctx context.Context, st store.SourceState, changed bool) error
	AddStories(ctx context.Context, feed, source string, at time.Time, items []store.NewStory) ([]int64, error)
	NamedTerms(ctx context.Context, since time.Time) ([]store.NamedTerm, error)
	PeopleNames(ctx context.Context) ([]store.NamedMember, error)
	StoriesSince(ctx context.Context, since time.Time) ([]store.StoryText, error)
	ReplaceMentions(ctx context.Context, want map[int64][]store.Mention, matchable map[string]bool) (int, error)
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

// Run reads every feed; one that fails is returned, once, naming its feed,
// and the others still run (go-conventions, Fetching). Then it retags the
// recent stories.
func (j *Job) Run(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	var errs []error
	for _, f := range j.feeds {
		n, err := j.readFeed(ctx, f)
		if err != nil {
			errs = append(errs, fmt.Errorf("feed %s: %w", f.ID, err))
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
	// A feed that answers with nothing usable has changed its format or
	// broken; recording it as checked would keep the job green for good.
	if len(items) == 0 {
		return 0, ErrNoItems
	}
	stories := make([]store.NewStory, 0, len(items))
	for _, it := range items {
		published := it.Published
		// A date ahead of now is a mistake (or a story pinning itself to
		// the top): stored as unknown, never clamped to a made-up date.
		if published.After(at.Add(maxAhead)) {
			published = time.Time{}
		}
		stories = append(stories, store.NewStory{GUID: it.GUID, URL: it.URL, Title: it.Title, Summary: it.Summary,
			Published: published})
	}
	added, err := j.st.AddStories(ctx, f.ID, f.URL, at, stories)
	if err != nil {
		return 0, err
	}
	return len(added), j.st.RecordSourceCheck(ctx, check, len(added) > 0)
}

// retag recomputes the mentions of the last TagWindow's stories, writing
// only the differences. Each story is read against the members who sat on
// the day it ran (ADR 0008): "Rep. Davis" in a December story means a
// December Davis, whoever sits in January.
func (j *Job) retag(ctx context.Context) (int, error) {
	since := j.now().Add(-TagWindow)
	terms, err := j.st.NamedTerms(ctx, since)
	if err != nil {
		return 0, err
	}
	people, err := j.st.PeopleNames(ctx)
	if err != nil {
		return 0, err
	}
	stories, err := j.st.StoriesSince(ctx, since)
	if err != nil {
		return 0, err
	}
	matchers := map[string]*tagging.Matcher{}
	// Only a person some day's matcher could find may lose a mention: one
	// whose time in office is outside the window keeps the stories that
	// named them, and an empty roster removes no one.
	matchable := map[string]bool{}
	want := make(map[int64][]store.Mention, len(stories))
	for _, s := range stories {
		day := s.When[:min(len(s.When), len(time.DateOnly))]
		m, ok := matchers[day]
		if !ok {
			members := sittingOn(terms, day)
			for _, mb := range members {
				matchable[mb.ID] = true
			}
			m = tagging.New(members, people)
			matchers[day] = m
		}
		want[s.ID] = m.Tag(s.Title + "\n" + s.Summary)
	}
	return j.st.ReplaceMentions(ctx, want, matchable)
}

// sittingOn returns the members whose term ran on day (YYYY-MM-DD): from its
// start through its end. On the day one Congress ends and the next begins,
// both sets sit, so a surname they share names no one that day. Each
// member once.
func sittingOn(terms []store.NamedTerm, day string) []store.NamedMember {
	byID := map[string]store.NamedMember{}
	var order []string
	for _, t := range terms {
		if t.Start > day || day > t.End {
			continue
		}
		if _, seen := byID[t.ID]; !seen {
			order = append(order, t.ID)
		}
		byID[t.ID] = t.NamedMember
	}
	out := make([]store.NamedMember, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}
