package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/news"
	"github.com/tcamick/eunomia/internal/store"
)

var feedList = []news.Feed{
	{ID: "npr-politics", Name: "NPR Politics", URL: "https://feeds.npr.org/1014/rss.xml", Label: "Public radio network"},
	{ID: "democracy-now", Name: "Democracy Now!", URL: "https://www.democracynow.org/democracynow.rss",
		Label: "Independent nonprofit daily news program"},
}

// fakeNews records the filter it was asked with and serves fixed rows.
type fakeNews struct {
	rows   []store.StoryRow
	asked  []store.StoryFilter
	marked map[int64]store.Marks
}

func (f *fakeNews) Stories(_ context.Context, flt store.StoryFilter) ([]store.StoryRow, error) {
	f.asked = append(f.asked, flt)
	end := min(flt.Offset+flt.Limit, len(f.rows))
	if flt.Offset >= end {
		return []store.StoryRow{}, nil
	}
	return f.rows[flt.Offset:end], nil
}

func (f *fakeNews) SetMarks(_ context.Context, story int64, read, saved *bool, at time.Time) (store.Marks, bool, error) {
	m, ok := f.marked[story]
	if !ok {
		return store.Marks{}, false, nil
	}
	stamp := at.UTC().Format(time.RFC3339)
	if read != nil {
		m.ReadAt = map[bool]string{true: stamp, false: ""}[*read]
	}
	if saved != nil {
		m.SavedAt = map[bool]string{true: stamp, false: ""}[*saved]
	}
	f.marked[story] = m
	return m, true, nil
}

func (f *fakeNews) SourceChecks(context.Context) ([]store.SourceState, error) {
	return []store.SourceState{{Source: "npr-politics", URL: feedList[0].URL, CheckedAt: t0, ChangedAt: t0.Add(-time.Hour)}}, nil
}

func newsHandler(t *testing.T, f *fakeNews) http.Handler {
	t.Helper()
	h, err := New(Options{Store: fakeStatus{}, UI: ui, Version: "v-test", Now: func() time.Time { return t0 }, Log: zerolog.Nop(),
		Registerer: prometheus.NewRegistry(), News: f, Feeds: feedList})
	require.NoError(t, err, "building the handler")
	return h
}

func threeStories() []store.StoryRow {
	return []store.StoryRow{
		{ID: 3, Feed: "democracy-now", Feeds: []string{"democracy-now"}, Title: "C", Mentions: []store.StoryMention{
			{Kind: "state", Ref: "OH"}, {Kind: "person", Ref: "A000001", Name: "Ann Able"}}},
		{ID: 2, Feed: "npr-politics", Feeds: []string{"npr-politics"}, Title: "B", Mentions: []store.StoryMention{}},
		{ID: 1, Feed: "npr-politics", Feeds: []string{"npr-politics"}, Title: "A", Mentions: []store.StoryMention{}},
	}
}

func TestStories_LabelsEachStoryAndNamesItsMentions(t *testing.T) {
	t.Parallel()
	got := decode[StoriesResponse](t, get(t, newsHandler(t, &fakeNews{rows: threeStories()}), "/api/stories"))
	require.Len(t, got.Stories, 3, "every story")
	c := got.Stories[0]
	assert.Equal(t, "Democracy Now!", c.FeedName, "the source is named on every story (PRD §6.6)")
	assert.Equal(t, "Independent nonprofit daily news program", c.Label, "with its label")
	assert.Equal(t, []store.StoryMention{{Kind: "state", Ref: "OH", Name: "Ohio"}, {Kind: "person", Ref: "A000001", Name: "Ann Able"}},
		c.Mentions, "a state's mention carries its name")
	require.Len(t, got.Feeds, 2, "the feeds, for the filter")
	require.Len(t, got.Sources, 2, "a source line per feed")
	assert.Equal(t, t0, got.Sources[0].CheckedAt, "a checked feed's line says when")
	assert.Zero(t, got.Next, "no more pages")
}

func TestStories_PassesItsFiltersAndPages(t *testing.T) {
	t.Parallel()
	f := &fakeNews{rows: threeStories()}
	h := newsHandler(t, f)
	got := decode[StoriesResponse](t, get(t, h, "/api/stories?feed=npr-politics&state=OH&person=A000001&saved=1&unread=1&limit=2"))
	require.NotEmpty(t, f.asked, "the store was asked")
	assert.Equal(t, store.StoryFilter{Feed: "npr-politics", State: "OH", Person: "A000001", Saved: true, Unread: true, Limit: 3},
		f.asked[0], "the filters reach the store, asking for one more than the page")
	assert.Len(t, got.Stories, 2, "a page of two")
	assert.Equal(t, 2, got.Next, "and where the next page starts")
}

func TestStories_RefusesAFilterItDoesNotKnow(t *testing.T) {
	t.Parallel()
	h := newsHandler(t, &fakeNews{})
	for name, path := range map[string]string{
		"an unknown feed":           "/api/stories?feed=nowhere",
		"a lower-case state":        "/api/stories?state=oh",
		"a malformed person":        "/api/stories?person=robert",
		"a negative offset":         "/api/stories?offset=-1",
		"a limit that is no number": "/api/stories?limit=lots",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, http.StatusBadRequest, get(t, h, path).StatusCode, "GET %s", path)
		})
	}
}

func put(t *testing.T, h http.Handler, path, contentType, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestMarks_SetAndClear(t *testing.T) {
	t.Parallel()
	f := &fakeNews{marked: map[int64]store.Marks{7: {}}}
	h := newsHandler(t, f)
	got := decode[store.Marks](t, put(t, h, "/api/stories/7/marks", "application/json", `{"read":true,"saved":true}`))
	assert.Equal(t, store.Marks{ReadAt: "2026-11-22T10:00:00Z", SavedAt: "2026-11-22T10:00:00Z"}, got, "both set")
	got = decode[store.Marks](t, put(t, h, "/api/stories/7/marks", "application/json; charset=utf-8", `{"read":false}`))
	assert.Equal(t, store.Marks{SavedAt: "2026-11-22T10:00:00Z"}, got, "read cleared, saved kept")
}

func TestMarks_RefusesWhatIsNotAMark(t *testing.T) {
	t.Parallel()
	h := newsHandler(t, &fakeNews{marked: map[int64]store.Marks{7: {}}})
	tests := map[string]struct {
		path, contentType, body string
		want                    int
	}{
		"a form, which another site could post": {path: "/api/stories/7/marks", contentType: "application/x-www-form-urlencoded", body: "read=true", want: http.StatusUnsupportedMediaType},
		"no content type":                       {path: "/api/stories/7/marks", body: `{"read":true}`, want: http.StatusUnsupportedMediaType},
		"an unknown field":                      {path: "/api/stories/7/marks", contentType: "application/json", body: `{"read":true,"title":"x"}`, want: http.StatusBadRequest},
		"nothing to set":                        {path: "/api/stories/7/marks", contentType: "application/json", body: `{}`, want: http.StatusBadRequest},
		"a story that does not exist":           {path: "/api/stories/8/marks", contentType: "application/json", body: `{"read":true}`, want: http.StatusNotFound},
		"an id that is no number":               {path: "/api/stories/x/marks", contentType: "application/json", body: `{"read":true}`, want: http.StatusNotFound},
		"a body far larger than any mark":       {path: "/api/stories/7/marks", contentType: "application/json", body: `{"read":true,"x":"` + strings.Repeat("y", 4096) + `"}`, want: http.StatusBadRequest},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, put(t, h, tc.path, tc.contentType, tc.body).StatusCode, "PUT %s", tc.path)
		})
	}
}
