// Package store owns the SQLite database. It is the only package that imports
// the driver and the only one that holds SQL (docs/go-conventions.md, Layout).
// The server and the job pods open the same file: WAL lets readers run while a
// job writes, and every transaction takes the write lock when it begins.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite" // also registers the "sqlite" driver
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed all:migrations
var migrationFS embed.FS

// Store is an open database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// openBusyWait bounds how long Open retries while another process holds the
// file exclusively (see pingUntilNotBusy).
const openBusyWait = 5 * time.Second

// Open opens (creating if needed) the SQLite file at path. now stamps the
// rows the store writes on its own (applied migrations). Pragmas travel in
// the DSN so the driver applies them to every connection database/sql opens,
// including a replacement after a bad connection.
func Open(ctx context.Context, path string, now func() time.Time) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite serialises writers; one connection avoids lock contention
	// between our own goroutines.
	db.SetMaxOpenConns(1)
	if err := pingUntilNotBusy(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return &Store{db: db, now: now}, nil
}

// pingUntilNotBusy opens the first connection, retrying on SQLITE_BUSY for up
// to openBusyWait. The first statement switches the file to WAL, which needs
// an exclusive lock and fails at once, without consulting busy_timeout, when
// another process is doing the same: a job pod and the server starting
// together on a new volume.
func pingUntilNotBusy(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, openBusyWait)
	defer cancel()
	wait := 5 * time.Millisecond
	for {
		err := db.PingContext(ctx)
		if err == nil || !isBusy(err) {
			return err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err // the SQLITE_BUSY says more than "deadline exceeded"
		case <-timer.C:
		}
		if wait < 100*time.Millisecond {
			wait *= 2
		}
	}
}

// isBusy reports whether err is SQLite's "database is locked".
func isBusy(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_BUSY
}

// dsn builds the driver's "file:" URI. OmitHost renders "file:<path>" with no
// "//", so a relative path is not read as a URI authority. _txlock=immediate
// makes every transaction take the write lock when it begins, where
// busy_timeout applies; a deferred transaction that reads and then writes
// would fail at once if another process wrote in between. A transaction that
// only reads says so with sql.TxOptions{ReadOnly: true}.
func dsn(path string) string {
	path = filepath.Clean(path)
	q := url.Values{}
	for _, pragma := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)"} {
		q.Add("_pragma", pragma)
	}
	q.Set("_txlock", "immediate")
	u := url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: q.Encode()}
	return u.String()
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Migrate applies the embedded migrations/*.sql in lexical order, once each,
// and returns the names it applied.
func (s *Store) Migrate(ctx context.Context) ([]string, error) {
	return s.migrate(ctx, migrationFS)
}

// migrate applies fsys's migrations/*.sql; the file system is a parameter so
// a test can supply a migration that fails. A file already recorded is
// skipped on a plain read, which a writer in another process never blocks
// under WAL; a file not yet recorded is checked again and applied inside one
// write transaction, so two processes starting together cannot both apply it.
func (s *Store) migrate(ctx context.Context, fsys fs.FS) ([]string, error) {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := migrationNames(fsys)
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, name := range names {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return applied, fmt.Errorf("check migration %s: %w", name, err)
		}
		if n > 0 {
			continue
		}
		body, err := fs.ReadFile(fsys, "migrations/"+name)
		if err != nil {
			return applied, fmt.Errorf("read migration %s: %w", name, err)
		}
		did, err := s.applyMigration(ctx, name, string(body))
		if err != nil {
			return applied, err
		}
		if did {
			applied = append(applied, name)
		}
	}
	return applied, nil
}

// applyMigration applies one file unless schema_migrations already has it,
// and reports whether it did. The check is repeated inside the transaction:
// another process may have applied the file since the caller's read.
func (s *Store) applyMigration(ctx context.Context, name, body string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("check migration %s: %w", name, err)
	}
	if n > 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return false, fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
		name, s.now().UTC().Format(time.RFC3339)); err != nil {
		return false, fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit migration %s: %w", name, err)
	}
	return true, nil
}

// migrationNames lists the *.sql files under migrations/ in lexical order.
func migrationNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("no migrations embedded")
	}
	return names, nil
}
