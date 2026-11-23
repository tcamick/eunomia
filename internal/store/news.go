package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// NewStory is a feed item to store (internal/news made it safe: plain text,
// an https link or none, a summary within the limit).
type NewStory struct {
	GUID      string
	URL       string // "" for none
	Title     string
	Summary   string
	Published time.Time // zero for unknown
}

// AddStories writes one feed's items, with the feed's URL as their source.
// An item already stored for this feed is left alone; one already stored
// from another feed (the same link) is listed under this feed too. It
// returns the ids of the stories it inserted, so a run that finds nothing
// new writes nothing and returns none.
func (s *Store) AddStories(ctx context.Context, feed, source string, at time.Time, items []NewStory) ([]int64, error) {
	if feed == "" || source == "" || at.IsZero() {
		return nil, errors.New("add stories: a feed, a source and a time are required")
	}
	fetched := at.UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("add stories from %s: %w", feed, err)
	}
	defer func() { _ = tx.Rollback() }()
	var added []int64
	for _, it := range items {
		id, isNew, err := addStory(ctx, tx, feed, source, fetched, it)
		if err != nil {
			return nil, fmt.Errorf("add story %q from %s: %w", it.GUID, feed, err)
		}
		if isNew {
			added = append(added, id)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("add stories from %s: %w", feed, err)
	}
	return added, nil
}

func addStory(ctx context.Context, tx *sql.Tx, feed, source, fetched string, it NewStory) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM stories WHERE feed = ? AND guid = ?`, feed, it.GUID).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if it.URL != "" {
		err = tx.QueryRowContext(ctx, `SELECT id FROM stories WHERE url = ?`, it.URL).Scan(&id)
		if err == nil {
			_, err = tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO story_feeds (story, feed, source, fetched_at) VALUES (?, ?, ?, ?)`, id, feed, source, fetched)
			return id, false, err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
	}
	var published any
	if !it.Published.IsZero() {
		published = it.Published.UTC().Format(time.RFC3339)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO stories (feed, guid, url, title, summary, published, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		feed, it.GUID, nullable(it.URL), it.Title, it.Summary, published, source, fetched)
	if err != nil {
		return 0, false, err
	}
	if id, err = res.LastInsertId(); err != nil {
		return 0, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO story_feeds (story, feed, source, fetched_at) VALUES (?, ?, ?, ?)`, id, feed, source, fetched)
	return id, true, err
}

// StoryText is what tagging reads of a story.
type StoryText struct {
	ID      int64
	Title   string
	Summary string
}

// StoriesSince lists the stories published (or, without a time, fetched)
// since a moment: the ones whose mentions are recomputed.
func (s *Store) StoriesSince(ctx context.Context, since time.Time) ([]StoryText, error) {
	var out []StoryText
	err := eachRow(ctx, s.db, `
		SELECT id, title, summary FROM stories WHERE COALESCE(published, fetched_at) >= ? ORDER BY id`,
		func(r *sql.Rows) error {
			var x StoryText
			if err := r.Scan(&x.ID, &x.Title, &x.Summary); err != nil {
				return err
			}
			out = append(out, x)
			return nil
		}, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list recent stories: %w", err)
	}
	return out, nil
}

// NamedMember is a sitting member as tagging matches them: the names the
// source gives, and the seat.
type NamedMember struct {
	ID       string
	First    string
	Nickname string
	Last     string
	Display  string
	Body     string // "senate" or "house"
	State    string
}

// NamedMembers lists the sitting members with their names, for tagging.
func (s *Store) NamedMembers(ctx context.Context) ([]NamedMember, error) {
	var out []NamedMember
	err := eachRow(ctx, s.db, `
		SELECT p.id, p.first_name, COALESCE(p.nickname, ''), p.last_name, p.display_name, t.body, t.state
		FROM people p JOIN terms t ON t.person = p.id AND t.current = 1 AND t.unlisted_on IS NULL
		WHERE p.in_congress = 1
		ORDER BY p.id`, func(r *sql.Rows) error {
		var m NamedMember
		if err := r.Scan(&m.ID, &m.First, &m.Nickname, &m.Last, &m.Display, &m.Body, &m.State); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list members for tagging: %w", err)
	}
	return out, nil
}

// Mention is one person or state a story names.
type Mention struct {
	Kind string `json:"kind"` // "person" or "state"
	Ref  string `json:"ref"`  // a person id or a state code
}

// ReplaceMentions sets each listed story's mentions to those given,
// writing only the differences, and returns how many rows it wrote. A story
// not listed is untouched. A person mention is removed only when the person
// is in matchable, the people the tagger could have found: a member who
// has left, and so is no longer matched, keeps the stories that named them
// (go-conventions, Provenance: history is never deleted because a source
// stopped listing someone). A nil matchable removes no person mention.
func (s *Store) ReplaceMentions(ctx context.Context, want map[int64][]Mention, matchable map[string]bool) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("replace mentions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	changed := 0
	for story, ms := range want {
		have := map[Mention]bool{}
		err := eachRow(ctx, tx, `SELECT kind, ref FROM mentions WHERE story = ?`, func(r *sql.Rows) error {
			var m Mention
			if err := r.Scan(&m.Kind, &m.Ref); err != nil {
				return err
			}
			have[m] = true
			return nil
		}, story)
		if err != nil {
			return 0, fmt.Errorf("read mentions of story %d: %w", story, err)
		}
		keep := map[Mention]bool{}
		for _, m := range ms {
			keep[m] = true
			if have[m] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO mentions (story, kind, ref) VALUES (?, ?, ?)`, story, m.Kind, m.Ref); err != nil {
				return 0, fmt.Errorf("add mention to story %d: %w", story, err)
			}
			changed++
		}
		for m := range have {
			if keep[m] || (m.Kind == "person" && !matchable[m.Ref]) {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM mentions WHERE story = ? AND kind = ? AND ref = ?`,
				story, m.Kind, m.Ref); err != nil {
				return 0, fmt.Errorf("drop mention from story %d: %w", story, err)
			}
			changed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("replace mentions: %w", err)
	}
	return changed, nil
}

// StoryMention is a mention as the feed shows it: a person's name comes
// with it (a state's name is the caller's, from internal/geo).
type StoryMention struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Name string `json:"name,omitempty"`
}

// StoryFilter narrows the feed. Empty fields do not filter.
type StoryFilter struct {
	Feed   string
	State  string // stories naming the state, or anyone who sits for it now
	Person string
	Saved  bool
	Unread bool
	Limit  int
	Offset int
}

// StoryRow is a story as the feed shows it.
type StoryRow struct {
	ID        int64          `json:"id"`
	Feed      string         `json:"feed"`
	Feeds     []string       `json:"feeds"`
	URL       string         `json:"url,omitempty"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary"`
	Published string         `json:"published,omitempty"`
	FetchedAt string         `json:"fetchedAt"`
	Source    string         `json:"source"`
	ReadAt    string         `json:"readAt,omitempty"`
	SavedAt   string         `json:"savedAt,omitempty"`
	Mentions  []StoryMention `json:"mentions"`
}

// Stories lists stories, newest first, with their feeds, mentions and the
// owner's marks.
func (s *Store) Stories(ctx context.Context, f StoryFilter) ([]StoryRow, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 30
	}
	var out []StoryRow
	err := eachRow(ctx, s.db, `
		SELECT s.id, s.feed, COALESCE(s.url, ''), s.title, s.summary, COALESCE(s.published, ''), s.fetched_at, s.source,
		       COALESCE(k.read_at, ''), COALESCE(k.saved_at, ''),
		       (SELECT json_group_array(feed) FROM (SELECT feed FROM story_feeds WHERE story = s.id ORDER BY feed)),
		       (SELECT json_group_array(json_object('kind', kind, 'ref', ref, 'name',
		                 CASE kind WHEN 'person' THEN (SELECT display_name FROM people WHERE id = ref) END))
		          FROM (SELECT kind, ref FROM mentions WHERE story = s.id ORDER BY kind, ref))
		FROM stories s LEFT JOIN marks k ON k.story = s.id
		WHERE (?1 = '' OR EXISTS (SELECT 1 FROM story_feeds sf WHERE sf.story = s.id AND sf.feed = ?1))
		  AND (?2 = '' OR EXISTS (SELECT 1 FROM mentions m WHERE m.story = s.id AND (
		        (m.kind = 'state' AND m.ref = ?2) OR
		        (m.kind = 'person' AND m.ref IN (
		          SELECT person FROM terms WHERE state = ?2 AND current = 1 AND unlisted_on IS NULL)))))
		  AND (?3 = '' OR EXISTS (SELECT 1 FROM mentions m WHERE m.story = s.id AND m.kind = 'person' AND m.ref = ?3))
		  AND (?4 = 0 OR k.saved_at IS NOT NULL)
		  AND (?5 = 0 OR k.read_at IS NULL)
		ORDER BY COALESCE(s.published, s.fetched_at) DESC, s.id DESC
		LIMIT ?6 OFFSET ?7`, func(r *sql.Rows) error {
		var (
			x                StoryRow
			feeds, mentioned string
		)
		if err := r.Scan(&x.ID, &x.Feed, &x.URL, &x.Title, &x.Summary, &x.Published, &x.FetchedAt, &x.Source,
			&x.ReadAt, &x.SavedAt, &feeds, &mentioned); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(feeds), &x.Feeds); err != nil {
			return fmt.Errorf("story %d feeds: %w", x.ID, err)
		}
		if err := json.Unmarshal([]byte(mentioned), &x.Mentions); err != nil {
			return fmt.Errorf("story %d mentions: %w", x.ID, err)
		}
		out = append(out, x)
		return nil
	}, f.Feed, f.State, f.Person, boolInt(f.Saved), boolInt(f.Unread), f.Limit, f.Offset)
	if err != nil {
		return nil, fmt.Errorf("list stories: %w", err)
	}
	return out, nil
}

// Marks are the owner's marks on a story; an empty time is "not marked".
type Marks struct {
	ReadAt  string `json:"readAt,omitempty"`
	SavedAt string `json:"savedAt,omitempty"`
}

// SetMarks sets or clears a story's read and saved marks (nil leaves one as
// it is), and reports false for a story that does not exist. The server's
// one write besides visits (go-conventions, Layout).
func (s *Store) SetMarks(ctx context.Context, story int64, read, saved *bool, at time.Time) (Marks, bool, error) {
	var m Marks
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return m, false, fmt.Errorf("mark story %d: %w", story, err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM stories WHERE id = ?`, story).Scan(&n); err != nil {
		return m, false, fmt.Errorf("mark story %d: %w", story, err)
	}
	if n == 0 {
		return m, false, nil
	}
	now := at.UTC().Format(time.RFC3339)
	value := func(set *bool) any {
		if set == nil {
			return nil
		}
		if *set {
			return now
		}
		return ""
	}
	// A NULL argument keeps the stored mark; "" clears it; a time sets it.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO marks (story, read_at, saved_at) VALUES (?1, NULLIF(?2, ''), NULLIF(?3, ''))
		ON CONFLICT (story) DO UPDATE SET
		  read_at = CASE WHEN ?2 IS NULL THEN marks.read_at ELSE NULLIF(?2, '') END,
		  saved_at = CASE WHEN ?3 IS NULL THEN marks.saved_at ELSE NULLIF(?3, '') END`,
		story, value(read), value(saved)); err != nil {
		return m, false, fmt.Errorf("mark story %d: %w", story, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(read_at, ''), COALESCE(saved_at, '') FROM marks WHERE story = ?`,
		story).Scan(&m.ReadAt, &m.SavedAt); err != nil {
		return m, false, fmt.Errorf("read marks of story %d: %w", story, err)
	}
	if err := tx.Commit(); err != nil {
		return m, false, fmt.Errorf("mark story %d: %w", story, err)
	}
	return m, true, nil
}
