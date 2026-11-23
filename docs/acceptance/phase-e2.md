# Phase E2 — News acceptance

Branch: `feat/phase-e2-news` · Started: 2026-11-23 · Merged:

Roadmap `docs/roadmap.md`, E2; plan `docs/implementation-plan.md` §4, E2; PRD §3, §6.6, §7, §8.

## Done-when checklist
- [ ] The feed shows stories from all three sources (NPR Politics, NPR News, Democracy Now), each labelled, within an hour or so of publication — evidence: *(amended 2026-11-23, owner: CalMatters and The Oaklandside wait on their terms; see Sources)*
- [ ] A story that names a senator appears on that senator's page and their state's page — evidence:
- [ ] Marking a story read on one device shows it read on another — evidence:
- [ ] No article body is in the database (a query in the acceptance file shows the longest stored summary) — evidence:
- [ ] Fifty mentions from the first day's stories are checked by hand and none is wrong (the week-long check is gate G3) — evidence:
- [ ] A backup restores into a working copy, and a missing disk shows as a failed backup on the status page — evidence:

## Prerequisites
- The backup directory on seabass: made by the owner, 2026-11-23, with a mount check and a marker file (`/srv/seabass/eunomia-backups/.eunomia-backup-disk`, owned by 65532, mode 0770; the plan's command is amended to match).

## Sources (checked 2026-11-23)
Each feed was fetched once (`eunomia/dev` User-Agent) and its terms read; three items of each are saved under `internal/news/testdata/`.
- **NPR Politics** `https://feeds.npr.org/1014/rss.xml` and **NPR News** `https://feeds.npr.org/1001/rss.xml`: 10 items each; summaries ~200 characters; no `ETag` or `Last-Modified` (only `cache-control: max-age` ~8 minutes), so an unchanged feed costs one full ~13 KB request and writes nothing. Terms, https://www.npr.org/about-npr/179876898/terms-of-use: "You may display, excerpt from, and link to the Content Feeds on your personal site or application … you must provide attribution to NPR … by including … 'NPR' … in text adjacent to the Content Feed." Allowed; the source's name is shown on every story.
- **Democracy Now!** `https://www.democracynow.org/democracynow.rss`: 42 items; summaries ~1,100 characters (cut to 500); `content:encoded` carries the article (never read); `ETag` and `Last-Modified`; links are `http://www.democracynow.org/…`, upgraded to https on its own site. Terms: none found (`/pages/about/copyright` is 404); https://www.democracynow.org/podcasting invites subscribing to the feed. The owner chose to include it (2026-11-23).
- **CalMatters** `https://calmatters.org/feed/` and **The Oaklandside** `https://oaklandside.org/feed/`: fetched once, not added. CalMatters' terms (https://calmatters.org/about/policies-and-standards/#j-terms-and-conditions, 2015) bar access "through any automated, unethical or unconventional means" and display "unless specifically authorized"; its republishing offer (https://calmatters.org/about/republish/) covers articles, with credit. The Oaklandside's terms page is gone (404) and https://oaklandside.org/policies/ says nothing on feeds. Both put HTML excerpts of 2–4 KB in `description`. The owner chose to add them once their terms are clear (2026-11-23).
- **Labels:** from each outlet's own about page: NPR, "the entire public radio system … network of member stations" (https://www.npr.org/about) → "Public radio network"; Democracy Now!, "a daily, global, independent news hour … a 501(c)3 non-profit news organization" (https://www.democracynow.org/about) → "Independent nonprofit daily news program".
- **Go module:** `go.yaml.in/yaml/v3` v3.0.5, already in the module graph through the Prometheus client, is now a direct dependency for `config/sources.yaml`; no new code enters the build.

## Open questions found during the phase
-

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
