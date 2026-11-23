package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/store"
)

const legislatorsURL = "https://unitedstates.github.io/congress-legislators/legislators-current.json"

// fakeRoster serves a fixed roster. Only what a test sets is there.
type fakeRoster struct {
	sittings    []store.Sitting
	people      map[string]store.PersonDetail
	committees  []store.CommitteeSummary
	members     map[string][]store.CommitteeMember
	checked     time.Time
	lastHolders map[string]store.LastHolder
}

func (f fakeRoster) LastHolders(_ context.Context, state string) (map[string]store.LastHolder, error) {
	out := map[string]store.LastHolder{}
	for office, h := range f.lastHolders {
		if parts := strings.Split(office, "/"); len(parts) == 3 && parts[1] == state {
			out[office] = h
		}
	}
	return out, nil
}

func (f fakeRoster) Sittings(context.Context) ([]store.Sitting, error)       { return f.sittings, nil }
func (f fakeRoster) CurrentLeadership(context.Context) ([]store.Role, error) { return nil, nil }
func (f fakeRoster) PersonDetail(_ context.Context, id string) (store.PersonDetail, bool, error) {
	d, ok := f.people[id]
	return d, ok, nil
}
func (f fakeRoster) Committees(context.Context) ([]store.CommitteeSummary, error) {
	return f.committees, nil
}
func (f fakeRoster) CommitteeMembers(_ context.Context, id string) ([]store.CommitteeMember, error) {
	return f.members[id], nil
}
func (f fakeRoster) SourceChecks(context.Context) ([]store.SourceState, error) {
	return []store.SourceState{{Source: "legislators", URL: legislatorsURL, CheckedAt: f.checked, ChangedAt: f.checked.Add(-time.Hour)}}, nil
}

func newRosterHandler(t *testing.T, r fakeRoster, portraitDir string, jobs []string, st StatusReader) http.Handler {
	t.Helper()
	if st == nil {
		st = fakeStatus{}
	}
	h, err := New(Options{Store: st, UI: ui, Version: "v-test", Now: func() time.Time { return t0 }, Log: zerolog.Nop(),
		Registerer: prometheus.NewRegistry(), Roster: r, PortraitDir: portraitDir, Jobs: jobs})
	require.NoError(t, err, "building the handler")
	return h
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode, "the response status")
	var v T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v), "decoding the response")
	return v
}

// sitter is a sitting House member of party, from Ohio's district.
func sitter(id, name, party string, district int) store.Sitting {
	return store.Sitting{ID: id, Name: name, LastName: name, Body: "house", State: "OH", District: &district,
		Party: party, Start: "2025-01-03", End: "2027-01-03", Portrait: true}
}

// person is a person's page data for a member of party.
func person(id, name, party, caucus string) store.PersonDetail {
	district := 3
	return store.PersonDetail{
		Person:     store.Person{ID: id, Bioguide: id, First: name, Last: name, Display: name, InCongress: true},
		Provenance: store.Provenance{Source: legislatorsURL, FetchedAt: t0.Add(-48 * time.Hour)},
		Portrait:   true, PortraitSource: "https://unitedstates.github.io/images/congress/225x275/" + id + ".jpg",
		PortraitChecked: t0.Add(-time.Hour), PortraitFetched: t0.Add(-72 * time.Hour),
		Terms: []store.PersonTerm{{Office: "house/OH/3", Body: "house", State: "OH", District: &district,
			Start: "2025-01-03", End: "2027-01-03", Party: party, Caucus: caucus, Current: true}},
		Roles: []store.Role{{Person: id, Name: name, Chamber: "house", Title: "Whip", Start: "2025-01-03"}},
		Seats: []store.PersonSeat{{Committee: "HSXX", Name: "Examples", Chamber: "house", Side: "minority", Rank: 2}},
	}
}

func keys(m map[string]json.RawMessage) []string { return slices.Sorted(maps.Keys(m)) }

func TestPerson_EveryPartyGetsTheSameFields(t *testing.T) {
	t.Parallel()
	// PRD §3, even-handedness: an Independent and a third-party member get
	// every field a major-party member gets.
	parties := map[string][2]string{
		"A000001": {"Democratic", ""},
		"A000002": {"Republican", ""},
		"A000003": {"Independent", "Democratic"},
		"A000004": {"Libertarian", ""},
	}
	r := fakeRoster{people: map[string]store.PersonDetail{}, checked: t0}
	for id, p := range parties {
		r.people[id] = person(id, "Member "+id, p[0], p[1])
	}
	h := newRosterHandler(t, r, "", nil, nil)

	var wantTop, wantTerm []string
	for _, id := range slices.Sorted(maps.Keys(parties)) {
		page := decode[map[string]json.RawMessage](t, get(t, h, "/api/people/"+id))
		var terms []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(page["terms"], &terms), "decoding %s's terms", id)
		require.Len(t, terms, 1, "%s's terms", id)
		if wantTop == nil {
			wantTop, wantTerm = keys(page), keys(terms[0])
			continue
		}
		assert.Equal(t, wantTop, keys(page), "%s (%s) has the same page fields as A000001", id, parties[id][0])
		assert.Equal(t, wantTerm, keys(terms[0]), "%s (%s) has the same term fields as A000001", id, parties[id][0])
	}
}

func TestPerson_EverySourceLineHasItsDates(t *testing.T) {
	t.Parallel()
	// PRD §3 and §8: each fact shows where it came from and when.
	r := fakeRoster{people: map[string]store.PersonDetail{"A000001": person("A000001", "Ann Able", "Democratic", "")}, checked: t0}
	page := decode[PersonResponse](t, get(t, newRosterHandler(t, r, "", nil, nil), "/api/people/A000001"))
	require.Len(t, page.Sources, 2, "the roster line and the portrait line")
	roster, portrait := page.Sources[0], page.Sources[1]
	assert.Equal(t, legislatorsURL, roster.URL)
	assert.Equal(t, t0, roster.CheckedAt, "the roster line says when the dataset was last checked")
	assert.Equal(t, t0.Add(-48*time.Hour), roster.ChangedAt, "the roster line says when this member's record last changed")
	assert.Equal(t, t0.Add(-time.Hour), portrait.CheckedAt, "the portrait line says when it was last checked")
	assert.Equal(t, t0.Add(-72*time.Hour), portrait.ChangedAt, "the portrait line says when the image last changed")
}

func TestRoster_UnknownIDsAre404(t *testing.T) {
	t.Parallel()
	h := newRosterHandler(t, fakeRoster{people: map[string]store.PersonDetail{}}, "", nil, nil)
	tests := map[string]string{
		"unknown person":         "/api/people/Z999999",
		"malformed person id":    "/api/people/robert",
		"unknown committee":      "/api/committees/ZZZZ",
		"malformed committee id": "/api/committees/hs-ag",
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, http.StatusNotFound, get(t, h, path).StatusCode, "GET %s", path)
		})
	}
}

func TestSearch_FindsByPartOfANameByStateAndByCommittee(t *testing.T) {
	t.Parallel()
	r := fakeRoster{
		sittings: []store.Sitting{
			sitter("A000001", "Charles Schumer", "Democratic", 1),
			sitter("A000002", "Jim Jordan", "Republican", 4),
		},
		committees: []store.CommitteeSummary{{ID: "HSJU", Name: "House Committee on the Judiciary", Chamber: "house"}},
		checked:    t0,
	}
	h := newRosterHandler(t, r, "", nil, nil)
	tests := map[string]struct {
		q    string
		want []string // kind:id, in order
	}{
		"part of a name":       {q: "schum", want: []string{"person:A000001"}},
		"a state by name":      {q: "ohio", want: []string{"state:OH"}},
		"a state by its code":  {q: "OH", want: []string{"state:OH"}},
		"a committee by word":  {q: "judiciary", want: []string{"committee:HSJU"}},
		"nothing":              {q: "zzzz", want: []string{}},
		"an empty query finds": {q: "", want: []string{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := decode[struct {
				Results []SearchResult `json:"results"`
			}](t, get(t, h, "/api/search?q="+tc.q))
			got := []string{}
			for _, x := range res.Results {
				if x.Kind == "person" && tc.q == "OH" {
					continue // "OH" also matches names containing "oh"; the state comes first
				}
				got = append(got, x.Kind+":"+x.ID)
			}
			assert.Equal(t, tc.want, got, "search for %q", tc.q)
		})
	}
}

func TestPortraits_ServesOnlyAMirroredFileByItsOwnName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A000001.jpg"), []byte("\xff\xd8\xff jpeg"), 0o600), "writing a portrait")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.jpg"), []byte("no"), 0o600), "writing a file outside")
	h := newRosterHandler(t, fakeRoster{}, dir, nil, nil)
	tests := map[string]struct {
		path string
		want int
	}{
		"a mirrored portrait": {path: "/portraits/A000001.jpg", want: http.StatusOK},
		"a missing portrait":  {path: "/portraits/A000002.jpg", want: http.StatusNotFound},
		"a parent directory":  {path: "/portraits/..%2Fsecret.jpg", want: http.StatusNotFound},
		"another name":        {path: "/portraits/secret.jpg", want: http.StatusNotFound},
		"a lower-case id":     {path: "/portraits/a000001.jpg", want: http.StatusNotFound},
		"another file type":   {path: "/portraits/A000001.png", want: http.StatusNotFound},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, get(t, h, tc.path).StatusCode, "GET %s", tc.path)
		})
	}
}

func TestPortraits_TheBareDirectoryIsTheAppShellNotAListing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A000001.jpg"), []byte("\xff\xd8\xff jpeg"), 0o600), "writing a portrait")
	resp := get(t, newRosterHandler(t, fakeRoster{}, dir, nil, nil), "/portraits/")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading the response")
	assert.NotContains(t, string(body), "A000001", "no file name is listed")
	assert.Contains(t, string(body), "<title>Eunomia</title>", "an unknown path gets the app shell")
}

func TestStatus_ListsOnlyRegisteredJobs(t *testing.T) {
	t.Parallel()
	// A job since removed (noop, after E1a) keeps its row as history; the
	// status page shows only the jobs that still run.
	st := fakeStatus{jobs: []store.JobStatus{
		{Name: "noop", LastRun: t0, LastOK: t0, OK: true},
		{Name: "roster", LastRun: t0, LastOK: t0, OK: true},
	}}
	h := newRosterHandler(t, fakeRoster{}, "", []string{"roster", "portraits"}, st)
	got := decode[Status](t, get(t, h, "/api/status"))
	names := make([]string, 0, len(got.Jobs))
	for _, j := range got.Jobs {
		names = append(names, j.Name)
	}
	assert.Equal(t, []string{"roster"}, names, "only registered jobs are listed")
}
