package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/congress"
	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/portraits"
	"github.com/tcamick/eunomia/internal/store"
)

// keySources is the JSON key every roster response carries its source lines under.
const keySources = "sources"

// RosterReader is what the roster pages read; *store.Store implements it.
type RosterReader interface {
	Sittings(ctx context.Context) ([]store.Sitting, error)
	CurrentLeadership(ctx context.Context) ([]store.Role, error)
	PersonDetail(ctx context.Context, id string) (store.PersonDetail, bool, error)
	Committees(ctx context.Context) ([]store.CommitteeSummary, error)
	CommitteeMembers(ctx context.Context, id string) ([]store.CommitteeMember, error)
	SourceChecks(ctx context.Context) ([]store.SourceState, error)
	LastHolders(ctx context.Context, state string) (map[string]store.LastHolder, error)
}

var (
	reBioguide  = regexp.MustCompile(`^[A-Z][0-9]{6}$`)
	reCommittee = regexp.MustCompile(`^[A-Z]{4}[0-9]{0,2}$`)
)

// SourceLine is where a page's data came from and how fresh it is
// (PRD §3: every fact has a source and a date).
type SourceLine struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CheckedAt time.Time `json:"checkedAt,omitzero"`
	ChangedAt time.Time `json:"changedAt,omitzero"`
}

// rosterSource names the roster's dataset.
const rosterSourceName = "congress-legislators (unitedstates project)"

func (a *api) rosterSource(ctx context.Context) []SourceLine {
	checks, err := a.roster.SourceChecks(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Could not read source checks")
		return nil
	}
	out := []SourceLine{}
	for _, c := range checks {
		if c.Source == "legislators" {
			out = append(out, SourceLine{Name: rosterSourceName, URL: c.URL, CheckedAt: c.CheckedAt, ChangedAt: c.ChangedAt})
		}
	}
	return out
}

// Member is a sitting member as the pages list them.
type Member struct {
	store.Sitting
	PartyCode string `json:"partyCode"`
	StateName string `json:"stateName"`
	Voting    bool   `json:"voting"`
}

func member(m store.Sitting) Member {
	j, ok := geo.Lookup(m.State)
	name := m.State
	if ok {
		name = j.Name
	}
	return Member{Sitting: m, PartyCode: congress.PartyCode(m.Party), StateName: name, Voting: m.Body == congress.Senate || (ok && j.Voting())}
}

func (a *api) congressHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ms, err := a.roster.Sittings(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	roles, err := a.roster.CurrentLeadership(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"chambers": congress.Summarize(ms, roles),
		keySources: a.rosterSource(ctx),
		"apportionment": SourceLine{Name: "Census Bureau, 2020 apportionment", URL: geo.ApportionmentSource,
			CheckedAt: geo.ApportionmentChecked},
	})
}

func (a *api) membersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ms, err := a.roster.Sittings(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]Member, 0, len(ms))
	for _, m := range ms {
		out = append(out, member(m))
	}
	writeJSON(w, map[string]any{"members": out, keySources: a.rosterSource(ctx), "states": geo.All()})
}

// PersonTerm is a term on a person's page, with its office in words.
type PersonTerm struct {
	store.PersonTerm
	PartyCode    string            `json:"partyCode"`
	StateName    string            `json:"stateName"`
	Affiliations []json.RawMessage `json:"affiliations,omitempty"`
}

// PersonResponse is the person's page. Every party gets the same fields
// (PRD §3).
type PersonResponse struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	First      string             `json:"first"`
	Last       string             `json:"last"`
	Birthday   string             `json:"birthday,omitempty"`
	InCongress bool               `json:"inCongress"`
	Portrait   bool               `json:"portrait"`
	Terms      []PersonTerm       `json:"terms"`
	Roles      []store.Role       `json:"roles"`
	Seats      []store.PersonSeat `json:"seats"`
	Links      []Link             `json:"links"`
	Sources    []SourceLine       `json:"sources"`
}

// Link is an outside page about the person, built from a fixed https base
// and an identifier the source gave (web-conventions, external links).
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func (a *api) personHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !reBioguide.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	d, ok, err := a.roster.PersonDetail(ctx, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	p := d.Person
	resp := PersonResponse{
		ID: p.ID, Name: p.Display, First: p.First, Last: p.Last, Birthday: p.Birthday, InCongress: p.InCongress,
		Portrait: d.Portrait, Terms: []PersonTerm{}, Roles: nonNil(d.Roles), Seats: nonNil(d.Seats), Links: []Link{},
	}
	for _, t := range d.Terms {
		pt := PersonTerm{PersonTerm: t, PartyCode: congress.PartyCode(t.Party), StateName: t.State}
		if j, ok := geo.Lookup(t.State); ok {
			pt.StateName = j.Name
		}
		var spans []json.RawMessage
		if json.Unmarshal([]byte(t.Affiliations), &spans) == nil && len(spans) > 0 {
			pt.Affiliations = spans
		}
		resp.Terms = append(resp.Terms, pt)
	}
	for _, t := range d.Terms {
		if t.Current && t.URL != "" {
			resp.Links = append(resp.Links, Link{Label: "Official website", URL: t.URL})
		}
	}
	if p.Bioguide != "" {
		resp.Links = append(resp.Links, Link{Label: "Congress.gov", URL: "https://www.congress.gov/member/" + p.Bioguide})
		resp.Links = append(resp.Links, Link{Label: "Biographical Directory", URL: "https://bioguide.congress.gov/search/bio/" + p.Bioguide})
	}
	// The roster line: when the dataset was last checked, and when this
	// member's own record last changed.
	line := SourceLine{Name: rosterSourceName, URL: d.Provenance.Source, ChangedAt: d.Provenance.FetchedAt}
	for _, c := range a.rosterSource(ctx) {
		line.CheckedAt = c.CheckedAt
	}
	resp.Sources = []SourceLine{line}
	if d.PortraitSource != "" {
		name := "Portrait: unitedstates/images"
		if !d.Portrait {
			name = "Portrait: unitedstates/images (none published for this member)"
		}
		resp.Sources = append(resp.Sources, SourceLine{Name: name, URL: d.PortraitSource,
			CheckedAt: d.PortraitChecked, ChangedAt: d.PortraitFetched})
	}
	writeJSON(w, resp)
}

func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

func (a *api) committeesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cs, err := a.roster.Committees(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"committees": nonNil(cs), keySources: a.rosterSource(ctx)})
}

func (a *api) committeeHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !reCommittee.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	cs, err := a.roster.Committees(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var (
		this *store.CommitteeSummary
		subs = []store.CommitteeSummary{}
	)
	for i := range cs {
		switch {
		case cs[i].ID == id:
			this = &cs[i]
		case cs[i].Parent == id:
			subs = append(subs, cs[i])
		}
	}
	if this == nil {
		http.NotFound(w, r)
		return
	}
	var parent *store.CommitteeSummary
	for i := range cs {
		if cs[i].ID == this.Parent {
			parent = &cs[i]
		}
	}
	ms, err := a.roster.CommitteeMembers(ctx, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type seat struct {
		store.CommitteeMember
		PartyCode string `json:"partyCode"`
	}
	seats := make([]seat, 0, len(ms))
	for _, m := range ms {
		seats = append(seats, seat{CommitteeMember: m, PartyCode: congress.PartyCode(m.Party)})
	}
	writeJSON(w, map[string]any{"committee": this, "parent": parent, "subcommittees": subs, "members": seats,
		keySources: a.rosterSource(ctx)})
}

// SearchResult is one hit.
type SearchResult struct {
	Kind   string `json:"kind"` // person, state or committee
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Code   string `json:"partyCode,omitempty"`
}

const (
	maxQuery   = 80
	maxResults = 25
)

func (a *api) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > maxQuery {
		q = q[:maxQuery]
	}
	out := []SearchResult{}
	if q == "" {
		writeJSON(w, map[string]any{"q": q, "results": out})
		return
	}
	ctx := r.Context()
	ms, err := a.roster.Sittings(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	cs, err := a.roster.Committees(ctx)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"q": q, "results": search(q, ms, cs)})
}

// search matches q against members' names, states (by name or code) and
// committee names, case-insensitively. States come first, then people, then
// committees; each group in its own order.
func search(q string, ms []store.Sitting, cs []store.CommitteeSummary) []SearchResult {
	needle := strings.ToLower(q)
	out := []SearchResult{}
	for _, j := range geo.All() {
		if strings.Contains(strings.ToLower(j.Name), needle) || strings.EqualFold(j.Code, q) {
			out = append(out, SearchResult{Kind: "state", ID: j.Code, Label: j.Name, Detail: string(j.Kind)})
		}
	}
	var people []SearchResult
	for _, m := range ms {
		if strings.Contains(strings.ToLower(m.Name), needle) {
			mm := member(m)
			people = append(people, SearchResult{Kind: "person", ID: m.ID, Label: m.Name, Detail: describe(mm), Code: mm.PartyCode})
		}
	}
	sort.SliceStable(people, func(i, k int) bool { return people[i].Label < people[k].Label })
	out = append(out, people...)
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.Name), needle) {
			detail := c.Chamber + " committee"
			if c.Parent != "" {
				detail = c.Chamber + " subcommittee"
			}
			out = append(out, SearchResult{Kind: "committee", ID: c.ID, Label: c.Name, Detail: detail})
		}
	}
	if len(out) > maxResults {
		out = out[:maxResults]
	}
	return out
}

// describe is a member's party and seat in a few words.
func describe(m Member) string {
	party := m.Party
	if party == "" {
		party = "party unknown"
	}
	if m.Body == congress.Senate {
		return party + " · Senator for " + m.StateName
	}
	if m.District == nil || *m.District == 0 {
		return party + " · " + m.StateName + " at large"
	}
	return party + " · " + m.StateName + " district " + strconv.Itoa(*m.District)
}

// portraitHandler serves a mirrored portrait from the data volume. The name
// is checked against the only form the portraits job writes, so a request
// cannot reach any other file.
func (a *api) portraitHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if a.portraitDir == "" || !portraits.ValidName(name) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(a.portraitDir, name)) //nolint:gosec // G304: name matches ^[A-Z][0-9]{6}\.jpg$
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// fail logs an unexpected error once and answers 500 without its text.
func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) { failRequest(w, r, err) }

func failRequest(w http.ResponseWriter, r *http.Request, err error) {
	zerolog.Ctx(r.Context()).Error().Err(err).Msg("Could not serve request")
	http.Error(w, "internal error", http.StatusInternalServerError)
}
