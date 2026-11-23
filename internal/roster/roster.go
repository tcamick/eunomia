// Package roster turns the congress-legislators files into the store's rows
// and works out what changed since the last run. Diff is a pure function of
// the stored state and the new snapshot, so the rules about history, changes
// and no-op runs are tested without a database.
package roster

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/legislators"
	"github.com/tcamick/eunomia/internal/store"
)

// Change kinds the roster records.
const (
	KindArrived          = "arrived"
	KindLeft             = "left"
	KindPartyChanged     = "party_changed"
	KindOfficeChanged    = "office_changed"
	KindLeadershipGained = "leadership_gained"
	KindLeadershipLost   = "leadership_lost"
	KindChairChanged     = "chair_changed"
	KindSeatJoined       = "seat_joined"
	KindSeatLeft         = "seat_left"
	// KindRecordCorrected: the source stopped listing a term or role of a
	// member it still lists, usually an upstream correction. The old row is
	// kept and dated, never deleted.
	KindRecordCorrected = "record_corrected"
)

// Party names the parties' own way. The source writes "Democrat" for the
// Democratic Party; every other value is kept as given (AGENTS.md,
// Even-handedness).
func Party(source string) string {
	if source == "Democrat" {
		return "Democratic"
	}
	return source
}

// OfficeID names a seat: senate/<state>/<class> or house/<state>/<district>.
func OfficeID(t legislators.Term) string {
	if t.Type == "sen" {
		return "senate/" + t.State + "/" + strconv.Itoa(*t.Class)
	}
	return "house/" + t.State + "/" + strconv.Itoa(*t.District)
}

// Bodies of Congress, as the store names them.
const (
	BodySenate = "senate"
	BodyHouse  = "house"
)

func body(t legislators.Term) string {
	if t.Type == "sen" {
		return BodySenate
	}
	return BodyHouse
}

// Desired is the snapshot as rows: every member in the file is in Congress,
// and each one's last term is current.
func Desired(snap *legislators.Snapshot) (store.RosterState, error) {
	d := store.RosterState{
		People:     map[string]store.Person{},
		Terms:      map[store.TermKey]store.Term{},
		Leadership: map[store.LeadershipKey]store.Leadership{},
		Committees: map[string]store.Committee{},
		Seats:      map[store.SeatKey]store.Seat{},
	}
	for _, l := range snap.Legislators {
		id := l.ID.Bioguide
		d.People[id] = store.Person{
			ID: id, Bioguide: id, LIS: l.ID.LIS, FECIDs: strings.Join(l.ID.FEC, ","), ICPSR: l.ID.ICPSR,
			GovTrack: l.ID.GovTrack, Wikidata: l.ID.Wikidata, Wikipedia: l.ID.Wikipedia, Ballotpedia: l.ID.Ballotpedia,
			First: l.Name.First, Middle: l.Name.Middle, Last: l.Name.Last, Suffix: l.Name.Suffix, Nickname: l.Name.Nickname,
			Display: l.Name.Display(), Birthday: l.Bio.Birthday, Gender: l.Bio.Gender, InCongress: true,
		}
		for i, t := range l.Terms {
			aff := "[]"
			if len(t.PartyAffiliations) > 0 {
				spans := make([]legislators.PartyAffiliation, len(t.PartyAffiliations))
				for k, a := range t.PartyAffiliations {
					a.Party, a.Caucus = Party(a.Party), Party(a.Caucus)
					spans[k] = a
				}
				b, err := json.Marshal(spans)
				if err != nil {
					return d, fmt.Errorf("encode affiliations of %s: %w", id, err)
				}
				aff = string(b)
			}
			row := store.Term{
				Person: id, Office: OfficeID(t), Body: body(t), State: t.State, District: t.District, Class: t.Class,
				Start: t.Start, End: t.End, Party: Party(t.Party), Caucus: Party(t.Caucus), How: t.How, EndType: t.EndType,
				StateRank: t.StateRank, URL: t.URL, Affiliations: aff, Current: i == len(l.Terms)-1,
			}
			d.Terms[row.Key()] = row
		}
		for _, r := range l.Leadership {
			row := store.Leadership{Person: id, Chamber: r.Chamber, Title: r.Title, Start: r.Start, End: r.End}
			d.Leadership[row.Key()] = row
		}
	}
	for _, c := range snap.Committees {
		d.Committees[c.ID] = store.Committee{ID: c.ID, Parent: c.Parent, Chamber: c.Chamber, Name: c.Name, URL: c.URL,
			Jurisdiction: c.Jurisdiction, Current: true}
	}
	for cid, ms := range snap.Membership {
		if _, ok := d.Committees[cid]; !ok {
			continue // a membership list for a committee the committees file does not list
		}
		for _, m := range ms {
			if _, ok := d.People[m.Bioguide]; !ok {
				continue // a member the members file does not list; the seat cannot be joined
			}
			x := store.Seat{Committee: cid, Person: m.Bioguide, Side: m.Side, Rank: m.Rank, Title: m.Title}
			d.Seats[x.Key()] = x
		}
	}
	return d, nil
}

// Diff decides what to write so the store matches want, and which changes to
// record. today is the date a member who disappeared is recorded as having
// left the source's list. A store with no people yet is a first load: rows
// are written, but no change is recorded for each of 539 arrivals.
//
// History is kept: a member missing from the file keeps their row, terms and
// roles; their current term gets left_on, and their open roles and committee
// seats are dated (unlisted_on), so no page shows them as current. For a
// member still listed, the file holds their whole history, so their stored
// terms and roles follow its corrections (a term the file no longer lists
// for them is kept, dated with unlisted_on, and recorded as a correction).
// Nothing is deleted.
func Diff(have, want store.RosterState, today string) store.RosterPlan {
	d := differ{have: have, want: want, today: today, first: len(have.People) == 0,
		haveCur: currentTerms(have), wantCur: currentTerms(want)}
	d.people()
	d.departures()
	d.terms()
	d.leadership()
	d.committees()
	d.seats()
	return d.p
}

type differ struct {
	have, want       store.RosterState
	haveCur, wantCur map[string]store.Term
	today            string
	first            bool
	p                store.RosterPlan
}

// change records c, unless this is the first load.
func (d *differ) change(c store.Change) {
	if !d.first {
		d.p.Changes = append(d.p.Changes, c)
	}
}

// people writes listed members and records arrivals.
func (d *differ) people() {
	for _, id := range sortedKeys(d.want.People) {
		w := d.want.People[id]
		h, existed := d.have.People[id]
		if !existed || h != w {
			d.p.People = append(d.p.People, w)
		}
		if existed && h.InCongress {
			continue
		}
		if t, ok := d.wantCur[id]; ok {
			d.change(store.Change{Kind: KindArrived, Person: id, Office: t.Office,
				Summary: fmt.Sprintf("%s (%s) is now listed as a member of the %s for %s", w.Display, partyOrUnknown(t.Party),
					chamberName(t.Body), seatName(t))})
		}
	}
}

// departures closes the current term of each member the file stopped listing.
func (d *differ) departures() {
	for _, id := range sortedKeys(d.have.People) {
		h := d.have.People[id]
		if _, still := d.want.People[id]; still || !h.InCongress {
			continue
		}
		h.InCongress = false
		d.p.People = append(d.p.People, h)
		office := ""
		if t, ok := d.haveCur[id]; ok {
			t.Current = false
			if t.End > d.today {
				t.LeftOn = d.today
			}
			d.p.Terms = append(d.p.Terms, t)
			office = t.Office
		}
		// Their open roles stop with their listing; one change says why.
		for _, k := range sortedLeadKeys(d.have.Leadership) {
			if k.Person == id && d.have.Leadership[k].End == "" {
				d.p.UnlistLeadership = append(d.p.UnlistLeadership, k)
			}
		}
		d.change(store.Change{Kind: KindLeft, Person: id, Office: office,
			Summary: fmt.Sprintf("%s is no longer listed as a member of Congress", h.Display)})
	}
}

// terms writes listed members' terms and records party and seat changes.
func (d *differ) terms() {
	for _, k := range sortedTermKeys(d.want.Terms) {
		w := d.want.Terms[k]
		h, ok := d.have.Terms[k]
		if ok && w.LeftOn == "" && h.LeftOn != "" {
			// The file never gives left_on; it is ours, the day a member
			// stopped being listed. A member who returns keeps that history.
			w.LeftOn = h.LeftOn
		}
		if !ok || !sameTerm(h, w) {
			d.p.Terms = append(d.p.Terms, w)
		}
	}
	for _, k := range sortedTermKeys(d.have.Terms) {
		if _, listed := d.want.People[k.Person]; !listed {
			continue
		}
		if _, ok := d.want.Terms[k]; !ok {
			d.p.UnlistTerms = append(d.p.UnlistTerms, k)
			h := d.have.Terms[k]
			d.change(store.Change{Kind: KindRecordCorrected, Person: k.Person, Office: k.Office,
				Summary: fmt.Sprintf("The source no longer lists %s's term for %s starting %s",
					d.want.People[k.Person].Display, seatName(h), h.Start)})
		}
	}
	for _, id := range sortedKeys(d.want.People) {
		old, hadOld := d.haveCur[id]
		cur := d.wantCur[id]
		if !hadOld || !d.have.People[id].InCongress {
			continue // an arrival, recorded by people
		}
		name := d.want.People[id].Display
		switch {
		case old.Office != cur.Office:
			d.change(store.Change{Kind: KindOfficeChanged, Person: id, Office: cur.Office,
				Summary: fmt.Sprintf("%s is now listed for %s in the %s", name, seatName(cur), chamberName(cur.Body))})
		case old.Party != cur.Party:
			d.change(store.Change{Kind: KindPartyChanged, Person: id, Office: cur.Office,
				Summary: fmt.Sprintf("%s's party changed from %s to %s", name, partyOrUnknown(old.Party), partyOrUnknown(cur.Party))})
		}
	}
}

// leadership writes listed members' roles and records roles gained and lost.
func (d *differ) leadership() {
	for _, k := range sortedLeadKeys(d.want.Leadership) {
		w := d.want.Leadership[k]
		h, ok := d.have.Leadership[k]
		if !ok || h != w {
			d.p.Leadership = append(d.p.Leadership, w)
		}
		name := d.want.People[w.Person].Display
		if w.End == "" && (!ok || h.End != "") {
			d.change(store.Change{Kind: KindLeadershipGained, Person: w.Person,
				Summary: fmt.Sprintf("%s is now listed as %s", name, w.Title)})
		}
		if w.End != "" && ok && h.End == "" {
			d.change(store.Change{Kind: KindLeadershipLost, Person: w.Person,
				Summary: fmt.Sprintf("%s is no longer listed as %s", name, w.Title)})
		}
	}
	for _, k := range sortedLeadKeys(d.have.Leadership) {
		if _, listed := d.want.People[k.Person]; !listed {
			continue
		}
		if _, ok := d.want.Leadership[k]; ok {
			continue
		}
		d.p.UnlistLeadership = append(d.p.UnlistLeadership, k)
		name := d.want.People[k.Person].Display
		if h := d.have.Leadership[k]; h.End == "" {
			d.change(store.Change{Kind: KindLeadershipLost, Person: k.Person,
				Summary: fmt.Sprintf("%s is no longer listed as %s", name, h.Title)})
		} else {
			d.change(store.Change{Kind: KindRecordCorrected, Person: k.Person,
				Summary: fmt.Sprintf("The source no longer lists %s as %s from %s", name, h.Title, h.Start)})
		}
	}
}

// committees writes the listed committees; one the source drops is kept and
// marked not current.
func (d *differ) committees() {
	for _, id := range sortedKeys(d.want.Committees) {
		w := d.want.Committees[id]
		if h, ok := d.have.Committees[id]; !ok || h != w {
			d.p.Committees = append(d.p.Committees, w)
		}
	}
	for _, id := range sortedKeys(d.have.Committees) {
		h := d.have.Committees[id]
		if _, ok := d.want.Committees[id]; !ok && h.Current {
			h.Current = false
			d.p.Committees = append(d.p.Committees, h)
		}
	}
}

// seats writes the listed committee seats and dates the ones the source
// stops listing (kept, never deleted). For full committees it records who
// joined, who left, and new chairs and ranking members; a member leaving
// Congress is one change (people), not one per seat.
func (d *differ) seats() {
	for _, k := range sortedSeatKeys(d.want.Seats) {
		w := d.want.Seats[k]
		h, ok := d.have.Seats[k]
		if !ok || h != w {
			d.p.Seats = append(d.p.Seats, w)
		}
		c := d.want.Committees[k.Committee]
		// Joining as chair or ranking member is the one change below.
		if c.Parent == "" && !ok && d.wasListed(k.Person) && !isLeadSeat(w.Title) {
			d.change(store.Change{Kind: KindSeatJoined, Person: k.Person, Committee: k.Committee,
				Summary: fmt.Sprintf("%s is now listed on the %s", d.want.People[k.Person].Display, c.Name)})
		}
		if c.Parent == "" && isLeadSeat(w.Title) && (!ok || h.Title != w.Title) {
			d.change(store.Change{Kind: KindChairChanged, Person: k.Person, Committee: k.Committee,
				Summary: fmt.Sprintf("%s is now listed as %s of the %s", d.want.People[k.Person].Display, w.Title, c.Name)})
		}
	}
	for _, k := range sortedSeatKeys(d.have.Seats) {
		if _, ok := d.want.Seats[k]; ok {
			continue
		}
		d.p.UnlistSeats = append(d.p.UnlistSeats, k)
		person, listed := d.want.People[k.Person]
		c, known := d.have.Committees[k.Committee]
		if listed && known && c.Parent == "" {
			d.change(store.Change{Kind: KindSeatLeft, Person: k.Person, Committee: k.Committee,
				Summary: fmt.Sprintf("%s is no longer listed on the %s", person.Display, c.Name)})
		}
	}
}

// sameTerm compares two terms by value: District and Class are pointers, and
// == on the structs would compare the pointers, so every run would rewrite
// every term.
func sameTerm(a, b store.Term) bool {
	if !sameInt(a.District, b.District) || !sameInt(a.Class, b.Class) {
		return false
	}
	a.District, a.Class, b.District, b.Class = nil, nil, nil, nil
	return a == b
}

func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// wasListed reports whether the last run listed the person in Congress, so a
// new seat is a change of committee, not part of an arrival.
func (d *differ) wasListed(id string) bool {
	h, ok := d.have.People[id]
	return ok && h.InCongress
}

// isLeadSeat reports whether a committee title is a chair or ranking member.
func isLeadSeat(title string) bool {
	switch title {
	case "Chair", "Chairman", "Chairwoman", "Ranking Member", "Cochairman":
		return true
	}
	return false
}

// currentTerms maps each person to their current term in s.
func currentTerms(s store.RosterState) map[string]store.Term {
	m := map[string]store.Term{}
	for _, t := range s.Terms {
		if t.Current {
			m[t.Person] = t
		}
	}
	return m
}

func chamberName(body string) string {
	if body == BodySenate {
		return "Senate"
	}
	return "House"
}

func partyOrUnknown(p string) string {
	if p == "" {
		return "party unknown"
	}
	return p
}

// seatName describes a seat in words: "Vermont", "California's 12th district",
// "Wyoming at large".
func seatName(t store.Term) string {
	name := t.State
	if j, ok := geo.Lookup(t.State); ok {
		name = j.Name
	}
	if t.Body == BodySenate {
		return name
	}
	if t.District == nil || *t.District == 0 {
		return name + " at large"
	}
	return fmt.Sprintf("%s's %s district", name, Ordinal(*t.District))
}

// Ordinal writes 1 as 1st, 2 as 2nd, 11 as 11th, 22 as 22nd.
func Ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedTermKeys(m map[store.TermKey]store.Term) []store.TermKey {
	ks := make([]store.TermKey, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.Person != b.Person {
			return a.Person < b.Person
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Office < b.Office
	})
	return ks
}

func sortedLeadKeys(m map[store.LeadershipKey]store.Leadership) []store.LeadershipKey {
	ks := make([]store.LeadershipKey, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.Person != b.Person {
			return a.Person < b.Person
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Title < b.Title
	})
	return ks
}

func sortedSeatKeys(m map[store.SeatKey]store.Seat) []store.SeatKey {
	ks := make([]store.SeatKey, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].Committee != ks[j].Committee {
			return ks[i].Committee < ks[j].Committee
		}
		return ks[i].Person < ks[j].Person
	})
	return ks
}

// Today is the calendar date of t in UTC, as the store's dates are written.
func Today(t time.Time) string { return t.UTC().Format(time.DateOnly) }
