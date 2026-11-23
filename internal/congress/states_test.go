package congress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/store"
)

func senator(state, party, caucus string, class int) store.Sitting {
	return store.Sitting{ID: state + party, Body: Senate, State: state, Class: ptr(class), Party: party, Caucus: caucus}
}

func rep(state, party string, district int) store.Sitting {
	return store.Sitting{ID: state + party, Body: House, State: state, District: ptr(district), Party: party}
}

func stateOf(t *testing.T, ss []StateSummary, code string) StateSummary {
	t.Helper()
	for _, s := range ss {
		if s.Code == code {
			return s
		}
	}
	require.Failf(t, "no summary", "no summary for %s", code)
	return StateSummary{}
}

func TestStates_TheColourModes(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		members []store.Sitting
		code    string
		body    string
		want    Delegation
	}{
		"an all-one-party Senate delegation": {
			members: []store.Sitting{senator("DE", "Democratic", "", 1), senator("DE", "Democratic", "", 2)},
			code:    "DE", body: Senate,
			want: Delegation{Seats: 2, Lead: "D", Parties: []PartyCount{{Party: "Democratic", Code: "D", Seats: 2}}},
		},
		"a split Senate delegation is even, not a blend": {
			members: []store.Sitting{senator("PA", "Democratic", "", 1), senator("PA", "Republican", "", 3)},
			code:    "PA", body: Senate,
			want: Delegation{Seats: 2, Lead: LeadEven, Stripes: []string{"D", "R"}, Parties: []PartyCount{
				{Party: "Democratic", Code: "D", Seats: 1}, {Party: "Republican", Code: "R", Seats: 1}}},
		},
		"an Independent beside a Democrat is a split, with whom they caucus": {
			members: []store.Sitting{senator("ME", "Republican", "", 2), senator("ME", "Independent", "Democratic", 1)},
			code:    "ME", body: Senate,
			want: Delegation{Seats: 2, Lead: LeadEven, Stripes: []string{"I", "R"}, Parties: []PartyCount{
				{Party: "Independent", Caucus: "Democratic", Code: "I", Seats: 1}, {Party: "Republican", Code: "R", Seats: 1}}},
		},
		"a vacancy beside a senator: the senator's party leads, the vacancy is counted": {
			members: []store.Sitting{senator("OH", "Republican", "", 1)},
			code:    "OH", body: Senate,
			want: Delegation{Seats: 2, Vacant: 1, Lead: "R", Parties: []PartyCount{{Party: "Republican", Code: "R", Seats: 1}}},
		},
		"both Senate seats vacant": {
			members: nil,
			code:    "OH", body: Senate,
			want: Delegation{Seats: 2, Vacant: 2, Lead: LeadVacant, Parties: []PartyCount{}},
		},
		"an at-large state's single seat": {
			members: []store.Sitting{rep("WY", "Republican", 0)},
			code:    "WY", body: House,
			want: Delegation{Seats: 1, Lead: "R", Parties: []PartyCount{{Party: "Republican", Code: "R", Seats: 1}}},
		},
		"a House delegation led by more seats, with a vacancy": {
			members: []store.Sitting{rep("NV", "Democratic", 1), rep("NV", "Democratic", 3), rep("NV", "Republican", 2)},
			code:    "NV", body: House,
			want: Delegation{Seats: 4, Vacant: 1, Lead: "D", Parties: []PartyCount{
				{Party: "Democratic", Code: "D", Seats: 2}, {Party: "Republican", Code: "R", Seats: 1}}},
		},
		"a House delegation evenly split": {
			members: []store.Sitting{rep("NH", "Democratic", 1), rep("NH", "Republican", 2)},
			code:    "NH", body: House,
			want: Delegation{Seats: 2, Lead: LeadEven, Stripes: []string{"D", "R"}, Parties: []PartyCount{
				{Party: "Democratic", Code: "D", Seats: 1}, {Party: "Republican", Code: "R", Seats: 1}}},
		},
		"a member with no party in the source leads as unknown, not as a guess": {
			members: []store.Sitting{rep("WY", "", 0)},
			code:    "WY", body: House,
			want: Delegation{Seats: 1, Lead: "", Parties: []PartyCount{{Party: "", Code: "", Seats: 1}}},
		},
		"a tie within one party code leads with that code, not a stripe of one colour": {
			members: []store.Sitting{senator("ME", "Independent", "Democratic", 1), senator("ME", "Independent", "Republican", 2)},
			code:    "ME", body: Senate,
			want: Delegation{Seats: 2, Lead: "I", Parties: []PartyCount{
				{Party: "Independent", Caucus: "Democratic", Code: "I", Seats: 1},
				{Party: "Independent", Caucus: "Republican", Code: "I", Seats: 1}}},
		},
		"DC has no senators": {
			members: nil,
			code:    "DC", body: Senate,
			want: Delegation{Seats: 0, Lead: LeadNone, Parties: []PartyCount{}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := stateOf(t, States(tc.members), tc.code)
			got := s.Senate
			if tc.body == House {
				got = s.House
			}
			assert.Equal(t, tc.want, got, "%s's %s delegation", tc.code, tc.body)
		})
	}
}

func TestDelegation_DecidesTheLeadOnSeatsPerPartyCode(t *testing.T) {
	t.Parallel()
	// PRD §3, the same count for every party: seats are totalled by the
	// code the map colours by before the lead is chosen, so an Independent's
	// or a third party's seats are not split by caucus or party name while
	// a major party's are counted whole (review, pass 2).
	type g = [2]string
	tests := map[string]struct {
		held        map[[2]string]int
		wantLead    string
		wantStripes []string
	}{
		"Independents caucusing apart count together: R 2 and I 2 tie": {
			held:     map[[2]string]int{g{"Republican", ""}: 2, g{"Independent", "Democratic"}: 1, g{"Independent", "Republican"}: 1},
			wantLead: LeadEven, wantStripes: []string{"I", "R"},
		},
		"two other parties are never added together: D 1, Green 1, Libertarian 1 tie": {
			held:     map[[2]string]int{g{"Democratic", ""}: 1, g{"Green", ""}: 1, g{"Libertarian", ""}: 1},
			wantLead: LeadEven, wantStripes: []string{"D", "O"},
		},
		"a major party with the most seats leads two smaller ones: R 2, Green 1, Libertarian 1": {
			held:     map[[2]string]int{g{"Republican", ""}: 2, g{"Green", ""}: 1, g{"Libertarian", ""}: 1},
			wantLead: "R",
		},
		"two other parties tied alone share one colour: no one-colour stripe": {
			held:     map[[2]string]int{g{"Green", ""}: 1, g{"Libertarian", ""}: 1},
			wantLead: "O",
		},
		"a three-way tie shows two stripes in the fixed letter order": {
			held:     map[[2]string]int{g{"Republican", ""}: 1, g{"Democratic", ""}: 1, g{"Independent", ""}: 1},
			wantLead: LeadEven, wantStripes: []string{"D", "I"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			seats := 0
			for _, n := range tc.held {
				seats += n
			}
			d := delegation(seats, tc.held)
			assert.Equal(t, tc.wantLead, d.Lead, "the lead")
			assert.Equal(t, tc.wantStripes, d.Stripes, "the tie's stripes")
		})
	}
}

func TestStates_TotalsEqualTheChambers(t *testing.T) {
	t.Parallel()
	// The map's totals are the Congress page's, state by state.
	ms := fullCongress()
	ms = ms[1:] // one Senate seat left empty
	chambers := Summarize(ms, nil)
	var senate, house, senateVacant, houseVacant int
	for _, s := range States(ms) {
		for _, p := range s.Senate.Parties {
			senate += p.Seats
		}
		senateVacant += s.Senate.Vacant
		if s.Voting {
			for _, p := range s.House.Parties {
				house += p.Seats
			}
			houseVacant += s.House.Vacant
		}
	}
	assert.Equal(t, chambers[0].Filled, senate, "Senate seats filled")
	assert.Equal(t, len(chambers[0].Vacancies), senateVacant, "Senate vacancies")
	assert.Equal(t, chambers[1].Filled, house, "voting House seats filled")
	assert.Equal(t, 0, houseVacant, "no House vacancy")
}

func TestNumber_IsTheCongressSittingOnADate(t *testing.T) {
	t.Parallel()
	// A Congress runs from noon on January 3 of an odd year to January 3
	// two years on (20th Amendment); the 119th began in 2025.
	tests := map[string]struct {
		date string
		want int
	}{
		"the 119th's first day":       {date: "2025-01-03", want: 119},
		"the day before it":           {date: "2025-01-02", want: 118},
		"late in its second year":     {date: "2026-11-23", want: 119},
		"the 120th's first day":       {date: "2027-01-03", want: 120},
		"an even year's first months": {date: "2024-06-01", want: 118},
		"not a date":                  {date: "soon", want: 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, Number(tc.date), "the Congress on %s", tc.date)
		})
	}
}

func TestSittingHouse_IsTheCongressOfTheNewestHouseTerm(t *testing.T) {
	t.Parallel()
	member := func(body, start string) store.Sitting { return store.Sitting{Body: body, State: "OH", Start: start} }
	tests := map[string]struct {
		members []store.Sitting
		want    int
	}{
		"the 119th, with a special election's winner": {
			members: []store.Sitting{member(House, "2025-01-03"), member(House, "2025-09-30")}, want: 119},
		"the 120th once the roster lists its members": {
			members: []store.Sitting{member(House, "2025-01-03"), member(House, "2027-01-03")}, want: 120},
		"a senator's term says nothing about the House": {
			members: []store.Sitting{member(House, "2025-01-03"), member(Senate, "2027-01-03")}, want: 119},
		"no House member": {members: nil, want: 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, SittingHouse(tc.members), "the sitting House's Congress")
		})
	}
}

func TestStates_OneSummaryPerJurisdictionInNameOrder(t *testing.T) {
	t.Parallel()
	ss := States(nil)
	require.Len(t, ss, 56, "50 states, DC and five territories")
	for i := 1; i < len(ss); i++ {
		assert.Less(t, ss[i-1].Name, ss[i].Name, "alphabetical, as the map's Tab order")
	}
}
