package roster

import (
	"context"
	"errors"
	"testing"
	"time"

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

const base = "https://unitedstates.github.io/congress-legislators/"

func TestJob_FirstRunLoadsAndTheSecondSendsTheValidatorsBack(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	src := &fakeSource{snap: snapshot(t)}
	j := NewJob(src, s, base, func() time.Time { return t0 })
	ctx := context.Background()

	require.NoError(t, j.Run(ctx), "the first run")
	st, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster")
	assert.Len(t, st.People, 15)

	require.NoError(t, j.Run(ctx), "the second run")
	require.Len(t, src.calls, 2)
	assert.Equal(t, served, src.calls[1], "the second run asks whether anything changed")

	after, err := s.RosterState(ctx)
	require.NoError(t, err, "reading the stored roster again")
	assert.Equal(t, st, after, "an unchanged source writes no roster row")

	ms, ok, err := s.SourceState(ctx, legislators.SourceLegislators)
	require.NoError(t, err, "reading the source's state")
	require.True(t, ok)
	assert.Equal(t, base+"legislators-current.json", ms.URL)
	assert.Equal(t, `"m1"`, ms.ETag)
	assert.False(t, ms.CheckedAt.IsZero())
	assert.False(t, ms.ChangedAt.IsZero(), "the first run changed it")
}

func TestJob_AFailedFetchWritesNothing(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	src := &fakeSource{err: errors.New("source said 503")}
	err := NewJob(src, s, base, func() time.Time { return t0 }).Run(context.Background())
	require.Error(t, err, "the fetch failed")
	st, err := s.RosterState(context.Background())
	require.NoError(t, err, "reading the stored roster")
	assert.Empty(t, st.People)
	_, ok, err := s.SourceState(context.Background(), legislators.SourceLegislators)
	require.NoError(t, err, "reading the source's state")
	assert.False(t, ok, "a failed fetch is not a check")
}
