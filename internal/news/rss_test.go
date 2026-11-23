package news

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "reading fixture %s", name)
	return b
}

var hostileFeed = Feed{ID: "hostile", Name: "Hostile", URL: "https://news.example/rss", Site: "news.example"}

func byGUID(t *testing.T, items []Item, guid string) Item {
	t.Helper()
	for _, it := range items {
		if it.GUID == guid {
			return it
		}
	}
	require.Failf(t, "missing item", "no item with guid %q", guid)
	return Item{}
}

func TestParse_TheRealFeedsYieldTheirStories(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		file  string
		feed  Feed
		title string
	}{
		"NPR Politics": {file: "npr-politics.xml", feed: Feed{ID: "npr-politics", Site: "www.npr.org"},
			title: "Democrats win House by largest margin in 50 years. And, COP31 ends with no deal"},
		"Democracy Now": {file: "democracy-now.xml", feed: Feed{ID: "democracy-now", Site: "www.democracynow.org"},
			title: "We Need a Third Reconstruction"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			items, err := Parse(fixture(t, tc.file), tc.feed)
			require.NoError(t, err, "parsing %s", tc.file)
			require.Len(t, items, 3, "the fixture's three items")
			first := items[0]
			assert.True(t, strings.HasPrefix(first.Title, tc.title), "the first headline, got %q", first.Title)
			assert.True(t, strings.HasPrefix(first.URL, "https://"), "links are stored as https, got %q", first.URL)
			assert.False(t, first.Published.IsZero(), "the publication time is read")
			for _, it := range items {
				assert.LessOrEqual(t, utf8.RuneCountInString(it.Summary), MaxSummary, "a summary is cut to the limit")
				assert.NotContains(t, it.Summary, "<", "a summary is plain text")
			}
		})
	}
}

func TestParse_DemocracyNowsHTTPLinksAreUpgradedOnItsOwnSite(t *testing.T) {
	t.Parallel()
	items, err := Parse(fixture(t, "democracy-now.xml"), Feed{ID: "democracy-now", Site: "www.democracynow.org"})
	require.NoError(t, err, "parsing the Democracy Now fixture")
	for _, it := range items {
		assert.True(t, strings.HasPrefix(it.URL, "https://www.democracynow.org/"), "upgraded to https, got %q", it.URL)
	}
}

func TestParse_NothingOfAnArticleBodyIsKept(t *testing.T) {
	t.Parallel()
	// AGENTS.md: never article bodies. content:encoded is not even read.
	items, err := Parse(fixture(t, "hostile.xml"), hostileFeed)
	require.NoError(t, err, "parsing the hostile fixture")
	for _, it := range items {
		for _, field := range []string{it.Title, it.Summary, it.URL, it.GUID} {
			assert.NotContains(t, field, "BODY-MARKER", "no field holds the article text")
		}
	}
}

func TestParse_EnforcesTheRulesForOutsideText(t *testing.T) {
	t.Parallel()
	items, err := Parse(fixture(t, "hostile.xml"), hostileFeed)
	require.NoError(t, err, "parsing the hostile fixture")

	assert.Empty(t, byGUID(t, items, "h-1").URL, "a javascript: link is not stored")
	assert.Empty(t, byGUID(t, items, "h-2").URL, "an http link on another site is not stored")
	assert.Equal(t, "https://news.example/2026/11/23/story", byGUID(t, items, "h-3").URL,
		"an http link on the source's own site is upgraded to https")

	html := byGUID(t, items, "h-4")
	assert.Equal(t, "HTML & script in the summary", html.Title, "entities in a headline are decoded")
	assert.Equal(t, "First bold line & more. Second line.", html.Summary,
		"tags are dropped, a script's and a style's content with them, entities decoded")

	long := byGUID(t, items, "h-5")
	n := utf8.RuneCountInString(long.Summary)
	assert.True(t, n <= MaxSummary && n > MaxSummary-12, "a long summary is cut at a word near the limit, got %d", n)
	assert.True(t, strings.HasSuffix(long.Summary, "…"), "and says it was cut")
	assert.False(t, strings.HasSuffix(strings.TrimSuffix(long.Summary, "…"), " "), "cut at a word, with no trailing space")

	undated := byGUID(t, items, "https://news.example/undated")
	assert.True(t, undated.Published.IsZero(), "a missing date is unknown, not today")

	for _, it := range items {
		assert.NotEmpty(t, it.Title, "an item with no headline is skipped")
	}
	assert.Len(t, items, 6, "six of eight items: one untitled, one a repeat of a guid")
}

func TestParsePublished_ReadsTheFeedsDateForms(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 11, 23, 13, 45, 45, 0, time.UTC)
	tests := map[string]string{
		"a numeric zone":          "Mon, 23 Nov 2026 08:45:45 -0500",
		"a named zone":            "Mon, 23 Nov 2026 13:45:45 GMT",
		"a single-digit day":      "Mon, 23 Nov 2026 13:45:45 +0000",
		"without the day of week": "23 Nov 2026 13:45:45 +0000",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, want.Equal(parsePublished(raw)), "parsing %q", raw)
		})
	}
	assert.True(t, parsePublished("yesterday").IsZero(), "an unreadable date is unknown")
}
