package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// RestoreJobName is the restore check's registered name.
const RestoreJobName = "restore-check"

// DefaultMaxBackupAge is how old the newest backup may be before the
// restore check fails: a nightly backup that missed one night is still
// fine, two is a problem.
const DefaultMaxBackupAge = 48 * time.Hour

// Verifier is a database opened (and migrated) from a backup's copy.
// *store.Store implements it.
type Verifier interface {
	Verify(ctx context.Context) (map[string]int64, error)
	Close() error
}

// Opener opens and migrates the database at path, as the server would on a
// restored data directory.
type Opener func(ctx context.Context, path string) (Verifier, error)

// RestoreCheck is the weekly restore drill, run for real: the newest backup
// is copied out, opened as the server would open it, checked and counted.
// A backup that was never restored is a hope, not a backup.
type RestoreCheck struct {
	Dest string
	Open Opener
	// Scratch is where the copy is made; empty is the system's temp
	// directory (an emptyDir in the pod).
	Scratch string
	Now     func() time.Time
	// MaxAge is how old the newest backup may be; zero is the default.
	MaxAge time.Duration
}

// Name implements jobs.Job.
func (*RestoreCheck) Name() string { return RestoreJobName }

// Run checks the newest backup. It never writes to the backup disk.
func (c *RestoreCheck) Run(ctx context.Context) error {
	if err := checkDisk(c.Dest); err != nil {
		return err
	}
	day, err := c.newest()
	if err != nil {
		return err
	}
	maxAge := c.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultMaxBackupAge
	}
	if age := c.Now().UTC().Sub(day); age > maxAge {
		return fmt.Errorf("restore check: the newest backup, %s, is %d days old", day.Format(dayLayout), int(age.Hours()/24))
	}
	dir := filepath.Join(c.Dest, day.Format(dayLayout))
	m, err := checkManifest(dir)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp(c.Scratch, "restore-check-*")
	if err != nil {
		return fmt.Errorf("restore check: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	copyPath := filepath.Join(tmp, databaseName)
	if err := copyFile(filepath.Join(dir, databaseName), copyPath); err != nil {
		return fmt.Errorf("restore check: copy the database: %w", err)
	}
	db, err := c.Open(ctx, copyPath)
	if err != nil {
		return fmt.Errorf("restore check: open %s's database: %w", day.Format(dayLayout), err)
	}
	counts, err := db.Verify(ctx)
	_ = db.Close()
	if err != nil {
		return fmt.Errorf("restore check: %s: %w", day.Format(dayLayout), err)
	}
	if counts["people"] == 0 {
		return fmt.Errorf("restore check: %s's database has no members", day.Format(dayLayout))
	}
	ev := zerolog.Ctx(ctx).Info().Str("backup", day.Format(dayLayout)).Int("files", m.Files).Int64("bytes", m.Bytes)
	for table, n := range counts {
		ev = ev.Int64(table, n)
	}
	ev.Msg("Restored and checked the newest backup")
	return nil
}

// newest returns the date of the newest finished backup.
func (c *RestoreCheck) newest() (time.Time, error) {
	entries, err := os.ReadDir(c.Dest)
	if err != nil {
		return time.Time{}, fmt.Errorf("restore check: %w", err)
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
	if len(days) == 0 {
		return time.Time{}, fmt.Errorf("restore check: no backup in %s", c.Dest)
	}
	return slices.MaxFunc(days, func(a, b time.Time) int { return a.Compare(b) }), nil
}

// checkManifest reads a backup's manifest and checks that the files it
// counts are all there, with the bytes it counts.
func checkManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, manifestName)) //nolint:gosec // inside the backup directory
	if err != nil {
		return m, fmt.Errorf("restore check: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("restore check: %s: %w", manifestName, err)
	}
	var files int
	var size int64
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || d.Name() == manifestName || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		size += info.Size()
		return nil
	})
	if err != nil {
		return m, fmt.Errorf("restore check: %w", err)
	}
	if files != m.Files || size != m.Bytes {
		return m, fmt.Errorf("restore check: the manifest counts %d files and %d bytes; the backup has %d and %d: %w",
			m.Files, m.Bytes, files, size, errIncomplete)
	}
	return m, nil
}

var errIncomplete = errors.New("the backup is incomplete")
