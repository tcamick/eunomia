package httpapi

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/congress"
	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

// ohio is a state with both senators, two of its 15 House seats filled out
// of order, and DC's delegate.
func ohio() fakeRoster {
	cls := 1
	return fakeRoster{
		checked: t0,
		sittings: []store.Sitting{
			{ID: "S000001", Name: "Sam Senate", LastName: "Senate", Body: "senate", State: "OH", Class: &cls, Party: "Republican"},
			{ID: "S000002", Name: "Sue Senate", LastName: "Senate", Body: "senate", State: "OH", Class: &cls, Party: "Democratic"},
			sitter("H000004", "Hal Four", "Republican", 4),
			sitter("H000001", "Hana One", "Democratic", 1),
			{ID: "D000001", Name: "Dee Delegate", LastName: "Delegate", Body: "house", State: "DC", District: new(int), Party: "Democratic"},
		},
		lastHolders: map[string]store.LastHolder{
			"house/OH/3": {Person: "L000003", Name: "Lou Left", Party: "Republican", LeftOn: "2026-11-30"},
		},
	}
}

func TestStates_SummarisesEveryJurisdiction(t *testing.T) {
	t.Parallel()
	got := decode[struct {
		States  []congress.StateSummary `json:"states"`
		Sources []SourceLine            `json:"sources"`
	}](t, get(t, newRosterHandler(t, ohio(), "", nil, nil), "/api/states"))
	require.Len(t, got.States, len(geo.All()), "one summary per state, DC and territory")
	var oh congress.StateSummary
	for _, s := range got.States {
		if s.Code == "OH" {
			oh = s
		}
	}
	assert.Equal(t, congress.LeadEven, oh.Senate.Lead, "Ohio's split Senate delegation")
	assert.Equal(t, 13, oh.House.Vacant, "Ohio's 13 unfilled House seats in this fixture")
	names := make([]string, 0, len(got.Sources))
	for _, s := range got.Sources {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, rosterSourceName, "the roster is cited")
	assert.Contains(t, names, "Census Bureau, 2020 apportionment", "the seat counts' source is cited")
	assert.Contains(t, names, stateShapesName, "the map's lines are credited")
}

func TestState_APageHasItsMembersVacanciesAndSources(t *testing.T) {
	t.Parallel()
	got := decode[StateResponse](t, get(t, newRosterHandler(t, ohio(), "", nil, nil), "/api/states/OH"))
	assert.Equal(t, "Ohio", got.State.Name, "the page is Ohio's")
	require.Len(t, got.Senators, 2, "both senators")
	houseIDs := make([]string, 0, len(got.House))
	for _, m := range got.House {
		houseIDs = append(houseIDs, m.ID)
	}
	assert.Equal(t, []string{"H000001", "H000004"}, houseIDs, "House members in district order")
	assert.Len(t, got.Vacancies, 13, "every unfilled seat is listed as vacant")
	var three StateVacancy
	for _, v := range got.Vacancies {
		if v.District != nil && *v.District == 3 {
			three = v
		}
	}
	require.NotNil(t, three.LastHolder, "OH-3's last holder, seen leaving mid-term")
	assert.Equal(t, "Lou Left", three.LastHolder.Name, "OH-3's last holder by name")
	assert.Equal(t, 119, got.DistrictMap, "the district map's lines are the 119th Congress's")
	names := make([]string, 0, len(got.Sources))
	for _, s := range got.Sources {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, districtShapesName, "the Census Bureau is credited for the district map")
}

func TestState_SaysWhenTheMembersAreNotTheDrawnLinesCongress(t *testing.T) {
	t.Parallel()
	// Review B2: from 2027-01-03 the roster lists the 120th's members while
	// the shapes are the 119th's lines, until gate G2.
	tests := map[string]struct {
		start string
		want  int
	}{
		"the 119th's members on the 119th's lines": {start: "2025-01-03", want: 119},
		"the 120th's members, the 119th's lines":   {start: "2027-01-03", want: 120},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := ohio()
			r.sittings[2].Start = tc.start
			got := decode[StateResponse](t, get(t, newRosterHandler(t, r, "", nil, nil), "/api/states/OH"))
			assert.Equal(t, geo.DistrictShapesCongress, got.DistrictMap, "the drawn lines' Congress")
			assert.Equal(t, tc.want, got.HouseCongress, "the sitting members' Congress")
		})
	}
}

func TestState_AVacantSenateSeatNamesItsLastHolder(t *testing.T) {
	t.Parallel()
	// Review should-fix 5: one senator left mid-term, and the roster saw it.
	r := ohio()
	r.sittings = slices.Delete(r.sittings, 1, 2) // Sue Senate is gone
	r.lastHolders["senate/OH/3"] = store.LastHolder{Person: "S000002", Name: "Sue Senate", Party: "Democratic", LeftOn: "2026-12-01"}
	got := decode[StateResponse](t, get(t, newRosterHandler(t, r, "", nil, nil), "/api/states/OH"))
	var senate []StateVacancy
	for _, v := range got.Vacancies {
		if v.Body == "senate" {
			senate = append(senate, v)
		}
	}
	require.Len(t, senate, 1, "one empty Senate seat")
	require.NotNil(t, senate[0].LastHolder, "it names the senator the roster saw leave")
	assert.Equal(t, "Sue Senate", senate[0].LastHolder.Name, "the last holder's name")
}

func TestState_DCHasItsDelegateAndNoDistrictMap(t *testing.T) {
	t.Parallel()
	got := decode[StateResponse](t, get(t, newRosterHandler(t, ohio(), "", nil, nil), "/api/states/DC"))
	assert.Empty(t, got.Senators, "DC has no senators")
	require.Len(t, got.House, 1, "DC's delegate")
	assert.False(t, got.House[0].Voting, "the delegate does not vote on the floor")
	assert.Zero(t, got.DistrictMap, "DC has no district map")
	assert.Empty(t, got.Vacancies, "DC's one seat is filled")
}

func TestState_UnknownCodesAre404(t *testing.T) {
	t.Parallel()
	h := newRosterHandler(t, ohio(), "", nil, nil)
	for name, path := range map[string]string{
		"an unknown code":  "/api/states/ZZ",
		"a lower-case one": "/api/states/oh",
		"a name":           "/api/states/Ohio",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, http.StatusNotFound, get(t, h, path).StatusCode, "GET %s", path)
		})
	}
}
