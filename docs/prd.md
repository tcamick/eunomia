# PRD — Eunomia

> **Status:** v1.0, accepted by the owner on 2026-11-22 with every recommendation in §13. Owner: Cole. Written from the owner's project prompt of 2026-11-22.
> Items marked *(proposed)* were additions to the prompt; the owner accepted all of them.
> Companions: `docs/implementation-plan.md` (decisions and how each phase is built), `docs/roadmap.md` (order and status), `AGENTS.md` and `docs/development-workflow.md` (how the work is done).
> Infrastructure follows `~/git/beholder` (neptune: k3s, Flux, Tailscale, Prometheus). Stack follows `~/git/plutus` (`app/`, `docs/go-conventions.md`, `docs/web-conventions.md`, `docs/development-workflow.md`).

## 1. Problem
The owner cares about politics and keeps losing track of it. Work and hobbies take the time that following the news would need, so after a few busy weeks he no longer knows who holds which office, what Congress did, or which races are coming. Catching up means reading many sites with no memory of what he already knew, and that cost is why it does not happen.

There is no single place that answers, in a few minutes: who governs me, who governs the country, what changed since I last looked, and what does this term or procedure mean.

## 2. Goals
1. **A map of the United States** that opens into each state and shows its political leaders, of every party, as people with faces, offices and terms.
2. **A dashboard of Congress:** who is in it, the balance of each chamber, leadership and committees.
3. **Elections:** which races are coming, who is running, and what the result was.
4. **A news feed** from NPR and Democracy Now, tied to the people and states it mentions.
5. **A learning center:** short explanations of how government works, reachable from wherever the term appears.
6. **Catching up in minutes** *(proposed)*: a home page that shows what changed since the last visit, starting with the owner's own representatives. This is the direct answer to §1; the other goals are what it draws on.
7. **Later, for fun:** a collectible-card presentation of political figures (§6.9).

## 3. Non-goals and principles
- **One user. Not a product.** No accounts, no multi-tenancy, no public exposure. The tailnet decides who can open it, as with Grafana in beholder.
- **Not a newsroom.** Eunomia stores a story's headline, summary, source, time and link, and sends the owner to the publisher to read it. It does not store or show article text.
- **United States only for now:** federal first, then states, then the owner's own city. Other countries are out of scope.
- **No predictions.** No polling averages, forecasts or race ratings of its own. It may link to them.
- **No advocacy tooling.** No donation links, petitions or contact-your-representative forms in v1.
- **Every fact has a source and a date.** A person's office, a vote, a candidacy: each row records where it came from and when it was fetched, and the page shows it. Nothing is written from a model's memory. A curated fact (a Cabinet post, a Supreme Court seat) names its source and goes stale visibly.
- **Every party gets the same page.** The owner chose the news sources. That choice does not change which officials are shown, which fields a profile has, or how a number is computed.
- **The 2026 midterm is over.** It was on 2026-11-03. The 119th Congress is finishing its term and the 120th convenes on or about 2027-01-03, so rosters change within weeks of this draft. The data model treats a change of Congress as an ordinary refresh, not a migration (§10).

## 4. User and cadence
One user: a software engineer in Oakland, California. Comfortable with technical detail; short on time. Expected use is a few minutes several times a week and a longer session on a weekend, not daily reading.

Home is Oakland, so "my representatives" means the President, California's two senators, Oakland's House member, the governor, the state senator and assembly member, and the mayor and council member. The exact districts are set once in a config file (§6.1), not looked up by address.

Devices (owner, 2026-11-22): Home and the feed are designed for a phone, since catching up happens in spare minutes; the map and the Congress pages for a laptop. Every page still works on both.

## 5. Jobs to be done
| When… | I want to… | So that… |
|---|---|---|
| I have been away for two weeks | see what changed since my last visit, my own representatives first | I am current in five minutes, not an evening |
| I hear a senator's name | open one page with their state, party, committees, recent votes and recent stories | I know who they are and what they have been doing |
| I wonder about a state | click it on the map and see its governor, senators and House delegation by party | I have a picture of who runs it |
| A vote is in the news | see how each chamber voted, by party, and how my own members voted | I know what happened, not just that it happened |
| An election is coming | see the races on my ballot and the important ones elsewhere, with who has filed | I am not learning the candidates the week before |
| A story uses a word I half know (cloture, reconciliation, discharge petition) | open a short explanation from that word | I learn it at the moment I need it |
| I have ten spare minutes | read today's headlines from sources I trust, grouped by who they are about | following the news does not need five tabs |

## 6. Functional requirements

### 6.1 Home *(proposed)*
- **My representatives:** the list in §4, from `config/home.yaml` (state, congressional district, state legislative districts, city, council district). Each shows the person, the office, the next election for that seat, and their latest vote and story.
- **Since you last looked:** the server records each visit. Home lists what is new since the previous one: roster changes (sworn in, resigned, died, switched party, new leadership or committee chair), notable votes, signed laws and executive orders, election dates passed or approaching, and new stories about the owner's representatives. Each line links to its page.
- **Coming up:** the next dated events from the election calendar (§6.5) and, where the source gives them, scheduled floor votes.

### 6.2 Map and state pages
- A map of the 50 states, DC and the territories that send delegates. Clickable and keyboard-reachable; each state is also in a plain list, since a map is a poor control on a small screen.
- Colour modes: governor's party, Senate delegation, House delegation split, next election date. One mode at a time, with a legend, and never colour alone (a label or pattern too).
- **State page:** governor and lieutenant governor; both US senators; the House delegation, as a district map of the state and a list; party totals; the state's next elections; recent stories that mention the state or its officials. State legislative leaders and chamber control for every state; the full legislature only for California at first (§13, question 3).
- Every party and independents are shown, with a party's own name and colour. Vacant seats are shown as vacant with the reason and the date of any special election.

### 6.3 People
One page per person, whatever office they hold:
- Photo, name, party, current office, state and district, when the term began and ends, when the seat is next up.
- Past offices, as a timeline.
- For members of Congress: committees and leadership posts, sponsored bills, recent roll-call votes, how often they vote with their party, and an ideology score from Voteview with a line explaining what it measures.
- For candidates: the office sought, filing status and, for federal races, money raised and spent from the FEC.
- Recent stories that mention them.
- Links out to their official site and to the source of each fact.
- Search by name, state or office from every page.

### 6.4 Congress
- **Chambers:** seats by party as a hemicycle or seat grid, the majority threshold, vacancies, and the leadership of each party.
- **Committees:** each committee's chair, ranking member and members.
- **Votes:** recent roll calls in each chamber with the question, the result, the split by party, and each member's position. The owner's members are marked.
- **Bills:** those that had a floor vote, became law, or are named in a tracked story. Title, sponsor, status and a link to Congress.gov. Not a full bill tracker.
- **The executive and the courts:** the President, Vice President and Cabinet; the Supreme Court; recent executive orders from the Federal Register. The Cabinet and the Court have no clean feed, so they are a curated file (§7).
- **A Congress is a parameter.** The 119th and 120th can be compared: who left, who arrived, which seats changed party.

### 6.5 Elections
- **Calendar:** election days, primaries, runoffs, special elections, filing deadlines and swearing-in dates, federal and for every state, plus Oakland's.
- **Races:** one page per race with the office, the incumbent, the candidates and their parties, and, for federal races, FEC filings. After the election: the result and its source.
- **My ballot:** the races the owner votes in, including California propositions and Oakland measures, each with a link to the official voter guide.
- Structured data on candidates is good for federal races and thin below that (§7). State and local races start as a curated file for the owner's own ballot and the governors' races.

### 6.6 News feed
- Sources at first: NPR Politics, NPR News and Democracy Now, read from their RSS feeds. *(Proposed)* CalMatters and The Oaklandside for California and Oakland, *once their terms for feeds are clear (owner, 2026-11-23)*.
- Each story shows the source, time, headline, the publisher's summary and a link. The source is always labelled, because the feed mixes a public broadcaster with an openly left publication.
- **Tagging:** a story is linked to the people, states, bills and terms it names, by matching against the roster. Matching is deterministic first (full names, "Sen. Surname" with a state check); a miss is better than a wrong link. A person's page and a state's page show their stories.
- Filters: source, mine (my representatives and state), state, person, topic. Read and saved marks.
- Feeds keep only recent items, so Eunomia's own store is the archive. It is not refetchable and is backed up (§8).

### 6.7 Learning center
- Short pages, a few hundred words, on how the government works: how a bill becomes law, the filibuster and cloture, reconciliation, committees, the appropriations process and shutdowns, confirmations, executive orders, the courts, primaries and runoffs, redistricting, California's proposition system, how Oakland's city government is organised.
- **A glossary** whose terms are linked wherever they appear in the app: in a vote's question, a bill's status, a story's headline.
- **Context where the question arises:** a vote page links to the explanation of that kind of vote; a race page explains that state's primary system.
- Content is markdown in the repository (`learn/`), with a source list and an as-of date on each page, rendered read-only by the app. It is written and revised in Claude Code sessions under the facts rule in §3.
- *(Proposed, later)* a short quiz at the end of a page, which the cards in §6.9 can use.

### 6.8 The brief *(proposed, later)*
A weekly summary written by Claude from what Eunomia stored that week: roster changes, votes, laws and orders, and the stories about the owner's representatives, in a few paragraphs with links. Shown on Home; delivery by email or push is question 9 in §13.

Stories are text that strangers wrote, so the run follows Plutus ADR 0012: no tools, input quoted between markers, output stored and shown only. Eunomia holds almost no private data, which makes the risk smaller than in Plutus, but the rule costs nothing.

### 6.9 Cards *(later)*
- Each figure gets a card: portrait, office, party, state, and a few stats.
- **Stats come from data, by one published formula for everyone:** years in office, bills sponsored and enacted, party-line voting rate, committee and leadership posts, the Voteview score, money raised, margin of the last win. A card links to how each number was computed. No stat is an opinion.
- Cards are a view of the same person record, not a second data set. A card flips or opens to the person's page.
- *(Proposed)* a card is collected by learning: reading the person's page, or passing the quiz on a related topic. Decks could be a state, a committee or a Congress.
- **An original design.** Its own frame, layout, icons, vocabulary and rules. It takes the idea of a collectible card game, not the look, symbols or terms of Magic: The Gathering or any other published game.
- Game rules beyond collecting are undecided (§13, question 10).

## 7. Data sources
Each was requested on 2026-11-22 and answered, except where the last column says otherwise. Keys, limits and licences still need reading before each is built on.

| Need | Source | Access | Notes |
|---|---|---|---|
| Members of Congress: terms, party, ids, contact | `unitedstates/congress-legislators` (`legislators-current.json`, also historical) | no key | 539 current records; carries Bioguide, FEC, Voteview (ICPSR) and Wikidata ids, which join the other sources |
| Committees and membership | same project (`committee-membership-current.json`) | no key | |
| Portraits | `unitedstates/images`, by Bioguide id | no key | check each image's licence note |
| Bills, member detail, House votes | Congress.gov API v3 (`api.congress.gov`) | free key from api.data.gov | `member`, `bill` and `house-vote` answered with the demo key |
| Roll-call votes | House Clerk XML (`clerk.house.gov/evs/<year>/roll<nnn>.xml`); Senate vote XML on senate.gov | no key | the primary source for both chambers |
| Ideology scores | Voteview member files (DW-NOMINATE) | no key | academic; explain what it measures |
| Executive orders, proclamations | Federal Register API | no key | |
| Federal candidates and campaign money | FEC API (`api.open.fec.gov/v1`) | same api.data.gov key | federal races only |
| Governors, state legislators, some city officials | `openstates/people` repository (`data/<state>/executive`, `legislature`, `municipalities`) | no key | layout confirmed for California; the Open States v3 API needs a key (403 without one) |
| Oakland council bodies, meetings, legislation | Legistar web API (`webapi.legistar.com/v1/oakland`) | no key | |
| State shapes | `us-atlas` TopoJSON (from Census boundaries) | no key | has pre-projected Albers files |
| Congressional district shapes | Census cartographic boundary file `cb_2024_us_cd119_500k` | no key | **119th Congress only.** Several states redrew their maps for 2026, so the 120th needs the new file when the Census Bureau publishes it |
| News | NPR Politics (`feeds.npr.org/1014/rss.xml`), NPR News (`1001`), Democracy Now (`democracynow.org/democracynow.rss`) | no key | Democracy Now's feed carries full text; Eunomia keeps only the summary |
| Local news *(proposed)* | CalMatters and Oaklandside RSS | no key | |

**No feed exists; curated in the repository** (`config/figures.yaml`, each entry with a source URL and a checked-on date, and a job that flags entries older than a set age): the Cabinet, the Supreme Court, party leadership outside Congress, and candidates in state and local races.

**Not available** (from memory, not rechecked today): ProPublica's Congress API was retired; Google's Civic Information API no longer returns representatives; Ballotpedia's API and AP election results are paid. Election results therefore come from each state's election office, entered or scraped per race, and history from MIT Election Lab if wanted.

## 8. Non-functional requirements
- **Hosting:** neptune, as its own namespace, at `https://eunomia.<tailnet>.ts.net` through the Tailscale operator's Ingress. Nothing listens outside the tailnet.
- **Access:** the tailnet identity alone; no password. A NetworkPolicy admits the `tailscale` namespace to the server and `monitoring` to the metrics port.
- **Freshness:** news every 30 minutes (amended 2026-11-23, E2: so a story appears within the hour); roster, committees, votes and bills daily; FEC and state data weekly; the calendar and curated figures checked for staleness weekly. Every page shows when its data was last fetched, and a status page lists each job's last success.
- **Politeness to sources:** a named User-Agent without personal details, conditional requests (`ETag`, `If-Modified-Since`) for feeds, a pace between requests, and backoff on errors.
- **Durability:** the SQLite file is backed up nightly to seabass with Plutus's method (`VACUUM INTO`, dated directories). Most of it can be refetched; the news archive, read and saved marks, and visit history cannot.
- **Observability:** Prometheus metrics with bounded labels; a request counter and duration histogram for every outside source; a PodMonitor and a Grafana dashboard ConfigMap in this repository (beholder README, "Adding an app's metrics"). Batch jobs push their counters to beholder's Pushgateway once at exit (beholder ADR 0002).
- **Cost:** $0 a month for data. The brief (§6.8) is the only feature with a running cost.
- **Privacy:** Eunomia holds the owner's home districts, visit times and reading marks, and no credentials for anything personal. No request to an outside service carries the owner's name, email or location.
- **Accessibility and devices:** follows Plutus's `web-conventions.md`: semantic HTML, keyboard access, `rem` type, light and dark themes, works at phone width.
- **Maintenance:** under an hour a month in steady state. A new Congress, a new session's vote URLs and a redistricting are data refreshes or config edits, not code changes.

## 9. Architecture (summary; ADRs to follow)
- **Backend: Go, one binary** with subcommands, as in Plutus: `eunomia serve`, `eunomia job <name>`, `eunomia version`. Stdlib `net/http` and `flag`, zerolog, Prometheus client, testify. Each source is an adapter behind an interface defined by its consumer, tested against recorded fixtures in `testdata/`.
- **Storage: SQLite only** (`modernc.org/sqlite`). The data is small and relational: a few thousand people, tens of thousands of votes and stories. Plutus's DuckDB store has no job here, and leaving it out removes CGO, so the image is a static binary on `distroless/static`. Revisit if vote analytics outgrow it.
- **Frontend: Svelte 5 + TypeScript + Vite,** built to static assets and embedded with `embed.FS` (Plutus ADR 0004); a thin client over a JSON API; installable as a PWA.
- **The map is SVG,** drawn from `us-atlas`'s pre-projected shapes and converted to paths at build time, so the browser needs no mapping library. District maps are generated the same way from the Census file. Charts are hand-written SVG until one needs more.
- **Jobs:** each fetch is a `job` subcommand, run by Kubernetes CronJobs that share the data volume with the server (single node, so one volume serves both). The server only reads and records visits and marks (owner, 2026-11-22; ADR 0003 records it).
- **Content in the repository:** `learn/` (markdown), `config/` (home, sources, curated figures, election calendar). *Amended in the implementation plan (2026-11-22; was a `git-sync` sidecar as in Plutus):* both are embedded in the binary. An edit reaches the cluster with the next deploy, which is one command and about two minutes, and the pod needs no sidecar, deploy key or second secret.
- **Deploy, as beholder and Plutus do it:**
  - `k8s/` in this repository holds the namespace, server, CronJobs, storage, network policies, Tailscale Ingress, PodMonitor and dashboard. Flux applies it.
  - The `GitRepository` and `Kustomization` pointing here are declared in Plutus's `deploy/k8s/clusters/neptune/` beside `beholder.yaml`, until the cluster has its own repository (Plutus ADR 0015). They depend on `tailscale` (the ingress class) and, for the monitoring objects, on `beholder` (the PodMonitor CRD).
  - Flux reads this repository with its own read-only deploy key (`eunomia-git-credentials`), made once by hand as in beholder's README.
  - Images are built on neptune, pushed to its loopback registry and pinned by digest in `k8s/`. A deploy is a commit to `main`.
  - API keys (api.data.gov, and Open States if its API is used) come from Google Secret Manager through the External Secrets Operator, which needs a grant added in Plutus's `deploy/tofu` (§13, question 6).
  - `claude-ops` gets a read-only Role in the namespace, as in beholder's `claude-ops.yaml`.
- **Repository layout:** `cmd/eunomia/`, `internal/`, `web/`, `k8s/`, `config/`, `learn/`, `docs/` (this PRD, `adr/`, a roadmap). Plutus's Go, web and workflow conventions are adapted into this repository's own `docs/` (owner, 2026-11-22), with what existed for private financial data removed.

## 10. Data model (core)
The central choice: **a person and the offices they hold are separate.** A change of Congress, a party switch, a resignation or a candidacy is a new or closed row, and history is kept.

- `people(id, name, born, portrait, bioguide, fec_id, icpsr, openstates_id, wikidata)`
- `terms(person, office, body, state, district, party, caucus, start, end, how, current, left_on, unlisted_on, source, fetched_at)`: who held a seat, when. In Congress, `office` is a text id for the seat (`senate/<state>/<class>`, `house/<state>/<district>`).
- `leadership(person, chamber, title, start, end, unlisted_on, source, fetched_at)` · `committees(id, parent, chamber, name, current, …)` · `committee_seats(committee, person, side, rank, title, unlisted_on, source, fetched_at)`: leadership posts and committee seats. A row the source stops listing is dated (`unlisted_on`), never deleted.
- `offices(id, level, branch, jurisdiction, title)`: added in E3b, when governors and state legislators need seats that are not Congress's; the Congress ids above stay as they are. *(Amended 2026-11-23, owner: E1a's model kept; it replaced the planned `offices` and `roles` tables for Congress.)*
- `bills(id, congress, type, number, title, sponsor, status, updated)` · `votes(id, chamber, congress, session, number, date, question, result, bill)` · `positions(vote, person, position)`
- `races(id, office, election_date, kind, result_source)` · `candidacies(race, person, party, status, votes, won)` · `calendar(date, jurisdiction, kind, title, source)`
- `stories(id, source, guid, url, title, summary, published, fetched_at)` · `mentions(story, kind, ref)`: the tags from §6.6
- `documents(id, kind, number, title, signed, url)`: executive orders and proclamations
- `changes(id, at, kind, subject, summary, source)`: what a job found different from the last run (sworn in, left office, switched party, new chair, vote held, law signed). Home's "Since you last looked" reads this.
- `visits(at)` · `marks(story, read_at, saved_at)` · `jobs(name, last_run, last_ok, message)`

## 11. Phases
Each phase is a branch with its own acceptance file, as in Plutus. The order after E2 is the owner's to change. `docs/roadmap.md` splits several of these into smaller phases (E1a and E1b, and so on) and holds their status.

| Phase | What | Done when |
|---|---|---|
| **E0** Skeleton | The binary with `serve` and `/healthz`, an empty Svelte shell, the image, `k8s/`, the Flux wiring in Plutus, the deploy key, metrics scraped | the page opens at `eunomia.<tailnet>.ts.net` and its target is up in Prometheus |
| **E1** Congress and the map | Roster and committee jobs, the map, state pages (federal delegation), person pages, chamber balance, search | clicking any state shows its senators and House members with photos and parties, and every number matches the source file |
| **E2** News | Feed jobs, the feed page, tagging to people and states, read and saved marks | a story naming a senator appears on that senator's page, with no wrong links in a hand-checked week |
| **E3** Home and the rest of government | `home.yaml`, My representatives, Since you last looked, governors and state leaders, the Cabinet and the Court | after a week away, Home lists the roster changes and stories of that week |
| **E4** What Congress did | Votes, bills, executive orders, party-line and Voteview numbers on person pages | a roll call from yesterday shows the party split and how the owner's members voted |
| **E5** Learning center | `learn/` pages, the glossary, term links across the app | the terms in a week's vote questions each open an explanation |
| **E6** Elections | Calendar, races, candidates, FEC money, My ballot, 119th-to-120th comparison | the owner's next ballot is listed with every candidate who has filed |
| **E7** The brief | The weekly Claude run and, if wanted, delivery | a brief is on Home on Monday morning with working links |
| **E8** Cards | The card view, the stat formulas, collecting | every member of Congress has a card whose stats trace to data |

The 120th Congress convenes during E1 or shortly after. E1 is built so that the roster job picks up new members when its source does, and departing members stay in history.

## 12. Success criteria (three months after E3)
- The owner opens Eunomia at least weekly, and a catch-up after a week away takes under ten minutes.
- Without looking, the owner can name their own representatives at every level and which party controls each chamber of Congress.
- Every person page shows a source and a fetch date for each fact, and no roster fact is more than two days behind its source.
- No story is linked to the wrong person in a monthly spot check of fifty.
- Steady-state maintenance is under an hour a month.

## 13. Decisions on the open questions (owner, 2026-11-22)
The owner accepted every recommendation of the draft. Each is recorded here as decided.

1. **News sources:** ship with NPR and Democracy Now (plus CalMatters and The Oaklandside for local news) and label every source. After E2, add a wire service and a conservative outlet if the feed feels one-sided.
2. **Home (§6.1):** built as described.
3. **Depth outside California:** governors, state leaders and chamber control for every state; the full legislature for California only.
4. **Oakland:** the mayor and council member appear in My representatives from a curated file in E3. Council meetings and legislation from Legistar wait until they are missed.
5. **Jobs:** Kubernetes CronJobs, not an in-process scheduler.
6. **API keys:** Google Secret Manager, read by the External Secrets Operator, with the grant added in Plutus's OpenTofu.
7. **The 120th Congress before it is sworn in:** not shown. It appears when `congress-legislators` lists it.
8. **Devices:** Home and the feed for a phone; the map and Congress pages for a laptop.
9. **The brief is sent:** a weekly email or push, in E7. The channel is chosen there.
10. **Cards:** a collection and a quiz reward first. A game with rules is decided after living with them.
11. **Races below the federal level:** the owner's own ballot and the governors' races are curated by hand; the rest link out.
