package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	politicsFeed = "https://feeds.example/politics.xml"
	newsFeed     = "https://feeds.example/news.xml"
)

func story(guid, url, title string, at time.Time) NewStory {
	return NewStory{GUID: guid, URL: url, Title: title, Summary: "A summary.", Published: at}
}

func TestAddStories_ASecondRunWithTheSameItemsWritesNothing(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	items := []NewStory{story("g1", "https://news.example/1", "One", t0), story("g2", "", "Two", time.Time{})}
	added, err := s.AddStories(ctx, "politics", politicsFeed, t0, items)
	require.NoError(t, err, "the first run")
	assert.Len(t, added, 2, "both stories are new")
	before := count(t, s, "SELECT count(*) FROM stories") + count(t, s, "SELECT count(*) FROM story_feeds")

	added, err = s.AddStories(ctx, "politics", politicsFeed, t0.Add(time.Hour), items)
	require.NoError(t, err, "the second run")
	assert.Empty(t, added, "nothing is new")
	assert.Equal(t, before, count(t, s, "SELECT count(*) FROM stories")+count(t, s, "SELECT count(*) FROM story_feeds"),
		"no row is written")
	assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM stories WHERE published IS NULL"), "an unknown time stays unknown")
}

func TestAddStories_OneStoryInTwoFeedsIsOneStoryListedUnderBoth(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	_, err := s.AddStories(ctx, "politics", politicsFeed, t0, []NewStory{story("p-1", "https://news.example/shared", "Shared", t0)})
	require.NoError(t, err, "adding from the first feed")
	added, err := s.AddStories(ctx, "news", newsFeed, t0.Add(time.Hour), []NewStory{story("n-1", "https://news.example/shared", "Shared", t0)})
	require.NoError(t, err, "adding from the second feed")
	assert.Empty(t, added, "the same link is not a new story")
	rows, err := s.Stories(ctx, StoryFilter{})
	require.NoError(t, err, "listing stories")
	require.Len(t, rows, 1, "one story")
	assert.Equal(t, []string{"news", "politics"}, rows[0].Feeds, "listed under both feeds")
	assert.Equal(t, "politics", rows[0].Feed, "labelled by the feed it was first seen in")
	var src, at string
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT source, fetched_at FROM story_feeds WHERE feed = 'news'`).Scan(&src, &at),
		"reading the second feed's row")
	assert.Equal(t, newsFeed, src, "each feed's row has that feed as its source")
	assert.Equal(t, t0.Add(time.Hour).UTC().Format(time.RFC3339), at, "and when that feed carried it")
}

func TestAddStories_TheStoreRefusesAnArticleBody(t *testing.T) {
	t.Parallel()
	// AGENTS.md: never article bodies. internal/news cuts a summary to 500
	// characters; the table refuses more even if it did not.
	s := open(t)
	long := story("g1", "", "One", t0)
	long.Summary = strings.Repeat("x", 501)
	_, err := s.AddStories(context.Background(), "politics", politicsFeed, t0, []NewStory{long})
	require.Error(t, err, "a 501-character summary is refused")
	assert.Zero(t, count(t, s, "SELECT count(*) FROM stories"), "and nothing of the run is written")
}

func TestAddStories_RefusesAStoryWithoutItsSourceOrTime(t *testing.T) {
	t.Parallel()
	s := open(t)
	for name, call := range map[string]func() error{
		"no source": func() error {
			_, err := s.AddStories(context.Background(), "politics", "", t0, []NewStory{story("g", "", "T", t0)})
			return err
		},
		"no time": func() error {
			_, err := s.AddStories(context.Background(), "politics", politicsFeed, time.Time{}, []NewStory{story("g", "", "T", t0)})
			return err
		},
	} {
		require.Error(t, call(), "adding with %s", name)
	}
	assert.Zero(t, count(t, s, "SELECT count(*) FROM stories"), "nothing is written")
}

func TestStories_FiltersAndOrder(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	// A sitting Ohio senator, so a story naming them is an Ohio story.
	require.NoError(t, s.ApplyRoster(ctx, onePlan(t0)), "seeding a member (A000001, Ohio)")
	ids, err := s.AddStories(ctx, "politics", politicsFeed, t0, []NewStory{
		story("old", "https://news.example/old", "Old", t0.Add(-2*time.Hour)),
		story("person", "https://news.example/person", "Names the member", t0.Add(-time.Hour)),
		story("state", "https://news.example/state", "Names Ohio", t0),
	})
	require.NoError(t, err, "adding stories")
	require.Len(t, ids, 3, "three new stories")
	_, err = s.ReplaceMentions(ctx, map[int64][]Mention{
		ids[1]: {{Kind: "person", Ref: "A000001"}},
		ids[2]: {{Kind: "state", Ref: "OH"}},
	})
	require.NoError(t, err, "tagging")
	_, ok, err := s.SetMarks(ctx, ids[0], ptrBool(true), ptrBool(true), t0)
	require.NoError(t, err, "marking the old story")
	require.True(t, ok, "the old story exists")

	titles := func(f StoryFilter) []string {
		t.Helper()
		rows, err := s.Stories(ctx, f)
		require.NoError(t, err, "listing stories with %+v", f)
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.Title)
		}
		return out
	}
	assert.Equal(t, []string{"Names Ohio", "Names the member", "Old"}, titles(StoryFilter{}), "newest first")
	assert.Equal(t, []string{"Names Ohio", "Names the member"}, titles(StoryFilter{State: "OH"}),
		"a state's stories include those naming its members")
	assert.Equal(t, []string{"Names the member"}, titles(StoryFilter{Person: "A000001"}), "a person's stories")
	assert.Equal(t, []string{"Old"}, titles(StoryFilter{Saved: true}), "saved stories")
	assert.Equal(t, []string{"Names Ohio", "Names the member"}, titles(StoryFilter{Unread: true}), "unread stories")
	assert.Empty(t, titles(StoryFilter{Feed: "news"}), "no story in another feed")
	assert.Equal(t, []string{"Names the member"}, titles(StoryFilter{Limit: 1, Offset: 1}), "a page")
}

func TestReplaceMentions_WritesOnlyTheDifferences(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	ids, err := s.AddStories(ctx, "politics", politicsFeed, t0, []NewStory{story("g", "", "T", t0)})
	require.NoError(t, err, "adding a story")
	oh, ca := Mention{Kind: "state", Ref: "OH"}, Mention{Kind: "state", Ref: "CA"}
	n, err := s.ReplaceMentions(ctx, map[int64][]Mention{ids[0]: {oh}})
	require.NoError(t, err, "the first tagging")
	assert.Equal(t, 1, n, "one mention added")
	n, err = s.ReplaceMentions(ctx, map[int64][]Mention{ids[0]: {oh}})
	require.NoError(t, err, "the same tagging again")
	assert.Zero(t, n, "nothing written")
	n, err = s.ReplaceMentions(ctx, map[int64][]Mention{ids[0]: {ca}})
	require.NoError(t, err, "a changed tagging")
	assert.Equal(t, 2, n, "one dropped, one added")
}

func TestSetMarks_SetsClearsAndKeeps(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	ids, err := s.AddStories(ctx, "politics", politicsFeed, t0, []NewStory{story("g", "", "T", t0)})
	require.NoError(t, err, "adding a story")
	m, ok, err := s.SetMarks(ctx, ids[0], ptrBool(true), nil, t0)
	require.NoError(t, err, "marking read")
	require.True(t, ok, "the story exists")
	assert.Equal(t, Marks{ReadAt: "2026-11-22T10:00:00Z"}, m, "read, not saved")
	m, _, err = s.SetMarks(ctx, ids[0], nil, ptrBool(true), t0.Add(time.Hour))
	require.NoError(t, err, "saving")
	assert.Equal(t, Marks{ReadAt: "2026-11-22T10:00:00Z", SavedAt: "2026-11-22T11:00:00Z"}, m, "saving keeps the read mark")
	m, _, err = s.SetMarks(ctx, ids[0], ptrBool(false), nil, t0)
	require.NoError(t, err, "marking unread")
	assert.Equal(t, Marks{SavedAt: "2026-11-22T11:00:00Z"}, m, "unread clears only the read mark")
	_, ok, err = s.SetMarks(ctx, 999, ptrBool(true), nil, t0)
	require.NoError(t, err, "marking a story that does not exist")
	assert.False(t, ok, "reported as missing")
	assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM marks"), "and not written")
}

func TestNamedMembers_ListsSittingMembersWithTheirNamesAndSeat(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	require.NoError(t, s.ApplyRoster(ctx, onePlan(t0)), "seeding a member")
	got, err := s.NamedMembers(ctx)
	require.NoError(t, err, "listing members for tagging")
	assert.Equal(t, []NamedMember{{ID: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", Body: "house", State: "OH"}},
		got, "the sitting member with her names and seat")
}

func ptrBool(b bool) *bool { return &b }
