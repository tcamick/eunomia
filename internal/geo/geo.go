// Package geo holds the fixed facts about places that the roster needs: the
// states, the District of Columbia and the territories that send a delegate,
// and how many House seats each has. Vacancies are seats these facts expect
// and no current term fills (docs/implementation-plan.md, E1a).
package geo

import "sort"

// Kind says what sort of place a jurisdiction is.
type Kind string

// Kinds of jurisdiction.
const (
	KindState     Kind = "state"
	KindDistrict  Kind = "district" // the District of Columbia
	KindTerritory Kind = "territory"
)

// Jurisdiction is a state, DC or a territory with a seat in the House.
type Jurisdiction struct {
	Code string `json:"code"` // USPS code, as the roster source uses
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	// HouseSeats is the number of voting representatives (states) or 1 for
	// a non-voting delegate or resident commissioner (DC, territories).
	HouseSeats int `json:"houseSeats"`
	// SenateSeats is 2 for a state and 0 otherwise.
	SenateSeats int `json:"senateSeats"`
}

// Voting reports whether the jurisdiction's House members vote on the floor.
func (j Jurisdiction) Voting() bool { return j.Kind == KindState }

// AtLarge reports whether the jurisdiction elects its House member(s) from
// the whole jurisdiction: one seat, which the roster source numbers 0.
func (j Jurisdiction) AtLarge() bool { return j.HouseSeats == 1 }

// ApportionmentSource is where the states' seat counts come from.
const ApportionmentSource = "https://www2.census.gov/programs-surveys/decennial/2020/data/apportionment/apportionment-2020-table01.xlsx"

// ApportionmentChecked is when the table below was compared with the source.
const ApportionmentChecked = "2026-11-22"

// Census Bureau, "Table 1. Apportionment Population and Number of
// Representatives by State: 2020 Census". Applies from the 118th Congress
// until the 2030 apportionment. DC and the territories are not apportioned;
// each sends one non-voting member under federal law.
var all = []Jurisdiction{
	{"AL", "Alabama", KindState, 7, 2},
	{"AK", "Alaska", KindState, 1, 2},
	{"AZ", "Arizona", KindState, 9, 2},
	{"AR", "Arkansas", KindState, 4, 2},
	{"CA", "California", KindState, 52, 2},
	{"CO", "Colorado", KindState, 8, 2},
	{"CT", "Connecticut", KindState, 5, 2},
	{"DE", "Delaware", KindState, 1, 2},
	{"FL", "Florida", KindState, 28, 2},
	{"GA", "Georgia", KindState, 14, 2},
	{"HI", "Hawaii", KindState, 2, 2},
	{"ID", "Idaho", KindState, 2, 2},
	{"IL", "Illinois", KindState, 17, 2},
	{"IN", "Indiana", KindState, 9, 2},
	{"IA", "Iowa", KindState, 4, 2},
	{"KS", "Kansas", KindState, 4, 2},
	{"KY", "Kentucky", KindState, 6, 2},
	{"LA", "Louisiana", KindState, 6, 2},
	{"ME", "Maine", KindState, 2, 2},
	{"MD", "Maryland", KindState, 8, 2},
	{"MA", "Massachusetts", KindState, 9, 2},
	{"MI", "Michigan", KindState, 13, 2},
	{"MN", "Minnesota", KindState, 8, 2},
	{"MS", "Mississippi", KindState, 4, 2},
	{"MO", "Missouri", KindState, 8, 2},
	{"MT", "Montana", KindState, 2, 2},
	{"NE", "Nebraska", KindState, 3, 2},
	{"NV", "Nevada", KindState, 4, 2},
	{"NH", "New Hampshire", KindState, 2, 2},
	{"NJ", "New Jersey", KindState, 12, 2},
	{"NM", "New Mexico", KindState, 3, 2},
	{"NY", "New York", KindState, 26, 2},
	{"NC", "North Carolina", KindState, 14, 2},
	{"ND", "North Dakota", KindState, 1, 2},
	{"OH", "Ohio", KindState, 15, 2},
	{"OK", "Oklahoma", KindState, 5, 2},
	{"OR", "Oregon", KindState, 6, 2},
	{"PA", "Pennsylvania", KindState, 17, 2},
	{"RI", "Rhode Island", KindState, 2, 2},
	{"SC", "South Carolina", KindState, 7, 2},
	{"SD", "South Dakota", KindState, 1, 2},
	{"TN", "Tennessee", KindState, 9, 2},
	{"TX", "Texas", KindState, 38, 2},
	{"UT", "Utah", KindState, 4, 2},
	{"VT", "Vermont", KindState, 1, 2},
	{"VA", "Virginia", KindState, 11, 2},
	{"WA", "Washington", KindState, 10, 2},
	{"WV", "West Virginia", KindState, 2, 2},
	{"WI", "Wisconsin", KindState, 8, 2},
	{"WY", "Wyoming", KindState, 1, 2},
	{"DC", "District of Columbia", KindDistrict, 1, 0},
	{"PR", "Puerto Rico", KindTerritory, 1, 0},
	{"GU", "Guam", KindTerritory, 1, 0},
	{"VI", "U.S. Virgin Islands", KindTerritory, 1, 0},
	{"AS", "American Samoa", KindTerritory, 1, 0},
	{"MP", "Northern Mariana Islands", KindTerritory, 1, 0},
}

var byCode = func() map[string]Jurisdiction {
	m := make(map[string]Jurisdiction, len(all))
	for _, j := range all {
		m[j.Code] = j
	}
	return m
}()

// All returns every jurisdiction, sorted by name.
func All() []Jurisdiction {
	out := append([]Jurisdiction(nil), all...)
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// Lookup returns the jurisdiction with the USPS code, if there is one.
func Lookup(code string) (Jurisdiction, bool) {
	j, ok := byCode[code]
	return j, ok
}

// VotingHouseSeats is the size of the House's voting membership.
func VotingHouseSeats() int {
	n := 0
	for _, j := range all {
		if j.Voting() {
			n += j.HouseSeats
		}
	}
	return n
}

// SenateSeats is the size of the Senate.
func SenateSeats() int {
	n := 0
	for _, j := range all {
		n += j.SenateSeats
	}
	return n
}

// HouseDistricts lists the district numbers a jurisdiction's House seats use
// in the roster source: 0 for an at-large seat, else 1..n.
func HouseDistricts(j Jurisdiction) []int {
	if j.AtLarge() {
		return []int{0}
	}
	ds := make([]int, j.HouseSeats)
	for i := range ds {
		ds[i] = i + 1
	}
	return ds
}
