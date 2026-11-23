package roster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/legislators"
)

// fakeSource returns snap the first time it is asked without validators and
// "unchanged" when the validators it handed out come back.
type fakeSource struct {
	snap  *legislators.Snapshot
	err   error
	calls []legislators.Validators
}

var served = legislators.Validators{
	Members:    fetch.Validators{ETag: `"m1"`},
	Committees: fetch.Validators{ETag: `"c1"`},
	Membership: fetch.Validators{ETag: `"s1"`},
}

func (f *fakeSource) Fetch(_ context.Context, prev legislators.Validators) (*legislators.Snapshot, legislators.Validators, bool, error) {
	f.calls = append(f.calls, prev)
	if f.err != nil {
		return nil, prev, false, f.err
	}
	if prev == served {
		return nil, prev, true, nil
	}
	return f.snap, served, false, nil
}

// newFile always serves its snapshot as a changed file.
type newFile struct{ snap *legislators.Snapshot }

func (f newFile) Fetch(context.Context, legislators.Validators) (*legislators.Snapshot, legislators.Validators, bool, error) {
	return f.snap, legislators.Validators{Members: fetch.Validators{ETag: `"m2"`}}, false, nil
}

const base = "https://unitedstates.github.io/congress-legislators/"

func TestJob_FirstRunLoadsAndTheSecondSendsTheValidatorsBack(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	src := &fakeSource{snap: snapshot(t)}
	j, err := NewJob(src, s, base, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	ctx := context.Background()

	require.NoError(t, j.Run(ctx), "the first run")
	st, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster")
	assert.Len(t, st.People, 15)

	require.NoError(t, j.Run(ctx), "the second run")
	require.Len(t, src.calls, 2, "one fetch per run")
	assert.Equal(t, served, src.calls[1], "the second run asks whether anything changed")

	after, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster again")
	assert.Equal(t, st, after, "an unchanged source writes no roster row")

	ms, ok, err := s.SourceState(ctx, legislators.SourceLegislators)
	require.NoError(t, err, "reading the source's state")
	require.True(t, ok, "the source has been checked")
	assert.Equal(t, base+"legislators-current.json", ms.URL)
	assert.Equal(t, `"m1"`, ms.ETag)
	assert.False(t, ms.CheckedAt.IsZero())
	assert.False(t, ms.ChangedAt.IsZero(), "the first run changed it")
}

func TestJob_AFailedFetchWritesNothing(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	src := &fakeSource{err: errors.New("source said 503")}
	j, err := NewJob(src, s, base, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	err = j.Run(context.Background())
	require.Error(t, err, "the fetch failed")
	st, err := s.RosterState(context.Background())
	require.NoError(t, err, "reading the stored roster")
	assert.Empty(t, st.People)
	_, ok, err := s.SourceState(context.Background(), legislators.SourceLegislators)
	require.NoError(t, err, "reading the source's state")
	assert.False(t, ok, "a failed fetch is not a check")
}

func TestJob_CountsChangesByKind(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	reg := prometheus.NewRegistry()
	snap := snapshot(t)
	src := &fakeSource{snap: snap}
	j, err := NewJob(src, s, base, func() time.Time { return t0 }, reg)
	require.NoError(t, err, "building the job")
	ctx := context.Background()
	require.NoError(t, j.Run(ctx), "the first run")
	assert.Equal(t, len(Kinds), testutil.CollectAndCount(reg, "eunomia_roster_changes_total"),
		"every kind is present from the first run, so a push carries all of them")
	assert.Zero(t, testutil.ToFloat64(j.changes.WithLabelValues(KindArrived)), "a first load records no change")

	j.src = newFile{edit(t, snap, func(l *legislators.Legislator) { l.Terms[len(l.Terms)-1].Party = "Independent" })}
	require.NoError(t, j.Run(ctx), "the run after a party switch")
	assert.InDelta(t, 1, testutil.ToFloat64(j.changes.WithLabelValues(KindPartyChanged)), 0, "one party switch is counted once")
	assert.Zero(t, testutil.ToFloat64(j.changes.WithLabelValues(KindLeft)), "nothing else is counted")
}

// fixtureFiles serves the three fixture files by name, as the source would.
type fixtureFiles struct{ t *testing.T }

func (f fixtureFiles) Get(_ context.Context, _, rawURL string, _ fetch.Validators) (*fetch.Response, error) {
	name := rawURL[strings.LastIndex(rawURL, "/")+1:]
	b, err := os.ReadFile(filepath.Join("..", "legislators", "testdata", name))
	require.NoError(f.t, err, "reading fixture %s", name)
	return &fetch.Response{Status: 200, Body: b, Validators: fetch.Validators{ETag: `"` + name + `"`}}, nil
}

func TestJob_AShortFileIsRejectedAndTheStoredRosterIsUntouched(t *testing.T) {
	t.Parallel()
	// The fixture lists 15 members, far under MinMembers: a file truncated
	// upstream must not end 524 terms.
	s := openStore(t)
	snap := snapshot(t)
	load(t, s, snap, today)
	ctx := context.Background()
	before, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster")

	src := legislators.NewClient(fixtureFiles{t}, base, legislators.MinMembers)
	j, err := NewJob(src, s, base, func() time.Time { return t0 }, nil)
	require.NoError(t, err, "building the job")
	err = j.Run(ctx)
	require.ErrorIs(t, err, legislators.ErrTooFew, "running against a short file")

	after, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster again")
	assert.Equal(t, before, after, "a rejected file writes no roster row")
	_, ok, err := s.SourceState(ctx, legislators.SourceLegislators)
	require.NoError(t, err, "reading the source's state")
	assert.False(t, ok, "a rejected file is not recorded as a check")
}
