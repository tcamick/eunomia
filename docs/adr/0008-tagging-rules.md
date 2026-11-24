# ADR 0008 — Tagging rules

**Date:** 2026-11-23 · **Status:** accepted · **Relates to:** PRD §3, §6.6; go-conventions, Provenance

## Context
A story should appear on the page of each member of Congress and each state it names (PRD §6.6). Names are ambiguous: two senators share a surname, "Washington" is a city and a state, "Georgia" is a country, "Virginia Foxx" is a person. A wrong link puts a story on the wrong official's page, which misleads; a missed link only leaves it off one page. Matching people by name happens nowhere else in Eunomia (go-conventions, Provenance).

## Decision
`internal/tagging` matches a story's headline and summary against the sitting members (from the roster) and the states, DC and territories (from `internal/geo`), deterministically, with no model:
- **A member's full name tags them:** the display name the source gives, first and last, or nickname and last. Matching is case-sensitive and on word boundaries that understand accented letters, so "Velázquez" matches and "Scotty" does not match "Scott". A full name two sitting members share tags neither.
- **A title and a surname tag a member** ("Sen.", "Senator"; "Rep.", "Representative", "Congressman", "Congresswoman") only when the surname belongs to one member of that chamber, or to several of whom exactly one sits for the state named right after it ("Sen. Scott of Florida" tags Florida's), or else exactly one is named in full in the same text ("Tim Scott … Sen. Scott"). "Sen. Scott" alone tags no one while two senators are named Scott, and a state named elsewhere in the text does not choose ("Sen. Scott (R-S.C.) criticized Florida's governor"; amended 2026-11-23, at review). A House title never tags a senator, or the reverse. A title after "state", "former", "late", "onetime", "retired", "ex-" or "then-" ("state Sen. Padilla", "former U.S. Rep. Scott", "then-Sen. Biden"; "U.S." is looked past) is someone else's and tags no one (added 2026-11-23, at review: state legislators share surnames with members). Nor does a surname followed by another capitalised word ("Rep. Grant Smith": Grant is a first name there); "Rep. Johnson, R-La." and "Sen. Sanders (I-Vt.)" still tag (added 2026-11-23, at review).
- **A state's full name tags it,** longest names first, so "West Virginia" is not also "Virginia". A name followed by "City" or "River" is the city or the river, not the state. "Washington", "Georgia" and "New York" tag the state only as "<name> state" or "state of <name>", and not after "of state of" ("the head of state of Georgia" may be the country's; amended 2026-11-23, at review).
- **Members' names are found first and blanked out,** so a state's name inside a member's name is not taken for the state; titles are then looked for in the blanked text. Former members' names are blanked out too, but tag no one: "Virginia Foxx" never becomes Virginia after the member leaves (added 2026-11-23, at review).
- A story's state page shows stories that name the state or any member who sits for it now; a member's page, stories that name them.
- Mentions are recomputed for every story of the last 30 days on each news run, writing only the differences. **Each story is read against the members who sat on the day it ran** (its publication date, or its fetch date without one; a term runs from its start through its end or the day the member left, so on the day one Congress gives way to the next both sit, and a surname they share names no one): "Rep. Davis" in a December story means a December Davis, whoever sits in January, so a change of Congress never moves an old story to someone else. Everyone else in the roster is a name blanked out, not tagged, and a full name a sitting member shares with anyone else tags no one. A run removes a person's mention only if some day in the window could find that person; an empty roster removes no one (amended 2026-11-23, at review: the first versions read old stories against today's roster, which would have dropped or moved stories when the roster turns over on 2027-01-03).

## Rejected
- **Surnames alone,** or titles with ambiguous surnames: "Johnson" and "Scott" each name two or more members.
- **Fuzzy matching or a language model:** neither gives the same answer twice for the same text, and a wrong match costs more than a miss.
- **Tagging a member's state when they are named:** the state page already includes stories naming its members; tagging the state too would count a story twice and would mark it as being about the state.
- **Bills and terms** (PRD §6.6 lists them): bills arrive with E4b and terms with the learning center (E5); they will be added to this matcher then.

## Consequences
- Some true mentions are missed: "Sen. Scott", "the senator from Ohio", demonyms ("Ohioans"), and stories about Washington state that call it "Washington". Gate G3 (a week of real stories, checked by hand) measures how many.
- Governors arrive with E3b; "Gov." will join the titles then.
- The matcher is rebuilt from the roster on each news run, so it follows the Congress the roster lists.
