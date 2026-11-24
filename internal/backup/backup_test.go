package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDB writes a stand-in database and records where it was asked to.
type fakeDB struct {
	err   error
	paths []string
}

func (f *fakeDB) SnapshotTo(_ context.Context, path string) error {
	f.paths = append(f.paths, path)
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(path, []byte("sqlite"), 0o600)
}

var now = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

// noFlush stands in for sync(2), which would flush the whole host.
func noFlush() error { return nil }

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750), "making %s's directory", path)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600), "writing %s", path)
}

// fixture is a data directory with portraits and the live database (never
// read directly: the backup takes a snapshot), and a backup disk with its
// marker.
func fixture(t *testing.T) (data, dest string) {
	t.Helper()
	data, dest = t.TempDir(), t.TempDir()
	write(t, filepath.Join(data, "portraits", "A000001.jpg"), "jpeg a")
	write(t, filepath.Join(data, "portraits", "B000002.jpg"), "jpeg b")
	write(t, filepath.Join(data, "eunomia.sqlite"), "never read directly")
	write(t, filepath.Join(dest, Marker), "")
	return data, dest
}

func job(data, dest string, db Snapshotter) *Job {
	return &Job{DataDir: data, Dest: dest, DB: db, Now: func() time.Time { return now }, Flush: noFlush}
}

func readManifest(t *testing.T, dir string) Manifest {
	t.Helper()
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err, "reading the manifest")
	require.NoError(t, json.Unmarshal(b, &m), "decoding the manifest")
	return m
}

func TestRun_CopiesTheDatabaseAndThePortraits(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	db := &fakeDB{}
	require.NoError(t, job(data, dest, db).Run(context.Background()), "taking a backup")

	day := filepath.Join(dest, "2026-11-16")
	assert.Equal(t, []string{filepath.Join(dest, ".partial-2026-11-16", "eunomia.sqlite")}, db.paths,
		"the database comes from a snapshot, written beside the final directory")
	for _, f := range []string{"eunomia.sqlite", "portraits/A000001.jpg", "portraits/B000002.jpg", "manifest.json"} {
		assert.FileExists(t, filepath.Join(day, f), "the backup has %s", f)
	}
	assert.NoDirExists(t, filepath.Join(dest, ".partial-2026-11-16"), "nothing half-written is left")

	m := readManifest(t, day)
	assert.Equal(t, now, m.TakenAt, "the manifest's time")
	assert.Equal(t, 3, m.Files, "the database and two portraits")
	assert.Equal(t, []string{"portraits"}, m.Copied, "what was copied beside the database")
	assert.Positive(t, m.Bytes, "the bytes written")
}

func TestRun_FilesAreTheJobUsersWithGroupRead(t *testing.T) {
	t.Parallel()
	// Exact modes: the group reads files and enters directories but writes
	// nothing; others get nothing.
	data, dest := fixture(t)
	require.NoError(t, job(data, dest, &fakeDB{}).Run(context.Background()), "taking a backup")

	day := filepath.Join(dest, "2026-11-16")
	seen, sawDatabase := 0, false
	require.NoError(t, filepath.WalkDir(day, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err, "walking %s", path)
		info, err := d.Info()
		require.NoError(t, err, "reading %s's mode", path)
		want := fs.FileMode(fileMode)
		if d.IsDir() {
			want = dirMode
		}
		assert.Equal(t, want, info.Mode().Perm(), "mode of %s", path)
		if d.Name() == databaseName {
			sawDatabase = true
		}
		seen++
		return nil
	}), "walking the backup")
	assert.True(t, sawDatabase, "the database snapshot was checked")
	assert.Equal(t, 6, seen, "the day, the database, the manifest, the portraits directory and two portraits")
}

func TestRun_AMissingDiskFailsAndWritesNothing(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	require.NoError(t, os.Remove(filepath.Join(dest, Marker)), "taking the marker away")
	db := &fakeDB{}

	err := job(data, dest, db).Run(context.Background())

	require.ErrorIs(t, err, ErrNoDisk, "a backup with no marker")
	assert.Contains(t, err.Error(), "is it mounted?", "the error says what to check")
	assert.Empty(t, db.paths, "no snapshot is taken")
	entries, err := os.ReadDir(dest)
	require.NoError(t, err, "listing the backup directory")
	assert.Empty(t, entries, "nothing is written to what may be the system disk")
}

func TestRun_AFailedSnapshotLeavesTheDaysEarlierBackup(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	write(t, filepath.Join(dest, "2026-11-16", "manifest.json"), `{"earlier":true}`)

	err := job(data, dest, &fakeDB{err: errors.New("disk full")}).Run(context.Background())

	require.Error(t, err, "a backup whose snapshot fails")
	assert.Contains(t, err.Error(), "disk full", "with the cause")
	b, err := os.ReadFile(filepath.Join(dest, "2026-11-16", "manifest.json"))
	require.NoError(t, err, "reading the earlier manifest")
	assert.Equal(t, `{"earlier":true}`, string(b), "the earlier backup of the day is untouched")
	assert.NoDirExists(t, filepath.Join(dest, ".partial-2026-11-16"), "nothing half-written is left")
}

func TestRun_ASecondRunTheSameDayReplacesThatDaysBackup(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	j := job(data, dest, &fakeDB{})
	require.NoError(t, j.Run(context.Background()), "the first backup")
	write(t, filepath.Join(data, "portraits", "C000003.jpg"), "jpeg c")

	require.NoError(t, j.Run(context.Background()), "the second backup")

	assert.FileExists(t, filepath.Join(dest, "2026-11-16", "portraits", "C000003.jpg"), "the new portrait is in")
	entries, err := os.ReadDir(dest)
	require.NoError(t, err, "listing the backup directory")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{Marker, "2026-11-16"}, names, "one backup for the day, nothing set aside")
}

func TestRun_PrunesToTheDailiesAndWeeklies(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	for d := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC); d.Before(now.Truncate(24 * time.Hour)); d = d.AddDate(0, 0, 1) {
		write(t, filepath.Join(dest, d.Format("2006-01-02"), "manifest.json"), "{}")
	}
	write(t, filepath.Join(dest, "notes.txt"), "the owner's")
	j := job(data, dest, &fakeDB{})
	j.Daily, j.Weekly = 3, 2

	require.NoError(t, j.Run(context.Background()), "taking a backup")

	entries, err := os.ReadDir(dest)
	require.NoError(t, err, "listing the backup directory")
	kept := make([]string, 0, len(entries))
	for _, e := range entries {
		kept = append(kept, e.Name())
	}
	// Three newest days (today included), then the newest left in each of the
	// two newest weeks: 2026-11-13 (Fri, week 46) and 2026-11-08 (Sun, week 45).
	assert.ElementsMatch(t, []string{Marker, "notes.txt", "2026-11-16", "2026-11-15", "2026-11-14", "2026-11-13", "2026-11-08"},
		kept, "the dailies, the weeklies, and what is not a backup")
}

func TestRun_ASecondRunWhileOneIsRunningRefusesAndTouchesNothing(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	write(t, filepath.Join(dest, ".lock"), "")
	require.NoError(t, os.Chtimes(filepath.Join(dest, ".lock"), now.Add(-10*time.Minute), now.Add(-10*time.Minute)),
		"dating the other run's lock")
	write(t, filepath.Join(dest, ".partial-2026-11-16", "eunomia.sqlite"), "the other run's")
	db := &fakeDB{}

	err := job(data, dest, db).Run(context.Background())

	require.ErrorIs(t, err, ErrBusy, "a backup while another runs")
	assert.Contains(t, err.Error(), "since 2026-11-16T08:50:00Z", "the error says since when")
	assert.Empty(t, db.paths, "no snapshot is taken")
	b, err := os.ReadFile(filepath.Join(dest, ".partial-2026-11-16", "eunomia.sqlite"))
	require.NoError(t, err, "reading the other run's work")
	assert.Equal(t, "the other run's", string(b), "the running backup's work is left alone")
	assert.FileExists(t, filepath.Join(dest, ".lock"), "and so is its lock")
}

func TestRun_AStaleLockFromARunThatDiedIsTakenOver(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	write(t, filepath.Join(dest, ".lock"), "")
	require.NoError(t, os.Chtimes(filepath.Join(dest, ".lock"), now.Add(-3*time.Hour), now.Add(-3*time.Hour)),
		"dating a dead run's lock")

	require.NoError(t, job(data, dest, &fakeDB{}).Run(context.Background()), "taking a backup")

	assert.FileExists(t, filepath.Join(dest, "2026-11-16", "manifest.json"), "the backup is taken")
	assert.NoFileExists(t, filepath.Join(dest, ".lock"), "released when the run ends")
}

func TestRun_SkipsAPortraitStillBeingWritten(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	// The portraits job writes .portrait-*.tmp, then renames it.
	write(t, filepath.Join(data, "portraits", ".portrait-123.tmp"), "half")

	require.NoError(t, job(data, dest, &fakeDB{}).Run(context.Background()), "taking a backup")

	assert.NoFileExists(t, filepath.Join(dest, "2026-11-16", "portraits", ".portrait-123.tmp"), "a half-written file is skipped")
	assert.FileExists(t, filepath.Join(dest, "2026-11-16", "portraits", "A000001.jpg"), "a finished one is copied")
}

// The no-op case, adapted: a backup always writes (that is its job), so the
// check is that the same inputs give the same backup.
func TestRun_TheSameInputsGiveTheSameBackup(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	j := job(data, dest, &fakeDB{})
	require.NoError(t, j.Run(context.Background()), "the first backup")
	first := readManifest(t, filepath.Join(dest, "2026-11-16"))
	require.NoError(t, j.Run(context.Background()), "the second backup")
	assert.Equal(t, first, readManifest(t, filepath.Join(dest, "2026-11-16")), "the same backup")
}

// A run killed mid-copy (its deadline, out of memory) leaves its partial
// directory; the next run, holding the lock, clears every one.
func TestRun_ClearsPartialDirectoriesLeftByRunsThatDied(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	write(t, filepath.Join(dest, ".partial-2026-11-02", "eunomia.sqlite"), "half")
	write(t, filepath.Join(dest, ".partial-2026-11-15", "eunomia.sqlite"), "half")

	require.NoError(t, job(data, dest, &fakeDB{}).Run(context.Background()), "taking a backup")

	assert.NoDirExists(t, filepath.Join(dest, ".partial-2026-11-02"), "an old partial is cleared")
	assert.NoDirExists(t, filepath.Join(dest, ".partial-2026-11-15"), "and yesterday's")
	assert.FileExists(t, filepath.Join(dest, "2026-11-16", "manifest.json"), "today's backup is taken")
}

func TestRun_RecoversADaySetAsideByACrash(t *testing.T) {
	t.Parallel()
	data, dest := fixture(t)
	// A crash between swap's two renames left yesterday's backup set aside.
	write(t, filepath.Join(dest, "2026-11-15.replaced", "manifest.json"), `{"yesterday":true}`)
	j := job(data, dest, &fakeDB{})
	j.Now = func() time.Time { return now.AddDate(0, 0, -1) }

	require.NoError(t, j.Run(context.Background()), "taking yesterday's backup again")

	assert.NoDirExists(t, filepath.Join(dest, "2026-11-15.replaced"), "nothing is left set aside")
	assert.FileExists(t, filepath.Join(dest, "2026-11-15", "manifest.json"), "the day's backup is in place")
}

func TestExpired_KeepsTheDailiesThenOnePerWeek(t *testing.T) {
	t.Parallel()
	d := func(s string) time.Time {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			panic(err)
		}
		return t
	}
	days := func(from, to string) []time.Time {
		var out []time.Time
		for t := d(from); !t.After(d(to)); t = t.AddDate(0, 0, 1) {
			out = append(out, t)
		}
		return out
	}
	names := func(ts []time.Time) []string {
		out := make([]string, 0, len(ts))
		for _, t := range ts {
			out = append(out, t.Format("2006-01-02"))
		}
		return out
	}
	tests := map[string]struct {
		days          []time.Time
		daily, weekly int
		keep          int
		mustKeep      []string
		mustRemove    []string
	}{
		"fewer than the dailies: nothing removed": {
			days: days("2026-11-07", "2026-11-16"), daily: 14, weekly: 8, keep: 10,
		},
		"the defaults over four months keep 14 dailies and 8 weeklies": {
			days: days("2026-07-20", "2026-11-16"), daily: 14, weekly: 8, keep: 22,
			mustKeep:   []string{"2026-11-16", "2026-11-03", "2026-11-02", "2026-11-01"},
			mustRemove: []string{"2026-10-31", "2026-07-20"},
		},
		"input order does not matter": {
			days: []time.Time{d("2026-11-01"), d("2026-11-16"), d("2026-11-08"), d("2026-11-15")}, daily: 2, weekly: 1, keep: 3,
			mustKeep: []string{"2026-11-16", "2026-11-15", "2026-11-08"}, mustRemove: []string{"2026-11-01"},
		},
		"no weeklies keeps only the dailies": {
			days: days("2026-11-01", "2026-11-16"), daily: 3, weekly: 0, keep: 3,
			mustKeep: []string{"2026-11-16", "2026-11-14"}, mustRemove: []string{"2026-11-13"},
		},
		"a week across the new year is one week": {
			// 2026-12-28 to 2027-01-03 is ISO week 2026-W53.
			days: []time.Time{d("2027-01-04"), d("2027-01-03"), d("2026-12-31"), d("2026-12-28")}, daily: 1, weekly: 2, keep: 2,
			mustKeep: []string{"2027-01-04", "2027-01-03"}, mustRemove: []string{"2026-12-31", "2026-12-28"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			removed := names(Expired(tc.days, tc.daily, tc.weekly))
			assert.Len(t, tc.days, tc.keep+len(removed), "kept + removed = all")
			for _, k := range tc.mustKeep {
				assert.NotContains(t, removed, k, "%s is kept", k)
			}
			for _, r := range tc.mustRemove {
				assert.Contains(t, removed, r, "%s is removed", r)
			}
		})
	}
}
