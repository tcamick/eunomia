package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"
)

// JobResult is one run of a job, as the runner records it.
type JobResult struct {
	Name     string
	RunAt    time.Time
	Duration time.Duration
	OK       bool
	Message  string
}

// JobStatus is the last known state of a job.
type JobStatus struct {
	Name         string        `json:"name"`
	LastRun      time.Time     `json:"lastRun"`
	LastOK       time.Time     `json:"lastOk,omitzero"`
	OK           bool          `json:"ok"`
	LastDuration time.Duration `json:"-"`
	Message      string        `json:"message,omitempty"`
}

// maxMessage bounds a job's stored message; a longer one (a wrapped error
// that quotes a response) is cut, since the status page shows one line.
const maxMessage = 500

// RecordJob upserts the job's last run. last_ok moves only on success.
func (s *Store) RecordJob(ctx context.Context, r JobResult) error {
	runAt := r.RunAt.UTC().Format(time.RFC3339)
	var lastOK any
	if r.OK {
		lastOK = runAt
	}
	msg := r.Message
	if len(msg) > maxMessage {
		// Back off to a rune boundary, so the cut never splits a character.
		cut := maxMessage
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut]
	}
	ok := 0
	if r.OK {
		ok = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs (name, last_run, last_ok, ok, last_duration_ms, message)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
		  last_run = excluded.last_run,
		  last_ok = COALESCE(excluded.last_ok, jobs.last_ok),
		  ok = excluded.ok,
		  last_duration_ms = excluded.last_duration_ms,
		  message = excluded.message`,
		r.Name, runAt, lastOK, ok, r.Duration.Milliseconds(), msg)
	if err != nil {
		return fmt.Errorf("record job %s: %w", r.Name, err)
	}
	return nil
}

// Jobs returns every job's last known state, by name.
func (s *Store) Jobs(ctx context.Context) ([]JobStatus, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		`SELECT name, last_run, last_ok, ok, last_duration_ms, message FROM jobs ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []JobStatus
	for rows.Next() {
		var (
			j       JobStatus
			lastRun string
			lastOK  sql.NullString
			ok      int
			ms      int64
		)
		if err := rows.Scan(&j.Name, &lastRun, &lastOK, &ok, &ms, &j.Message); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		if j.LastRun, err = time.Parse(time.RFC3339, lastRun); err != nil {
			return nil, fmt.Errorf("job %s last_run %q: %w", j.Name, lastRun, err)
		}
		if lastOK.Valid {
			if j.LastOK, err = time.Parse(time.RFC3339, lastOK.String); err != nil {
				return nil, fmt.Errorf("job %s last_ok %q: %w", j.Name, lastOK.String, err)
			}
		}
		j.OK = ok == 1
		j.LastDuration = time.Duration(ms) * time.Millisecond
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return out, nil
}
