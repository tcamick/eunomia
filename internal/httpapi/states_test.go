package httpapi

import (
	"net/http"
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
	require.NotEmpty(t, got.Sources, "the roster source line")
}

func TestState_APageHasItsMembersVacanciesAndSources(t *testing.T) {
	t.Parallel()
	got := decode[StateResponse](t, get(t, newRosterHandler(t, ohio(), "", nil, nil), "/api/states/OH"))
	assert.Equal(t, "Ohio", got.State.Name)
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
	assert.Equal(t, "Lou Left", three.LastHolder.Name)
	assert.Equal(t, 119, got.DistrictMap, "the district map's lines are the 119th Congress's")
	names := make([]string, 0, len(got.Sources))
	for _, s := range got.Sources {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, districtShapesName, "the Census Bureau is credited for the district map")
}

func TestState_DCHasItsDelegateAndNoDistrictMap(t *testing.T) {
	t.Parallel()
	got := decode[StateResponse](t, get(t, newRosterHandler(t, ohio(), "", nil, nil), "/api/states/DC"))
	assert.Empty(t, got.Senators, "DC has no senators")
	require.Len(t, got.House, 1, "DC's delegate")
	assert.False(t, got.House[0].Voting, "the delegate does not vote on the floor")
	assert.Zero(t, got.DistrictMap, "DC has no district map")
	assert.Empty(t, got.Vacancies)
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
