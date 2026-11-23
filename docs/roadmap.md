# Roadmap — Eunomia

> Started 2026-11-22 (owner). Spec: [`docs/prd.md`](prd.md). How each phase is built: [`docs/implementation-plan.md`](implementation-plan.md). Workflow: [`docs/development-workflow.md`](development-workflow.md), `/phase <id>` and `/ship`.
> This file holds the order of the phases, what each must show before it merges, and the status table. Update the table in the branch that completes a phase.

## Principles
1. **Catching up comes first.** The owner's problem is losing track in busy weeks (PRD §1). Each phase is judged by whether it makes "what changed, and who is this?" faster to answer.
2. **Every phase ends on neptune.** A phase is done when the owner can open its pages at `https://eunomia.<tailnet>.ts.net`, not when its tests pass.
3. **Sourced and dated, or not shown** (PRD §3; `AGENTS.md`, Rules for facts).
4. **Small upkeep.** A phase that adds a moving part (a secret, a curated file, a new source) says what keeping it costs.
5. **Phone for reading, laptop for exploring** (PRD §4).
6. **The calendar does not wait.** The 120th Congress convenes on or about 2027-01-03. The roster must be running before then, so the change is seen as it happens (gate G1).

## Order
```
E0 ─► E1a ─► E1b ─► E2 ─► E3a ─► E3b ─► E4a ─► E4b ─► E5 ─► E6a ─► E6b ─► E7 ─► E8
        │                   ▲
        └── roster running before 2027-01-03 (G1)
```
E0 to E2 are fixed. After E2 the order is the owner's to change. What depends on what: E3a needs E1a and E2; E4b needs the owner's API key; E6b needs E4b's key; E7 needs E3a; E8 needs E4a and E5. E5 depends on nothing after E1a and can move earlier.

## E0 — Skeleton and deploy (1–2 sessions)
The empty app, built, deployed and monitored the way every later phase will be.
- **Needs from the owner:** the GitHub repository, Flux's deploy key, and a go-ahead for the change in Plutus (plan §3).
- **Done when:**
  - `make test lint` pass on neptune and in the image's check stage.
  - The Status page opens at `https://eunomia.<tailnet>.ts.net` and shows the `noop` job's last run.
  - The `noop` CronJob has run on the cluster, written its row through the shared volume, and its pushed counters are in Prometheus.
  - The server's target is up in Prometheus.
  - `make deploy` deployed `main`, and `make deploy-status` shows pinned, `main` and the cluster in agreement. *(Amended 2026-11-22: E0 is merged by hand, so `make land` is first checked by E1a.)*
  - A request from a pod outside the `tailscale` namespace to the server's port is refused.

## E1a — Roster and people (2 sessions)
*Who is in Congress?*
- **Done when:**
  - The roster job has run on the cluster at least twice, and the later runs wrote only what the source changed.
  - The Congress page shows each chamber's seats by party, the majority threshold, vacancies and leadership, and the totals match a count of the source file made by hand that day.
  - Any member's page shows photo, party, state or district, term dates, committees and leadership posts, each with its source and fetch date.
  - Search finds a member by part of their name, by state and by committee.
  - It lands with `make land`, the first branch to do so (E0 is merged by hand), and `make deploy-status` then shows pinned, `main` and the cluster in agreement.
  - The look is recorded in ADR 0007, and the party colours pass the palette checks in both themes.
  - The pages have been looked at on a phone-width and a laptop-width window, by keyboard alone, in both themes.

## E1b — The map and state pages (2 sessions)
*Who represents each state?*
- **Done when:**
  - Clicking any state, or choosing it from the list, opens its page with both senators and every House member, with photos and parties.
  - Each colour mode's map agrees with the list beside it for every state.
  - Every state can be reached and opened with the keyboard alone, and by touch at 360 px through the list.
  - A state's district map loads only on that state's page (the bundle check shows it), and is labelled with its Congress.
  - A vacant seat is shown as vacant.

## E2 — News (2 sessions)
*What are my sources saying, and about whom?*
- **Needs from the owner:** the backup directory on seabass.
- **Done when:**
  - The feed shows stories from all five sources, each labelled, within an hour or so of publication.
  - A story that names a senator appears on that senator's page and their state's page.
  - Marking a story read on one device shows it read on another.
  - No article body is in the database (a query in the acceptance file shows the longest stored summary).
  - Fifty mentions from the first day's stories are checked by hand and none is wrong (the week-long check is gate G3).
  - A backup restores into a working copy, and a missing disk shows as a failed backup on the status page.

## E3a — Home (1–2 sessions)
*What changed since I last looked?*
- **Needs from the owner:** the home districts.
- **Done when:**
  - Home lists the owner's representatives at every level, each with office, term and latest story.
  - With the previous visit a week back, Home lists that week's roster changes and stories about the owner's representatives, and nothing older.
  - Home is comfortable to read on the owner's phone (manual), and the app installs to its home screen.

## E3b — The rest of government (2 sessions)
*Who runs the states, the executive and the Court?*
- **Done when:**
  - Every state's page shows its governor and each chamber's party totals; California's shows its full legislature.
  - `/government` shows the President, Vice President, Cabinet and Supreme Court, each with a source and a checked date.
  - The map's governor mode agrees with the state pages.
  - An entry in `figures.yaml` made stale on purpose shows on the status page.

## E4a — Votes (2 sessions)
*What did Congress decide, and how did my members vote?*
- **Done when:**
  - A roll call from the previous day in each chamber shows the question, the result, the party split and every position, with the owner's members marked.
  - The party totals for ten votes chosen at random equal the source's own totals.
  - A member's page shows recent votes and a party-line rate that links to how it is computed.
  - Home shows notable votes since the last visit.

## E4b — Bills, laws, orders and scores (1–2 sessions)
*What became law, and what did the President sign?*
- **Needs from the owner:** the api.data.gov key in Secret Manager and `tofu apply`; and, before that key reaches the `eunomia` namespace, a decision on `claude-ops`' Job grant there, since a Job it creates could mount the key (k8s-conventions rule 11).
- **Done when:**
  - A vote on a bill links to the bill's page, which shows title, sponsor and status and links to Congress.gov.
  - Laws signed and executive orders issued since the last visit appear on Home.
  - A member's page shows their Voteview score with a line on what it measures.
  - The key is not in any log, error or stored URL (a test and a search of a day's logs).

## E5 — Learning center (2 sessions, then ongoing)
*What does this word mean, and how does this work?*
- **Done when:**
  - The first twelve pages and forty terms are written, sourced and dated, and the owner has read them.
  - Every glossary term in one week's vote questions and bill statuses opens its definition.
  - A vote's page links to the explanation of that kind of vote.

## E6a — Calendar and My ballot (1–2 sessions)
*What is coming, and what will I vote on?*
- **Done when:**
  - The calendar shows the next federal, California and Oakland dates, and each state's next statewide election, each with its source.
  - Coming up on Home shows the next dated events.
  - `/ballot` lists the owner's next ballot as far as it is known, with links to the official guides, and says what is not yet known.

## E6b — Races and candidates (2 sessions)
*Who is running?*
- **Done when:**
  - Each federal race for the next cycle shows the candidates who have filed with the FEC and their totals.
  - A sitting member who is running is one person with both roles.
  - `/congress` compares the 119th and the 120th: who left, who arrived, which seats changed party, and the counts match a hand count.

## E7 — The brief (2 sessions)
*Tell me the week in three paragraphs.*
- **Needs from the owner:** the decisions in ADR 0010.
- **Done when:**
  - A brief written by the scheduled job on the cluster is on Home, every reference in it opens the row or story it cites, and its sources are listed.
  - It arrives by the channel chosen.
  - A week with nothing to say produces a short brief that says so.

## E8 — Cards (2–3 sessions)
*Make it fun.*
- **Done when:**
  - Every member of Congress has a card, and each stat on it links to its formula and its inputs.
  - Reading a person's page or passing a quiz collects their card, on both devices.
  - The design is recorded in ADR 0011 as original work.

## Gates that wait on the calendar
| Gate | What | When |
|---|---|---|
| **G1** The new Congress | After the 120th is sworn in: new members have appeared, departed members have end dates and keep their pages, Home listed the change, the seat totals match the Clerk's and the Senate's own, and the votes job follows the new Congress's numbering | from 2027-01-03, checked again when committee rosters fill in |
| **G2** New district shapes | When the Census Bureau publishes the 120th Congress's district file: regenerate, and remove the "119th" label | when published |
| **G3** Tagging by hand | A week of real stories, fifty mentions checked, none wrong | a week after E2 deploys |
| **G4** The owner's devices | Each UI phase's manual line: the owner's phone and laptop | with each UI phase |

## Later
Ideas that are not phases yet; each needs the owner's word.
- A wire service and a conservative outlet in the feed (PRD §13, decision 1), after living with E2.
- Oakland council meetings and legislation from Legistar (decision 4).
- The full legislature for states other than California; it is a display choice once E3b stores them all.
- Committee hearings and the floor schedule in Coming up.
- A game with rules for the cards (decision 10).
- CI on GitHub (`docs/development-workflow.md` §5).
- Countries other than the United States.

## Status
| Id | What | Status | Branch | Acceptance | Merged |
|---|---|---|---|---|---|
| — | PRD, plan, roadmap, conventions, agent files | done (owner reviewed, 2026-11-22) | `main` (root commit) | — | 2026-11-22 |
| E0 | Skeleton and deploy | merged; Prometheus and NetworkPolicy checks wait on the owner | `feat/phase-e0-skeleton` | [phase-e0](acceptance/phase-e0.md) | 2026-11-22 |
| E1a | Roster and people | merged; keyboard check and two cluster runs of the roster pending | `feat/phase-e1a-roster` | [phase-e1a](acceptance/phase-e1a.md) | 2026-11-23 |
| E1b | The map and state pages | not started | | | |
| E2 | News | not started | | | |
| E3a | Home | not started | | | |
| E3b | The rest of government | not started | | | |
| E4a | Votes | not started | | | |
| E4b | Bills, laws, orders and scores | not started | | | |
| E5 | Learning center | not started | | | |
| E6a | Calendar and My ballot | not started | | | |
| E6b | Races and candidates | not started | | | |
| E7 | The brief | not started | | | |
| E8 | Cards | not started | | | |
| G1 | The new Congress | waiting for 2027-01-03 | | | |
| G2 | New district shapes | waiting for the Census file | | | |
| G3 | Tagging by hand | waiting for E2 | | | |

## Immediate next steps
Updated 2026-11-22.
1. **E0**, then **E1a** straight after it, so the roster is running well before 2027-01-03. The repository exists and the owner gave the go-ahead for the change in Plutus (2026-11-22); Flux's deploy key is the owner's, due within hours.
