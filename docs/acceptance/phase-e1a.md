# Phase E1a — Roster and people acceptance

Branch: `feat/phase-e1a-roster` (cut from `feat/phase-e0-skeleton`, since E0 cannot land before its deploy; `main` is merged in once E0 lands) · Started: 2026-11-22 · Merged:

Roadmap `docs/roadmap.md`, E1a; plan `docs/implementation-plan.md` §4, E1a; PRD §3, §6.3, §6.4, §7, §10.

## Done-when checklist
- [ ] The roster job has run on the cluster at least twice, and the later runs wrote only what the source changed — evidence:
- [ ] The Congress page shows each chamber's seats by party, the majority threshold, vacancies and leadership, and the totals match a count of the source file made by hand that day — evidence:
- [ ] Any member's page shows photo, party, state or district, term dates, committees and leadership posts, each with its source and fetch date — evidence:
- [ ] Search finds a member by part of their name, by state and by committee — evidence:
- [ ] The look is recorded in ADR 0007, and the party colours pass the palette checks in both themes — evidence:
- [ ] The pages have been looked at on a phone-width and a laptop-width window, by keyboard alone, in both themes — evidence:

## Sources (checked 2026-11-22)
- `https://unitedstates.github.io/congress-legislators/legislators-current.json`, `committees-current.json`, `committee-membership-current.json` — the `unitedstates/congress-legislators` project; public domain (CC0). No key, no stated rate limit; fetched once a day with conditional requests.
- `https://unitedstates.github.io/images/congress/225x275/<bioguide>.jpg` — the `unitedstates/images` project; public domain works of the US government. Fetched once per member, then conditionally.
- House seats per state: Census Bureau, "Table 1. Apportionment Population and Number of Representatives by State: 2020 Census" (`https://www2.census.gov/programs-surveys/decennial/2020/data/apportionment/apportionment-2020-table01.xlsx`), checked 2026-11-22; embedded as `internal/geo/apportionment.go`.

## Open questions found during the phase
- The source lists two senators whose appointed terms ended on 2026-11-03 (`end-type: special-election`) and who are still in the current file. The roster shows what the source lists, with the term's end date; it does not decide on its own that the seat is vacant. Confirm this is wanted.

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
