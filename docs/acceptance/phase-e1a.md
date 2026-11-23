# Phase E1a — Roster and people acceptance

Branch: `feat/phase-e1a-roster` (cut from `feat/phase-e0-skeleton`, since E0 cannot land before its deploy; `main` is merged in once E0 lands) · Started: 2026-11-22 · Merged:

Roadmap `docs/roadmap.md`, E1a; plan `docs/implementation-plan.md` §4, E1a; PRD §3, §6.3, §6.4, §7, §10.

## Done-when checklist
- [ ] The roster job has run on the cluster at least twice, and the later runs wrote only what the source changed — evidence: (cluster, after landing; the CronJob runs daily at 10:23 UTC). Locally: `TestDiff_ASecondRunWithTheSameFileWritesNothing` and `TestJob_FirstRunLoadsAndTheSecondSendsTheValidatorsBack` in `internal/roster`.
- [x] The Congress page shows each chamber's seats by party, the majority threshold, vacancies and leadership, and the totals match a count of the source file made by hand that day — evidence: 2026-11-23, a Python tally of `legislators-current.json` (539 records, last term of each) independent of the Go code: Senate 53 R, 45 D, 2 I caucusing with Democrats; House 218 R, 214 D, 1 I caucusing with Republicans, 6 delegates. `/api/congress` gave the same, with 2 House vacancies (Florida 20th, Texas 23rd: 433 + 2 = 435). Screenshot of the page in both themes.
- [x] Any member's page shows photo, party, state or district, term dates, committees and leadership posts, each with its source and fetch date — evidence: `/people/T000250` at 1280 px and 390 px (photo, party mark and name, seat with state rank, current term dates, five leadership posts with dates, committees with subcommittees, service history), with the roster source and the portrait source in the page's footer, each with its fetch time.
- [x] Search finds a member by part of their name, by state and by committee — evidence: `/api/search` on the live roster: `schum` → Charles E. Schumer; `ohio` and `OH` → Ohio first (its link opens Members filtered to Ohio, 17 members, screenshot); `judiciary` → both Judiciary committees. `/search?q=ohio` screenshot at 390 px.
- [x] The look is recorded in ADR 0007, and the party colours pass the palette checks in both themes — evidence: `docs/adr/0007-the-look.md`; the dataviz skill's `validate_palette.js`, all pairs, against `--bg`: light all checks pass (CVD ≥ 14.3, normal ≥ 15.6, ≥ 3:1), dark all pass (CVD ≥ 12.1, normal ≥ 15.5, ≥ 3:1). The first tokens failed and were changed (ADR 0007).
- [ ] The pages have been looked at on a phone-width and a laptop-width window, by keyboard alone, in both themes — evidence: every page at 360 px, at 200% zoom (640 CSS px, scale 2) and in dark at 1280 px, 2026-11-23 (below); **keyboard alone not yet done** by a person.

## Sources (checked 2026-11-22)
- `https://unitedstates.github.io/congress-legislators/legislators-current.json`, `committees-current.json`, `committee-membership-current.json` — the `unitedstates/congress-legislators` project; public domain (CC0). No key, no stated rate limit; fetched once a day with conditional requests.
- `https://unitedstates.github.io/images/congress/225x275/<bioguide>.jpg` — the `unitedstates/images` project; public domain works of the US government. Fetched once per member, then conditionally.
- House seats per state: Census Bureau, "Table 1. Apportionment Population and Number of Representatives by State: 2020 Census" (`https://www2.census.gov/programs-surveys/decennial/2020/data/apportionment/apportionment-2020-table01.xlsx`), checked 2026-11-22; embedded in `internal/geo/geo.go`.

## Open questions found during the phase
- The source lists two senators whose appointed terms ended on 2026-11-03 (`end-type: special-election`) and who are still in the current file. The roster shows what the source lists, with the term's end date; it does not decide on its own that the seat is vacant. Confirm this is wanted.

## Review findings not fixed, and why
Reviewer pass 1, 2026-11-23: four blocking, fifteen should-fix, eight nits.
- **Fixed, blocking:** B1 history deleted (terms, roles and committee seats the source stops listing are now kept with `unlisted_on`; seat joins and departures and upstream corrections are recorded as changes); B2 person-page source dates (the roster line says when the dataset was checked and when this member's record last changed; the portrait line has both dates); B3 negative tests (a plan without a source or time is refused and writes nothing; every roster row carries its source and time; a Democratic, Republican, Independent and Libertarian member get the same fields); B4 this file's overclaim (line 3 corrected below).
- **Fixed, should-fix:** 1 API and read-side tests; 2 a test that serving every page writes nothing; 3 `portraits.fetched_at` NOT NULL; 4 source URLs are flags; 5 the apportionment line is dated; 6 "no longer listed from" instead of "left"; 7 party unknown has its own mark; 8 focus kept on the heading after data loads; 9 search announces only the count; 10 the delegates sentence; 11 `eunomia_roster_changes_total{kind}`; 14 the looked-at checks (below); 15 test names, `t.Run` and messages. All nits fixed except the stale Pushgateway group, which waits for the deploy.
- **Decided by the owner, 2026-11-23:**
  - 12, the data model: E1a's model is kept (text office ids on `terms`, `leadership`, `committee_seats`); the plan (§4, E1a) and PRD §10 are amended to match, and an `offices` table arrives with E3b for governors and state legislators.
  - 13, the npm dependency `@fontsource-variable/public-sans` (ADR 0007's face, self-hosted): go-ahead given.

## Manual verifications
- Headless Chrome screenshots of `make run` on the live roster: Congress at 1280 px light and dark; Person at 1280 and 390 px; Committee and Committees at 1280 px; Search at 390 px; Members `?state=OH` at 800 px; Not found · Claude · 2026-11-23 · Chrome headless on neptune
- Every page (Congress, Members, Person, Committees, Committee, Search, Status) at 360 px light, at 640 CSS px with scale factor 2 (200% zoom on a 1280 px window), and at 1280 px dark, on the live roster: nothing clipped or overlapping after one fix (below) · Claude · 2026-11-23 · Chrome headless on neptune
- Reduced motion: the only motion is the seat chart's 120 ms hover fade, declared inside `prefers-reduced-motion: no-preference`; without it the dimmed end state still applies · Claude · 2026-11-23 · code reading
- Found at 200% zoom and fixed: the Members filters' State select ran past the page edge and the Name box touched Chamber (a control's built-in minimum width); controls now shrink to their column · Claude · 2026-11-23
- Found by looking and fixed: the Congress page crashed on the Senate's missing `delegates` list; a missing space before "(senior senator)" · Claude · 2026-11-23
- Keyboard alone (Tab order, skip link, focus ring on every link and control, the heading focused after navigation): not yet · owner · — · —
- The owner's phone: not yet · owner · — · —
