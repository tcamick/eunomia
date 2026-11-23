// Package portraits mirrors members' official portraits from the
// unitedstates/images project to the data volume, so the browser loads them
// from this origin and never from a third party (docs/web-conventions.md,
// Security). Each is checked again at most once per MaxAge.
package portraits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/rs/zerolog"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/store"
)

// Source is the fetch source name.
const Source = "portraits"

// DefaultBase is where the portraits are, by Bioguide id. Public domain
// works of the US government.
const DefaultBase = "https://unitedstates.github.io/images/congress/225x275/"

// Defaults for a run.
const (
	DefaultMaxAge = 30 * 24 * time.Hour
	DefaultPerRun = 600
	// maxFailures stops a run after this many failed requests in a row: the
	// source is down, and asking 500 more times will not help.
	maxFailures = 5
)

// reName is a file this package writes: a Bioguide id and .jpg. Nothing
// from outside chooses a file name (go-conventions, Security).
var reName = regexp.MustCompile(`^[A-Z][0-9]{6}\.jpg$`)

// ValidName reports whether name is a portrait file name this package writes.
func ValidName(name string) bool { return reName.MatchString(name) }

// Store is what the job needs of the store; *store.Store implements it.
type Store interface {
	PortraitsDue(ctx context.Context, before time.Time, limit int) ([]store.PortraitState, error)
	RecordPortrait(ctx context.Context, ps store.PortraitState) error
}

// Getter is the part of fetch.Client the job uses.
type Getter interface {
	Get(ctx context.Context, source, rawURL string, prev fetch.Validators) (*fetch.Response, error)
}

// Job is `eunomia job portraits`.
type Job struct {
	get    Getter
	st     Store
	base   string
	dir    string
	now    func() time.Time
	maxAge time.Duration
	perRun int
}

// NewJob returns the portraits job writing under dir.
func NewJob(get Getter, st Store, base, dir string, now func() time.Time) *Job {
	return &Job{get: get, st: st, base: base, dir: dir, now: now, maxAge: DefaultMaxAge, perRun: DefaultPerRun}
}

// JobName is the job's registered name.
const JobName = "portraits"

// Name is the job's registered name.
func (*Job) Name() string { return JobName }

// jpegMagic starts every JPEG file.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

// Run mirrors the portraits that are due.
func (j *Job) Run(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	now := j.now()
	due, err := j.st.PortraitsDue(ctx, now.Add(-j.maxAge), j.perRun)
	if err != nil {
		return err
	}
	if len(due) == 0 {
		log.Debug().Msg("Found no portrait due")
		return nil
	}
	if err := os.MkdirAll(j.dir, 0o750); err != nil {
		return fmt.Errorf("create portrait dir: %w", err)
	}
	var written, unchanged, missing, failed, inRow int
	var lastErr error
	for _, ps := range due {
		name := ps.Person + ".jpg"
		if !ValidName(name) {
			return fmt.Errorf("portrait for %q: not a Bioguide id", ps.Person)
		}
		res, err := j.one(ctx, ps, name)
		if err != nil {
			failed++
			inRow++
			lastErr = err
			if inRow >= maxFailures || ctx.Err() != nil {
				break
			}
			continue
		}
		inRow = 0
		switch res {
		case resWritten:
			written++
		case resUnchanged:
			unchanged++
		case resMissing:
			missing++
		}
	}
	log.Info().Int("due", len(due)).Int("written", written).Int("unchanged", unchanged).Int("missing", missing).
		Int("failed", failed).Msg("Mirrored portraits")
	if failed > 0 {
		return fmt.Errorf("%d portraits failed, the last: %w", failed, lastErr)
	}
	return nil
}

type result int

const (
	resWritten result = iota
	resUnchanged
	resMissing
)

func (j *Job) one(ctx context.Context, ps store.PortraitState, name string) (result, error) {
	prev := fetch.Validators{}
	if ps.File != "" {
		prev = fetch.Validators{ETag: ps.ETag, LastModified: ps.LastModified}
	}
	url := j.base + name
	rec := store.PortraitState{Person: ps.Person, CheckedAt: j.now(), Source: url}
	resp, err := j.get.Get(ctx, Source, url, prev)
	var se *fetch.StatusError
	switch {
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		// The source has no portrait for this member; shown as initials. If
		// it had one before, that is a change: the date is now, and the old
		// file goes, so the page never shows a photo the source withdrew.
		// The store is written first: the page reads it, so an orphaned file
		// is harmless, while a missing file the store still names is not.
		if ps.File == "" {
			return resMissing, j.st.RecordPortrait(ctx, rec)
		}
		rec.FetchedAt = rec.CheckedAt
		if err := j.st.RecordPortrait(ctx, rec); err != nil {
			return 0, err
		}
		if err := os.Remove(filepath.Join(j.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, fmt.Errorf("remove withdrawn portrait %s: %w", ps.Person, err)
		}
		return resMissing, nil
	case err != nil:
		return 0, err
	case resp.NotModified:
		rec.File, rec.ETag, rec.LastModified = ps.File, resp.Validators.ETag, resp.Validators.LastModified
		return resUnchanged, j.st.RecordPortrait(ctx, rec)
	}
	if !bytes.HasPrefix(resp.Body, jpegMagic) {
		return 0, fmt.Errorf("portrait %s is not a JPEG", ps.Person)
	}
	if err := writeAtomic(filepath.Join(j.dir, name), resp.Body); err != nil {
		return 0, err
	}
	rec.File, rec.ETag, rec.LastModified, rec.FetchedAt = name, resp.Validators.ETag, resp.Validators.LastModified, rec.CheckedAt
	return resWritten, j.st.RecordPortrait(ctx, rec)
}

// writeAtomic writes b to path through a temporary file in the same
// directory, so the server never serves half an image.
func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".portrait-*.tmp") // .tmp: never copied by a backup
	if err != nil {
		return fmt.Errorf("write portrait: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write portrait: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write portrait: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("write portrait: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write portrait: %w", err)
	}
	return nil
}
