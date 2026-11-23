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
