package store

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
)

// ErrCorrupt is a database whose integrity check did not say "ok".
var ErrCorrupt = errors.New("the database failed its integrity check")

// SnapshotTo writes a consistent copy of the database to path with VACUUM
// INTO, which reads one transaction's view and so is safe while the server
// and jobs write. path must not exist. It is the one way a backup reads the
// database: the file itself is never copied, since a copy taken mid-write
// is not a database. Ported from Plutus (its internal/store/snapshot.go).
func (s *Store) SnapshotTo(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("snapshot %s: already exists", path)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("snapshot %s: %w", path, err)
	}
	return nil
}

// countQueries are the tables a restore check counts, each with its fixed
// query: the ones whose rows cannot be refetched (the news archive, the
// owner's marks, the history of changes) and the roster that every page
// reads. Constant strings, so no SQL is built at run time.
var countQueries = map[string]string{
	"people":   "SELECT count(*) FROM people",
	"terms":    "SELECT count(*) FROM terms",
	"changes":  "SELECT count(*) FROM changes",
	"stories":  "SELECT count(*) FROM stories",
	"mentions": "SELECT count(*) FROM mentions",
	"marks":    "SELECT count(*) FROM marks",
}

// VerifiedTables returns the tables Verify counts, sorted.
func VerifiedTables() []string {
	return slices.Sorted(maps.Keys(countQueries))
}

// Verify runs SQLite's integrity check and counts the rows of each of
// VerifiedTables. A restore check runs it on a backup opened in a scratch
// directory.
func (s *Store) Verify(ctx context.Context) (map[string]int64, error) {
	var result string
	if err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return nil, fmt.Errorf("integrity check: %w", err)
	}
	if result != "ok" {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	counts := make(map[string]int64, len(countQueries))
	for table, q := range countQueries {
		var n int64
		if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}
