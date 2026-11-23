package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tcamick/eunomia/internal/store"
)

// members is a small Congress with the traps the rules exist for: two
// senators named Scott, a senator and a representative named Johnson, a
// nickname, an accented name, and two representatives with one full name.
var members = []store.NamedMember{
	{ID: "S001217", First: "Richard", Nickname: "Rick", Last: "Scott", Display: "Rick Scott", Body: "senate", State: "FL"},
	{ID: "S001184", First: "Tim", Last: "Scott", Display: "Tim Scott", Body: "senate", State: "SC"},
	{ID: "S000033", First: "Bernard", Nickname: "Bernie", Last: "Sanders", Display: "Bernard Sanders", Body: "senate", State: "VT"},
	{ID: "J000293", First: "Ron", Last: "Johnson", Display: "Ron Johnson", Body: "senate", State: "WI"},
	{ID: "J000299", First: "Mike", Last: "Johnson", Display: "Mike Johnson", Body: "house", State: "LA"},
	{ID: "V000081", First: "Nydia", Last: "Velázquez", Display: "Nydia M. Velázquez", Body: "house", State: "NY"},
	{ID: "D000001", First: "Sam", Last: "Doe", Display: "Sam Doe", Body: "house", State: "OH"},
	{ID: "D000002", First: "Sam", Last: "Doe", Display: "Sam Doe", Body: "house", State: "TX"},
	{ID: "F000450", First: "Virginia", Last: "Foxx", Display: "Virginia Foxx", Body: "house", State: "NC"},
}

func person(id string) store.Mention  { return store.Mention{Kind: KindPerson, Ref: id} }
func state(code string) store.Mention { return store.Mention{Kind: KindState, Ref: code} }

func TestTag(t *testing.T) {
	t.Parallel()
	m := New(members, nil)
	tests := map[string]struct {
		text string
		want []store.Mention
	}{
		"a full name":                   {text: "Tim Scott spoke on Tuesday.", want: []store.Mention{person("S001184")}},
		"a nickname and surname":        {text: "Bernie Sanders said no.", want: []store.Mention{person("S000033")}},
		"the official full name":        {text: "Bernard Sanders voted.", want: []store.Mention{person("S000033")}},
		"an accented name":              {text: "Rep. Nydia Velázquez, of New York State, said.", want: []store.Mention{person("V000081"), state("NY")}},
		"a title and a unique surname":  {text: "Sen. Sanders's bill passed.", want: []store.Mention{person("S000033")}},
		"two senators with one surname": {text: "Sen. Scott said the vote would wait.", want: nil},
		"the surname and one's state":   {text: "Sen. Scott of Florida objected.", want: []store.Mention{person("S001217"), state("FL")}},
		"a shared surname after the full name": {
			text: "Tim Scott toured Florida on Monday. Sen. Scott said the plan works.", want: []store.Mention{person("S001184"), state("FL")},
		},
		"a shared surname and a state elsewhere": {
			text: "Sen. Scott (R-S.C.) criticized Florida's governor.", want: []store.Mention{state("FL")},
		},
		"a surname unique to its chamber":     {text: "Speaker Johnson and Senator Johnson met.", want: []store.Mention{person("J000293")}},
		"a House title with a senator's name": {text: "Rep. Sanders is not a member.", want: nil},
		"a state senator with a US senator's surname": {
			text: "State Sen. Sanders of the Vermont legislature spoke.", want: []store.Mention{state("VT")},
		},
		"a state representative": {text: "Lawmakers heard state Rep. Sanders.", want: nil},
		"a former senator":       {text: "Former Sen. Sanders spoke.", want: nil},
		"a title, then a first name that is a member's surname": {
			text: "Rep. Johnson Smith, a former aide, spoke.", want: nil,
		},
		"a title, then a first name that is a senator's surname": {text: "Sen. Sanders Ford of Michigan spoke.", want: []store.Mention{state("MI")}},
		"a title and surname, then the party and state":          {text: "Rep. Johnson, R-La., spoke.", want: []store.Mention{person("J000299")}},
		"a title and surname, then a parenthesis":                {text: "Sen. Sanders (I-Vt.) spoke.", want: []store.Mention{person("S000033")}},
		"an ex-senator":                  {text: "Ex-Sen. Sanders spoke.", want: nil},
		"two members with one full name": {text: "Sam Doe spoke.", want: nil},
		"a surname inside a longer word": {text: "Sen. Scottsdale is no one.", want: nil},
		"a name inside a longer name":    {text: "Tim Scotty is not a member.", want: nil},
		"Washington alone":               {text: "Washington is gridlocked.", want: nil},
		"Washington state":               {text: "Voters in Washington state approved it.", want: []store.Mention{state("WA")}},
		"the state of Georgia":           {text: "In the state of Georgia, polls close at 7.", want: []store.Mention{state("GA")}},
		"a head of state of Georgia":     {text: "The head of state of Georgia arrived.", want: nil},
		"Georgia alone":                  {text: "Georgia and Armenia signed a treaty.", want: nil},
		"New York City":                  {text: "New York City's mayor spoke.", want: nil},
		"West Virginia is not Virginia":  {text: "West Virginia voted.", want: []store.Mention{state("WV")}},
		"both Virginias":                 {text: "Virginia and West Virginia.", want: []store.Mention{state("VA"), state("WV")}},
		"Kansas City is not Kansas":      {text: "Kansas City won.", want: nil},
		"Arkansas is not Kansas":         {text: "Arkansas voted.", want: []store.Mention{state("AR")}},
		"a territory":                    {text: "Puerto Rico's delegate.", want: []store.Mention{state("PR")}},
		"a state's name inside a member's name is not the state": {
			text: "Rep. Virginia Foxx chaired it.", want: []store.Mention{person("F000450")}},
		"lower case is not a name": {text: "a sam doe and a florida orange", want: nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, m.Tag(tc.text), "tags for %q", tc.text)
		})
	}
}

// A former member's name is blanked out before states are looked for, so
// it never tags a state, and never tags the person: only sitting members
// are tagged (ADR 0008).
func TestTag_AFormerMembersNameTagsNoOne(t *testing.T) {
	t.Parallel()
	var sitting []store.NamedMember
	for _, p := range members {
		if p.ID != "F000450" {
			sitting = append(sitting, p)
		}
	}
	former := []store.NamedMember{{ID: "F000450", First: "Virginia", Last: "Foxx", Display: "Virginia Foxx"}}
	m := New(sitting, former)
	assert.Nil(t, m.Tag("Virginia Foxx retired."), "neither the person nor Virginia")
	assert.Equal(t, []store.Mention{state("VA")}, m.Tag("Virginia Foxx spoke in Virginia."), "the state, named on its own, still is")
}

// A full name a sitting member shares with someone who is not sitting names
// no one: the story may be about either.
func TestTag_ANameASittingMemberSharesWithAFormerOneTagsNoOne(t *testing.T) {
	t.Parallel()
	sitting := []store.NamedMember{{ID: "R000001", First: "Sam", Last: "Roe", Display: "Sam Roe", Body: "house", State: "OH"}}
	former := store.NamedMember{ID: "R000002", First: "Sam", Last: "Roe", Display: "Sam Roe"}
	m := New(sitting, []store.NamedMember{former, sitting[0]})
	assert.Nil(t, m.Tag("Sam Roe spoke."), "either Sam Roe")
	assert.Equal(t, []store.Mention{person("R000001")}, m.Tag("Rep. Roe spoke."), "the title still finds the one sitting Roe")
}
