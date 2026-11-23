package news

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/store"
)

var t0 = time.Date(2026, 11, 23, 22, 0, 0, 0, time.UTC)

var testFeeds = []Feed{
	{ID: "npr-politics", Name: "NPR Politics", URL: "https://feeds.example/npr-politics.xml", Site: "www.npr.org"},
	{ID: "npr-news", Name: "NPR News", URL: "https://feeds.example/npr-news.xml", Site: "www.npr.org"},
	{ID: "democracy-now", Name: "Democracy Now!", URL: "https://feeds.example/democracy-now.xml", Site: "www.democracynow.org"},
}

// fakeFeeds serves each feed's fixture. A feed with an etag answers 304
// when that etag comes back; one with an error fails.
type fakeFeeds struct {
	mu     sync.Mutex
	bodies map[string][]byte
	etags  map[string]string
	fail   map[string]error
	asked  []fetch.Validators
	urls   []string
}

func (f *fakeFeeds) Get(_ context.Context, _, rawURL string, prev fetch.Validators) (*fetch.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, rawURL)
	f.asked = append(f.asked, prev)
	if err := f.fail[rawURL]; err != nil {
		return nil, err
	}
	etag := f.etags[rawURL]
	if etag != "" && prev.ETag == etag {
		return &fetch.Response{Status: 304, NotModified: true, Validators: prev}, nil
	}
	return &fetch.Response{Status: 200, Body: f.bodies[rawURL], Validators: fetch.Validators{ETag: etag}}, nil
}

func servingFixtures(t *testing.T) *fakeFeeds {
	t.Helper()
	f := &fakeFeeds{bodies: map[string][]byte{}, etags: map[string]string{}, fail: map[string]error{}}
	for _, feed := range testFeeds {
		f.bodies[feed.URL] = fixture(t, feed.ID+".xml")
	}
	// Democracy Now sends an ETag; NPR does not (acceptance file, Sources).
	f.etags["https://feeds.example/democracy-now.xml"] = `"dn-1"`
	return f
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "eunomia.sqlite"), func() time.Time { return t0 })
	require.NoError(t, err, "opening a new database")
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.Migrate(context.Background())
	require.NoError(t, err, "applying migrations")
	return s
}

func countStories(t *testing.T, s *store.Store) int {
	t.Helper()
	rows, err := s.Stories(context.Background(), store.StoryFilter{Limit: 100})
	require.NoError(t, err, "listing stories")
	return len(rows)
}

func TestJob_StoresEachFeedAndAnUnchangedFeedCostsOneRequestAndWritesNothing(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	feeds := servingFixtures(t)
	reg := prometheus.NewRegistry()
	j, err := NewJob(testFeeds, feeds, s, func() time.Time { return t0 }, reg)
	require.NoError(t, err, "building the job")
	ctx := context.Background()

	require.NoError(t, j.Run(ctx), "the first run")
	first := countStories(t, s)
	assert.Positive(t, first, "stories are stored")
	for _, f := range testFeeds {
		rows, err := s.Stories(ctx, store.StoryFilter{Feed: f.ID, Limit: 100})
		require.NoError(t, err, "listing %s", f.ID)
		assert.NotEmpty(t, rows, "%s has stories", f.ID)
		for _, r := range rows {
			if r.Feed != f.ID {
				continue // first seen in another feed (NPR's two share stories)
			}
			assert.Equal(t, f.URL, r.Source, "story %d's source is its feed", r.ID)
			assert.Equal(t, t0.Format(time.RFC3339), r.FetchedAt, "and it was fetched in this run")
		}
	}
	assert.InDelta(t, 3, testutil.ToFloat64(j.added.WithLabelValues("democracy-now")), 0, "the counter counts what was stored")

	feeds.urls, feeds.asked = nil, nil
	require.NoError(t, j.Run(ctx), "the second run")
	assert.Equal(t, first, countStories(t, s), "a second run stores nothing new")
	assert.Len(t, feeds.urls, len(testFeeds), "one request per feed")
	for i, u := range feeds.urls {
		if u == "https://feeds.example/democracy-now.xml" {
			assert.Equal(t, `"dn-1"`, feeds.asked[i].ETag, "the request is conditional where the feed allows it")
		}
	}
	dn, ok, err := s.SourceState(ctx, "democracy-now")
	require.NoError(t, err, "reading the feed's check")
	require.True(t, ok, "the feed's check is recorded")
	assert.Equal(t, "https://feeds.example/democracy-now.xml", dn.URL, "with the feed's URL")
}

func TestJob_AFailingFeedDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	feeds := servingFixtures(t)
	feeds.fail["https://feeds.example/npr-news.xml"] = errors.New("the source said 503")
	j, err := NewJob(testFeeds, feeds, s, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	err = j.Run(context.Background())
	require.Error(t, err, "the run reports the failed feed")
	assert.Contains(t, err.Error(), "503", "with its cause")
	rows, err := s.Stories(context.Background(), store.StoryFilter{Feed: "democracy-now", Limit: 100})
	require.NoError(t, err, "listing a feed that worked")
	assert.Len(t, rows, 3, "the other feeds are stored")
}

func TestJob_TagsStoriesWithTheMembersAndStatesTheyName(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	district := 3
	require.NoError(t, s.ApplyRoster(ctx, store.RosterPlan{At: t0, Source: "https://example.org/",
		People: []store.Person{{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", InCongress: true}},
		Terms: []store.Term{{Person: "A000001", Office: "house/OH/3", Body: "house", State: "OH", District: &district,
			Start: "2025-01-03", End: "2027-01-03", Party: "Democratic", Current: true}},
	}), "seeding a member")
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/tagged.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{feed.URL: []byte(`<rss><channel><item>
		<title>Ann Able wins a vote</title><link>https://www.npr.org/a</link><guid>a</guid>
		<pubDate>Mon, 23 Nov 2026 21:00:00 +0000</pubDate><description>In Ohio, a vote.</description>
		</item><item><title>Washington waits</title><link>https://www.npr.org/b</link><guid>b</guid>
		<pubDate>Mon, 23 Nov 2026 21:00:00 +0000</pubDate><description>Nothing here.</description>
		</item></channel></rss>`)}, etags: map[string]string{}, fail: map[string]error{}}
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	require.NoError(t, j.Run(ctx), "running the job")

	rows, err := s.Stories(ctx, store.StoryFilter{Person: "A000001"})
	require.NoError(t, err, "listing the member's stories")
	require.Len(t, rows, 1, "the story naming the member is on their page")
	assert.Equal(t, []store.StoryMention{{Kind: "person", Ref: "A000001", Name: "Ann Able"}, {Kind: "state", Ref: "OH"}}, rows[0].Mentions,
		"tagged with the member and the state it names")
	rows, err = s.Stories(ctx, store.StoryFilter{State: "WA"})
	require.NoError(t, err, "listing Washington's stories")
	assert.Empty(t, rows, `"Washington" alone tags no state`)
}

// allMentions is every stored story's mentions, by story id.
func allMentions(t *testing.T, s *store.Store) map[int64][]store.StoryMention {
	t.Helper()
	rows, err := s.Stories(context.Background(), store.StoryFilter{Limit: 100})
	require.NoError(t, err, "listing stories")
	out := map[int64][]store.StoryMention{}
	for _, r := range rows {
		out[r.ID] = r.Mentions
	}
	return out
}

func seedAnn(t *testing.T, s *store.Store) {
	t.Helper()
	district := 3
	require.NoError(t, s.ApplyRoster(context.Background(), store.RosterPlan{At: t0, Source: "https://example.org/",
		People: []store.Person{{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", InCongress: true}},
		Terms: []store.Term{{Person: "A000001", Office: "house/OH/3", Body: "house", State: "OH", District: &district,
			Start: "2025-01-03", End: "2027-01-03", Party: "Democratic", Current: true}},
	}), "seeding a member")
}

func oneItem(guid, title, date string) []byte {
	return []byte(`<rss><channel><item><title>` + title + `</title><link>https://www.npr.org/` + guid +
		`</link><guid>` + guid + `</guid><pubDate>` + date + `</pubDate></item></channel></rss>`)
}

// A member who joins the roster is found in the stories of the last
// TagWindow that name them, and not in older ones (ADR 0008).
func TestJob_RetagsTheWindowWhenTheRosterChanges(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/f.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{}, etags: map[string]string{}, fail: map[string]error{}}
	now := t0
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return now }, nil)
	require.NoError(t, err, "building the job")

	feeds.bodies[feed.URL] = oneItem("old", "Ann Able speaks", "Fri, 09 Oct 2026 12:00:00 +0000") // 45 days before t0
	require.NoError(t, j.Run(ctx), "storing an old story")
	feeds.bodies[feed.URL] = oneItem("new", "Ann Able speaks again", "Mon, 23 Nov 2026 20:00:00 +0000")
	require.NoError(t, j.Run(ctx), "storing a recent story")
	for id, m := range allMentions(t, s) {
		assert.Empty(t, m, "story %d: no member is on the roster yet", id)
	}

	seedAnn(t, s)
	now = t0.Add(time.Hour)
	require.NoError(t, j.Run(ctx), "the run after the roster changed")

	rows, err := s.Stories(ctx, store.StoryFilter{Person: "A000001"})
	require.NoError(t, err, "listing the member's stories")
	require.Len(t, rows, 1, "only the story inside the window is tagged")
	assert.Equal(t, "Ann Able speaks again", rows[0].Title, "the recent one")
}

func TestJob_ASecondRunChangesNoMention(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	seedAnn(t, s)
	ctx := context.Background()
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/f.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{feed.URL: oneItem("a", "Ann Able in Ohio", "Mon, 23 Nov 2026 20:00:00 +0000")},
		etags: map[string]string{}, fail: map[string]error{}}
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	require.NoError(t, j.Run(ctx), "the first run")
	before := allMentions(t, s)
	require.NotEmpty(t, before, "the story is tagged")

	tagged, err := j.retag(ctx)
	require.NoError(t, err, "retagging again")
	assert.Zero(t, tagged, "no mention row is written")
	require.NoError(t, j.Run(ctx), "a second run")
	assert.Equal(t, before, allMentions(t, s), "the mentions are as they were")
}

func TestJob_AFeedWithNoItemsFails(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/f.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{feed.URL: []byte(`<rss><channel><title>NPR</title></channel></rss>`)},
		etags: map[string]string{}, fail: map[string]error{}}
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")

	err = j.Run(context.Background())

	require.ErrorIs(t, err, ErrNoItems, "an empty feed is a failure")
	assert.Contains(t, err.Error(), "feed npr-politics", "naming the feed")
	_, ok, err := s.SourceState(context.Background(), feed.ID)
	require.NoError(t, err, "reading the feed's check")
	assert.False(t, ok, "and is not recorded as checked")
}

func TestJob_ADateAheadOfNowIsStoredAsUnknown(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/f.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{feed.URL: []byte(`<rss><channel>
		<item><title>From the future</title><link>https://www.npr.org/f</link><guid>f</guid><pubDate>Thu, 01 Jan 2099 00:00:00 +0000</pubDate></item>
		<item><title>Tomorrow by a zone</title><link>https://www.npr.org/z</link><guid>z</guid><pubDate>Tue, 24 Nov 2026 08:00:00 +0000</pubDate></item>
		</channel></rss>`)}, etags: map[string]string{}, fail: map[string]error{}}
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	require.NoError(t, j.Run(context.Background()), "running the job")

	rows, err := s.Stories(context.Background(), store.StoryFilter{Limit: 10})
	require.NoError(t, err, "listing stories")
	byTitle := map[string]store.StoryRow{}
	for _, r := range rows {
		byTitle[r.Title] = r
	}
	assert.Empty(t, byTitle["From the future"].Published, "a date years ahead is unknown")
	assert.Equal(t, "2026-11-24T08:00:00Z", byTitle["Tomorrow by a zone"].Published, "ten hours ahead is within a day: kept")
}

// A member who leaves is no longer matched, but keeps the stories that
// named them: a departure is history, not a mistake to undo
// (go-conventions, Provenance).
func TestJob_AMemberWhoLeavesKeepsTheStoriesThatNamedThem(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	seedAnn(t, s)
	ctx := context.Background()
	feed := Feed{ID: "npr-politics", URL: "https://feeds.example/f.xml", Site: "www.npr.org"}
	feeds := &fakeFeeds{bodies: map[string][]byte{feed.URL: oneItem("a", "Ann Able retires", "Mon, 23 Nov 2026 20:00:00 +0000")},
		etags: map[string]string{}, fail: map[string]error{}}
	now := t0
	j, err := NewJob([]Feed{feed}, feeds, s, func() time.Time { return now }, nil)
	require.NoError(t, err, "building the job")
	require.NoError(t, j.Run(ctx), "the run that tags Ann")

	require.NoError(t, s.ApplyRoster(ctx, store.RosterPlan{At: t0.Add(time.Hour), Source: "https://example.org/",
		People: []store.Person{{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", InCongress: false}},
	}), "Ann leaves")
	members, err := s.NamedMembers(ctx)
	require.NoError(t, err, "listing the members the tagger matches")
	require.Empty(t, members, "Ann is no longer matched, and no one else is")
	now = t0.Add(2 * time.Hour)
	require.NoError(t, j.Run(ctx), "the run after Ann left")

	rows, err := s.Stories(ctx, store.StoryFilter{Person: "A000001"})
	require.NoError(t, err, "listing Ann's stories")
	assert.Len(t, rows, 1, "Ann's page keeps the story")
}
