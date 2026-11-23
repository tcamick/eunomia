package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PortraitState is a row of portraits: where the mirrored image is and the
// validators to ask the source whether it changed.
type PortraitState struct {
	Person       string
	File         string // empty when the source has no portrait
	ETag         string
	LastModified string
	CheckedAt    time.Time
	Source       string
	FetchedAt    time.Time // when the file was last written; zero if never
}

// PortraitsDue lists the members in Congress whose portrait was never
// checked, or was last checked before before, at most limit of them, the
// never-checked first.
func (s *Store) PortraitsDue(ctx context.Context, before time.Time, limit int) ([]PortraitState, error) {
	var out []PortraitState
	err := eachRow(ctx, s.db, `
		SELECT p.id, COALESCE(x.file, ''), COALESCE(x.etag, ''), COALESCE(x.last_modified, '')
		FROM people p LEFT JOIN portraits x ON x.person = p.id
		WHERE p.in_congress = 1 AND (x.person IS NULL OR x.checked_at < ?)
		ORDER BY x.checked_at IS NOT NULL, x.checked_at, p.id
		LIMIT ?`, func(r *sql.Rows) error {
		var ps PortraitState
		if err := r.Scan(&ps.Person, &ps.File, &ps.ETag, &ps.LastModified); err != nil {
			return err
		}
		out = append(out, ps)
		return nil
	}, before.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("list portraits due: %w", err)
	}
	return out, nil
}

// RecordPortrait upserts a portrait check. A zero FetchedAt keeps the stored
// one (the file did not change).
func (s *Store) RecordPortrait(ctx context.Context, ps PortraitState) error {
	var fetched any
	if !ps.FetchedAt.IsZero() {
		fetched = ps.FetchedAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO portraits (person, file, etag, last_modified, checked_at, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (person) DO UPDATE SET
		  file = excluded.file, etag = excluded.etag, last_modified = excluded.last_modified,
		  checked_at = excluded.checked_at, source = excluded.source,
		  fetched_at = COALESCE(excluded.fetched_at, portraits.fetched_at)`,
		ps.Person, nullable(ps.File), ps.ETag, ps.LastModified, ps.CheckedAt.UTC().Format(time.RFC3339), ps.Source, fetched)
	if err != nil {
		return fmt.Errorf("record portrait %s: %w", ps.Person, err)
	}
	return nil
}

// PortraitFile returns the mirrored file's name for a person, if there is one.
func (s *Store) PortraitFile(ctx context.Context, person string) (string, bool, error) {
	var file sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT file FROM portraits WHERE person = ?`, person).Scan(&file)
	if err == sql.ErrNoRows { //nolint:errorlint // database/sql returns the sentinel itself
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read portrait %s: %w", person, err)
	}
	return file.String, file.Valid, nil
}

