package portraits

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/fetch"
	"github.com/tcamick/eunomia/internal/store"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	mu      sync.Mutex
	due     []store.PortraitState
	records []store.PortraitState
}

func (f *fakeStore) PortraitsDue(context.Context, time.Time, int) ([]store.PortraitState, error) {
	return f.due, nil
}

func (f *fakeStore) RecordPortrait(_ context.Context, ps store.PortraitState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, ps)
	return nil
}

// fakeGetter answers by URL.
type fakeGetter struct {
	answers map[string]func(fetch.Validators) (*fetch.Response, error)
	asked   []string
}

func (f *fakeGetter) Get(_ context.Context, _, url string, prev fetch.Validators) (*fetch.Response, error) {
	f.asked = append(f.asked, url)
	if a, ok := f.answers[url]; ok {
		return a(prev)
	}
	return nil, &fetch.StatusError{Code: 404}
}

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("rest of a jpeg")...)

func TestRun_WritesUnchangedAndMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := &fakeStore{due: []store.PortraitState{
		{Person: "A000001"},
		{Person: "B000002", File: "B000002.jpg", ETag: `"b"`},
		{Person: "C000003"},
	}}
	g := &fakeGetter{answers: map[string]func(fetch.Validators) (*fetch.Response, error){
		"https://img/A000001.jpg": func(fetch.Validators) (*fetch.Response, error) {
			return &fetch.Response{Status: 200, Body: jpeg, Validators: fetch.Validators{ETag: `"a"`}}, nil
		},
		"https://img/B000002.jpg": func(prev fetch.Validators) (*fetch.Response, error) {
			if prev.ETag != `"b"` {
				return nil, errors.New("expected the stored ETag to be sent")
			}
			return &fetch.Response{Status: 304, NotModified: true, Validators: prev}, nil
		},
	}}
	j := NewJob(g, st, "https://img/", dir, func() time.Time { return t0 })
	require.NoError(t, j.Run(context.Background()), "running the job")

	got, err := os.ReadFile(filepath.Join(dir, "A000001.jpg"))
	require.NoError(t, err, "reading the written portrait")
	assert.Equal(t, jpeg, got)
	require.Len(t, st.records, 3, "one record per member checked")
	assert.Equal(t, store.PortraitState{Person: "A000001", File: "A000001.jpg", ETag: `"a"`, CheckedAt: t0,
		Source: "https://img/A000001.jpg", FetchedAt: t0}, st.records[0])
	assert.Equal(t, "B000002.jpg", st.records[1].File, "an unchanged portrait keeps its file")
	assert.True(t, st.records[1].FetchedAt.IsZero(), "and its fetch date")
	assert.Empty(t, st.records[2].File, "a member with no portrait is recorded as having none")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "listing the portrait dir")
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestRun_RefusesWhatIsNotAJPEG(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := &fakeStore{due: []store.PortraitState{{Person: "A000001"}}}
	g := &fakeGetter{answers: map[string]func(fetch.Validators) (*fetch.Response, error){
		"https://img/A000001.jpg": func(fetch.Validators) (*fetch.Response, error) {
			return &fetch.Response{Status: 200, Body: []byte("<html>not an image</html>")}, nil
		},
	}}
	err := NewJob(g, st, "https://img/", dir, func() time.Time { return t0 }).Run(context.Background())
	require.Error(t, err, "an HTML page in place of an image is refused")
	_, statErr := os.Stat(filepath.Join(dir, "A000001.jpg"))
	assert.True(t, os.IsNotExist(statErr), "nothing is written")
	assert.Empty(t, st.records)
}

func TestRun_StopsAfterFiveFailuresInARow(t *testing.T) {
	t.Parallel()
	ids := []string{"A000001", "A000002", "A000003", "A000004", "A000005", "A000006", "A000007"}
	due := make([]store.PortraitState, 0, len(ids))
	for _, id := range ids {
		due = append(due, store.PortraitState{Person: id})
	}
	st := &fakeStore{due: due}
	down := func(fetch.Validators) (*fetch.Response, error) { return nil, &fetch.StatusError{Code: 503} }
	g := &fakeGetter{answers: map[string]func(fetch.Validators) (*fetch.Response, error){}}
	for _, ps := range due {
		g.answers["https://img/"+ps.Person+".jpg"] = down
	}
	err := NewJob(g, st, "https://img/", t.TempDir(), func() time.Time { return t0 }).Run(context.Background())
	require.Error(t, err, "a source that is down fails the run")
	assert.Len(t, g.asked, maxFailures, "the run gives up instead of asking for every portrait")
}

func TestRun_NothingDueAsksNothing(t *testing.T) {
	t.Parallel()
	g := &fakeGetter{}
	require.NoError(t, NewJob(g, &fakeStore{}, "https://img/", t.TempDir(), func() time.Time { return t0 }).Run(context.Background()),
		"running with nothing due")
	assert.Empty(t, g.asked)
}

func TestValidName_AcceptsOnlyABioguideIDDotJpg(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		name string
		want bool
	}{
		"a Bioguide id and .jpg":  {name: "A000001.jpg", want: true},
		"a path out of the dir":   {name: "../etc/passwd", want: false},
		"another file type":       {name: "A000001.png", want: false},
		"a lower-case id":         {name: "a000001.jpg", want: false},
		"a name with a path tail": {name: "A000001.jpg/..", want: false},
		"nothing":                 {name: "", want: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ValidName(tc.name), "ValidName(%q)", tc.name)
		})
	}
}

func TestRun_AWithdrawnPortraitIsRemovedAndDatedNow(t *testing.T) {
	t.Parallel()
	// The member had a portrait; the source now answers 404. The page must
	// not show the old photo, nor date "none published" by the old file.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A000001.jpg"), jpeg, 0o600), "writing the old portrait")
	st := &fakeStore{due: []store.PortraitState{{Person: "A000001", File: "A000001.jpg", ETag: `"a"`}}}
	j := NewJob(&fakeGetter{}, st, "https://img/", dir, func() time.Time { return t0 })
	require.NoError(t, j.Run(context.Background()), "running the job")

	require.Len(t, st.records, 1, "one record for the one member checked")
	assert.Empty(t, st.records[0].File, "the member now has no portrait")
	assert.Equal(t, t0, st.records[0].FetchedAt, "the change is dated now")
	_, err := os.Stat(filepath.Join(dir, "A000001.jpg"))
	assert.ErrorIs(t, err, fs.ErrNotExist, "the withdrawn file is gone")
}
