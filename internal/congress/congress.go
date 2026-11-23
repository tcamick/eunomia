// Package congress summarises the sitting members into what the Congress
// page shows: each chamber's seats by party, the majority threshold,
// vacancies, non-voting delegates and leaders. It computes; it fetches and
// stores nothing.
package congress

import (
	"sort"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

// Bodies, as the store names them.
const (
	Senate = "senate"
	House  = "house"
)

// PartyCount is the number of voting seats held by members of one party who
// caucus with one party (an Independent who caucuses with the Democrats is
// its own row, so the page can say so).
type PartyCount struct {
	Party  string `json:"party"`
	Caucus string `json:"caucus,omitempty"`
	Code   string `json:"code"`
	Seats  int    `json:"seats"`
}

// Vacancy is a seat the apportionment expects and no current term fills.
type Vacancy struct {
	State     string `json:"state"`
	StateName string `json:"stateName"`
	District  *int   `json:"district,omitempty"`
	Voting    bool   `json:"voting"`
}

// Chamber is one body's summary.
type Chamber struct {
	Body string `json:"body"`
	// Seats is the voting membership when full; Filled is how many voting
	// seats have a member.
	Seats  int `json:"seats"`
	Filled int `json:"filled"`
	// Majority is the number of seats that is more than half of a full
	// chamber.
	Majority  int             `json:"majority"`
	Parties   []PartyCount    `json:"parties"`
	Vacancies []Vacancy       `json:"vacancies"`
	Delegates []store.Sitting `json:"delegates"`
	Leaders   []store.Role    `json:"leaders"`
}

// topPosts are the chamber leadership posts the Congress page lists, in the
// order it lists them; any other current post is on the member's own page.
var topPosts = map[string][]string{
	Senate: {
		"Senate Majority Leader", "Senate Minority Leader", "Senate Majority Whip", "Senate Minority Whip",
		"President Pro Tempore of the Senate",
	},
	House: {
		"Speaker of the House", "House Majority Leader", "House Minority Leader", "House Majority Whip",
		"House Minority Whip",
	},
}

// PartyCode is the short code the party colour tokens use: D, R, I or O.
// Every party outside the two largest and independents is O, with its own
// name shown beside the colour (docs/web-conventions.md, party colour).
func PartyCode(party string) string {
	switch party {
	case "Democratic":
		return "D"
	case "Republican":
		return "R"
	case "Independent":
		return "I"
	case "":
		return ""
	default:
		return "O"
	}
}

// Summarize builds both chambers from the sitting members and current posts.
func Summarize(members []store.Sitting, roles []store.Role) []Chamber {
	senate := Chamber{Body: Senate, Seats: geo.SenateSeats(), Vacancies: []Vacancy{}, Parties: []PartyCount{}}
	house := Chamber{Body: House, Seats: geo.VotingHouseSeats(), Vacancies: []Vacancy{}, Parties: []PartyCount{}}
	senate.Majority = senate.Seats/2 + 1
	house.Majority = house.Seats/2 + 1

	senateByState := map[string]int{}
	houseSeats := map[string]map[int]bool{}
	counts := map[string]map[[2]string]int{Senate: {}, House: {}}
	for _, m := range members {
		j, known := geo.Lookup(m.State)
		switch {
		case m.Body == Senate:
			senateByState[m.State]++
			senate.Filled++
			counts[Senate][[2]string{m.Party, m.Caucus}]++
		case known && !j.Voting():
			house.Delegates = append(house.Delegates, m)
			markSeat(houseSeats, m)
		default:
			house.Filled++
			counts[House][[2]string{m.Party, m.Caucus}]++
			markSeat(houseSeats, m)
		}
	}
	senate.Parties = partyCounts(counts[Senate])
	house.Parties = partyCounts(counts[House])

	for _, j := range geo.All() {
		for n := senateByState[j.Code]; n < j.SenateSeats; n++ {
			senate.Vacancies = append(senate.Vacancies, Vacancy{State: j.Code, StateName: j.Name, Voting: true})
		}
		for _, d := range geo.HouseDistricts(j) {
			if !houseSeats[j.Code][d] {
				house.Vacancies = append(house.Vacancies, Vacancy{State: j.Code, StateName: j.Name, District: &d, Voting: j.Voting()})
			}
		}
	}
	senate.Leaders = leaders(roles, Senate)
	house.Leaders = leaders(roles, House)
	// The page reads every list; an empty one is [], never missing or null.
	senate.Delegates = []store.Sitting{}
	if house.Delegates == nil {
		house.Delegates = []store.Sitting{}
	}
	return []Chamber{senate, house}
}

func markSeat(seats map[string]map[int]bool, m store.Sitting) {
	if m.District == nil {
		return
	}
	if seats[m.State] == nil {
		seats[m.State] = map[int]bool{}
	}
	seats[m.State][*m.District] = true
}

// partyCounts orders the groups by seats, largest first, then by name, so the
// order says nothing the numbers do not.
func partyCounts(m map[[2]string]int) []PartyCount {
	out := make([]PartyCount, 0, len(m))
	for k, n := range m {
		out = append(out, PartyCount{Party: k[0], Caucus: k[1], Code: PartyCode(k[0]), Seats: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seats != out[j].Seats {
			return out[i].Seats > out[j].Seats
		}
		if out[i].Party != out[j].Party {
			return out[i].Party < out[j].Party
		}
		return out[i].Caucus < out[j].Caucus
	})
	return out
}

func leaders(roles []store.Role, body string) []store.Role {
	out := []store.Role{}
	for _, title := range topPosts[body] {
		for _, r := range roles {
			if r.Chamber == body && r.Title == title {
				out = append(out, r)
			}
		}
	}
	return out
}
