package news

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/config"
)

func TestLoadFeeds_TheCommittedListIsValid(t *testing.T) {
	t.Parallel()
	feeds, err := LoadFeeds(config.FS)
	require.NoError(t, err, "loading config/sources.yaml")
	ids := make([]string, 0, len(feeds))
	for _, f := range feeds {
		ids = append(ids, f.ID)
	}
	assert.Equal(t, []string{"npr-politics", "npr-news", "democracy-now"}, ids,
		"the owner's three sources (CalMatters and The Oaklandside wait on their terms)")
}

func TestLoadFeeds_RefusesABadListAndNamesEveryProblem(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		yaml string
		want []string
	}{
		"an unknown field": {
			yaml: "feeds:\n  - id: a1\n    colour: red\n",
			want: []string{"field colour not found"},
		},
		"every field wrong at once": {
			yaml: "feeds:\n  - id: Bad_ID\n    name: ''\n    url: http://example.org/rss\n    site: 'not a host!'\n" +
				"    label: ''\n    source: ftp://x\n    checked: soon\n",
			want: []string{"id must be", "name must be", "label must be", "url must be an https", "source must be an https",
				"site must be a host", "checked must be a date"},
		},
		"an id twice": {
			yaml: "feeds:\n" +
				"  - {id: aa, name: A, url: 'https://a.example/rss', site: a.example, label: L, source: 'https://a.example', checked: 2026-11-23}\n" +
				"  - {id: aa, name: B, url: 'https://b.example/rss', site: b.example, label: L, source: 'https://b.example', checked: 2026-11-23}\n",
			want: []string{"listed twice"},
		},
		"no feeds": {yaml: "feeds: []\n", want: []string{"no feeds"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadFeeds(fstest.MapFS{"sources.yaml": {Data: []byte(tc.yaml)}})
			require.Error(t, err, "loading a bad list")
			for _, w := range tc.want {
				assert.Contains(t, err.Error(), w, "the error names the problem")
			}
		})
	}
}
