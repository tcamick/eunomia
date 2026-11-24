// Package backup is the backup job (PRD §8; docs/implementation-plan.md,
// E2): a consistent copy of the SQLite database, whose news archive and
// owner's marks cannot be refetched, and the mirrored portraits beside it,
// written to a dated directory on the backup disk (seabass), with older
// copies pruned to a fixed number of dailies and weeklies.
//
// Ported from Plutus's internal/backup (its ADR 0011), which has run there
// nightly since 2026-11-16: the marker, the lock, the swap and the pruning
// are unchanged; the marker's name and what is copied are Eunomia's.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Marker is the file that must exist at the top of the backup directory. It
// is put there once, on the disk itself, so a disk that is not mounted
// (an empty mount point on the system disk) fails the backup instead of
// filling the wrong disk.
const Marker = ".eunomia-backup-disk"

// ErrNoDisk is a backup directory without the marker: the disk is missing.
var ErrNoDisk = errors.New("the backup disk is not there")

// ErrNoDir is a job run with no backup directory set (-backup-dir).
var ErrNoDir = errors.New("no backup directory is set (-backup-dir)")

// checkDisk refuses an unset directory or one without the marker.
func checkDisk(dest string) error {
	if dest == "" {
		return ErrNoDir
	}
	if _, err := os.Stat(filepath.Join(dest, Marker)); err != nil {
		return fmt.Errorf("%w: no %s in %s; is it mounted?", ErrNoDisk, Marker, dest)
	}
	return nil
}

// ErrBusy is another backup running in the same directory. Two at once
// would share today's partial directory and spoil each other (Plutus NP3,
// 2026-11-16: a run started by hand overlapped the nightly CronJob, and
// both failed).
var ErrBusy = errors.New("another backup is running")

// staleLock is how old a lock may be before it is taken to be left by a
// run that died: longer than the CronJob's half-hour deadline (k8s/app/backup.yaml).
const staleLock = 2 * time.Hour

const lockName = ".lock"

// Snapshotter writes a consistent copy of the state database. *store.Store
// implements it (SnapshotTo).
type Snapshotter interface {
	SnapshotTo(ctx context.Context, path string) error
}

// Defaults for how many backups are kept.
const (
	DefaultDaily  = 14
	DefaultWeekly = 8
)

// copied lists what is copied from the data directory beside the database,
// when present: the mirrored portraits (refetchable, but slowly, and the
// source may withdraw one).
func copied() []string { return []string{"portraits"} }

// Job is the backup job.
type Job struct {
	DataDir string
	Dest    string
	DB      Snapshotter
	Now     func() time.Time
	// Daily and Weekly are how many to keep; zero means the default.
	Daily, Weekly int
	// Flush makes what was written durable before the swap; nil is sync(2).
	// Tests pass their own, so they cause no host-wide sync.
	Flush func() error
}

// Manifest is written into every backup.
type Manifest struct {
	TakenAt time.Time `json:"takenAt"`
	Files   int       `json:"files"`
	Bytes   int64     `json:"bytes"`
	Copied  []string  `json:"copied"`
}

const (
	dayLayout    = "2006-01-02"
	partialStart = ".partial-"
	manifestName = "manifest.json"
	databaseName = "eunomia.sqlite"
)

// JobName is the backup job's registered name.
const JobName = "backup"

// Name implements jobs.Job.
func (j *Job) Name() string { return JobName }

// Run takes today's backup (UTC date), replacing one already taken today,
// then prunes. A failure leaves the previous backups as they were.
func (j *Job) Run(ctx context.Context) error {
	if err := checkDisk(j.Dest); err != nil {
		return err
	}
	now := j.Now().UTC()
	unlock, err := lock(j.Dest, now)
	if err != nil {
		return err
	}
	defer unlock()
	day := now.Format(dayLayout)
	partial := filepath.Join(j.Dest, partialStart+day)
	final := filepath.Join(j.Dest, day)
	recoverReplaced(final)

	// Every partial directory is a run that died (the lock is ours): today's
	// and any earlier one, which nothing else would ever clear.
	stale, err := filepath.Glob(filepath.Join(j.Dest, partialStart+"*"))
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	for _, dir := range stale {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("backup: clear %s: %w", dir, err)
		}
	}
	if err := os.MkdirAll(partial, dirMode); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if _, err := j.write(ctx, partial, now); err != nil {
		_ = os.RemoveAll(partial)
		return err
	}
	// One flush of everything written, before the backup is put in place:
	// syncing each file instead took about 25 ms a file on the USB disk
	// (Plutus NP3, 2026-11-16); the portraits are some 500 files.
	doFlush := j.Flush
	if doFlush == nil {
		doFlush = flush
	}
	if err := doFlush(); err != nil {
		_ = os.RemoveAll(partial)
		return fmt.Errorf("backup: flush: %w", err)
	}
	if err := swap(partial, final); err != nil {
		_ = os.RemoveAll(partial)
		return err
	}
	return j.prune(ctx)
}

// What the job writes is the pod user's (65532), with group read for
// whoever the owner gives the directory's group to; others get nothing.
const (
	fileMode = 0o640
	dirMode  = 0o750
)

func (j *Job) write(ctx context.Context, dir string, now time.Time) (Manifest, error) {
	m := Manifest{TakenAt: now, Copied: []string{}}
	if err := j.DB.SnapshotTo(ctx, filepath.Join(dir, databaseName)); err != nil {
		return m, fmt.Errorf("backup: %w", err)
	}
	// VACUUM INTO creates the file with SQLite's own mode; set it to fileMode
	// like the files this package creates. All of them assume a umask that
	// keeps group read and search, as the container's 022 does.
	if err := os.Chmod(filepath.Join(dir, databaseName), fileMode); err != nil {
		return m, fmt.Errorf("backup: %w", err)
	}
	if err := count(filepath.Join(dir, databaseName), &m); err != nil {
		return m, err
	}
	for _, name := range copied() {
		src := filepath.Join(j.DataDir, name)
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := copyTree(ctx, src, filepath.Join(dir, name), &m); err != nil {
			return m, fmt.Errorf("backup: copy %s: %w", name, err)
		}
		m.Copied = append(m.Copied, name)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, fmt.Errorf("backup: manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), append(b, '\n'), fileMode); err != nil {
		return m, fmt.Errorf("backup: manifest: %w", err)
	}
	return m, nil
}

// lock takes the backup directory for this run: the lock file is created
// exclusively, so of two runs one wins. A lock older than staleLock was
// left by a run that died and is taken over.
func lock(dest string, now time.Time) (func(), error) {
	path := filepath.Join(dest, lockName)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // inside the backup directory
		if err == nil {
			_, _ = f.WriteString(now.Format(time.RFC3339) + "\n")
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("backup: lock: %w", err)
		}
		fi, statErr := os.Stat(path)
		if statErr != nil {
			continue // removed between the two calls: try again
		}
		if now.Sub(fi.ModTime()) < staleLock {
			return nil, fmt.Errorf("%w: since %s", ErrBusy, fi.ModTime().UTC().Format(time.RFC3339))
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("backup: clear a stale lock: %w", err)
		}
	}
	return nil, fmt.Errorf("%w: could not take the lock", ErrBusy)
}

// swap puts the finished directory in place of today's, removing an earlier
// backup of the same day only after the new one is complete.
func swap(partial, final string) error {
	old := final + ".replaced"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return fmt.Errorf("backup: set aside %s: %w", final, err)
		}
	}
	if err := os.Rename(partial, final); err != nil {
		_ = os.Rename(old, final)
		return fmt.Errorf("backup: put %s in place: %w", final, err)
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("backup: remove %s: %w", old, err)
	}
	return nil
}

// recoverReplaced puts back a day's backup that a crash left set aside
// between swap's two renames, so it is neither lost nor left unpruned.
func recoverReplaced(final string) {
	old := final + ".replaced"
	if _, err := os.Stat(old); err != nil {
		return
	}
	if _, err := os.Stat(final); errors.Is(err, fs.ErrNotExist) {
		_ = os.Rename(old, final)
	}
}

func count(path string, m *Manifest) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	m.Files++
	m.Bytes += fi.Size()
	return nil
}

// copyTree copies a file or a directory tree, checking ctx between files.
func copyTree(ctx context.Context, src, dst string, m *Manifest) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A file renamed or removed by a job writing beside us since the
			// directory was listed is not an error (Plutus NP3 review).
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirMode)
		}
		if !d.Type().IsRegular() {
			return nil // symlinks and devices are not data
		}
		// Jobs write a file as a .tmp and rename it into place (the
		// portraits job); a .tmp is never finished data.
		if strings.HasSuffix(d.Name(), ".tmp") {
			return nil
		}
		if err := copyFile(path, target); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // renamed or removed since it was listed
			}
			return err
		}
		return count(target, m)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // a path inside the configured data directory
	if err != nil {
		return err // fs.ErrNotExist is the caller's to skip
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode) //nolint:gosec // a path inside the backup directory
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// prune keeps the newest Daily backups and, of the older ones, the newest of
// each ISO week for Weekly weeks. Only directories named as a date are
// considered; anything else in the backup directory is left alone.
func (j *Job) prune(ctx context.Context) error {
	daily, weekly := j.Daily, j.Weekly
	if daily <= 0 {
		daily = DefaultDaily
	}
	if weekly <= 0 {
		weekly = DefaultWeekly
	}
	entries, err := os.ReadDir(j.Dest)
	if err != nil {
		return fmt.Errorf("backup: prune: %w", err)
	}
	var days []time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if t, err := time.Parse(dayLayout, e.Name()); err == nil {
			days = append(days, t)
		}
	}
	for _, t := range Expired(days, daily, weekly) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("backup: prune: %w", err)
		}
		if err := os.RemoveAll(filepath.Join(j.Dest, t.Format(dayLayout))); err != nil {
			return fmt.Errorf("backup: prune %s: %w", t.Format(dayLayout), err)
		}
	}
	return nil
}

// Expired returns the backup days to remove: all but the newest daily, and
// of the rest all but the newest in each of the newest weekly ISO weeks.
func Expired(days []time.Time, daily, weekly int) []time.Time {
	sorted := append([]time.Time(nil), days...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].After(sorted[b]) })
	var out []time.Time
	weeks := map[[2]int]bool{}
	for i, t := range sorted {
		if i < daily {
			continue
		}
		y, w := t.ISOWeek()
		key := [2]int{y, w}
		if !weeks[key] && len(weeks) < weekly {
			weeks[key] = true
			continue
		}
		out = append(out, t)
	}
	return out
}
