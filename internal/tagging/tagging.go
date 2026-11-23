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
}

// New builds a matcher from the sitting members. A full name two members
// share names neither.
func New(members []store.NamedMember) *Matcher {
	m := &Matcher{
		fullNames: map[string]string{},
		surnames:  map[string]map[string][]string{},
		stateOf:   map[string]string{},
	}
	shared := map[string]bool{}
	for _, p := range members {
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
	for name := range shared {
		delete(m.fullNames, name)
	}
	for name := range m.fullNames {
		m.nameOrder = append(m.nameOrder, name)
	}
	sort.Slice(m.nameOrder, func(a, b int) bool {
		if len(m.nameOrder[a]) != len(m.nameOrder[b]) {
			return len(m.nameOrder[a]) > len(m.nameOrder[b])
		}
		return m.nameOrder[a] < m.nameOrder[b]
	})
	for _, j := range geo.All() {
		m.places = append(m.places, place{name: j.Name, code: j.Code})
	}
	sort.Slice(m.places, func(a, b int) bool { return len(m.places[a].name) > len(m.places[b].name) })
	return m
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
	states := m.tagStates(masked)
	for code := range states {
		found[store.Mention{Kind: KindState, Ref: code}] = true
	}
	for body, words := range titles {
		for surname, ids := range m.surnames[body] {
			if !afterTitle(text, words, surname) {
				continue
			}
			if id, ok := m.pick(ids, states); ok {
				found[store.Mention{Kind: KindPerson, Ref: id}] = true
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

// pick chooses the member a title and surname name: the only one in the
// chamber with that surname, or the only one whose state the text names.
func (m *Matcher) pick(ids []string, states map[string]bool) (string, bool) {
	if len(ids) == 1 {
		return ids[0], true
	}
	var match []string
	for _, id := range ids {
		if states[m.stateOf[id]] {
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
			if strings.HasPrefix(rest, " City") {
				continue // Kansas City, New York City, Oklahoma City: a city
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
	if strings.HasSuffix(b, "state of") {
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

// afterTitle reports whether text has one of the titles, then the surname
// as a whole word: "Sen. Scott", "Senator Scott's".
func afterTitle(text string, words []string, surname string) bool {
	for _, w := range words {
		if containsWord(text, w+" "+surname) {
			return true
		}
	}
	return false
}

// containsWord reports whether text has needle with no letter or digit
// right before or after it.
func containsWord(text, needle string) bool {
	return len(wordIndexes(text, needle)) > 0
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
