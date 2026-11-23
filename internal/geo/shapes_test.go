package geo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manifest is what `make geo` wrote beside the shapes it generated
// (scripts/geo/generate.mjs; docs/adr/0006).
type manifest struct {
	Congress  int              `json:"congress"`
	States    []string         `json:"states"`
	Districts map[string][]int `json:"districts"`
}

func readManifest(t *testing.T) manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "lib", "geo", "manifest.json"))
	require.NoError(t, err, "reading the generated shapes' manifest")
	var m manifest
	require.NoError(t, json.Unmarshal(b, &m), "decoding the manifest")
	return m
}

func TestShapes_EveryStateAndDCHasAShapeAndNothingElseDoes(t *testing.T) {
	t.Parallel()
	// The territories are drawn as labelled boxes, not shapes (plan, E1b).
	var want []string
	for _, j := range All() {
		if j.Kind == KindState || j.Kind == KindDistrict {
			want = append(want, j.Code)
		}
	}
	slices.Sort(want)
	got := slices.Sorted(slices.Values(readManifest(t).States))
	assert.Equal(t, want, got, "the map's shapes are the 50 states and DC")
}

func TestShapes_EveryApportionedDistrictHasAShapeAndTheReverse(t *testing.T) {
	t.Parallel()
	// The roster's House seats are the 2020 apportionment's (HouseDistricts),
	// so this covers every district the roster can list.
	m := readManifest(t)
	assert.Equal(t, DistrictShapesCongress, m.Congress, "the shapes are the Congress the API labels them with (roadmap, gate G2)")
	states := map[string]bool{}
	for _, j := range All() {
		if j.Kind != KindState {
			continue
		}
		states[j.Code] = true
		assert.Equal(t, HouseDistricts(j), m.Districts[j.Code], "%s's district shapes match its apportioned seats", j.Code)
	}
	for code := range m.Districts {
		assert.True(t, states[code], "district shapes for %s, which has no apportioned seats", code)
	}
}
