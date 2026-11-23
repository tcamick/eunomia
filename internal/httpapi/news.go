package httpapi

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/news"
	"github.com/tcamick/eunomia/internal/store"
)

// NewsReader is what the news handlers need of the store. SetMarks is the
// server's one write besides visits (go-conventions, Layout).
type NewsReader interface {
	Stories(ctx context.Context, f store.StoryFilter) ([]store.StoryRow, error)
	SetMarks(ctx context.Context, story int64, read, saved *bool, at time.Time) (store.Marks, bool, error)
	SourceChecks(ctx context.Context) ([]store.SourceState, error)
}

// Story is a story as the feed shows it: always with its source's name and
// label (PRD §6.6), and its mentions named.
type Story struct {
	store.StoryRow
	FeedName string `json:"feedName"`
	Label    string `json:"label"`
}

// FeedInfo is a feed in the filter.
type FeedInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

// StoriesResponse is a page of the feed. Next is the offset of the next
// page, 0 when there is none.
type StoriesResponse struct {
	Stories []Story      `json:"stories"`
	Feeds   []FeedInfo   `json:"feeds"`
	Next    int          `json:"next,omitempty"`
	Sources []SourceLine `json:"sources"`
}

const (
	defaultStories = 30
	maxStories     = 50
	maxOffset      = 100_000
)

type newsAPI struct {
	news  NewsReader
	feeds []news.Feed
	now   func() time.Time
}

func (a *newsAPI) feed(id string) (news.Feed, bool) {
	i := slices.IndexFunc(a.feeds, func(f news.Feed) bool { return f.ID == id })
	if i < 0 {
		return news.Feed{}, false
	}
	return a.feeds[i], true
}

// filter reads the query into a filter, refusing a value it does not know:
// an unknown feed, a state code or person id of the wrong form, a bad
// number. A filter is never quietly dropped.
func (a *newsAPI) filter(r *http.Request) (store.StoryFilter, bool) {
	q := r.URL.Query()
	f := store.StoryFilter{Feed: q.Get("feed"), State: q.Get("state"), Person: q.Get("person"),
		Saved: q.Get("saved") == "1", Unread: q.Get("unread") == "1", Limit: defaultStories}
	if _, ok := a.feed(f.Feed); f.Feed != "" && !ok {
		return f, false
	}
	if _, ok := geo.Lookup(f.State); f.State != "" && !ok {
		return f, false
	}
	if f.Person != "" && !reBioguide.MatchString(f.Person) {
		return f, false
	}
	for key, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		raw := q.Get(key)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > maxOffset {
			return f, false
		}
		*dst = n
	}
	f.Limit = min(max(f.Limit, 1), maxStories)
	return f, true
}

func (a *newsAPI) storiesHandler(w http.ResponseWriter, r *http.Request) {
	f, ok := a.filter(r)
	if !ok {
		http.Error(w, "bad filter", http.StatusBadRequest)
		return
	}
	page := f.Limit
	f.Limit = page + 1 // one more, to know whether there is a next page
	ctx := r.Context()
	rows, err := a.news.Stories(ctx, f)
	if err != nil {
		failRequest(w, r, err)
		return
	}
	resp := StoriesResponse{Stories: make([]Story, 0, min(len(rows), page)), Feeds: make([]FeedInfo, 0, len(a.feeds))}
	if len(rows) > page {
		rows = rows[:page]
		resp.Next = f.Offset + page
	}
	for _, row := range rows {
		feed, _ := a.feed(row.Feed)
		for i, m := range row.Mentions {
			if j, ok := geo.Lookup(m.Ref); m.Kind == "state" && ok {
				row.Mentions[i].Name = j.Name
			}
		}
		resp.Stories = append(resp.Stories, Story{StoryRow: row, FeedName: feed.Name, Label: feed.Label})
	}
	checks, err := a.news.SourceChecks(ctx)
	if err != nil {
		failRequest(w, r, err)
		return
	}
	for _, feed := range a.feeds {
		resp.Feeds = append(resp.Feeds, FeedInfo{ID: feed.ID, Name: feed.Name, Label: feed.Label})
		line := SourceLine{Name: feed.Name, URL: feed.URL}
		for _, c := range checks {
			if c.Source == feed.ID {
				line.CheckedAt, line.ChangedAt = c.CheckedAt, c.ChangedAt
			}
		}
		resp.Sources = append(resp.Sources, line)
	}
	writeJSON(w, resp)
}

// maxMarkBody bounds a marks request; a real one is a few dozen bytes.
const maxMarkBody = 1 << 10

// marksHandler sets or clears a story's read and saved marks. It takes only
// a JSON body: a page on another site cannot send that cross-origin
// without a CORS preflight, which this server never answers, so a site the
// owner visits cannot change the marks (the tailnet decides who else can).
func (a *newsAPI) marksHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		http.Error(w, "marks are JSON", http.StatusUnsupportedMediaType)
		return
	}
	var body struct {
		Read  *bool `json:"read"`
		Saved *bool `json:"saved"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMarkBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || (body.Read == nil && body.Saved == nil) {
		http.Error(w, "a mark is {\"read\": bool, \"saved\": bool}", http.StatusBadRequest)
		return
	}
	m, ok, err := a.news.SetMarks(r.Context(), id, body.Read, body.Saved, a.now())
	if err != nil {
		failRequest(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, m)
}
