package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rosterSrc = "https://example.org/legislators-current.json"

// onePlan is a plan with one row in every roster table and one change.
func onePlan(at time.Time) RosterPlan {
	district := 3
	return RosterPlan{
		At:     at,
		Source: rosterSrc,
		People: []Person{{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", InCongress: true}},
		Terms: []Term{{Person: "A000001", Office: "house/OH/3", Body: "house", State: "OH", District: &district,
			Start: "2025-01-03", End: "2027-01-03", Party: "Independent", Current: true}},
		Leadership: []Leadership{{Person: "A000001", Chamber: "house", Title: "Whip", Start: "2025-01-03"}},
		Committees: []Committee{{ID: "HSXX", Chamber: "house", Name: "House Committee on Examples", Current: true}},
		Seats:      []Seat{{Committee: "HSXX", Person: "A000001", Side: "minority", Rank: 2}},
		Changes:    []Change{{Kind: "arrived", Person: "A000001", Summary: "Ann Able is now listed"}},
	}
}

// count runs a count(*) query.
func count(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(context.Background(), query, args...).Scan(&n), "running %q", query)
	return n
}

func TestApplyRoster_APlanWithoutASourceOrTimeIsRefusedAndWritesNothing(t *testing.T) {
	t.Parallel()
	// go-conventions, Provenance: a row without a source is refused.
	tests := map[string]func(*RosterPlan){
		"no source": func(p *RosterPlan) { p.Source = "" },
		"no time":   func(p *RosterPlan) { p.At = time.Time{} },
	}
	for name, spoil := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := open(t)
			p := onePlan(t0)
			spoil(&p)
			require.Error(t, s.ApplyRoster(context.Background(), p), "applying a plan with %s", name)
			for _, table := range []string{"people", "terms", "leadership", "committees", "committee_seats", "changes"} {
				assert.Zero(t, count(t, s, "SELECT count(*) FROM "+table), "%s after a refused plan", table)
			}
		})
	}
}

func TestApplyRoster_EveryRowCarriesItsSourceAndFetchTime(t *testing.T) {
	t.Parallel()
	s := open(t)
	require.NoError(t, s.ApplyRoster(context.Background(), onePlan(t0)), "applying a plan")
	at := t0.UTC().Format(time.RFC3339)
	for _, table := range []string{"people", "terms", "leadership", "committees", "committee_seats"} {
		assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM "+table+" WHERE source = ? AND fetched_at = ?", rosterSrc, at),
			"%s row carries the plan's source and time", table)
	}
	assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM changes WHERE source = ? AND at = ?", rosterSrc, at),
		"the change carries the plan's source and time")
}

func TestApplyRoster_AnUnlistedRowIsKeptDatedAndAListedOneIsCurrentAgain(t *testing.T) {
	t.Parallel()
	// go-conventions, Provenance: a job never deletes history because a
	// source stopped listing something.
	s := open(t)
	ctx := context.Background()
	first := onePlan(t0)
	require.NoError(t, s.ApplyRoster(ctx, first), "applying the first plan")

	later := t0.Add(24 * time.Hour)
	require.NoError(t, s.ApplyRoster(ctx, RosterPlan{At: later, Source: rosterSrc,
		UnlistTerms:      []TermKey{first.Terms[0].Key()},
		UnlistLeadership: []LeadershipKey{first.Leadership[0].Key()},
		UnlistSeats:      []SeatKey{first.Seats[0].Key()},
	}), "unlisting every row")
	laterAt := later.UTC().Format(time.RFC3339)
	for _, table := range []string{"terms", "leadership", "committee_seats"} {
		assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM "+table+" WHERE unlisted_on = ?", laterAt),
			"the %s row is kept and dated", table)
	}
	st, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster")
	assert.Empty(t, st.Terms, "an unlisted term is not part of the listed roster")
	assert.Empty(t, st.Leadership, "an unlisted role is not part of the listed roster")
	assert.Empty(t, st.Seats, "an unlisted seat is not part of the listed roster")

	again := onePlan(later.Add(24 * time.Hour))
	again.People, again.Committees, again.Changes = nil, nil, nil
	require.NoError(t, s.ApplyRoster(ctx, again), "listing the rows again")
	for _, table := range []string{"terms", "leadership", "committee_seats"} {
		assert.Equal(t, 1, count(t, s, "SELECT count(*) FROM "+table+" WHERE unlisted_on IS NULL"),
			"the %s row is listed again", table)
	}
}

func TestLastHolders_OnlyAMemberSeenLeavingMidTermIsALastHolder(t *testing.T) {
	t.Parallel()
	// "Last held by" is said only when we saw the source stop listing the
	// holder mid-term (left_on); an old term of someone still serving
	// elsewhere is not a last holder.
	s := open(t)
	ctx := context.Background()
	d3, d5 := 3, 5
	require.NoError(t, s.ApplyRoster(ctx, RosterPlan{At: t0, Source: rosterSrc,
		People: []Person{
			{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able"},
			{ID: "B000002", Bioguide: "B000002", First: "Bo", Last: "Baker", Display: "Bo Baker", InCongress: true},
		},
		Terms: []Term{
			// Ann left OH-3 mid-term: seen, so she is its last holder.
			{Person: "A000001", Office: "house/OH/3", Body: "house", State: "OH", District: &d3,
				Start: "2025-01-03", End: "2027-01-03", Party: "Democratic", LeftOn: "2026-11-30"},
			// Bo held OH-5 long ago and is a senator now: not OH-5's last holder.
			{Person: "B000002", Office: "house/OH/5", Body: "house", State: "OH", District: &d5,
				Start: "2001-01-03", End: "2003-01-03", Party: "Republican"},
			{Person: "B000002", Office: "senate/OH/1", Body: "senate", State: "OH",
				Start: "2025-01-03", End: "2031-01-03", Party: "Republican", Current: true},
		},
	}), "seeding the roster")

	got, err := s.LastHolders(ctx, "OH")
	require.NoError(t, err, "reading Ohio's last holders")
	assert.Equal(t, map[string]LastHolder{
		"house/OH/3": {Person: "A000001", Name: "Ann Able", Party: "Democratic", LeftOn: "2026-11-30"},
	}, got, "only the member seen leaving mid-term")
}
