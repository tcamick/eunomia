-- The last run of each job, for the status page and for /metrics (read at
-- scrape time, since a job's pod is gone before Prometheus could scrape it).
-- last_ok moves only on success, so a run of failures still shows when the
-- job last worked. Times are RFC 3339 in UTC.
CREATE TABLE jobs (
  name             TEXT PRIMARY KEY,
  last_run         TEXT NOT NULL,
  last_ok          TEXT,
  ok               INTEGER NOT NULL CHECK (ok IN (0, 1)),
  last_duration_ms INTEGER NOT NULL CHECK (last_duration_ms >= 0),
  message          TEXT NOT NULL DEFAULT ''
) STRICT;
