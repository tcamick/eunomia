package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Person is a row of people. An empty string or a zero number is stored as
// NULL: unknown, never a value that looks like data (go-conventions,
// Provenance).
type Person struct {
	ID          string
	Bioguide    string
	LIS         string
	FECIDs      string
	ICPSR       int
	GovTrack    int
	Wikidata    string
	Wikipedia   string
	Ballotpedia string
	First       string
	Middle      string
	Last        string
	Suffix      string
	Nickname    string
	Display     string
	Birthday    string
	Gender      string
	InCongress  bool
}

// Term is a row of terms.
type Term struct {
	Person       string
	Office       string
	Body         string
	State        string
	District     *int
	Class        *int
	Start        string
	End          string
	Party        string
	Caucus       string
	How          string
	EndType      string
	StateRank    string
	URL          string
	Affiliations string // JSON
	Current      bool
	LeftOn       string
}

// Key returns the term's primary key.
func (t Term) Key() TermKey { return TermKey{Person: t.Person, Office: t.Office, Start: t.Start} }

// TermKey is a term's primary key.
type TermKey struct{ Person, Office, Start string }

// Leadership is a row of leadership.
type Leadership struct {
	Person  string
	Chamber string
	Title   string
	Start   string
	End     string
}

// Key returns the role's primary key.
func (l Leadership) Key() LeadershipKey {
	return LeadershipKey{Person: l.Person, Title: l.Title, Start: l.Start}
}

// LeadershipKey is a leadership role's primary key.
type LeadershipKey struct{ Person, Title, Start string }

// Committee is a row of committees.
type Committee struct {
	ID           string
	Parent       string
	Chamber      string
	Name         string
	URL          string
	Jurisdiction string
	Current      bool
}

// Seat is a row of committee_seats.
type Seat struct {
	Committee string
	Person    string
	Side      string
	Rank      int
	Title     string
}

// Key returns the seat's primary key.
func (s Seat) Key() SeatKey { return SeatKey{Committee: s.Committee, Person: s.Person} }

// SeatKey is a committee seat's primary key.
type SeatKey struct{ Committee, Person string }

// Change is a row of changes, as a job records it.
type Change struct {
	Kind      string
	Person    string
	Office    string
	Committee string
	Summary   string
}

// RosterState is every roster row, keyed, for the roster job to diff.
type RosterState struct {
	People     map[string]Person
	Terms      map[TermKey]Term
	Leadership map[LeadershipKey]Leadership
	Committees map[string]Committee
	Seats      map[SeatKey]Seat
}

// RosterPlan is what the roster job decided to write: rows to upsert or
// delete, and the changes to record, all in one transaction.
type RosterPlan struct {
	At               time.Time
	Source           string
	People           []Person
	Terms            []Term
	DeleteTerms      []TermKey
	Leadership       []Leadership
	DeleteLeadership []LeadershipKey
	Committees       []Committee
	Seats            []Seat
	DeleteSeats      []SeatKey
	Changes          []Change
}

// Empty reports whether the plan writes nothing.
func (p RosterPlan) Empty() bool {
	return len(p.People)+len(p.Terms)+len(p.DeleteTerms)+len(p.Leadership)+len(p.DeleteLeadership)+
		len(p.Committees)+len(p.Seats)+len(p.DeleteSeats)+len(p.Changes) == 0
}

// nullable maps the zero value to NULL.
func nullable[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RosterState reads every roster row.
func (s *Store) RosterState(ctx context.Context) (RosterState, error) {
	st := RosterState{
		People:     map[string]Person{},
		Terms:      map[TermKey]Term{},
		Leadership: map[LeadershipKey]Leadership{},
		Committees: map[string]Committee{},
		Seats:      map[SeatKey]Seat{},
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return st, fmt.Errorf("read roster: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := eachRow(ctx, tx, `SELECT id, COALESCE(bioguide,''), COALESCE(lis,''), COALESCE(fec_ids,''), COALESCE(icpsr,0),
		COALESCE(govtrack,0), COALESCE(wikidata,''), COALESCE(wikipedia,''), COALESCE(ballotpedia,''), first_name,
		COALESCE(middle_name,''), last_name, COALESCE(suffix,''), COALESCE(nickname,''), display_name,
		COALESCE(birthday,''), COALESCE(gender,''), in_congress FROM people`, func(r *sql.Rows) error {
		var p Person
		var in int
		if err := r.Scan(&p.ID, &p.Bioguide, &p.LIS, &p.FECIDs, &p.ICPSR, &p.GovTrack, &p.Wikidata, &p.Wikipedia,
			&p.Ballotpedia, &p.First, &p.Middle, &p.Last, &p.Suffix, &p.Nickname, &p.Display, &p.Birthday, &p.Gender, &in); err != nil {
			return err
		}
		p.InCongress = in == 1
		st.People[p.ID] = p
		return nil
	}); err != nil {
		return st, fmt.Errorf("read people: %w", err)
	}

	if err := eachRow(ctx, tx, `SELECT person, office, body, state, district, senate_class, start, end, COALESCE(party,''),
		COALESCE(caucus,''), COALESCE(how,''), COALESCE(end_type,''), COALESCE(state_rank,''), COALESCE(url,''),
		affiliations, current, COALESCE(left_on,'') FROM terms`, func(r *sql.Rows) error {
		var t Term
		var district, class sql.NullInt64
		var cur int
		if err := r.Scan(&t.Person, &t.Office, &t.Body, &t.State, &district, &class, &t.Start, &t.End, &t.Party, &t.Caucus,
			&t.How, &t.EndType, &t.StateRank, &t.URL, &t.Affiliations, &cur, &t.LeftOn); err != nil {
			return err
		}
		t.District = intPtr(district)
		t.Class = intPtr(class)
		t.Current = cur == 1
		st.Terms[t.Key()] = t
		return nil
	}); err != nil {
		return st, fmt.Errorf("read terms: %w", err)
	}

	if err := eachRow(ctx, tx, `SELECT person, chamber, title, start, COALESCE(end,'') FROM leadership`, func(r *sql.Rows) error {
		var l Leadership
		if err := r.Scan(&l.Person, &l.Chamber, &l.Title, &l.Start, &l.End); err != nil {
			return err
		}
		st.Leadership[l.Key()] = l
		return nil
	}); err != nil {
		return st, fmt.Errorf("read leadership: %w", err)
	}

	if err := eachRow(ctx, tx, `SELECT id, COALESCE(parent,''), chamber, name, COALESCE(url,''), COALESCE(jurisdiction,''),
		current FROM committees`, func(r *sql.Rows) error {
		var c Committee
		var cur int
		if err := r.Scan(&c.ID, &c.Parent, &c.Chamber, &c.Name, &c.URL, &c.Jurisdiction, &cur); err != nil {
			return err
		}
		c.Current = cur == 1
		st.Committees[c.ID] = c
		return nil
	}); err != nil {
		return st, fmt.Errorf("read committees: %w", err)
	}

	if err := eachRow(ctx, tx, `SELECT committee, person, side, COALESCE(rank,0), COALESCE(title,'') FROM committee_seats`,
		func(r *sql.Rows) error {
			var x Seat
			if err := r.Scan(&x.Committee, &x.Person, &x.Side, &x.Rank, &x.Title); err != nil {
				return err
			}
			st.Seats[x.Key()] = x
			return nil
		}); err != nil {
		return st, fmt.Errorf("read committee seats: %w", err)
	}
	return st, nil
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// queryer is what eachRow needs of a *sql.Tx or *sql.DB.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// eachRow runs query and calls fn for each row.
func eachRow(ctx context.Context, q queryer, query string, fn func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ApplyRoster writes a plan in one transaction: the rows and the changes
// they cause land together, so "Since you last looked" cannot disagree with
// the data. An empty plan writes nothing.
func (s *Store) ApplyRoster(ctx context.Context, p RosterPlan) error {
	if p.Empty() {
		return nil
	}
	if p.Source == "" || p.At.IsZero() {
		return fmt.Errorf("apply roster: a plan needs a source and a time")
	}
	at := p.At.UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("apply roster: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, step := range []func(context.Context, *sql.Tx, RosterPlan, string) error{
		writePeople, deleteTerms, writeTerms, deleteLeadership, writeLeadership,
		writeCommittees, deleteSeats, writeSeats, writeChanges,
	} {
		if err := step(ctx, tx, p, at); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply roster: %w", err)
	}
	return nil
}

// The steps of ApplyRoster, in an order the foreign keys allow: people before
// their terms and roles, committees before their seats.
func writePeople(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, x := range p.People {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO people (id, bioguide, lis, fec_ids, icpsr, govtrack, wikidata, wikipedia, ballotpedia,
			  first_name, middle_name, last_name, suffix, nickname, display_name, birthday, gender, in_congress, source, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
			  bioguide = excluded.bioguide, lis = excluded.lis, fec_ids = excluded.fec_ids, icpsr = excluded.icpsr,
			  govtrack = excluded.govtrack, wikidata = excluded.wikidata, wikipedia = excluded.wikipedia,
			  ballotpedia = excluded.ballotpedia, first_name = excluded.first_name, middle_name = excluded.middle_name,
			  last_name = excluded.last_name, suffix = excluded.suffix, nickname = excluded.nickname,
			  display_name = excluded.display_name, birthday = excluded.birthday, gender = excluded.gender,
			  in_congress = excluded.in_congress, source = excluded.source, fetched_at = excluded.fetched_at`,
			x.ID, nullable(x.Bioguide), nullable(x.LIS), nullable(x.FECIDs), nullable(x.ICPSR), nullable(x.GovTrack),
			nullable(x.Wikidata), nullable(x.Wikipedia), nullable(x.Ballotpedia), x.First, nullable(x.Middle), x.Last,
			nullable(x.Suffix), nullable(x.Nickname), x.Display, nullable(x.Birthday), nullable(x.Gender),
			boolInt(x.InCongress), p.Source, at); err != nil {
			return fmt.Errorf("write person %s: %w", x.ID, err)
		}
	}
	return nil
}

func deleteTerms(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, k := range p.DeleteTerms {
		if _, err := tx.ExecContext(ctx, `DELETE FROM terms WHERE person = ? AND office = ? AND start = ?`,
			k.Person, k.Office, k.Start); err != nil {
			return fmt.Errorf("delete term %v: %w", k, err)
		}
	}
	return nil
}

func writeTerms(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, t := range p.Terms {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO terms (person, office, body, state, district, senate_class, start, end, party, caucus, how,
			  end_type, state_rank, url, affiliations, current, left_on, source, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (person, office, start) DO UPDATE SET
			  body = excluded.body, state = excluded.state, district = excluded.district,
			  senate_class = excluded.senate_class, end = excluded.end, party = excluded.party,
			  caucus = excluded.caucus, how = excluded.how, end_type = excluded.end_type,
			  state_rank = excluded.state_rank, url = excluded.url, affiliations = excluded.affiliations,
			  current = excluded.current, left_on = excluded.left_on, source = excluded.source,
			  fetched_at = excluded.fetched_at`,
			t.Person, t.Office, t.Body, t.State, nullInt(t.District), nullInt(t.Class), t.Start, t.End, nullable(t.Party),
			nullable(t.Caucus), nullable(t.How), nullable(t.EndType), nullable(t.StateRank), nullable(t.URL),
			orEmptyJSON(t.Affiliations), boolInt(t.Current), nullable(t.LeftOn), p.Source, at); err != nil {
			return fmt.Errorf("write term %v: %w", t.Key(), err)
		}
	}
	return nil
}

func deleteLeadership(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, k := range p.DeleteLeadership {
		if _, err := tx.ExecContext(ctx, `DELETE FROM leadership WHERE person = ? AND title = ? AND start = ?`,
			k.Person, k.Title, k.Start); err != nil {
			return fmt.Errorf("delete leadership %v: %w", k, err)
		}
	}
	return nil
}

func writeLeadership(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, l := range p.Leadership {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO leadership (person, chamber, title, start, end, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (person, title, start) DO UPDATE SET
			  chamber = excluded.chamber, end = excluded.end, source = excluded.source, fetched_at = excluded.fetched_at`,
			l.Person, l.Chamber, l.Title, l.Start, nullable(l.End), p.Source, at); err != nil {
			return fmt.Errorf("write leadership %v: %w", l.Key(), err)
		}
	}
	return nil
}

func writeCommittees(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, c := range p.Committees {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO committees (id, parent, chamber, name, url, jurisdiction, current, source, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
			  parent = excluded.parent, chamber = excluded.chamber, name = excluded.name, url = excluded.url,
			  jurisdiction = excluded.jurisdiction, current = excluded.current, source = excluded.source,
			  fetched_at = excluded.fetched_at`,
			c.ID, nullable(c.Parent), c.Chamber, c.Name, nullable(c.URL), nullable(c.Jurisdiction), boolInt(c.Current),
			p.Source, at); err != nil {
			return fmt.Errorf("write committee %s: %w", c.ID, err)
		}
	}
	return nil
}

func deleteSeats(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, k := range p.DeleteSeats {
		if _, err := tx.ExecContext(ctx, `DELETE FROM committee_seats WHERE committee = ? AND person = ?`,
			k.Committee, k.Person); err != nil {
			return fmt.Errorf("delete seat %v: %w", k, err)
		}
	}
	return nil
}

func writeSeats(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, x := range p.Seats {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO committee_seats (committee, person, side, rank, title, source, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (committee, person) DO UPDATE SET
			  side = excluded.side, rank = excluded.rank, title = excluded.title, source = excluded.source,
			  fetched_at = excluded.fetched_at`,
			x.Committee, x.Person, x.Side, nullable(x.Rank), nullable(x.Title), p.Source, at); err != nil {
			return fmt.Errorf("write seat %v: %w", x.Key(), err)
		}
	}
	return nil
}

func writeChanges(ctx context.Context, tx *sql.Tx, p RosterPlan, at string) error {
	for _, c := range p.Changes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO changes (at, kind, person, office, committee, summary, source) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			at, c.Kind, nullable(c.Person), nullable(c.Office), nullable(c.Committee), c.Summary, p.Source); err != nil {
			return fmt.Errorf("write change: %w", err)
		}
	}
	return nil
}

func orEmptyJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return "[]"
	}
	return s
}

// SourceState is a source's conditional-request validators and check times.
type SourceState struct {
	Source       string
	URL          string
	ETag         string
	LastModified string
	CheckedAt    time.Time
	ChangedAt    time.Time
}

// SourceState returns what was recorded for source, and whether anything was.
func (s *Store) SourceState(ctx context.Context, source string) (SourceState, bool, error) {
	var (
		st               SourceState
		checked, changed sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `SELECT source, url, etag, last_modified, checked_at, changed_at FROM sources WHERE source = ?`,
		source).Scan(&st.Source, &st.URL, &st.ETag, &st.LastModified, &checked, &changed)
	if err == sql.ErrNoRows { //nolint:errorlint // database/sql returns the sentinel itself
		return SourceState{}, false, nil
	}
	if err != nil {
		return st, false, fmt.Errorf("read source %s: %w", source, err)
	}
	if st.CheckedAt, err = parseTime(checked); err != nil {
		return st, false, fmt.Errorf("source %s checked_at: %w", source, err)
	}
	if st.ChangedAt, err = parseTime(changed); err != nil {
		return st, false, fmt.Errorf("source %s changed_at: %w", source, err)
	}
	return st, true, nil
}

// RecordSourceCheck upserts a source's validators and check time. changed
// also moves changed_at.
func (s *Store) RecordSourceCheck(ctx context.Context, st SourceState, changed bool) error {
	checked := st.CheckedAt.UTC().Format(time.RFC3339)
	var changedAt any
	if changed {
		changedAt = checked
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (source, url, etag, last_modified, checked_at, changed_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (source) DO UPDATE SET
		  url = excluded.url, etag = excluded.etag, last_modified = excluded.last_modified,
		  checked_at = excluded.checked_at, changed_at = COALESCE(excluded.changed_at, sources.changed_at)`,
		st.Source, st.URL, st.ETag, st.LastModified, checked, changedAt)
	if err != nil {
		return fmt.Errorf("record source %s: %w", st.Source, err)
	}
	return nil
}

func parseTime(s sql.NullString) (time.Time, error) {
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s.String)
}
