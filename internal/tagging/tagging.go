// Package tagging links a story to the members of Congress and the states
// it names (ADR 0008). It is the one place in Eunomia that matches people by
// name (go-conventions, Provenance), so its rules prefer a miss to a wrong
// link: exact, case-sensitive names on word boundaries, a title with a
// surname only when the surname is unambiguous, and the states whose names
// are also a city or a country only with words that make them the state.
package tagging

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tcamick/eunomia/internal/geo"
	"github.com/tcamick/eunomia/internal/store"
)

// Mention kinds.
const (
	KindPerson = "person"
	KindState  = "state"
)

// titles are the words before a surname that say which chamber a member
// sits in. "Speaker" and "Leader" are not here: a full name covers them.
var titles = map[string][]string{
	"senate": {"Sen.", "Senator"},
	"house":  {"Rep.", "Representative", "Congressman", "Congresswoman"},
}

// ambiguous are state names that are also a city or a country; they tag the
// state only as "<name> state" or "state of <name>".
var ambiguous = map[string]bool{"Washington": true, "Georgia": true, "New York": true}

type place struct {
	name, code string
}

// Matcher tags text with the members and states it names.
type Matcher struct {
	fullNames map[string]string              // a full name → the one member it names
	nameOrder []string                       // fullNames' keys, longest first, then alphabetical
	surnames  map[string]map[string][]string // body → surname → members
	stateOf   map[string]string              // member → state code
	places    []place                        // longest name first
	masks     []string                       // others' names: blanked, never tagged; longest first
}

// New builds a matcher from the members sitting on a story's day. Everyone
// else in people (others may include the members; they are skipped) is a
// name to blank out before states and titles are looked for, so "Virginia
// Foxx" never tags Virginia when the member is not sitting, but tags no
// one. A full name a member shares with anyone else, sitting or not, names
// no one: a miss before a wrong link (ADR 0008).
func New(members, others []store.NamedMember) *Matcher {
	m := &Matcher{
		fullNames: map[string]string{},
		surnames:  map[string]map[string][]string{},
		stateOf:   map[string]string{},
	}
	shared := map[string]bool{}
	sitting := map[string]bool{}
	for _, p := range members {
		sitting[p.ID] = true
		m.stateOf[p.ID] = p.State
		for _, name := range fullNames(p) {
			if other, ok := m.fullNames[name]; ok && other != p.ID {
				shared[name] = true
			}
			m.fullNames[name] = p.ID
		}
		if m.surnames[p.Body] == nil {
			m.surnames[p.Body] = map[string][]string{}
		}
		if p.Last != "" {
			m.surnames[p.Body][p.Last] = append(m.surnames[p.Body][p.Last], p.ID)
		}
	}
	masks := map[string]bool{}
	for _, p := range others {
		if sitting[p.ID] {
			continue
		}
		for _, name := range fullNames(p) {
			masks[name] = true
			if _, ok := m.fullNames[name]; ok {
				shared[name] = true
			}
		}
	}
	for name := range shared {
		delete(m.fullNames, name)
		masks[name] = true
	}
	m.nameOrder = longestFirst(m.fullNames)
	m.masks = longestFirst(masks)
	for _, j := range geo.All() {
		m.places = append(m.places, place{name: j.Name, code: j.Code})
	}
	sort.Slice(m.places, func(a, b int) bool { return len(m.places[a].name) > len(m.places[b].name) })
	return m
}

// longestFirst returns a map's keys, longest first, then alphabetical, so
// matching is deterministic and a longer name wins over one inside it.
func longestFirst[V any](set map[string]V) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool {
		if len(out[a]) != len(out[b]) {
			return len(out[a]) > len(out[b])
		}
		return out[a] < out[b]
	})
	return out
}

// fullNames are the forms of a member's name a story might use: the
// display name, first and last, nickname and last. One-word names are not
// used: a lone word is too easily something else.
func fullNames(p store.NamedMember) []string {
	var out []string
	for _, n := range []string{p.Display, p.First + " " + p.Last, p.Nickname + " " + p.Last} {
		n = strings.TrimSpace(n)
		if strings.Contains(n, " ") && !strings.HasPrefix(n, " ") {
			out = append(out, n)
		}
	}
	return out
}

// Tag returns the members and states text names, people first, each once.
func (m *Matcher) Tag(text string) []store.Mention {
	found := map[store.Mention]bool{}
	// Members' names first, blanked out once found, so a state's name
	// inside one ("Virginia Foxx") is not taken for the state.
	masked := text
	for _, name := range m.nameOrder {
		id := m.fullNames[name]
		for _, at := range wordIndexes(masked, name) {
			found[store.Mention{Kind: KindPerson, Ref: id}] = true
			masked = masked[:at] + strings.Repeat(" ", len(name)) + masked[at+len(name):]
		}
	}
	for _, name := range m.masks {
		for _, at := range wordIndexes(masked, name) {
			masked = masked[:at] + strings.Repeat(" ", len(name)) + masked[at+len(name):]
		}
	}
	states := m.tagStates(masked)
	for code := range states {
		found[store.Mention{Kind: KindState, Ref: code}] = true
	}
	// Those named in full, before any title is read, so the order titles
	// are looked at in never matters.
	named := map[string]bool{}
	for k := range found {
		if k.Kind == KindPerson {
			named[k.Ref] = true
		}
	}
	for body, words := range titles {
		for surname, ids := range m.surnames[body] {
			for _, after := range titleUses(masked, words, surname) {
				if id, ok := m.pick(ids, after, named); ok {
					found[store.Mention{Kind: KindPerson, Ref: id}] = true
				}
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	out := make([]store.Mention, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Kind != out[b].Kind {
			return out[a].Kind < out[b].Kind // person before state
		}
		return out[a].Ref < out[b].Ref
	})
	return out
}

// pick chooses the member a title and surname mean. One member with the
// surname: that one. Several: the one whose state follows the name ("Sen.
// Scott of Florida"), or else the one the text names in full ("Tim Scott
// … Sen. Scott"); otherwise no one. A state named elsewhere in the text
// does not choose: "Sen. Scott (R-S.C.) criticized Florida's governor" is
// not about Florida's senator (ADR 0008).
func (m *Matcher) pick(ids []string, after string, named map[string]bool) (string, bool) {
	if len(ids) == 1 {
		return ids[0], true
	}
	if rest, ok := strings.CutPrefix(after, " of "); ok {
		for _, p := range m.places {
			if !startsWord(rest, p.name) {
				continue
			}
			var match []string
			for _, id := range ids {
				if m.stateOf[id] == p.code {
					match = append(match, id)
				}
			}
			if len(match) == 1 {
				return match[0], true
			}
			return "", false // a state is given and settles nothing: no guess
		}
	}
	var match []string
	for _, id := range ids {
		if named[id] {
			match = append(match, id)
		}
	}
	if len(match) == 1 {
		return match[0], true
	}
	return "", false
}

// tagStates finds the states text names, longest name first, so "West
// Virginia" is not also "Virginia".
func (m *Matcher) tagStates(text string) map[string]bool {
	out := map[string]bool{}
	masked := text
	for _, p := range m.places {
		for _, at := range wordIndexes(masked, p.name) {
			end := at + len(p.name)
			rest := masked[end:]
			if strings.HasPrefix(rest, " City") || strings.HasPrefix(rest, " River") {
				continue // Kansas City, the Mississippi River: a city or a river
			}
			if ambiguous[p.name] && !stateContext(masked[:at], rest) {
				continue
			}
			out[p.code] = true
			masked = masked[:at] + strings.Repeat(" ", len(p.name)) + masked[end:]
		}
	}
	return out
}

// stateContext reports whether the words around an ambiguous name make it
// the state: "state of Georgia", "Washington state", "New York State".
func stateContext(before, after string) bool {
	b := strings.ToLower(strings.TrimRightFunc(before, unicode.IsSpace))
	// "the state of Georgia" is the state; "the head of state of Georgia"
	// and "the secretary of state of Georgia" may be the country's.
	if strings.HasSuffix(b, "state of") && !strings.HasSuffix(b, "of state of") {
		return true
	}
	a := strings.TrimLeftFunc(after, unicode.IsSpace)
	return len(a) < len(after) && (startsWord(a, "state") || startsWord(a, "State"))
}

func startsWord(s, word string) bool {
	if !strings.HasPrefix(s, word) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[len(word):])
	return !isWordRune(r)
}

// titleUses returns the text after each place text has one of the titles,
// then the surname as a whole word: "Sen. Scott", "Senator Scott's". A title that is not a
// sitting member's ("state Sen. Scott", "former Rep. Scott", "ex-Sen.
// Scott") does not count, nor does a surname followed by another name
// ("Rep. Grant Smith": Grant is a first name there). A miss before a wrong
// link (ADR 0008).
func titleUses(text string, words []string, surname string) []string {
	var out []string
	for _, w := range words {
		needle := w + " " + surname
		for _, at := range wordIndexes(text, needle) {
			after := text[at+len(needle):]
			if !notSitting(text[:at]) && !nameFollows(after) {
				out = append(out, after)
			}
		}
	}
	return out
}

// nameFollows reports whether after starts with a space and a capitalised
// word: another part of a name. "Rep. Johnson, R-La." and "Sen. Sanders
// (I-Vt.)" are not followed by a name; "Rep. Johnson Smith" is.
func nameFollows(after string) bool {
	rest, ok := strings.CutPrefix(after, " ")
	if !ok {
		return false
	}
	first, n := utf8.DecodeRuneInString(rest)
	second, _ := utf8.DecodeRuneInString(rest[n:])
	return unicode.IsUpper(first) && unicode.IsLower(second)
}

// notSitting reports whether the text just before a title makes it someone
// other than a sitting member of Congress.
func notSitting(before string) bool {
	lower := strings.ToLower(before)
	if strings.HasSuffix(lower, "ex-") || strings.HasSuffix(lower, "then-") {
		return true
	}
	fields := strings.Fields(lower)
	// "former U.S. Sen. X": look past the country to the word before it.
	if n := len(fields); n > 0 && (fields[n-1] == "u.s." || fields[n-1] == "u.s") {
		fields = fields[:n-1]
	}
	if len(fields) == 0 {
		return false
	}
	switch fields[len(fields)-1] {
	case "state", "former", "late", "onetime", "retired":
		return true
	}
	return false
}

// wordIndexes are the byte offsets where needle occurs in text as whole
// words. Unicode-aware, so "Velázquez" and "Scotty" are judged right.
func wordIndexes(text, needle string) []int {
	var out []int
	for from := 0; from <= len(text)-len(needle); {
		i := strings.Index(text[from:], needle)
		if i < 0 {
			break
		}
		at := from + i
		before, _ := utf8.DecodeLastRuneInString(text[:at])
		after, _ := utf8.DecodeRuneInString(text[at+len(needle):])
		if (at == 0 || !isWordRune(before)) && !isWordRune(after) {
			out = append(out, at)
		}
		from = at + 1
	}
	return out
}

func isWordRune(r rune) bool {
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}
