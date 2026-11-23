package legislators

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tcamick/eunomia/internal/fetch"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err, "reading fixture %s", name)
	return b
}

func TestParseLegislators_ReadsTheFixture(t *testing.T) {
	t.Parallel()
	ls, err := ParseLegislators(readFixture(t, "legislators-current.json"), 1)
	require.NoError(t, err)
	require.Len(t, ls, 15)

	byID := map[string]Legislator{}
	for _, l := range ls {
		byID[l.ID.Bioguide] = l
	}
	sanders := byID["S000033"]
	assert.Equal(t, "Independent", sanders.Current().Party)
	assert.Equal(t, "Democrat", sanders.Current().Caucus, "the caucus is kept beside the party")
	require.NotNil(t, sanders.Current().Class)

	kiley := byID["K000401"]
	require.Len(t, kiley.Current().PartyAffiliations, 2, "a mid-term switch keeps both spans")
	assert.Equal(t, "Independent", kiley.Current().Party)

	norton := byID["N000147"]
	require.NotNil(t, norton.Current().District)
	assert.Equal(t, 0, *norton.Current().District, "a delegate's seat is at large")
	assert.Equal(t, "DC", norton.Current().State)

	assert.NotEmpty(t, byID["T000250"].Leadership)
}

func TestParseLegislators_RejectsAShortFile(t *testing.T) {
	t.Parallel()
	_, err := ParseLegislators(readFixture(t, "legislators-current.json"), MinMembers)
	require.ErrorIs(t, err, ErrTooFew, "15 members is far short of Congress")
}

func TestParseLegislators_Validation(t *testing.T) {
	t.Parallel()
	good := `{"id":{"bioguide":"A000001"},"name":{"first":"Ann","last":"Example"},"bio":{},
	  "terms":[{"type":"rep","start":"2025-01-03","end":"2027-01-03","state":"CA","district":12,"party":"Democrat","url":"https://example.house.gov"}]}`
	tests := map[string]struct {
		json    string
		wantErr string
	}{
		"a good member":               {json: good},
		"a malformed bioguide id":     {json: strings.Replace(good, "A000001", "a1", 1), wantErr: "bioguide"},
		"a term with no district":     {json: strings.Replace(good, `"district":12,`, "", 1), wantErr: "district"},
		"a senate term with no class": {json: strings.Replace(good, `"type":"rep"`, `"type":"sen"`, 1), wantErr: "class"},
		"an unknown term type":        {json: strings.Replace(good, `"type":"rep"`, `"type":"gov"`, 1), wantErr: "type"},
		"a lowercase state":           {json: strings.Replace(good, `"CA"`, `"ca"`, 1), wantErr: "state"},
		"an end before the start":     {json: strings.Replace(good, `"end":"2027-01-03"`, `"end":"2020-01-03"`, 1), wantErr: "dates"},
		"an impossible date":          {json: strings.Replace(good, `"start":"2025-01-03"`, `"start":"2025-02-30"`, 1), wantErr: "dates"},
		"no terms":                    {json: `{"id":{"bioguide":"A000001"},"name":{"first":"Ann","last":"Example"},"terms":[]}`, wantErr: "no terms"},
		"no name":                     {json: strings.Replace(good, `"last":"Example"`, `"last":""`, 1), wantErr: "no name"},
		"an overlong field":           {json: strings.Replace(good, `"Ann"`, `"`+strings.Repeat("A", 600)+`"`, 1), wantErr: "too long"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseLegislators([]byte("["+tc.json+"]"), 1)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestParseLegislators_TheSameMemberTwiceIsRefused(t *testing.T) {
	t.Parallel()
	one := `{"id":{"bioguide":"A000001"},"name":{"first":"Ann","last":"Example"},
	  "terms":[{"type":"rep","start":"2025-01-03","end":"2027-01-03","state":"CA","district":12}]}`
	_, err := ParseLegislators([]byte("["+one+","+one+"]"), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "twice")
}

func TestParseLegislators_KeepsOnlyHTTPSLinksAndUnknownBirthdays(t *testing.T) {
	t.Parallel()
	body := `[{"id":{"bioguide":"A000001"},"name":{"first":"Ann","last":"Example"},"bio":{"birthday":"1970-13-45"},
	  "terms":[{"type":"rep","start":"2025-01-03","end":"2027-01-03","state":"CA","district":1,"url":"javascript:alert(1)"},
	           {"type":"rep","start":"2023-01-03","end":"2025-01-03","state":"CA","district":1,"url":"http://example.house.gov"}]}]`
	ls, err := ParseLegislators([]byte(body), 1)
	require.NoError(t, err)
	assert.Empty(t, ls[0].Terms[0].URL, "a javascript: link is dropped")
	assert.Empty(t, ls[0].Terms[1].URL, "an http link is dropped")
	assert.Empty(t, ls[0].Bio.Birthday, "an impossible birthday is unknown, not an error")
}

func TestName_Display(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Bernard Sanders", Name{First: "Bernard", Last: "Sanders", OfficialFull: "Bernard Sanders"}.Display())
	assert.Equal(t, "Bob Example Jr.", Name{First: "Robert", Nickname: "Bob", Last: "Example", Suffix: "Jr."}.Display())
}

func TestParseCommittees_SubcommitteesTakeTheirParentsPrefix(t *testing.T) {
	t.Parallel()
	cs, err := ParseCommittees(readFixture(t, "committees-current.json"))
	require.NoError(t, err)
	byID := map[string]Committee{}
	for _, c := range cs {
		byID[c.ID] = c
	}
	require.Contains(t, byID, "HSED")
	assert.Equal(t, "house", byID["HSED"].Chamber)
	assert.Empty(t, byID["HSED"].Parent)
	var sub *Committee
	for _, c := range cs {
		if c.Parent == "HSED" {
			sub = &c
			break
		}
	}
	require.NotNil(t, sub, "the fixture has HSED's subcommittees")
	assert.True(t, strings.HasPrefix(sub.ID, "HSED"))
	assert.Equal(t, "house", sub.Chamber)
}

func TestParseMembership_ReadsSidesAndTitles(t *testing.T) {
	t.Parallel()
	m, err := ParseMembership(readFixture(t, "committee-membership-current.json"))
	require.NoError(t, err)
	require.NotEmpty(t, m["HSED"])
	for _, x := range m["HSED"] {
		assert.Contains(t, []string{"majority", "minority"}, x.Side)
	}
	_, err = ParseMembership([]byte(`{"HSED":[{"bioguide":"A000001","party":"left"}]}`))
	require.Error(t, err, "a side that is neither majority nor minority is refused")
}

// fakeGetter serves fixed bodies per source and records what it was asked.
type fakeGetter struct {
	bodies   map[string][]byte
	notMod   map[string]bool // answer 304 when validators were sent
	requests []string
}

func (f *fakeGetter) Get(_ context.Context, source, _ string, prev fetch.Validators) (*fetch.Response, error) {
	f.requests = append(f.requests, source+" etag="+prev.ETag)
	if f.notMod[source] && prev.ETag != "" {
		return &fetch.Response{Status: 304, NotModified: true, Validators: prev}, nil
	}
	return &fetch.Response{Status: 200, Body: f.bodies[source], Validators: fetch.Validators{ETag: `"` + source + `"`}}, nil
}

func fixtures(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		SourceLegislators: readFixture(t, "legislators-current.json"),
		SourceCommittees:  readFixture(t, "committees-current.json"),
		SourceMembership:  readFixture(t, "committee-membership-current.json"),
	}
}

func TestClient_FirstFetchReadsAllThree(t *testing.T) {
	t.Parallel()
	g := &fakeGetter{bodies: fixtures(t)}
	snap, next, unchanged, err := NewClient(g, "https://example.org/base", 1).Fetch(context.Background(), Validators{})
	require.NoError(t, err)
	assert.False(t, unchanged)
	assert.Len(t, snap.Legislators, 15)
	assert.NotEmpty(t, snap.Committees)
	assert.NotEmpty(t, snap.Membership)
	assert.Equal(t, `"legislators"`, next.Members.ETag)
	assert.Len(t, g.requests, 3)
}

func TestClient_NothingChangedCostsThreeConditionalRequests(t *testing.T) {
	t.Parallel()
	g := &fakeGetter{bodies: fixtures(t), notMod: map[string]bool{SourceLegislators: true, SourceCommittees: true, SourceMembership: true}}
	prev := Validators{
		Members:    fetch.Validators{ETag: `"legislators"`},
		Committees: fetch.Validators{ETag: `"committees"`},
		Membership: fetch.Validators{ETag: `"committee-membership"`},
	}
	snap, next, unchanged, err := NewClient(g, "https://example.org/", 1).Fetch(context.Background(), prev)
	require.NoError(t, err)
	assert.True(t, unchanged)
	assert.Nil(t, snap)
	assert.Equal(t, prev, next)
	assert.Len(t, g.requests, 3, "no body was downloaded")
}

func TestClient_OneChangedFileRefetchesTheOthersInFull(t *testing.T) {
	t.Parallel()
	g := &fakeGetter{bodies: fixtures(t), notMod: map[string]bool{SourceCommittees: true, SourceMembership: true}}
	prev := Validators{
		Members:    fetch.Validators{ETag: `"old"`},
		Committees: fetch.Validators{ETag: `"committees"`},
		Membership: fetch.Validators{ETag: `"committee-membership"`},
	}
	snap, _, unchanged, err := NewClient(g, "https://example.org/", 1).Fetch(context.Background(), prev)
	require.NoError(t, err)
	assert.False(t, unchanged)
	require.NotNil(t, snap)
	assert.NotEmpty(t, snap.Committees, "the unchanged files were read again so the snapshot is whole")
	assert.Equal(t, []string{
		`legislators etag="old"`, `committees etag="committees"`, `committee-membership etag="committee-membership"`,
		"committees etag=", "committee-membership etag=",
	}, g.requests)
}
