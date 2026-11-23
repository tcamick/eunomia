-- News (E2). A story is a feed item's headline, a short plain-text summary,
-- its time and a link, never the article (PRD §3; AGENTS.md): the CHECKs
-- below refuse a longer summary or headline, so a body cannot be stored even
-- by mistake. Feeds keep only recent items, so this table is the archive,
-- and it is backed up (PRD §8).
CREATE TABLE stories (
  id         INTEGER PRIMARY KEY,
  feed       TEXT NOT NULL,                   -- the feed it was first seen in (config/sources.yaml id)
  guid       TEXT NOT NULL,
  url        TEXT,                            -- NULL when the feed's link is not https
  title      TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
  summary    TEXT NOT NULL CHECK (length(summary) <= 500),
  published  TEXT,                            -- RFC 3339 UTC; NULL when the feed gives none it can read
  source     TEXT NOT NULL,                   -- the feed's URL
  fetched_at TEXT NOT NULL,
  UNIQUE (feed, guid)
) STRICT;

-- One story carried by two feeds (NPR Politics and NPR News often share
-- one) is one row here, listed under both.
CREATE UNIQUE INDEX stories_url ON stories (url) WHERE url IS NOT NULL;
CREATE INDEX stories_when ON stories (COALESCE(published, fetched_at));

CREATE TABLE story_feeds (
  story INTEGER NOT NULL REFERENCES stories (id),
  feed  TEXT NOT NULL,
  PRIMARY KEY (story, feed)
) STRICT;

-- Whom and which places a story names, by the tagging package's rules
-- (ADR 0008). Computed from the stories and the roster, not fetched, so it
-- has no source column; it is recomputed for recent stories on every news run.
CREATE TABLE mentions (
  story INTEGER NOT NULL REFERENCES stories (id),
  kind  TEXT NOT NULL CHECK (kind IN ('person', 'state')),
  ref   TEXT NOT NULL,                        -- a person id or a state code
  PRIMARY KEY (story, kind, ref)
) STRICT;

CREATE INDEX mentions_ref ON mentions (kind, ref);

-- The owner's own marks: the one table the server writes (go-conventions,
-- Layout). A story read on one device shows read on another.
CREATE TABLE marks (
  story    INTEGER PRIMARY KEY REFERENCES stories (id),
  read_at  TEXT,
  saved_at TEXT
) STRICT;
