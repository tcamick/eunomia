# Fixtures

Real responses, trimmed (docs/go-conventions.md, Testing). Fetched 2026-11-23 with the `eunomia/dev` User-Agent; each keeps its channel and its first three items, unchanged except as noted. The sources' terms are in `docs/acceptance/phase-e2.md`, Sources.

- `npr-politics.xml`: `https://feeds.npr.org/1014/rss.xml` (10 items when fetched).
- `npr-news.xml`: `https://feeds.npr.org/1001/rss.xml` (10 items).
- `democracy-now.xml`: `https://www.democracynow.org/democracynow.rss` (42 items); each `content:encoded` (the article body) is replaced by a one-line placeholder, so no article is committed; `hostile.xml` shows a body is never read.

`hostile.xml` is made by hand, not fetched: the things a feed should never get past the parser (a `javascript:` link, http links off the source's site, HTML and a script in a summary, an article body in `content:encoded`, a summary over 500 characters, an item with no title, one with no date, a repeated guid).
