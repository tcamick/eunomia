package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

// open returns a migrated store on a fresh file under t.TempDir().
func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "eunomia.sqlite"), func() time.Time { return t0 })
	require.NoError(t, err, "opening a new database")
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.Migrate(context.Background())
	require.NoError(t, err, "applying migrations")
	return s
}

func TestMigrate_AppliesEachFileOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "eunomia.sqlite")
	ctx := context.Background()
	s, err := Open(ctx, path, func() time.Time { return t0 })
	require.NoError(t, err, "calling Open")
	first, err := s.Migrate(ctx)
	require.NoError(t, err, "calling s.Migrate")
	assert.NotEmpty(t, first, "a new file gets every migration")
	require.NoError(t, s.Close(), "calling s.Close")

	s, err = Open(ctx, path, func() time.Time { return t0 })
	require.NoError(t, err, "calling Open")
	defer func() { _ = s.Close() }()
	second, err := s.Migrate(ctx)
	require.NoError(t, err, "calling s.Migrate")
	assert.Empty(t, second, "reopening applies nothing again")
}

func TestMigrate_AFailingFileIsNotRecorded(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	bad := fstest.MapFS{
		"migrations/0001_jobs.sql":   {Data: []byte("SELECT 1;")},
		"migrations/9999_broken.sql": {Data: []byte("CREATE TABLE nope (")},
	}
	_, err := s.migrate(ctx, bad)
	require.Error(t, err, "expected s.migrate to fail")
	assert.Contains(t, err.Error(), "9999_broken.sql")
	var n int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name = '9999_broken.sql'`).Scan(&n), "calling s.db.QueryRowContext")
	assert.Zero(t, n, "a failed migration must be retried next start, not marked done")
}

func TestRecordJob_KeepsLastSuccessThroughFailures(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	require.NoError(t, s.RecordJob(ctx, JobResult{Name: "noop", RunAt: t0, Duration: 1500 * time.Millisecond, OK: true}), "calling s.RecordJob")
	require.NoError(t, s.RecordJob(ctx, JobResult{Name: "noop", RunAt: t0.Add(time.Hour), OK: false, Message: "fetch failed"}), "calling s.RecordJob")

	jobs, err := s.Jobs(ctx)
	require.NoError(t, err, "calling s.Jobs")
	require.Len(t, jobs, 1)
	j := jobs[0]
	assert.Equal(t, "noop", j.Name)
	assert.False(t, j.OK, "the last run failed")
	assert.Equal(t, t0.Add(time.Hour), j.LastRun)
	assert.Equal(t, t0, j.LastOK, "last success survives a failure")
	assert.Equal(t, "fetch failed", j.Message)
}

func TestRecordJob_SameRunTwiceLeavesOneRow(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	r := JobResult{Name: "noop", RunAt: t0, OK: true}
	require.NoError(t, s.RecordJob(ctx, r), "calling s.RecordJob")
	require.NoError(t, s.RecordJob(ctx, r), "calling s.RecordJob")
	jobs, err := s.Jobs(ctx)
	require.NoError(t, err, "calling s.Jobs")
	assert.Len(t, jobs, 1, "one row per job, however often it runs")
}

func TestRecordJob_CutsALongMessage(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	require.NoError(t, s.RecordJob(ctx, JobResult{Name: "noop", RunAt: t0, Message: strings.Repeat("x", 5000)}), "recording a long message")
	jobs, err := s.Jobs(ctx)
	require.NoError(t, err, "listing jobs")
	require.Len(t, jobs, 1)
	assert.Len(t, jobs[0].Message, maxMessage)
}

func TestRecordJob_CutsOnACharacterBoundary(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	// "é" is two bytes; 499 bytes of x then é puts the limit inside it.
	msg := strings.Repeat("x", maxMessage-1) + strings.Repeat("é", 10)
	require.NoError(t, s.RecordJob(ctx, JobResult{Name: "noop", RunAt: t0, Message: msg}), "recording a long message")
	jobs, err := s.Jobs(ctx)
	require.NoError(t, err, "listing jobs")
	require.Len(t, jobs, 1)
	assert.True(t, utf8.ValidString(jobs[0].Message), "the stored message is valid UTF-8")
	assert.Len(t, jobs[0].Message, maxMessage-1, "the half character is dropped")
}

func TestOpen_TwoHandlesShareTheFile(t *testing.T) {
	t.Parallel()
	// The server and a job pod open the same file (ADR 0003): what one
	// records, the other reads.
	path := filepath.Join(t.TempDir(), "eunomia.sqlite")
	ctx := context.Background()
	now := func() time.Time { return t0 }
	job, err := Open(ctx, path, now)
	require.NoError(t, err, "calling Open")
	defer func() { _ = job.Close() }()
	server, err := Open(ctx, path, now)
	require.NoError(t, err, "calling Open")
	defer func() { _ = server.Close() }()
	_, err = job.Migrate(ctx)
	require.NoError(t, err, "calling job.Migrate")
	_, err = server.Migrate(ctx)
	require.NoError(t, err, "calling server.Migrate")

	require.NoError(t, job.RecordJob(ctx, JobResult{Name: "noop", RunAt: t0, OK: true}), "calling job.RecordJob")
	jobs, err := server.Jobs(ctx)
	require.NoError(t, err, "calling server.Jobs")
	require.Len(t, jobs, 1, "the server sees the job's row")
}
