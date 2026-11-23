package store_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/httpapi"
	"github.com/tcamick/eunomia/internal/store"
)

// tablesDigest hashes every row of every table in the file, read through a
// connection of its own.
func tablesDigest(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	require.NoError(t, err, "opening the file read-only")
	defer func() { _ = db.Close() }()
	var tables []string
	rows, err := db.QueryContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	require.NoError(t, err, "listing the tables")
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n), "reading a table name")
		tables = append(tables, n)
	}
	require.NoError(t, rows.Close(), "closing the table list")
	h := sha256.New()
	for _, table := range tables {
		// The names come from sqlite_master, not from outside text.
		rs, err := db.QueryContext(context.Background(), `SELECT * FROM "`+table+`" ORDER BY 1`) //nolint:gosec // see above
		require.NoError(t, err, "reading %s", table)
		cols, err := rs.Columns()
		require.NoError(t, err, "reading %s's columns", table)
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rs.Next() {
			require.NoError(t, rs.Scan(ptrs...), "reading a row of %s", table)
			_, _ = fmt.Fprintf(h, "%s %v\n", table, vals)
		}
		require.NoError(t, rs.Close(), "closing %s", table)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestServer_ServingPagesWritesNothing(t *testing.T) {
	t.Parallel()
	// go-conventions, Layout: the server writes only the owner's visits and
	// reading marks (and migrations at startup). E1a has neither, so serving
	// every page must leave every table as it was.
	path := filepath.Join(t.TempDir(), "eunomia.sqlite")
	ctx := context.Background()
	t0 := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	st, err := store.Open(ctx, path, func() time.Time { return t0 })
	require.NoError(t, err, "opening the store")
	defer func() { _ = st.Close() }()
	_, err = st.Migrate(ctx)
	require.NoError(t, err, "migrating")
	district := 3
	require.NoError(t, st.ApplyRoster(ctx, store.RosterPlan{At: t0, Source: "https://example.org/",
		People: []store.Person{{ID: "A000001", Bioguide: "A000001", First: "Ann", Last: "Able", Display: "Ann Able", InCongress: true}},
		Terms: []store.Term{{Person: "A000001", Office: "house/OH/3", Body: "house", State: "OH", District: &district,
			Start: "2025-01-03", End: "2027-01-03", Party: "Democratic", Current: true}},
		Committees: []store.Committee{{ID: "HSXX", Chamber: "house", Name: "Examples", Current: true}},
		Seats:      []store.Seat{{Committee: "HSXX", Person: "A000001", Side: "majority", Rank: 1, Title: "Chair"}},
	}), "seeding the roster")
	require.NoError(t, st.RecordJob(ctx, store.JobResult{Name: "roster", RunAt: t0, OK: true}), "seeding a job run")

	h, err := httpapi.New(httpapi.Options{Store: st, Roster: st, UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Version: "v-test", Now: func() time.Time { return t0 }, Log: zerolog.Nop(), Registerer: prometheus.NewRegistry(),
		PortraitDir: t.TempDir(), Jobs: []string{"roster"}})
	require.NoError(t, err, "building the server")

	before := tablesDigest(t, path)
	for _, p := range []string{"/api/status", "/api/congress", "/api/members", "/api/people/A000001", "/api/committees",
		"/api/committees/HSXX", "/api/search?q=able", "/api/people/Z999999", "/portraits/A000001.jpg", "/", "/members"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, p, nil))
		assert.Less(t, rec.Code, http.StatusInternalServerError, "GET %s", p)
	}
	assert.Equal(t, before, tablesDigest(t, path), "no table changed while serving pages")
}
