-- The roster (E1a). A person and the seats they hold are separate rows, and
-- history is kept: a member who leaves keeps their row and their terms
-- (PRD §10; docs/go-conventions.md, Provenance). Every row filled from
-- outside carries its source URL and when it was last written (fetched_at,
-- RFC 3339 UTC). Dates are calendar dates, YYYY-MM-DD.

CREATE TABLE people (
  id           TEXT PRIMARY KEY,           -- the Bioguide id for members of Congress
  bioguide     TEXT UNIQUE,
  lis          TEXT,
  fec_ids      TEXT,                        -- comma-separated FEC candidate ids
  icpsr        INTEGER,
  govtrack     INTEGER,
  wikidata     TEXT,
  wikipedia    TEXT,
  ballotpedia  TEXT,
  first_name   TEXT NOT NULL,
  middle_name  TEXT,
  last_name    TEXT NOT NULL,
  suffix       TEXT,
  nickname     TEXT,
  display_name TEXT NOT NULL,
  birthday     TEXT,                        -- NULL when the source does not give it
  gender       TEXT,
  in_congress  INTEGER NOT NULL CHECK (in_congress IN (0, 1)),
  source       TEXT NOT NULL,
  fetched_at   TEXT NOT NULL
) STRICT;

CREATE INDEX people_last_name ON people (last_name);

-- A term in a seat. office is 'senate/<state>/<class>' or
-- 'house/<state>/<district>' (0 for at large).
CREATE TABLE terms (
  person       TEXT NOT NULL REFERENCES people (id),
  office       TEXT NOT NULL,
  body         TEXT NOT NULL CHECK (body IN ('senate', 'house')),
  state        TEXT NOT NULL,
  district     INTEGER,
  senate_class INTEGER,
  start        TEXT NOT NULL,
  end          TEXT NOT NULL,                -- the scheduled end, as the source gives it
  party        TEXT,                         -- NULL when the source does not give it
  caucus       TEXT,
  how          TEXT,                         -- appointment, special-election, or NULL
  end_type     TEXT,
  state_rank   TEXT,
  url          TEXT,
  affiliations TEXT NOT NULL DEFAULT '[]',   -- JSON: party spans within the term, when it changed
  current      INTEGER NOT NULL CHECK (current IN (0, 1)),
  left_on      TEXT,                         -- when the source stopped listing the member, if it did mid-term
  unlisted_on  TEXT,                         -- when the source stopped listing this term for a still-listed member (a correction); NULL while listed
  source       TEXT NOT NULL,
  fetched_at   TEXT NOT NULL,
  PRIMARY KEY (person, office, start)
) STRICT;

CREATE INDEX terms_current ON terms (current, body, state);

CREATE TABLE leadership (
  person     TEXT NOT NULL REFERENCES people (id),
  chamber    TEXT NOT NULL CHECK (chamber IN ('senate', 'house')),
  title      TEXT NOT NULL,
  start      TEXT NOT NULL,
  end        TEXT,                           -- NULL while held
  unlisted_on TEXT,          -- when the source stopped listing this role; NULL while listed
  source     TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (person, title, start)
) STRICT;

CREATE TABLE committees (
  id           TEXT PRIMARY KEY,             -- the source's thomas_id; a subcommittee's has its parent's prepended
  parent       TEXT,
  chamber      TEXT NOT NULL CHECK (chamber IN ('house', 'senate', 'joint')),
  name         TEXT NOT NULL,
  url          TEXT,
  jurisdiction TEXT,
  current      INTEGER NOT NULL CHECK (current IN (0, 1)),
  source       TEXT NOT NULL,
  fetched_at   TEXT NOT NULL
) STRICT;

-- Who sits, or sat, on each committee. A seat the source stops listing is
-- kept with the date it stopped (go-conventions, Provenance: history is
-- never deleted); a seat listed again is current again.
CREATE TABLE committee_seats (
  committee  TEXT NOT NULL REFERENCES committees (id),
  person     TEXT NOT NULL REFERENCES people (id),
  side       TEXT NOT NULL CHECK (side IN ('majority', 'minority')),
  rank       INTEGER,
  title      TEXT,
  unlisted_on TEXT,          -- when the source stopped listing this seat; NULL while listed
  source     TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (committee, person)
) STRICT;

CREATE INDEX committee_seats_person ON committee_seats (person);

-- What a job found different from its last run, written in the same
-- transaction as the data (go-conventions, Provenance). Home's "Since you
-- last looked" reads it (E3a).
CREATE TABLE changes (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  at        TEXT NOT NULL,
  kind      TEXT NOT NULL,
  person    TEXT,
  office    TEXT,
  committee TEXT,
  summary   TEXT NOT NULL,
  source    TEXT NOT NULL
) STRICT;

CREATE INDEX changes_at ON changes (at);

-- Each source's conditional-request validators and when it was last checked
-- and last changed. Job bookkeeping, like the jobs table.
CREATE TABLE sources (
  source        TEXT PRIMARY KEY,
  url           TEXT NOT NULL,
  etag          TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL DEFAULT '',
  checked_at    TEXT NOT NULL,
  changed_at    TEXT
) STRICT;

-- Mirrored portraits (data/portraits/<person>.jpg).
CREATE TABLE portraits (
  person        TEXT PRIMARY KEY REFERENCES people (id),
  file          TEXT,                         -- NULL when the source has none
  etag          TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL DEFAULT '',
  checked_at    TEXT NOT NULL,
  source        TEXT NOT NULL,
  fetched_at    TEXT NOT NULL                 -- when the source last answered with a new file or with none (a 404 is a fetch)
) STRICT;
