package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApportionment_MatchesTheCensusTotals(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 435, VotingHouseSeats(), "the 2020 table's total")
	assert.Equal(t, 100, SenateSeats())
	states := 0
	for _, j := range All() {
		if j.Kind == KindState {
			states++
		}
	}
	assert.Equal(t, 50, states)
	assert.Len(t, All(), 56, "50 states, DC and five territories")
}

func TestApportionment_SpotChecksAgainstTheTable(t *testing.T) {
	t.Parallel()
	// Read off the Census table by hand on 2026-11-22.
	tests := map[string]struct {
		code  string
		seats int
	}{
		"California lost a seat to 52":   {code: "CA", seats: 52},
		"Texas gained two to 38":         {code: "TX", seats: 38},
		"Montana gained its second":      {code: "MT", seats: 2},
		"Wyoming has one, at large":      {code: "WY", seats: 1},
		"West Virginia dropped to two":   {code: "WV", seats: 2},
		"Puerto Rico sends one delegate": {code: "PR", seats: 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			j, ok := Lookup(tc.code)
			require.True(t, ok, "lookup %s", tc.code)
			assert.Equal(t, tc.seats, j.HouseSeats)
		})
	}
}

func TestHouseDistricts_AtLargeIsZero(t *testing.T) {
	t.Parallel()
	wy, _ := Lookup("WY")
	assert.Equal(t, []int{0}, HouseDistricts(wy))
	dc, _ := Lookup("DC")
	assert.Equal(t, []int{0}, HouseDistricts(dc))
	assert.False(t, dc.Voting(), "DC's delegate does not vote on the floor")
	ri, _ := Lookup("RI")
	assert.Equal(t, []int{1, 2}, HouseDistricts(ri))
}

func TestLookup_UnknownCode(t *testing.T) {
	t.Parallel()
	_, ok := Lookup("ZZ")
	assert.False(t, ok)
}
