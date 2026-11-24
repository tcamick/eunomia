package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotTo_WritesACopyThatOpensWithTheSameRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := open(t)
	ids, err := s.AddStories(ctx, "npr-politics", "https://feeds.npr.org/1014/rss.xml", t0, []NewStory{
		{GUID: "g1", URL: "https://www.npr.org/a", Title: "A story"},
	})
	require.NoError(t, err, "adding a story")
	_, _, err = s.SetMarks(ctx, ids[0], new(true), nil, t0)
	require.NoError(t, err, "marking it read")

	path := filepath.Join(t.TempDir(), "copy.sqlite")
	require.NoError(t, s.SnapshotTo(ctx, path), "taking a snapshot")

	c, err := Open(ctx, path, func() time.Time { return t0 })
	require.NoError(t, err, "opening the copy")
	t.Cleanup(func() { _ = c.Close() })
	counts, err := c.Verify(ctx)
	require.NoError(t, err, "verifying the copy")
	assert.Equal(t, int64(1), counts["stories"], "the story is in the copy")
	assert.Equal(t, int64(1), counts["marks"], "and the owner's mark")
}

func TestSnapshotTo_RefusesAnExistingFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := open(t)
	path := filepath.Join(t.TempDir(), "copy.sqlite")
	require.NoError(t, s.SnapshotTo(ctx, path), "the first snapshot")
	err := s.SnapshotTo(ctx, path)
	require.Error(t, err, "a second snapshot to the same path")
	assert.Contains(t, err.Error(), "already exists", "says why")
}

func TestVerify_CountsEveryTableARestoreMustHave(t *testing.T) {
	t.Parallel()
	counts, err := open(t).Verify(context.Background())
	require.NoError(t, err, "verifying an empty store")
	for _, table := range VerifiedTables() {
		v, ok := counts[table]
		assert.True(t, ok, "%s is counted", table)
		assert.Zero(t, v, "%s is empty", table)
	}
}
