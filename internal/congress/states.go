package congress

import (
	"sort"
	"time"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

// What a delegation's Lead is when no single party has the most seats.
const (
	// LeadEven: two or more parties tie for the most filled seats, as in a
	// split Senate delegation. The map draws the tied parties' colours as
	// stripes, never a blend (docs/web-conventions.md, party colour).
	LeadEven = "even"
	// LeadVacant: the chamber has seats here and none is filled.
	LeadVacant = "vacant"
	// LeadNone: the chamber has no seats here (DC and the territories
	// have no senators).
	LeadNone = "none"
)

// Delegation is one chamber's seats for one state: what the map's colour
// mode for that chamber shows.
type Delegation struct {
	// Seats is how many seats the state has in the chamber (for DC and a
	// territory's House row, its one non-voting delegate).
	Seats  int `json:"seats"`
	Vacant int `json:"vacant"`
	// Lead is the party code that holds more filled seats than any other,
	// or LeadEven, LeadVacant or LeadNone. A member with no party in the
	// source has code "", so a lead of "" is "party unknown".
	Lead    string       `json:"lead"`
	Parties []PartyCount `json:"parties"`
}

// StateSummary is one state's, DC's or a territory's delegation in both
// chambers: a row of the map's list and the data for its colour modes.
type StateSummary struct {
	Code string   `json:"code"`
	Name string   `json:"name"`
	Kind geo.Kind `json:"kind"`
	// Voting is false for DC and the territories, whose delegate does not
	// vote on the House floor.
	Voting bool       `json:"voting"`
	Senate Delegation `json:"senate"`
	House  Delegation `json:"house"`
}

// States summarises every jurisdiction's delegation, in name order (the
// map's Tab order and its list's order).
func States(members []store.Sitting) []StateSummary {
	type key struct{ state, body string }
	counts := map[key]map[[2]string]int{}
	for _, m := range members {
		k := key{m.State, m.Body}
		if counts[k] == nil {
			counts[k] = map[[2]string]int{}
		}
		counts[k][[2]string{m.Party, m.Caucus}]++
	}
	all := geo.All()
	out := make([]StateSummary, 0, len(all))
	for _, j := range all {
		out = append(out, StateSummary{
			Code: j.Code, Name: j.Name, Kind: j.Kind, Voting: j.Voting(),
			Senate: delegation(j.SenateSeats, counts[key{j.Code, Senate}]),
			House:  delegation(j.HouseSeats, counts[key{j.Code, House}]),
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

func delegation(seats int, held map[[2]string]int) Delegation {
	d := Delegation{Seats: seats, Parties: partyCounts(held)}
	filled := 0
	for _, p := range d.Parties {
		filled += p.Seats
	}
	d.Vacant = max(0, seats-filled)
	switch {
	case seats == 0:
		d.Lead = LeadNone
	case filled == 0:
		d.Lead = LeadVacant
	case len(d.Parties) > 1 && d.Parties[0].Seats == d.Parties[1].Seats:
		d.Lead = LeadEven
	default:
		d.Lead = d.Parties[0].Code
	}
	return d
}

// Number is the Congress sitting on a YYYY-MM-DD date: each runs from
// January 3 of an odd year to January 3 two years on, the 1st from 1789.
// It returns 0 for a value that is not a date.
func Number(date string) int {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return 0
	}
	n := (t.Year()-1789)/2 + 1
	if t.Year()%2 == 1 && t.Before(time.Date(t.Year(), time.January, 3, 0, 0, 0, 0, time.UTC)) {
		n--
	}
	return n
}

// SittingHouse is the Congress the sitting House members belong to, from
// the newest House term's start: the roster's word, not the clock's, so a
// change of Congress shows when the source lists it (PRD §3). It is 0 with
// no House member.
func SittingHouse(members []store.Sitting) int {
	newest := ""
	for _, m := range members {
		if m.Body == House && m.Start > newest {
			newest = m.Start
		}
	}
	return Number(newest)
}
