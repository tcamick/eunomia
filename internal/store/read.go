package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Sitting is a member of Congress in their current seat, with what the pages
// need to list them.
type Sitting struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LastName  string `json:"lastName"`
	Body      string `json:"body"`
	State     string `json:"state"`
	District  *int   `json:"district,omitempty"`
	Class     *int   `json:"class,omitempty"`
	StateRank string `json:"stateRank,omitempty"`
	Party     string `json:"party,omitempty"`
	Caucus    string `json:"caucus,omitempty"`
	Start     string `json:"start"`
	End       string `json:"end"`
	How       string `json:"how,omitempty"`
	Portrait  bool   `json:"portrait"`
}

// Sittings lists every member in Congress in their current seat.
func (s *Store) Sittings(ctx context.Context) ([]Sitting, error) {
	var out []Sitting
	err := eachRow(ctx, s.db, `
		SELECT p.id, p.display_name, p.last_name, t.body, t.state, t.district, t.senate_class,
		       COALESCE(t.state_rank, ''), COALESCE(t.party, ''), COALESCE(t.caucus, ''), t.start, t.end,
		       COALESCE(t.how, ''), x.file IS NOT NULL
		FROM people p JOIN terms t ON t.person = p.id AND t.current = 1 AND t.unlisted_on IS NULL
		LEFT JOIN portraits x ON x.person = p.id
		WHERE p.in_congress = 1
		ORDER BY t.state, t.body DESC, t.district, p.last_name`, func(r *sql.Rows) error {
		var (
			m               Sitting
			district, class sql.NullInt64
		)
		if err := r.Scan(&m.ID, &m.Name, &m.LastName, &m.Body, &m.State, &district, &class, &m.StateRank, &m.Party,
			&m.Caucus, &m.Start, &m.End, &m.How, &m.Portrait); err != nil {
			return err
		}
		m.District, m.Class = intPtr(district), intPtr(class)
		out = append(out, m)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list sitting members: %w", err)
	}
	return out, nil
}

// Role is a leadership post held now or in the past.
type Role struct {
	Person  string `json:"person"`
	Name    string `json:"name"`
	Party   string `json:"party,omitempty"`
	Chamber string `json:"chamber"`
	Title   string `json:"title"`
	Start   string `json:"start"`
	End     string `json:"end,omitempty"`
	// UnlistedOn is when the source stopped listing the role while it was
	// open (an RFC 3339 instant), as when its holder left Congress.
	UnlistedOn string `json:"unlistedOn,omitempty"`
}

// CurrentLeadership lists the posts held now by members in Congress.
func (s *Store) CurrentLeadership(ctx context.Context) ([]Role, error) {
	var out []Role
	err := eachRow(ctx, s.db, `
		SELECT l.person, p.display_name, COALESCE(t.party, ''), l.chamber, l.title, l.start
		FROM leadership l JOIN people p ON p.id = l.person
		LEFT JOIN terms t ON t.person = p.id AND t.current = 1 AND t.unlisted_on IS NULL
		WHERE l.end IS NULL AND l.unlisted_on IS NULL AND p.in_congress = 1
		ORDER BY l.chamber, l.title`, func(r *sql.Rows) error {
		var x Role
		if err := r.Scan(&x.Person, &x.Name, &x.Party, &x.Chamber, &x.Title, &x.Start); err != nil {
			return err
		}
		out = append(out, x)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list leadership: %w", err)
	}
	return out, nil
}

// Provenance says where rows came from and when.
type Provenance struct {
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// PersonTerm is one term on a person's page.
type PersonTerm struct {
	Office   string `json:"office"`
	Body     string `json:"body"`
	State    string `json:"state"`
	District *int   `json:"district,omitempty"`
	Class    *int   `json:"class,omitempty"`
	Start    string `json:"start"`
	End      string `json:"end"`
	// Party and Caucus are always present, empty when the source gives none,
	// so every member's page has the same fields (PRD §3).
	Party        string `json:"party"`
	Caucus       string `json:"caucus"`
	How          string `json:"how,omitempty"`
	StateRank    string `json:"stateRank,omitempty"`
	URL          string `json:"url,omitempty"`
	Affiliations string `json:"-"`
	Current      bool   `json:"current"`
	LeftOn       string `json:"leftOn,omitempty"`
}

// PersonSeat is a committee seat on a person's page.
type PersonSeat struct {
	Committee string `json:"committee"`
	Name      string `json:"name"`
	Parent    string `json:"parent,omitempty"`
	Chamber   string `json:"chamber"`
	Side      string `json:"side"`
	Rank      int    `json:"rank,omitempty"`
	Title     string `json:"title,omitempty"`
	// UnlistedOn is when the source stopped listing the seat (an RFC 3339
	// instant); empty while it is listed.
	UnlistedOn string `json:"unlistedOn,omitempty"`
}

// PersonDetail is everything a person's page shows from the roster.
type PersonDetail struct {
	Person   Person
	Terms    []PersonTerm
	Roles    []Role
	Seats    []PersonSeat
	Portrait bool
	// Provenance's FetchedAt is when this person's record last changed: the
	// newest write, or stop-listing, across their person, term, role and
	// seat rows.
	Provenance Provenance
	// PortraitSource is where the portrait was mirrored from, with when it
	// was last checked and when the source last sent a new answer.
	PortraitSource  string
	PortraitChecked time.Time
	PortraitFetched time.Time
}

// PersonDetail reads one person, or reports that there is none.
func (s *Store) PersonDetail(ctx context.Context, id string) (PersonDetail, bool, error) {
	var d PersonDetail
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return d, false, fmt.Errorf("read person %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		in                 int
		fetched            string
		portrait           sql.NullString
		pChecked, pFetched string
	)
	p := &d.Person
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, COALESCE(p.bioguide,''), COALESCE(p.lis,''), COALESCE(p.fec_ids,''), COALESCE(p.icpsr,0),
		       COALESCE(p.govtrack,0), COALESCE(p.wikidata,''), COALESCE(p.wikipedia,''), COALESCE(p.ballotpedia,''),
		       p.first_name, COALESCE(p.middle_name,''), p.last_name, COALESCE(p.suffix,''), COALESCE(p.nickname,''),
		       p.display_name, COALESCE(p.birthday,''), COALESCE(p.gender,''), p.in_congress, p.source,
		       (SELECT max(f) FROM (
		          SELECT p.fetched_at AS f
		          UNION ALL SELECT max(fetched_at, COALESCE(unlisted_on, '')) FROM terms WHERE person = p.id
		          UNION ALL SELECT max(fetched_at, COALESCE(unlisted_on, '')) FROM leadership WHERE person = p.id
		          UNION ALL SELECT max(fetched_at, COALESCE(unlisted_on, '')) FROM committee_seats WHERE person = p.id)),
		       x.file, COALESCE(x.source, ''), COALESCE(x.checked_at, ''), COALESCE(x.fetched_at, '')
		FROM people p LEFT JOIN portraits x ON x.person = p.id WHERE p.id = ?`, id).Scan(
		&p.ID, &p.Bioguide, &p.LIS, &p.FECIDs, &p.ICPSR, &p.GovTrack, &p.Wikidata, &p.Wikipedia, &p.Ballotpedia,
		&p.First, &p.Middle, &p.Last, &p.Suffix, &p.Nickname, &p.Display, &p.Birthday, &p.Gender, &in,
		&d.Provenance.Source, &fetched, &portrait, &d.PortraitSource, &pChecked, &pFetched)
	if err == sql.ErrNoRows { //nolint:errorlint // database/sql returns the sentinel itself
		return d, false, nil
	}
	if err != nil {
		return d, false, fmt.Errorf("read person %s: %w", id, err)
	}
	p.InCongress = in == 1
	d.Portrait = portrait.Valid
	if d.Provenance.FetchedAt, err = time.Parse(time.RFC3339, fetched); err != nil {
		return d, false, fmt.Errorf("person %s fetched_at: %w", id, err)
	}
	for _, x := range []struct {
		raw string
		to  *time.Time
	}{{pChecked, &d.PortraitChecked}, {pFetched, &d.PortraitFetched}} {
		if x.raw == "" {
			continue // no portrait row yet
		}
		if *x.to, err = time.Parse(time.RFC3339, x.raw); err != nil {
			return d, false, fmt.Errorf("person %s portrait time: %w", id, err)
		}
	}

	err = eachRow(ctx, tx, `
		SELECT office, body, state, district, senate_class, start, end, COALESCE(party,''), COALESCE(caucus,''),
		       COALESCE(how,''), COALESCE(state_rank,''), COALESCE(url,''), affiliations, current, COALESCE(left_on,'')
		FROM terms WHERE person = ? AND unlisted_on IS NULL ORDER BY start DESC`, func(r *sql.Rows) error {
		var (
			t               PersonTerm
			district, class sql.NullInt64
			cur             int
		)
		if err := r.Scan(&t.Office, &t.Body, &t.State, &district, &class, &t.Start, &t.End, &t.Party, &t.Caucus, &t.How,
			&t.StateRank, &t.URL, &t.Affiliations, &cur, &t.LeftOn); err != nil {
			return err
		}
		t.District, t.Class, t.Current = intPtr(district), intPtr(class), cur == 1
		d.Terms = append(d.Terms, t)
		return nil
	}, id)
	if err != nil {
		return d, false, fmt.Errorf("read terms of %s: %w", id, err)
	}

	// Listed roles, and open roles the source stopped listing (the holder left,
	// or the post ended without a date): those show as "no longer listed".
	// A past role the source dropped is a correction, and stays hidden.
	err = eachRow(ctx, tx, `
		SELECT chamber, title, start, COALESCE(end,''), COALESCE(unlisted_on,'') FROM leadership
		WHERE person = ? AND (unlisted_on IS NULL OR end IS NULL)
		ORDER BY end IS NOT NULL OR unlisted_on IS NOT NULL, start DESC`, func(r *sql.Rows) error {
		x := Role{Person: id, Name: p.Display}
		if err := r.Scan(&x.Chamber, &x.Title, &x.Start, &x.End, &x.UnlistedOn); err != nil {
			return err
		}
		d.Roles = append(d.Roles, x)
		return nil
	}, id)
	if err != nil {
		return d, false, fmt.Errorf("read roles of %s: %w", id, err)
	}

	err = eachRow(ctx, tx, `
		SELECT c.id, c.name, COALESCE(c.parent,''), c.chamber, s.side, COALESCE(s.rank,0), COALESCE(s.title,''),
		       COALESCE(s.unlisted_on,'')
		FROM committee_seats s JOIN committees c ON c.id = s.committee
		WHERE s.person = ? AND (c.current = 1 OR s.unlisted_on IS NOT NULL)
		ORDER BY s.unlisted_on IS NOT NULL, COALESCE(c.parent, c.id), c.parent IS NOT NULL, c.name`, func(r *sql.Rows) error {
		var x PersonSeat
		if err := r.Scan(&x.Committee, &x.Name, &x.Parent, &x.Chamber, &x.Side, &x.Rank, &x.Title, &x.UnlistedOn); err != nil {
			return err
		}
		d.Seats = append(d.Seats, x)
		return nil
	}, id)
	if err != nil {
		return d, false, fmt.Errorf("read seats of %s: %w", id, err)
	}
	return d, true, nil
}

// CommitteeSummary is a committee in the list.
type CommitteeSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Chamber string `json:"chamber"`
	Parent  string `json:"parent,omitempty"`
	Members int    `json:"members"`
	Chair   string `json:"chair,omitempty"`
	ChairID string `json:"chairId,omitempty"`
	Ranking string `json:"ranking,omitempty"`
	RankID  string `json:"rankingId,omitempty"`
	Subs    int    `json:"subcommittees"`
	URL     string `json:"url,omitempty"`
	Juris   string `json:"jurisdiction,omitempty"`
}

// chairTitles are the titles that name a committee's chair or ranking member.
const chairTitles = `('Chair', 'Chairman', 'Chairwoman', 'Cochairman')`

// Committees lists the current committees and subcommittees.
func (s *Store) Committees(ctx context.Context) ([]CommitteeSummary, error) {
	var out []CommitteeSummary
	err := eachRow(ctx, s.db, `
		SELECT c.id, c.name, c.chamber, COALESCE(c.parent, ''), COALESCE(c.url, ''), COALESCE(c.jurisdiction, ''),
		  (SELECT count(*) FROM committee_seats s WHERE s.committee = c.id AND s.unlisted_on IS NULL),
		  (SELECT count(*) FROM committees sub WHERE sub.parent = c.id AND sub.current = 1),
		  COALESCE((SELECT p.display_name || '|' || p.id FROM committee_seats s JOIN people p ON p.id = s.person
		            WHERE s.committee = c.id AND s.unlisted_on IS NULL AND s.title IN `+chairTitles+` ORDER BY s.side, s.rank LIMIT 1), ''),
		  COALESCE((SELECT p.display_name || '|' || p.id FROM committee_seats s JOIN people p ON p.id = s.person
		            WHERE s.committee = c.id AND s.unlisted_on IS NULL AND s.title = 'Ranking Member' LIMIT 1), '')
		FROM committees c WHERE c.current = 1
		ORDER BY c.chamber, COALESCE(c.parent, c.id), c.parent IS NOT NULL, c.name`, func(r *sql.Rows) error {
		var (
			c              CommitteeSummary
			chair, ranking string
		)
		if err := r.Scan(&c.ID, &c.Name, &c.Chamber, &c.Parent, &c.URL, &c.Juris, &c.Members, &c.Subs, &chair, &ranking); err != nil {
			return err
		}
		c.Chair, c.ChairID, _ = strings.Cut(chair, "|")
		c.Ranking, c.RankID, _ = strings.Cut(ranking, "|")
		out = append(out, c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list committees: %w", err)
	}
	return out, nil
}

// CommitteeMember is a seat on a committee's page.
type CommitteeMember struct {
	Person   string `json:"person"`
	Name     string `json:"name"`
	Party    string `json:"party,omitempty"`
	State    string `json:"state,omitempty"`
	Side     string `json:"side"`
	Rank     int    `json:"rank,omitempty"`
	Title    string `json:"title,omitempty"`
	Portrait bool   `json:"portrait"`
}

// CommitteeMembers lists a committee's seats, majority first, by rank.
func (s *Store) CommitteeMembers(ctx context.Context, id string) ([]CommitteeMember, error) {
	var out []CommitteeMember
	err := eachRow(ctx, s.db, `
		SELECT s.person, p.display_name, COALESCE(t.party, ''), COALESCE(t.state, ''), s.side, COALESCE(s.rank, 0),
		       COALESCE(s.title, ''), x.file IS NOT NULL
		FROM committee_seats s JOIN people p ON p.id = s.person
		LEFT JOIN terms t ON t.person = p.id AND t.current = 1 AND t.unlisted_on IS NULL
		LEFT JOIN portraits x ON x.person = p.id
		WHERE s.committee = ? AND s.unlisted_on IS NULL
		ORDER BY s.side, COALESCE(s.rank, 999), p.last_name`, func(r *sql.Rows) error {
		var m CommitteeMember
		if err := r.Scan(&m.Person, &m.Name, &m.Party, &m.State, &m.Side, &m.Rank, &m.Title, &m.Portrait); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	}, id)
	if err != nil {
		return nil, fmt.Errorf("list members of %s: %w", id, err)
	}
	return out, nil
}

// SourceChecks lists every source's last check, for provenance lines.
func (s *Store) SourceChecks(ctx context.Context) ([]SourceState, error) {
	var out []SourceState
	err := eachRow(ctx, s.db, `SELECT source, url, etag, last_modified, checked_at, changed_at FROM sources ORDER BY source`,
		func(r *sql.Rows) error {
			var (
				st               SourceState
				checked, changed sql.NullString
			)
			if err := r.Scan(&st.Source, &st.URL, &st.ETag, &st.LastModified, &checked, &changed); err != nil {
				return err
			}
			var err error
			if st.CheckedAt, err = parseTime(checked); err != nil {
				return err
			}
			if st.ChangedAt, err = parseTime(changed); err != nil {
				return err
			}
			out = append(out, st)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	return out, nil
}

// LastHolder is who held a seat that is now empty, as far as the roster
// saw: a member the source stopped listing mid-term.
type LastHolder struct {
	Person string `json:"person"`
	Name   string `json:"name"`
	Party  string `json:"party"`
	// LeftOn is the date the source stopped listing them (YYYY-MM-DD), not
	// a date the source gave for their leaving.
	LeftOn string `json:"leftOn"`
}

// LastHolders returns, for each of a state's offices that no one holds now,
// the member the roster saw leave it mid-term, keyed by office id. An old
// term of a member who moved on is not a last holder: only a term with
// left_on counts, so "last held by" is said only when it is known.
func (s *Store) LastHolders(ctx context.Context, state string) (map[string]LastHolder, error) {
	out := map[string]LastHolder{}
	err := eachRow(ctx, s.db, `
		SELECT t.office, p.id, p.display_name, COALESCE(t.party, ''), t.left_on
		FROM terms t JOIN people p ON p.id = t.person
		WHERE t.state = ? AND t.left_on IS NOT NULL AND t.unlisted_on IS NULL AND t.current = 0
		  AND NOT EXISTS (SELECT 1 FROM terms c WHERE c.office = t.office AND c.current = 1 AND c.unlisted_on IS NULL)
		ORDER BY t.office, t.left_on DESC`, func(r *sql.Rows) error {
		var (
			office string
			h      LastHolder
		)
		if err := r.Scan(&office, &h.Person, &h.Name, &h.Party, &h.LeftOn); err != nil {
			return err
		}
		if _, seen := out[office]; !seen {
			out[office] = h // the latest, by the ORDER BY
		}
		return nil
	}, state)
	if err != nil {
		return nil, fmt.Errorf("read last holders in %s: %w", state, err)
	}
	return out, nil
}
