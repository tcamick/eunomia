// Package legislators reads the unitedstates/congress-legislators project's
// files: the members of Congress, the committees, and who sits on them. It
// parses and validates them at the boundary (docs/go-conventions.md,
// Security) and returns plain values; the roster package decides what they
// mean for the store.
package legislators

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tcamick/eunomia/internal/fetch"
)

// Source names, the values of the fetch metric's source label.
const (
	SourceLegislators = "legislators"
	SourceCommittees  = "committees"
	SourceMembership  = "committee-membership"
)

// Default URLs of the three files. Public domain (CC0).
const (
	DefaultBase     = "https://unitedstates.github.io/congress-legislators/"
	fileLegislators = "legislators-current.json"
	fileCommittees  = "committees-current.json"
	fileMembership  = "committee-membership-current.json"
)

// Name is how a member is named in the source.
type Name struct {
	First        string `json:"first"`
	Middle       string `json:"middle,omitempty"`
	Last         string `json:"last"`
	Suffix       string `json:"suffix,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	OfficialFull string `json:"official_full,omitempty"`
}

// Display is the name to show: the official full name, else first and last
// (with the nickname, if any, in place of the first name).
func (n Name) Display() string {
	if n.OfficialFull != "" {
		return n.OfficialFull
	}
	first := n.First
	if n.Nickname != "" {
		first = n.Nickname
	}
	parts := []string{first, n.Last}
	if n.Suffix != "" {
		parts = append(parts, n.Suffix)
	}
	return strings.Join(parts, " ")
}

// IDs are the member's identifiers in other datasets, the join keys.
type IDs struct {
	Bioguide    string   `json:"bioguide"`
	LIS         string   `json:"lis,omitempty"`
	FEC         []string `json:"fec,omitempty"`
	ICPSR       int      `json:"icpsr,omitempty"`
	GovTrack    int      `json:"govtrack,omitempty"`
	Wikidata    string   `json:"wikidata,omitempty"`
	Wikipedia   string   `json:"wikipedia,omitempty"`
	Ballotpedia string   `json:"ballotpedia,omitempty"`
}

// Bio is what the source records about the person.
type Bio struct {
	Birthday string `json:"birthday,omitempty"`
	Gender   string `json:"gender,omitempty"`
}

// PartyAffiliation is a span within one term during which the member sat
// with one party: a member who switches mid-term has more than one.
type PartyAffiliation struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Party  string `json:"party"`
	Caucus string `json:"caucus,omitempty"`
}

// Term is one term in one office.
type Term struct {
	Type      string `json:"type"` // "sen" or "rep"
	Start     string `json:"start"`
	End       string `json:"end"`
	State     string `json:"state"`
	District  *int   `json:"district,omitempty"`
	Class     *int   `json:"class,omitempty"`
	StateRank string `json:"state_rank,omitempty"`
	Party     string `json:"party,omitempty"`
	Caucus    string `json:"caucus,omitempty"`
	How       string `json:"how,omitempty"`
	EndType   string `json:"end-type,omitempty"`
	URL       string `json:"url,omitempty"`
	// PartyAffiliations is set only when the party changed during the term.
	PartyAffiliations []PartyAffiliation `json:"party_affiliations,omitempty"`
}

// LeadershipRole is a party or chamber leadership post.
type LeadershipRole struct {
	Title   string `json:"title"`
	Chamber string `json:"chamber"`
	Start   string `json:"start"`
	End     string `json:"end,omitempty"`
}

// Legislator is one member as the source lists them.
type Legislator struct {
	ID         IDs              `json:"id"`
	Name       Name             `json:"name"`
	Bio        Bio              `json:"bio"`
	Terms      []Term           `json:"terms"`
	Leadership []LeadershipRole `json:"leadership_roles,omitempty"`
}

// Current is the member's last term, which the source keeps current.
func (l Legislator) Current() Term { return l.Terms[len(l.Terms)-1] }

// Committee is a committee or, when Parent is set, a subcommittee. Its ID is
// the source's thomas_id, with the parent's prepended for a subcommittee, as
// the membership file keys them.
type Committee struct {
	ID           string
	Parent       string
	Chamber      string // house, senate or joint
	Name         string
	URL          string
	Jurisdiction string
}

// Member is one seat on a committee.
type Member struct {
	Bioguide string `json:"bioguide"`
	Name     string `json:"name"`
	// Side is majority or minority.
	Side  string `json:"party"`
	Rank  int    `json:"rank,omitempty"`
	Title string `json:"title,omitempty"`
}

// Snapshot is the three files, parsed and checked.
type Snapshot struct {
	Legislators []Legislator
	Committees  []Committee
	// Membership maps a committee ID to its members, in the source's order.
	Membership map[string][]Member
}

// MinMembers is the fewest current members a file may list and be stored.
// Congress has 541 seats; a file far short of that is broken or cut off,
// and storing it would end the terms of everyone missing (go-conventions,
// Fetching from public sources).
const MinMembers = 500

// ErrTooFew is a members file with fewer than MinMembers entries.
var ErrTooFew = errors.New("too few members")

var (
	reBioguide = regexp.MustCompile(`^[A-Z][0-9]{6}$`)
	reDate     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	reState    = regexp.MustCompile(`^[A-Z]{2}$`)
	reThomas   = regexp.MustCompile(`^[A-Z]{4}[0-9]{0,2}$`)
)

// maxText bounds any one string from the source; longer is a broken file.
const maxText = 500

// ParseLegislators parses and validates legislators-current.json. A member
// with a malformed field is an error: the file is the source's, and a
// partly-read roster is worse than yesterday's (the job keeps the old one).
func ParseLegislators(body []byte, minMembers int) ([]Legislator, error) {
	var ls []Legislator
	if err := json.Unmarshal(body, &ls); err != nil {
		return nil, fmt.Errorf("parse legislators: %w", err)
	}
	if len(ls) < minMembers {
		return nil, fmt.Errorf("%w: %d listed, at least %d expected", ErrTooFew, len(ls), minMembers)
	}
	seen := make(map[string]bool, len(ls))
	for i := range ls {
		if err := validateLegislator(&ls[i]); err != nil {
			return nil, fmt.Errorf("legislator %d (%s): %w", i, ls[i].ID.Bioguide, err)
		}
		if seen[ls[i].ID.Bioguide] {
			return nil, fmt.Errorf("legislator %s listed twice", ls[i].ID.Bioguide)
		}
		seen[ls[i].ID.Bioguide] = true
	}
	return ls, nil
}

func validateLegislator(l *Legislator) error {
	if !reBioguide.MatchString(l.ID.Bioguide) {
		return fmt.Errorf("bioguide id %q", l.ID.Bioguide)
	}
	if l.Name.Last == "" || (l.Name.First == "" && l.Name.OfficialFull == "") {
		return errors.New("no name")
	}
	for _, s := range []string{l.Name.First, l.Name.Middle, l.Name.Last, l.Name.Suffix, l.Name.Nickname, l.Name.OfficialFull,
		l.ID.LIS, l.ID.Wikidata, l.ID.Wikipedia, l.ID.Ballotpedia, l.Bio.Gender} {
		if len(s) > maxText {
			return errors.New("a field is too long")
		}
	}
	if l.Bio.Birthday != "" && !validDate(l.Bio.Birthday) {
		l.Bio.Birthday = "" // unknown, not an error: shown as unknown
	}
	if len(l.Terms) == 0 {
		return errors.New("no terms")
	}
	for i := range l.Terms {
		if err := validateTerm(&l.Terms[i]); err != nil {
			return fmt.Errorf("term %d: %w", i, err)
		}
	}
	for _, r := range l.Leadership {
		if r.Title == "" || len(r.Title) > maxText || !validDate(r.Start) || (r.End != "" && !validDate(r.End)) {
			return fmt.Errorf("leadership role %q", r.Title)
		}
		if r.Chamber != "house" && r.Chamber != "senate" {
			return fmt.Errorf("leadership chamber %q", r.Chamber)
		}
	}
	return nil
}

func validateTerm(t *Term) error {
	switch t.Type {
	case "sen":
		if t.Class == nil || *t.Class < 1 || *t.Class > 3 {
			return errors.New("a senate term without a class 1-3")
		}
	case "rep":
		if t.District == nil || *t.District < 0 || *t.District > 60 {
			return errors.New("a house term without a district")
		}
	default:
		return fmt.Errorf("type %q", t.Type)
	}
	if !reState.MatchString(t.State) {
		return fmt.Errorf("state %q", t.State)
	}
	if !validDate(t.Start) || !validDate(t.End) || t.End < t.Start {
		return fmt.Errorf("dates %q to %q", t.Start, t.End)
	}
	for _, s := range []string{t.Party, t.Caucus, t.How, t.EndType, t.StateRank} {
		if len(s) > maxText {
			return errors.New("a field is too long")
		}
	}
	t.URL = httpsOnly(t.URL)
	for _, a := range t.PartyAffiliations {
		if !validDate(a.Start) || !validDate(a.End) || a.Party == "" || len(a.Party) > maxText {
			return errors.New("a party affiliation")
		}
	}
	return nil
}

// ParseCommittees parses committees-current.json into committees and
// subcommittees.
func ParseCommittees(body []byte) ([]Committee, error) {
	var raw []struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		URL          string `json:"url"`
		ThomasID     string `json:"thomas_id"`
		Jurisdiction string `json:"jurisdiction"`
		Subs         []struct {
			Name     string `json:"name"`
			ThomasID string `json:"thomas_id"`
		} `json:"subcommittees"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse committees: %w", err)
	}
	var out []Committee
	for _, c := range raw {
		if !reThomas.MatchString(c.ThomasID) {
			return nil, fmt.Errorf("committee id %q", c.ThomasID)
		}
		if c.Type != "house" && c.Type != "senate" && c.Type != "joint" {
			return nil, fmt.Errorf("committee %s type %q", c.ThomasID, c.Type)
		}
		if c.Name == "" || len(c.Name) > maxText {
			return nil, fmt.Errorf("committee %s name", c.ThomasID)
		}
		juris := c.Jurisdiction
		if len(juris) > 4*maxText {
			juris = juris[:4*maxText]
		}
		out = append(out, Committee{ID: c.ThomasID, Chamber: c.Type, Name: c.Name, URL: httpsOnly(c.URL), Jurisdiction: juris})
		for _, s := range c.Subs {
			id := c.ThomasID + s.ThomasID
			if !reThomas.MatchString(id) || s.Name == "" || len(s.Name) > maxText {
				return nil, fmt.Errorf("subcommittee %q of %s", s.ThomasID, c.ThomasID)
			}
			out = append(out, Committee{ID: id, Parent: c.ThomasID, Chamber: c.Type, Name: s.Name})
		}
	}
	return out, nil
}

// ParseMembership parses committee-membership-current.json.
func ParseMembership(body []byte) (map[string][]Member, error) {
	var m map[string][]Member
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse committee membership: %w", err)
	}
	for id, ms := range m {
		if !reThomas.MatchString(id) {
			return nil, fmt.Errorf("membership committee id %q", id)
		}
		for i, x := range ms {
			if !reBioguide.MatchString(x.Bioguide) {
				return nil, fmt.Errorf("committee %s member %d bioguide %q", id, i, x.Bioguide)
			}
			if x.Side != "majority" && x.Side != "minority" {
				return nil, fmt.Errorf("committee %s member %s side %q", id, x.Bioguide, x.Side)
			}
			if len(x.Title) > maxText || len(x.Name) > maxText {
				return nil, fmt.Errorf("committee %s member %s: a field is too long", id, x.Bioguide)
			}
		}
	}
	return m, nil
}

func validDate(s string) bool {
	if !reDate.MatchString(s) {
		return false
	}
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// httpsOnly keeps a URL only if it parses and is https with a host
// (go-conventions, Security: URLs from outside).
func httpsOnly(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(s) > maxText {
		return ""
	}
	return s
}

// Getter is the part of fetch.Client the source needs.
type Getter interface {
	Get(ctx context.Context, source, rawURL string, prev fetch.Validators) (*fetch.Response, error)
}

// Validators are the conditional-request headers of the three files.
type Validators struct {
	Members    fetch.Validators
	Committees fetch.Validators
	Membership fetch.Validators
}

// Client fetches the three files.
type Client struct {
	get        Getter
	base       string
	minMembers int
}

// NewClient returns a client for the files under base (DefaultBase in
// production, a test server's URL in tests).
func NewClient(get Getter, base string, minMembers int) *Client {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return &Client{get: get, base: base, minMembers: minMembers}
}

// Fetch asks for the three files with prev's conditional headers. It returns
// nil and unchanged=true when none of them changed. When any changed, the
// ones that did not are fetched again in full, so the snapshot is whole.
func (c *Client) Fetch(ctx context.Context, prev Validators) (snap *Snapshot, next Validators, unchanged bool, err error) {
	type file struct {
		source, name string
		prev         fetch.Validators
		resp         *fetch.Response
	}
	files := []*file{
		{source: SourceLegislators, name: fileLegislators, prev: prev.Members},
		{source: SourceCommittees, name: fileCommittees, prev: prev.Committees},
		{source: SourceMembership, name: fileMembership, prev: prev.Membership},
	}
	anyChanged := false
	for _, f := range files {
		if f.resp, err = c.get.Get(ctx, f.source, c.base+f.name, f.prev); err != nil {
			return nil, prev, false, fmt.Errorf("fetch %s: %w", f.name, err)
		}
		if !f.resp.NotModified {
			anyChanged = true
		}
	}
	if !anyChanged {
		return nil, prev, true, nil
	}
	for _, f := range files {
		if f.resp.NotModified {
			if f.resp, err = c.get.Get(ctx, f.source, c.base+f.name, fetch.Validators{}); err != nil {
				return nil, prev, false, fmt.Errorf("fetch %s: %w", f.name, err)
			}
		}
	}
	snap = &Snapshot{}
	if snap.Legislators, err = ParseLegislators(files[0].resp.Body, c.minMembers); err != nil {
		return nil, prev, false, err
	}
	if snap.Committees, err = ParseCommittees(files[1].resp.Body); err != nil {
		return nil, prev, false, err
	}
	if snap.Membership, err = ParseMembership(files[2].resp.Body); err != nil {
		return nil, prev, false, err
	}
	next = Validators{Members: files[0].resp.Validators, Committees: files[1].resp.Validators, Membership: files[2].resp.Validators}
	return snap, next, false, nil
}
