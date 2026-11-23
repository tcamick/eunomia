package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVerifier stands in for a store opened on the restored copy.
type fakeVerifier struct {
	counts map[string]int64
	err    error
	closed bool
}

func (f *fakeVerifier) Verify(context.Context) (map[string]int64, error) { return f.counts, f.err }
func (f *fakeVerifier) Close() error                                     { f.closed = true; return nil }

// opener records the path it was asked to open and what was there.
type opener struct {
	v       *fakeVerifier
	path    string
	body    string
	openErr error
}

func (o *opener) open(_ context.Context, path string) (Verifier, error) {
	o.path = path
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	o.body = string(b)
	if o.openErr != nil {
		return nil, o.openErr
	}
	return o.v, nil
}

func goodCounts() map[string]int64 {
	return map[string]int64{"people": 540, "terms": 4000, "changes": 10, "stories": 60, "mentions": 35, "marks": 2}
}

// backedUp is a backup disk with one real backup taken by the backup job.
func backedUp(t *testing.T) string {
	t.Helper()
	data, dest := fixture(t)
	require.NoError(t, job(data, dest, &fakeDB{}).Run(context.Background()), "taking a backup")
	return dest
}

func check(dest string, o *opener, scratch string) *RestoreCheck {
	return &RestoreCheck{Dest: dest, Open: o.open, Scratch: scratch, Now: func() time.Time { return now.Add(3 * time.Hour) }}
}

func TestRestoreCheck_OpensACopyOfTheNewestBackupAndLeavesTheBackupAlone(t *testing.T) {
	t.Parallel()
	dest := backedUp(t)
	write(t, filepath.Join(dest, "2026-11-10", "manifest.json"), "{}") // an older backup
	o := &opener{v: &fakeVerifier{counts: goodCounts()}}
	scratch := t.TempDir()

	require.NoError(t, check(dest, o, scratch).Run(context.Background()), "checking the backup")

	assert.Equal(t, "sqlite", o.body, "the newest backup's database was opened")
	assert.Equal(t, scratch, filepath.Dir(filepath.Dir(o.path)), "from a copy in the scratch directory, not on the backup disk")
	assert.True(t, o.v.closed, "the copy is closed")
	assert.NoDirExists(t, filepath.Dir(o.path), "and removed")
	b, err := os.ReadFile(filepath.Join(dest, "2026-11-16", databaseName))
	require.NoError(t, err, "reading the backup's database")
	assert.Equal(t, "sqlite", string(b), "the backup itself is unchanged")
}

func TestRestoreCheck_Fails(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		setup func(t *testing.T, dest string, o *opener)
		want  string
		is    error
	}{
		"no marker: the disk is missing": {
			setup: func(t *testing.T, dest string, _ *opener) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dest, Marker)), "taking the marker away")
			},
			is: ErrNoDisk,
		},
		"no backup at all": {
			setup: func(t *testing.T, dest string, _ *opener) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(dest, "2026-11-16")), "removing the backup")
			},
			want: "no backup",
		},
		"the newest backup is too old": {
			setup: func(t *testing.T, dest string, _ *opener) {
				t.Helper()
				require.NoError(t, os.Rename(filepath.Join(dest, "2026-11-16"), filepath.Join(dest, "2026-11-12")), "dating it back")
			},
			want: "the newest backup, 2026-11-12, is 4 days old",
		},
		"a file the manifest counts is gone": {
			setup: func(t *testing.T, dest string, _ *opener) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dest, "2026-11-16", "portraits", "A000001.jpg")), "losing a portrait")
			},
			want: "the manifest counts 3 files",
		},
		"the database does not open": {
			setup: func(t *testing.T, _ string, o *opener) { t.Helper(); o.openErr = errors.New("file is not a database") },
			want:  "file is not a database",
		},
		"the integrity check fails": {
			setup: func(t *testing.T, _ string, o *opener) {
				t.Helper()
				o.v.err = errors.New("integrity check: row 3 missing")
			},
			want: "row 3 missing",
		},
		"the roster is empty": {
			setup: func(t *testing.T, _ string, o *opener) { t.Helper(); o.v.counts["people"] = 0 },
			want:  "no members",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dest := backedUp(t)
			o := &opener{v: &fakeVerifier{counts: goodCounts()}}
			tc.setup(t, dest, o)
			scratch := t.TempDir()

			err := check(dest, o, scratch).Run(context.Background())

			require.Error(t, err, "the check fails")
			if tc.is != nil {
				require.ErrorIs(t, err, tc.is, "with the right error")
			}
			assert.Contains(t, err.Error(), tc.want, "and says why")
			entries, rerr := os.ReadDir(scratch)
			require.NoError(t, rerr, "listing the scratch directory")
			assert.Empty(t, entries, "no copy is left behind")
		})
	}
}

func TestBothJobs_RefuseWithNoBackupDirectory(t *testing.T) {
	t.Parallel()
	data, _ := fixture(t)
	db := &fakeDB{}
	require.ErrorIs(t, job(data, "", db).Run(context.Background()), ErrNoDir, "the backup")
	assert.Empty(t, db.paths, "takes no snapshot")
	o := &opener{v: &fakeVerifier{counts: goodCounts()}}
	require.ErrorIs(t, check("", o, t.TempDir()).Run(context.Background()), ErrNoDir, "the restore check")
}
