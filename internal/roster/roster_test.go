package roster

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/legislators"
	"github.com/tcamick/eunomia/internal/store"
)

const today = "2026-11-22"

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// snapshot reads the legislators package's fixtures (real files, trimmed).
func snapshot(t *testing.T) *legislators.Snapshot {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "legislators", "testdata", name))
		require.NoError(t, err, "reading fixture %s", name)
		return b
	}
	ls, err := legislators.ParseLegislators(read("legislators-current.json"), 1)
	require.NoError(t, err, "parsing the members fixture")
	cs, err := legislators.ParseCommittees(read("committees-current.json"))
	require.NoError(t, err, "parsing the committees fixture")
	ms, err := legislators.ParseMembership(read("committee-membership-current.json"))
	require.NoError(t, err, "parsing the membership fixture")
	return &legislators.Snapshot{Legislators: ls, Committees: cs, Membership: ms}
}

func desired(t *testing.T, snap *legislators.Snapshot) store.RosterState {
	t.Helper()
	d, err := Desired(snap)
	require.NoError(t, err, "building rows from the snapshot")
	return d
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "eunomia.sqlite"), func() time.Time { return t0 })
	require.NoError(t, err, "opening a new database")
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.Migrate(context.Background())
	require.NoError(t, err, "applying migrations")
	return s
}

// load applies the snapshot to s as a run of the job would, and returns the plan.
func load(t *testing.T, s *store.Store, snap *legislators.Snapshot, day string) store.RosterPlan {
	t.Helper()
	ctx := context.Background()
	have, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster")
	p := Diff(have, desired(t, snap), day)
	p.At, p.Source = t0, "https://example.org/legislators-current.json"
	require.NoError(t, s.ApplyRoster(ctx, p), "applying the plan")
	return p
}

func withoutMember(snap *legislators.Snapshot, bioguide string) *legislators.Snapshot {
	out := *snap
	out.Legislators = slices.DeleteFunc(slices.Clone(snap.Legislators), func(l legislators.Legislator) bool {
		return l.ID.Bioguide == bioguide
	})
	return &out
}

// edited is the fixture member the edit tests change.
const edited = "W000800"

// edit returns a copy of snap with fn applied to a deep copy of the edited member.
func edit(t *testing.T, snap *legislators.Snapshot, fn func(*legislators.Legislator)) *legislators.Snapshot {
	t.Helper()
	out := *snap
	out.Legislators = slices.Clone(snap.Legislators)
	for i, l := range out.Legislators {
		if l.ID.Bioguide == edited {
			l.Terms = slices.Clone(l.Terms)
			l.Leadership = slices.Clone(l.Leadership)
			fn(&l)
			out.Legislators[i] = l
			return &out
		}
	}
	t.Fatalf("no member %s in the fixture", edited)
	return nil
}

func kinds(cs []store.Change) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Kind)
	}
	return out
}

func TestDiff_FirstLoadWritesEveryRowAndRecordsNoChanges(t *testing.T) {
	t.Parallel()
	snap := snapshot(t)
	p := Diff(store.RosterState{}, desired(t, snap), today)
	assert.Len(t, p.People, 15)
	assert.NotEmpty(t, p.Terms)
	assert.NotEmpty(t, p.Committees)
	assert.NotEmpty(t, p.Seats)
	assert.Empty(t, p.Changes, "fifteen arrivals on a first load are not news")
}

func TestDiff_ASecondRunWithTheSameFileWritesNothing(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)

	have, err := s.RosterState(context.Background())
	require.NoError(t, err, "reading the stored roster")
	p := Diff(have, desired(t, snap), today)
	assert.True(t, p.Empty(), "an unchanged file must write nothing; got %d people, %d terms, %d leadership, %d committees, %d seats, %d changes",
		len(p.People), len(p.Terms), len(p.Leadership), len(p.Committees), len(p.Seats), len(p.Changes))
}

func TestDiff_AMemberMissingFromTheFileKeepsTheirHistory(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)

	p := load(t, s, withoutMember(snap, "S000033"), "2026-12-01")
	assert.Equal(t, []string{KindLeft}, kinds(p.Changes))
	assert.Equal(t, "S000033", p.Changes[0].Person)
	assert.Empty(t, p.UnlistTerms, "a departed member's terms stay listed, as history")
	var open, past []store.LeadershipKey
	for k, l := range desired(t, snap).Leadership {
		if k.Person != "S000033" {
			continue
		}
		if l.End == "" {
			open = append(open, k)
		} else {
			past = append(past, k)
		}
	}
	require.NotEmpty(t, open, "the fixture gives S000033 an open role")
	require.NotEmpty(t, past, "the fixture gives S000033 a past role")
	assert.ElementsMatch(t, open, p.UnlistLeadership, "exactly the departed member's open roles are dated")
	d, _, err := s.PersonDetail(context.Background(), "S000033")
	require.NoError(t, err, "reading the departed member's page")
	shown := map[store.LeadershipKey]bool{}
	for _, r := range d.Roles {
		assert.True(t, r.End != "" || r.UnlistedOn != "", "role %q is not shown as current", r.Title)
		shown[store.LeadershipKey{Person: "S000033", Title: r.Title, Start: r.Start}] = true
	}
	for _, k := range past {
		assert.True(t, shown[k], "the past role %q is still on the page", k.Title)
	}

	st, err := s.RosterState(context.Background())
	require.NoError(t, err, "reading the stored roster")
	sanders, ok := st.People["S000033"]
	require.True(t, ok, "the person's row stays")
	assert.False(t, sanders.InCongress)
	var current, closed int
	for _, tm := range st.Terms {
		if tm.Person != "S000033" {
			continue
		}
		if tm.Current {
			current++
		}
		if tm.LeftOn == "2026-12-01" {
			closed++
		}
	}
	assert.Zero(t, current, "no current term once the source stops listing them")
	assert.Equal(t, 1, closed, "the term they were serving records when they left the list")
	assert.Greater(t, countTerms(st, "S000033"), 1, "every past term is kept")

	again := Diff(st, desired(t, withoutMember(snap, "S000033")), "2026-12-02")
	assert.True(t, again.Empty(), "a departure is recorded once")
}

func countTerms(st store.RosterState, person string) int {
	n := 0
	for k := range st.Terms {
		if k.Person == person {
			n++
		}
	}
	return n
}

func TestDiff_AReturningMemberIsAnArrival(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	load(t, s, withoutMember(snap, "N000147"), today)
	p := load(t, s, snap, "2026-12-01")
	require.Equal(t, []string{KindArrived}, kinds(p.Changes), "changes after an arrival")
	assert.Contains(t, p.Changes[0].Summary, "District of Columbia at large")
}

func TestDiff_LeftOnIsKeptOnAnEarlierTermAndClearedOnTheTermBeingServed(t *testing.T) {
	t.Parallel()
	// left_on is ours: the day the source stopped listing a member.
	leftOn := func(t *testing.T, s *store.Store) map[string]string {
		t.Helper()
		d, _, err := s.PersonDetail(context.Background(), edited)
		require.NoError(t, err, "reading the member's page")
		out := map[string]string{}
		for _, term := range d.Terms {
			out[term.Start] = term.LeftOn
		}
		return out
	}
	t.Run("returning to the same term clears it", func(t *testing.T) {
		t.Parallel()
		s := openStore(t)
		snap := snapshot(t)
		load(t, s, snap, today)
		load(t, s, withoutMember(snap, edited), today)
		load(t, s, snap, "2026-12-01")
		for start, left := range leftOn(t, s) {
			assert.Empty(t, left, "term from %s: the member serves it again", start)
		}
	})
	t.Run("returning for a new term keeps it on the earlier one", func(t *testing.T) {
		t.Parallel()
		s := openStore(t)
		snap := snapshot(t)
		load(t, s, snap, today)
		load(t, s, withoutMember(snap, edited), today)
		var served string
		back := edit(t, snap, func(l *legislators.Legislator) {
			last := l.Terms[len(l.Terms)-1]
			served = last.Start
			next := last
			next.Start, next.End = "2027-01-03", "2029-01-03"
			l.Terms = append(l.Terms, next)
		})
		load(t, s, back, "2027-01-05")
		got := leftOn(t, s)
		assert.Equal(t, today, got[served], "the earlier term keeps the day the member stopped being listed")
		assert.Empty(t, got["2027-01-03"], "the new term has none")
	})
}

func TestDiff_APartySwitchIsOneChange(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	switched := edit(t, snap, func(l *legislators.Legislator) {
		last := len(l.Terms) - 1
		l.Terms[last].Party = "Independent"
	})
	p := load(t, s, switched, today)
	require.Equal(t, []string{KindPartyChanged}, kinds(p.Changes), "changes after a party switch")
	assert.Contains(t, p.Changes[0].Summary, "from Democratic to Independent", "the party's own name, not the source's 'Democrat'")
}

func TestDiff_LeadershipGainedAndLost(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)

	gained := edit(t, snap, func(l *legislators.Legislator) {
		l.Leadership = append(l.Leadership, legislators.LeadershipRole{Title: "Senate Example Whip", Chamber: "senate", Start: "2026-11-20"})
	})
	p := load(t, s, gained, today)
	require.Equal(t, []string{KindLeadershipGained}, kinds(p.Changes), "changes after a role is gained")

	ended := edit(t, gained, func(l *legislators.Legislator) {
		l.Leadership[len(l.Leadership)-1].End = "2026-11-21"
	})
	p = load(t, s, ended, today)
	require.Equal(t, []string{KindLeadershipLost}, kinds(p.Changes), "changes after a role is lost")
}

func TestDiff_ANewChairIsRecorded(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)

	// Give the chair's title on HSED to another listed member.
	out := *snap
	out.Membership = map[string][]legislators.Member{}
	for id, ms := range snap.Membership {
		out.Membership[id] = slices.Clone(ms)
	}
	var chair string
	for i, m := range out.Membership["HSED"] {
		if isLeadSeat(m.Title) && m.Side == "majority" {
			chair = m.Bioguide
			out.Membership["HSED"][i].Title = ""
		}
	}
	require.NotEmpty(t, chair, "the fixture lists HSED's chair")
	out.Membership["HSED"] = append(out.Membership["HSED"], legislators.Member{Bioguide: "S001231", Side: "majority", Rank: 1, Title: "Chair"})

	p := load(t, s, &out, today)
	require.Equal(t, []string{KindChairChanged}, kinds(p.Changes), "changes after a new chair")
	assert.Equal(t, "HSED", p.Changes[0].Committee)
}

func TestDiff_ADepartedMembersCommitteeSeatsAreKeptAndDated(t *testing.T) {
	t.Parallel()
	// H001103 holds three seats in the fixture.
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)

	p := load(t, s, withoutMember(snap, "H001103"), "2026-12-01")
	assert.Equal(t, []string{KindLeft}, kinds(p.Changes), "a departure is one change, not one per seat")
	assert.Len(t, p.UnlistSeats, 3, "each seat is dated, not deleted")

	d, ok, err := s.PersonDetail(context.Background(), "H001103")
	require.NoError(t, err, "reading the departed member's page")
	require.True(t, ok, "the departed member's page still exists")
	require.Len(t, d.Seats, 3, "the departed member's committee seats are kept")
	for _, x := range d.Seats {
		assert.NotEmpty(t, x.UnlistedOn, "seat on %s carries the date the source stopped listing it", x.Committee)
	}
}

func TestDiff_ACorrectedTermStartKeepsTheOldRowAndSaysSo(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	i := slices.IndexFunc(snap.Legislators, func(l legislators.Legislator) bool { return l.ID.Bioguide == edited })
	require.GreaterOrEqual(t, i, 0, "the fixture has the edited member")
	original := snap.Legislators[i].Terms[0].Start
	require.NotEqual(t, "2007-01-05", original, "the correction changes the date")
	fixed := edit(t, snap, func(l *legislators.Legislator) { l.Terms[0].Start = "2007-01-05" })
	p := load(t, s, fixed, today)
	assert.Len(t, p.UnlistTerms, 1, "the old term is dated, not deleted")
	assert.Equal(t, []string{KindRecordCorrected}, kinds(p.Changes), "the correction is recorded")
	d, _, err := s.PersonDetail(context.Background(), edited)
	require.NoError(t, err, "reading the member's page")
	for _, term := range d.Terms {
		assert.NotEqual(t, original, term.Start, "the page shows the source's corrected record only")
	}
}

func TestDiff_ACommitteeTheSourceDropsIsKeptAndMarkedPast(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	out := *snap
	out.Committees = slices.DeleteFunc(slices.Clone(snap.Committees), func(c legislators.Committee) bool { return c.ID == "HSBU" })
	delete(out.Membership, "HSBU") // a copy of the map is not needed: snap is not used again
	load(t, s, &out, today)
	st, err := s.RosterState(context.Background())
	require.NoError(t, err, "reading the stored roster")
	c, ok := st.Committees["HSBU"]
	require.True(t, ok, "the committee's row stays")
	assert.False(t, c.Current)
	for k := range st.Seats {
		assert.NotEqual(t, "HSBU", k.Committee, "its seats are no longer listed")
	}
	listed := map[string]bool{}
	for _, l := range snap.Legislators {
		listed[l.ID.Bioguide] = true
	}
	for _, m := range snap.Membership["HSBU"] {
		if !listed[m.Bioguide] {
			continue // the membership fixture names people the trimmed members fixture lacks
		}
		d, _, err := s.PersonDetail(context.Background(), m.Bioguide)
		require.NoError(t, err, "reading %s's page", m.Bioguide)
		var dated bool
		for _, x := range d.Seats {
			if x.Committee == "HSBU" {
				dated = x.UnlistedOn != ""
			}
		}
		assert.True(t, dated, "%s's HSBU seat is kept, dated", m.Bioguide)
	}
}

func TestDiff_AMemberMovedOffAndOntoACommitteeIsRecordedAndKept(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	// The membership fixture names people the trimmed members fixture does
	// not have; pick an HSBU member it does.
	listed := map[string]bool{}
	for _, l := range snap.Legislators {
		listed[l.ID.Bioguide] = true
	}
	i := slices.IndexFunc(snap.Membership["HSBU"], func(m legislators.Member) bool { return listed[m.Bioguide] })
	require.GreaterOrEqual(t, i, 0, "the fixture has a listed HSBU member")
	who := snap.Membership["HSBU"][i].Bioguide

	out := *snap
	out.Membership = maps.Clone(snap.Membership)
	out.Membership["HSBU"] = slices.Delete(slices.Clone(snap.Membership["HSBU"]), i, i+1)
	p := load(t, s, &out, today)
	assert.Equal(t, []string{KindSeatLeft}, kinds(p.Changes), "leaving a full committee is one change")
	d, _, err := s.PersonDetail(context.Background(), who)
	require.NoError(t, err, "reading the member's page")
	var dated bool
	for _, x := range d.Seats {
		if x.Committee == "HSBU" {
			dated = x.UnlistedOn != ""
		}
	}
	assert.True(t, dated, "the HSBU seat is kept with the date it stopped being listed")

	p = load(t, s, snap, "2026-12-01")
	assert.Equal(t, []string{KindSeatJoined}, kinds(p.Changes), "listed again is one change")
	d, _, err = s.PersonDetail(context.Background(), who)
	require.NoError(t, err, "reading the member's page again")
	for _, x := range d.Seats {
		if x.Committee == "HSBU" {
			assert.Empty(t, x.UnlistedOn, "a seat listed again is current again")
		}
	}
}

func TestParty_UsesThePartysOwnName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Democratic", Party("Democrat"))
	assert.Equal(t, "Republican", Party("Republican"))
	assert.Equal(t, "Independent", Party("Independent"))
	assert.Equal(t, "Libertarian", Party("Libertarian"), "any other party is kept as the source gives it")
}

func TestOrdinal_UsesTheEnglishSuffixesIncludingTheTeens(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		n    int
		want string
	}{
		"1st": {1, "1st"}, "2nd": {2, "2nd"}, "3rd": {3, "3rd"}, "4th": {4, "4th"},
		"the teens take th": {11, "11th"}, "12th": {12, "12th"}, "13th": {13, "13th"},
		"21st": {21, "21st"}, "22nd": {22, "22nd"}, "52nd": {52, "52nd"}, "a hundred and eleventh": {111, "111th"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, Ordinal(tc.n), "ordinal of %d", tc.n)
		})
	}
}
