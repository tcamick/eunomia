package congress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

func ptr(n int) *int { return &n }

// fullCongress seats every apportioned seat: senators alternate D and R,
// House seats alternate R and D, delegates are D.
func fullCongress() []store.Sitting {
	var ms []store.Sitting
	i := 0
	for _, j := range geo.All() {
		for k := 0; k < j.SenateSeats; k++ {
			p := "Democratic"
			if k == 1 {
				p = "Republican"
			}
			ms = append(ms, store.Sitting{ID: j.Code + "S" + string(rune('0'+k)), Body: Senate, State: j.Code, Class: ptr(k + 1), Party: p})
		}
		for _, d := range geo.HouseDistricts(j) {
			p := "Republican"
			if i%2 == 1 || !j.Voting() {
				p = "Democratic"
			}
			i++
			ms = append(ms, store.Sitting{ID: j.Code + "H", Body: House, State: j.Code, District: ptr(d), Party: p})
		}
	}
	return ms
}

func TestSummarize_AFullCongressHasNoVacancies(t *testing.T) {
	t.Parallel()
	cs := Summarize(fullCongress(), nil)
	require.Len(t, cs, 2)
	senate, house := cs[0], cs[1]
	assert.Equal(t, 100, senate.Seats)
	assert.Equal(t, 100, senate.Filled)
	assert.Equal(t, 51, senate.Majority)
	assert.Empty(t, senate.Vacancies)
	assert.Equal(t, 435, house.Seats)
	assert.Equal(t, 435, house.Filled, "delegates are not voting seats")
	assert.Equal(t, 218, house.Majority)
	assert.Empty(t, house.Vacancies)
	assert.Len(t, house.Delegates, 6, "DC and five territories")
	total := 0
	for _, p := range house.Parties {
		total += p.Seats
	}
	assert.Equal(t, 435, total, "the party counts add up to the filled voting seats")
}

func TestSummarize_FindsVacancies(t *testing.T) {
	t.Parallel()
	ms := fullCongress()
	var kept []store.Sitting
	for _, m := range ms {
		switch {
		case m.Body == House && m.State == "TX" && *m.District == 23:
		case m.Body == Senate && m.State == "OH" && *m.Class == 1:
		case m.Body == House && m.State == "GU":
		default:
			kept = append(kept, m)
		}
	}
	cs := Summarize(kept, nil)
	senate, house := cs[0], cs[1]
	require.Len(t, senate.Vacancies, 1)
	assert.Equal(t, "OH", senate.Vacancies[0].State)
	assert.Equal(t, 99, senate.Filled)
	require.Len(t, house.Vacancies, 2)
	byState := map[string]Vacancy{}
	for _, v := range house.Vacancies {
		byState[v.State] = v
	}
	require.NotNil(t, byState["TX"].District)
	assert.Equal(t, 23, *byState["TX"].District)
	assert.True(t, byState["TX"].Voting)
	assert.False(t, byState["GU"].Voting, "Guam's delegate seat is vacant but does not vote")
	assert.Equal(t, 434, house.Filled)
}

func TestSummarize_IndependentsAreCountedWithTheirCaucus(t *testing.T) {
	t.Parallel()
	ms := []store.Sitting{
		{ID: "a", Body: Senate, State: "VT", Party: "Independent", Caucus: "Democratic"},
		{ID: "b", Body: Senate, State: "ME", Party: "Independent", Caucus: "Democratic"},
		{ID: "c", Body: Senate, State: "VT", Party: "Democratic"},
		{ID: "d", Body: Senate, State: "ME", Party: "Republican"},
		{ID: "e", Body: Senate, State: "WY", Party: "Republican"},
		{ID: "f", Body: Senate, State: "WY", Party: "Republican"},
	}
	senate := Summarize(ms, nil)[0]
	assert.Equal(t, []PartyCount{
		{Party: "Republican", Code: "R", Seats: 3},
		{Party: "Independent", Caucus: "Democratic", Code: "I", Seats: 2},
		{Party: "Democratic", Code: "D", Seats: 1},
	}, senate.Parties, "largest first; an Independent keeps their own party and says whom they caucus with")
}

func TestSummarize_LeadersInTheFixedOrder(t *testing.T) {
	t.Parallel()
	roles := []store.Role{
		{Person: "w", Chamber: Senate, Title: "Senate Minority Whip"},
		{Person: "m", Chamber: Senate, Title: "Senate Majority Leader"},
		{Person: "x", Chamber: Senate, Title: "Senate Democratic Steering Committee Chair"},
		{Person: "s", Chamber: House, Title: "Speaker of the House"},
	}
	cs := Summarize(nil, roles)
	senateTitles := make([]string, 0, len(cs[0].Leaders))
	for _, r := range cs[0].Leaders {
		senateTitles = append(senateTitles, r.Title)
	}
	assert.Equal(t, []string{"Senate Majority Leader", "Senate Minority Whip"}, senateTitles,
		"top posts only, in the page's order")
	require.Len(t, cs[1].Leaders, 1)
	assert.Equal(t, "s", cs[1].Leaders[0].Person)
}

func TestPartyCode(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "D", PartyCode("Democratic"))
	assert.Equal(t, "R", PartyCode("Republican"))
	assert.Equal(t, "I", PartyCode("Independent"))
	assert.Equal(t, "O", PartyCode("Libertarian"))
	assert.Empty(t, PartyCode(""), "an unknown party has no colour, not a guessed one")
}
