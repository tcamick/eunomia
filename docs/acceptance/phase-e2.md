# Phase E2 — News acceptance

Branch: `feat/phase-e2-news` · Started: 2026-11-23 · Merged:

Roadmap `docs/roadmap.md`, E2; plan `docs/implementation-plan.md` §4, E2; PRD §3, §6.6, §7, §8.

## Done-when checklist
- [ ] The feed shows stories from all three sources (NPR Politics, NPR News, Democracy Now), each labelled, within an hour or so of publication — evidence: *(amended 2026-11-23, owner: CalMatters and The Oaklandside wait on their terms; see Sources)* Local, 2026-11-23: one run stored 61 stories from the three feeds, each shown with its source and label (screenshots at 360 and 1280 px). Live: pending the deploy (news CronJob every 30 minutes).
- [ ] A story that names a senator appears on that senator's page and their state's page — evidence: local, 2026-11-23: `/api/stories?person=S000033` and `?state=VT` both return story 50 (Sanders); `?person=H001079` returns 51 and 52, and `?state=MS` returns 51, 52 and 53 (53 names Mississippi itself). The member and state pages show these under "In the news" (looked at, Schumer's page). Live: pending.
- [ ] Marking a story read on one device shows it read on another — evidence: local, 2026-11-23: story 11 marked read through `PUT /api/stories/11/marks` showed "Read" (`aria-pressed="true"`) in a fresh browser profile; a form-encoded PUT was refused (415). Live, on two of the owner's devices: pending.
- [ ] No article body is in the database (a query in the acceptance file shows the longest stored summary) — evidence: the parser never reads `content:encoded` (the struct has no field for it; `hostile.xml`'s BODY-MARKER test), and the schema refuses a summary over 500 characters. Local, 2026-11-23: `SELECT max(length(summary)), max(length(title)), count(*) FROM stories` → 499, 102, 61. Live: pending.
- [ ] Fifty mentions from the first day's stories are checked by hand and none is wrong (the week-long check is gate G3) — evidence: local, 2026-11-23: one run's 61 stories gave 35 mentions (20 people, 15 states), all plausible on a read-through; fewer than fifty, so the line waits for the live first day.
- [ ] A backup restores into a working copy, and a missing disk shows as a failed backup on the status page — evidence: local, 2026-11-23, with a scratch directory standing in for seabass: `backup` wrote `2026-11-23/` (database and manifest, modes 0640/0750); `restore-check` opened the copy and counted 539 people, 2,787 terms, 61 stories, 35 mentions and 1 mark. The first version did not migrate the copy (review finding 1); fixed, and `TestOpenVerifier_MigratesTheCopy` opens a database with no migrations applied and finds every counted table. Rerun after the fix on a rebuilt database: 539 people, 2,787 terms, 61 stories, 35 mentions. With the marker removed, `backup` failed at once, writing nothing, and the status page showed "✗ Failed — the backup disk is not there: … is it mounted?". A job with a limit that never ran shows "Never ran / Overdue" (screenshot). Live: pending (the first nightly run on seabass, a restore check run by hand, and the status page with the disk's marker hidden).

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
- A missing disk: with seabass unplugged the backup pod cannot start, so the status page shows the backup *Never ran / Overdue* (26 hours after its last success); with the disk present and its marker gone, *Failed*. The owner, 2026-11-23: either meets the line; the live check records which it saw.
- Reviews: after five passes, each finding another tagging edge case, the owner chose one more review and then landing, with should-fix findings of that pass fixed without a further pass (2026-11-23).

## Review findings (first pass, 2026-11-23)
All fixed:
1. *Blocking:* the restore check did not migrate the copy. `openVerifier` now migrates, with a test.
2. "state Sen. X", "former Rep. X" and "ex-Sen. X" no longer tag a member; four `TestTag` cases; ADR 0008 amended.
3. A failed feed is reported once: the job returns it, wrapped with the feed's id, and no longer logs it too.
4. Retagging: `TestJob_RetagsTheWindowWhenTheRosterChanges` (a member who joins is found in a story inside the 30-day window, not in one 45 days old).
5. The no-op case checks mentions: `TestJob_ASecondRunChangesNoMention`.
6. `internal/news/testdata/README.md` says where and when each fixture was fetched.
7. A publication date over a day ahead of the fetch is stored as unknown: `TestJob_ADateAheadOfNowIsStoredAsUnknown`.
8. A feed that answers 200 with no usable item fails the run and is not recorded as checked: `TestJob_AFeedWithNoItemsFails`.
9. `story_feeds` has `source` and `fetched_at`; 0003 amended in place, since it had not landed.
10. The restore check's counts are constant queries; the table list, `copied()` and `jobMaxAge()` are functions.
11. A story from a feed no longer in `config/sources.yaml` is named by its feed's host and labelled "No longer followed", with a test.
12. Marks are announced in one polite live region; a failed mark is shown in place, without `role="alert"`.
13. The story list has `content-visibility: auto` with an intrinsic block size. At first this let a 360 px list overflow sideways (both axes were sized); fixed with `contain-intrinsic-block-size` and a `minmax(0, 1fr)` column, then looked at again.
14. The looked-at checks are recorded below.
Nits: the roadmap row, a comment on why `cronjobs_test.go` reads `k8s/`, and the state page's link text (now "See them all", placed above the list so it is not read as a source line).

## Review findings (second pass, 2026-11-23)
All fixed:
1. *Blocking:* retagging deleted the mentions of any member who left, since only sitting members are matched: on 2027-01-03 every departing member would have lost their last month of stories. `ReplaceMentions` now removes a person's mention only for people the tagger can still find; an empty roster removes none. `TestJob_AMemberWhoLeavesKeepsTheStoriesThatNamedThem` and `TestReplaceMentions_RemovesAPersonOnlyIfTheTaggerCouldStillFindThem`; ADR 0008 amended.
2. The restore check skipped dotfiles that the backup copies and counts, so a stray temp file would have failed every drill on a good backup. It now counts the same files; `TestRestoreCheck_ADotfileTheBackupCopiedIsCounted`.
3. The news job's test asserts each story's `source` (its feed's URL) and `fetched_at` (the run's time).
4. The looked-at checks are completed below, keyboard included.
Nits: the plan's marks route and the news schedule (plan and PRD: every 30 minutes, dated) are amended; go-conventions' hermetic-test rule now allows reading the repository's own files, read only.

## Review findings (third pass, 2026-11-23)
All fixed:
1. *Blocking:* "Rep. Grant Smith" tagged a member surnamed Grant. Confirmed with failing tests first ("Rep. Johnson Smith", "Sen. Sanders Ford"); a title and surname now count only when no capitalised name word follows, looked for in the text with members' names blanked out. "Rep. Johnson, R-La." and "Sen. Sanders (I-Vt.)" still tag. ADR 0008 amended.
2. A former member's name ("Virginia Foxx" after the member leaves) would have tagged the state. Former members' names are blanked out and tag no one: `TestTag_AFormerMembersNameTagsNoOne`, `TestJob_ADepartedMembersNameDoesNotBecomeAState`.
3. The Democracy Now fixture's three article bodies are replaced by a one-line placeholder; the README says so.
Nits: the backup's stale-lock comment names the half-hour deadline. On the last line's "missing disk": with seabass unplugged the pod cannot start, so the status page shows the backup *Never ran / Overdue* (26 hours after its last success); with the disk present but the marker gone, *Failed*. Both are a failed backup on the status page; the live check records which it saw.

## Review findings (fourth pass, 2026-11-23)
All fixed:
1. *Blocking:* retagging read 30-day-old stories against today's roster, so at a change of members "Rep. Davis" could move to whichever Davis remained, or gain the new Smith beside the old. Each story is now read against the members sitting on the day it ran (`store.NamedTerms`, a term from its start up to its end or the day the member left); everyone else's names are blanked, not tagged. `TestJob_ATitleIsReadAgainstTheMembersSittingWhenTheStoryRan` covers both cases (it would fail on the earlier code: the 20 November "Rep. Davis" story would have gone to the remaining Davis). ADR 0008 amended. Real-data run after the change: 35 mentions, and a second run changes none.
2. *Blocking:* the oversized-marks-body case failed on an unknown field, not its size. The body is now valid JSON padded with spaces; with the size limit removed, the case fails (checked).
3. A full name a sitting member shares with anyone else in the roster tags no one: `TestTag_ANameASittingMemberSharesWithAFormerOneTagsNoOne`.
4. The looked-at checks: Sanders's page at 360 px is recorded below; the "In the news" sections on member and state pages are the same `StoryList` component keyboard-checked on `/news`, with the same controls.
Nits: a feed id may not be another source's name (`legislators`, `committees`, `committee-membership`, `portraits`), with a test; a test comment and a test message corrected.

## Review findings (fifth pass, 2026-11-23)
All fixed:
1. *Blocking:* a shared surname after a title was settled by a state named anywhere in the text: "Tim Scott toured Florida. Sen. Scott said…" tagged Rick Scott. Now only a state right after the name ("Sen. Scott of Florida") or a member with that surname named in full in the same text settles it; otherwise no one. Both of the reviewer's sentences are `TestTag` cases and failed before the change. ADR 0008 amended. Real data: unchanged (35 mentions, a run after the change wrote none).
2. *Blocking:* the "single-digit day" date case had a two-digit day. Each case now has its own expected time; the single-digit case is 3 November.
3. The US zone names (EST, PST, EDT, CDT) have cases; with the zone map removed, all four fail (checked).
4. "The head of state of Georgia" no longer tags the state ("of state of" is refused), with a case.
5. `k8s/app/backup.yaml` is named in k8s-conventions' Layout (amended), rather than splitting the seabass pieces across `storage.yaml` and `jobs.yaml`.
Nit: the provenance-refusal cases are parallel subtests, each on its own store.

## Review findings not fixed, and why
-

## Manual verifications
- News at 360 and 1280 px, light theme · Claude · 2026-11-23 · headless Chrome on the dev machine
- News at 360 px, dark theme; Vermont's "In the news" at 1280 px, dark · Claude · 2026-11-23 · headless Chrome, forced dark
- News at 200% (a 640 px window at device scale 2): wraps, no sideways scroll · Claude · 2026-11-23 · headless Chrome
- Status page with a failed backup and a job that never ran, at 360 and 1280 px · Claude · 2026-11-23 · headless Chrome
- Reduced motion: the new pages add no animation or transition · Claude · 2026-11-23 · by reading the components
- News at 1280 px, dark; the status page at 1280 px dark and at 200% · Claude · 2026-11-23 · headless Chrome
- Vermont's page at 360 px and 200%, Sanders's page at 1280 px dark and 200%: "In the news" wraps, its link reads above the list · Claude · 2026-11-23 · headless Chrome
- Keyboard only, the news page · Claude · 2026-11-23 · headless Chrome driven over the DevTools protocol with real key events: 24 tab stops in reading order (skip link, sections, the two filters, the two checkboxes, then each headline, Mark read, Save), every one with a visible focus ring; Space on "Mark read" sets it (`aria-pressed="true"`) and announces "Marked read", again clears it ("Marked unread"); Space on "Unread only" filters and puts `?unread=1` in the address
- Sanders's page at 360 px, light: "In the news", its link and the source lines wrap, no sideways scroll · Claude · 2026-11-23 · headless Chrome
- Keyboard on the member and state pages' "In the news": the same `StoryList` (headline link, mention links, Read and Save buttons) as `/news`, checked there; the section adds one link above the list · Claude · 2026-11-23 · by reading the components
- The owner's phone, and keyboard on the owner's own laptop: pending, the owner, after the deploy
