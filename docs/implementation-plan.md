# Implementation plan — Eunomia

> Written 2026-11-22, after the owner accepted `docs/prd.md` v1.0. Companion to `docs/roadmap.md`, which holds the order, each phase's "done when" and the status table. This document holds the decisions taken before any code, the shape of the system, and how each phase is built.
> Workflow: `docs/development-workflow.md` (`/phase <id>`, `/ship`). Standards: `docs/go-conventions.md`, `docs/web-conventions.md`, `docs/k8s-conventions.md`.
> Estimates assume focused sessions of two to four hours.

## 1. Decisions locked before E0
| Decision | Choice | Why |
|---|---|---|
| Backend | Go 1.26 (toolchain pinned to the latest patch; neptune has 1.26.11), one binary, subcommands with stdlib `flag`: `serve`, `job <name>`, `version` | The owner's language and Plutus's shape |
| Frontend | Svelte 5 + TypeScript + Vite, built to `web/dist`, embedded with `embed.FS`, installable as a PWA from E3a | One artifact; Plutus ADR 0004 |
| Store | SQLite through `modernc.org/sqlite`; WAL; plain numbered SQL migrations, embedded | Small relational data; no server to run |
| CGO | Off. The image is a static binary on `distroless/static` | No DuckDB here, so Plutus's CGO build has no reason to exist (ADR 0002) |
| Jobs | Each fetch is `eunomia job <name>`, run by a Kubernetes CronJob that mounts the same data volume as the server | PRD §13, decision 5; ADR 0003 |
| Content | `learn/` and `config/` embedded in the binary | **Amends PRD §9**, which said a `git-sync` sidecar. Embedding removes the sidecar, its deploy key and its secret; the cost is that a content edit needs a deploy, which is one command (ADR 0004) |
| Maps | Shapes generated before the build by a pinned script and committed as TypeScript modules of SVG paths; no mapping library in the browser | `docs/web-conventions.md`, Platform before code (ADR 0006) |
| Portraits | Mirrored by a job to the data volume and served from this origin | Keeps the CSP same-origin; the browser never calls a third party |
| Feeds and XML | stdlib `encoding/xml`; no feed library | RSS 2.0 and two vote formats are small |
| Markdown, YAML | `goldmark` with raw HTML escaped; `go.yaml.in/yaml/v3` | As Plutus |
| Logging, metrics | zerolog; Prometheus client on an internal port; batch counters pushed to beholder's Pushgateway | As Plutus and beholder ADR 0002 |
| Access | None in the app. The Tailscale Ingress and a NetworkPolicy decide who reaches it | One user; beholder's model |
| Secrets | None until E4b. Then Google Secret Manager through the External Secrets Operator | E0 to E4a use sources that need no key |
| Tests | testify, map-keyed table tests, hand-written fakes, `-race`; real responses trimmed into `testdata/` | `docs/go-conventions.md` |
| Lint | golangci-lint with Plutus's set; `govulncheck` | Same bar |
| Module and remote | `github.com/tcamick/eunomia`; a private GitHub repository of that name | Flux reads it as it reads `tcamick/beholder` |
| Deploy | `make land` and `make deploy`, a trimmed port of Plutus's `scripts/deploy.sh` for one image and one pin file | Same habits in every repository |
| Git | The primary checkout stays on `main`; a worktree and a branch per change; `--no-ff` | `docs/development-workflow.md` |

ADRs, one short page each, written in the phase that builds the thing: 0001 Go backend and embedded Svelte UI · 0002 SQLite only, no CGO · 0003 CronJobs sharing the data volume · 0004 content embedded · 0005 hosting on neptune from this repository · 0006 maps generated before the build (E1b) · 0007 the look (E1a) · 0008 tagging rules (E2) · 0009 secrets (E4b) · 0010 the brief (E7) · 0011 cards (E8).

## 2. Shape of the system
```
 CronJobs ── eunomia job <name> ──► internal/fetch ──► public sources
     │                                   (pace, conditional requests, generic User-Agent)
     ▼ writes
 ┌─ data volume ──────────────────┐
 │ eunomia.sqlite · portraits/    │◄── nightly backup to seabass (from E2)
 └────────────────────────────────┘
     ▲ reads; writes only visits and marks
 eunomia serve ── :8080 JSON API + embedded web app ◄── Tailscale Ingress ◄── tailnet
              └─ :8081 /metrics ◄── Prometheus (beholder)
```

### Packages (`internal/`)
`config` (flags, curated YAML) · `logging` · `version` · `store` (the only SQL) · `fetch` (the only outbound HTTP) · `jobs` (the `Job` interface, registry, runner, Pushgateway push) · `httpapi` · one adapter per source (`legislators`, `openstates`, `clerk`, `senate`, `congressgov`, `fedreg`, `voteview`, `fec`) · domain packages that turn adapter output into rows and changes (`roster`, `news`, `tagging`, `votes`, `bills`, `elections`) · `learn` (markdown, glossary, term linking) · `geo` (states, seats per state).

### Jobs and schedules (UTC; staggered)
| Job | From | Runs | What it writes |
|---|---|---|---|
| `roster` | E1a | daily | people, terms, leadership roles, committees and members, portraits, changes |
| `news` | E2 | hourly | stories, mentions |
| `backup` | E2 | daily | a dated copy of the SQLite file and portraits on seabass |
| `states` | E3b | weekly | governors, state legislators, chamber totals, changes |
| `stale` | E3b | weekly | a status line for each curated entry past its age |
| `votes` | E4a | daily | roll calls and positions for both chambers, changes |
| `bills` | E4b | daily | bills named by a vote or signed into law |
| `orders` | E4b | daily | executive orders and proclamations |
| `scores` | E4b | weekly | Voteview scores |
| `fec` | E6b | weekly | federal candidates and their totals |
| `brief` | E7 | weekly | the brief |

Every job records its outcome in `jobs`, shows on the status page, and has the no-op and idempotency tests.

### HTTP
`GET` under `/api/` returns JSON. The only writes are `POST /api/visit` and `PUT`/`DELETE /api/marks/{story}` (E8 adds the card collection). Routes of the web app: `/` (Home from E3a; the Congress page before that) · `/map` · `/states/{code}` · `/people/{id}` · `/congress` · `/committees/{id}` · `/votes` and `/votes/{id}` · `/bills/{id}` · `/government` · `/news` · `/elections`, `/races/{id}`, `/ballot` · `/learn` and `/learn/{slug}` · `/cards` · `/status`. The server answers any extension-less path with the app shell.

### Schema
PRD §10 is the model. Tables arrive with the phase that fills them, one migration each. Two rules shape every table: a person and the seats they hold are separate rows, and every row from outside has `source` and `fetched_at` (`docs/go-conventions.md`, Provenance).

## 3. What the owner provides
| When | What | How |
|---|---|---|
| E0 | A private GitHub repository `tcamick/eunomia` | github.com (`gh` is not installed on neptune) |
| E0 | Flux's read-only deploy key and its Secret | the three commands in beholder's README, "Wiring", with `eunomia` for `beholder`; needs `sudo` |
| E0 | A go-ahead for one change in the Plutus repository | `deploy/k8s/clusters/neptune/eunomia.yaml`, its line in that directory's `kustomization.yaml`, and a grant so `claude-ops` may read and nudge the `eunomia` Flux objects |
| E2 | A backup directory on seabass, with a marker the job checks for | `mountpoint -q /srv/seabass && sudo install -d -o 65532 -g 65532 -m 0770 /srv/seabass/eunomia-backups && sudo install -o 65532 -g 65532 -m 0640 /dev/null /srv/seabass/eunomia-backups/.eunomia-backup-disk` (done 2026-11-23) |
| E3a | The home districts and local officials | the owner looks up their own address on the official sites and gives the districts; the address itself is never entered here |
| E4b | An api.data.gov key, stored in Secret Manager, and `tofu apply` for its grant | signing up needs the owner's email, so the owner does it |
| E7 | How the brief is paid for and how it is sent | decided in ADR 0010 |
| any UI phase | A look on the owner's own phone | recorded as a manual line in the acceptance file |

## 4. Phases

### E0 — Skeleton and deploy (1–2 sessions)
Before the branch: rename the unborn `master` to `main`, and make the root commit from the documents and agent files (the only commit made on `main` by hand; there is nothing to branch from yet). Add the remote and push.

- `go.mod`; `cmd/eunomia` with `serve`, `job`, `version`; `internal/config` (`Bind`, `Validate`) for `-data-dir`, `-listen`, `-internal-listen`, `-log-level`, `-log-format`, `-pushgateway-url`.
- `internal/store`: open with WAL and a busy timeout, apply migrations, the `jobs` table. `internal/jobs`: interface, registry, runner (records the outcome, emits metrics, pushes once at exit), and a `noop` job.
- `internal/fetch`: the client every later adapter uses, with its counter and histogram, built and tested now so no adapter is written without it.
- `internal/httpapi`: `/healthz`, `/api/status`, the security headers, request logging with a request id, panic recovery, the embedded app with shell fallback. `/metrics` on the internal listener, with job outcomes read from `jobs` at scrape time.
- `web/`: Vite, Svelte, TypeScript; one Status page reading `/api/status`.
- `Makefile` (Plutus's targets, minus CGO, plus `web-docker`), `.golangci.yml`, `Dockerfile` (the `web`, `deps`, `tools`, `check` and `build` stages of Plutus's, on `distroless/static`, every base pinned by digest), `.dockerignore`, `.gitignore`, `README.md` with the flags.
- `k8s/app` and `k8s/monitoring` per `docs/k8s-conventions.md`; a `noop` CronJob to prove the shared volume and the push (removed when `roster` replaces it).
- `scripts/deploy.sh`: `land`, `deploy`, `status`, ported from Plutus for one image and one pin file.
- In Plutus, with the owner's go-ahead: the Flux objects and the `claude-ops` grant.
- ADRs 0001 to 0005.
- **Tests:** config validation reports every problem at once; migrations apply once; the runner records success and failure; a second `noop` run changes nothing but its timestamp; `fetch` sends the generic User-Agent, honours a 304, backs off on 429 and stops at the size limit; the headers are on every response; an unknown asset is a 404 and an unknown page is the shell.

### E1a — Roster and people (2 sessions)
- **Schema:** `people`, `terms` (a text office id per seat), `leadership`, `committees`, `committee_seats`, `changes`, `sources`, `portraits`; the states and seats per state (the 2020 apportionment, with its Census source) embedded in `internal/geo`. *(Amended 2026-11-23, owner: this replaced the planned `offices` and `roles` tables; an `offices` table arrives with E3b.)*
- **`legislators` adapter** for `legislators-current.json`, `committees-current.json` and `committee-membership-current.json`. Checked on 2026-11-22: 539 records; ids for Bioguide, LIS, FEC, ICPSR and Wikidata; terms carry type, state, district, party, start and end; `leadership_roles` on 40 members; committee members carry rank and title.
- **`roster` job:** upsert people by Bioguide id; reconcile terms; write a change for each arrival, departure, party change and leadership or chair change. A file with fewer than 500 members is rejected. A member missing from a later file gets an end date, never a delete. Portraits are mirrored by Bioguide id with conditional requests; a missing one is recorded as missing.
- **Vacancies** are seats the apportionment expects and no current term fills.
- **API:** chamber summary (seats by party, majority threshold, vacancies, leadership), people (filter by chamber, state, party), one person, committees, one committee, search over names, states and offices.
- **The look** (ADR 0007): with the `frontend-design` skill, choose the faces, the tokens, the shell and the navigation; define the party tokens and run the `dataviz` palette checks on them in both themes.
- **Pages:** Congress (a seat chart per chamber with its table twin, totals, leadership), the roster with filters, a person's page, committees, search, Status.
- **Tests:** the fixture's counts by chamber and party match a hand count; a second run writes nothing; a removed member keeps their history and produces one change; a party switch produces one change; a short file is rejected and the stored roster is untouched; an Independent and a third-party member get every field a major-party member gets.

### E1b — The map and state pages (2 sessions)
- **`scripts/geo/`:** its own `package.json` with pinned dev tools, run in the Node container. States from `us-atlas`'s pre-projected file (version and checksum pinned). Districts from the Census file `cb_2024_us_cd119_500k`, projected the same way, simplified, and written as one module per state so a state's districts load only on its page. Output committed under `web/src/lib/geo/` with a header naming the input and the script.
- **API:** every state's summary for the map's colour modes; one state's page data.
- **Map page:** inline SVG, each state a link; colour modes for the Senate delegation and the House split now (the governor's party arrives with E3b, the next election with E6a); a legend with letters and patterns; call-outs for the small eastern states; DC and the territories as labelled boxes; the list twin.
- **State page:** senators, the House delegation as a district map and a list, party totals, vacancies with their reason where the source gives one.
- **Label on district maps:** "Districts of the 119th Congress", since several states redrew for 2026 (roadmap, gate G2).
- **Tests:** every state and district in the roster has a shape, and the reverse (a test over the generated modules); the summary's totals equal the roster's; colour-mode logic for a split delegation, an all-one-party delegation, a vacancy and an at-large state. By hand: Tab order, focus ring, 360 px, both themes, 200% zoom.

### E2 — News (2 sessions)
- **`config/sources.yaml`:** each feed's id, name, URL, and a short neutral label of what kind of outlet it is. NPR Politics, NPR News and Democracy Now; CalMatters and The Oaklandside once their terms for feeds are clear (owner, 2026-11-23; NPR's terms allow a personal app to show its feeds, CalMatters' bar automated access without authorisation, The Oaklandside's were not found). Each label is from the outlet's own description, with its source and checked date.
- **`news` job:** conditional fetch; parse RSS; keep guid, link, title, publication time and a plain-text summary cut to a fixed length. `content:encoded` is never read into a struct, so it cannot be stored. Dedupe by feed and guid, and across feeds by link.
- **`tagging`** (ADR 0008): a matcher built from the roster. It matches a full name; a title with a surname ("Sen. X", "Rep. X", "Gov. X") when that surname is unique for that title, or when the text also names the member's state; a state by its full name, with a list of ambiguous names (Washington, Georgia, New York as a city) that need more context. Everything else is no tag. Mentions are recomputed for recent stories when the roster changes.
- **API and pages:** the feed (phone first) with filters by source, state and person, read and saved marks kept on the server; stories on a person's page and a state's page.
- **Backup:** the seabass volume, a nightly CronJob (`VACUUM INTO`, portraits, 14 dailies and 8 weeklies), a restore drill, the backup's age on the status page.
- **Tests:** a fixture feed yields the expected stories and nothing of the body; a second run adds none; an unchanged feed costs one conditional request; two senators with one surname are not tagged by surname alone; "Washington" alone tags no state; a `javascript:` or `http:` link is not stored as a link; HTML in a summary is shown as text.
- **By hand:** a week of real stories, fifty mentions checked, none wrong (gate G3).

### E3a — Home (1–2 sessions)
- **`config/home.yaml`:** state, congressional district, state senate and assembly districts, city and council district, and the local officials not yet covered by a source, each with a source and a checked date.
- **Visits:** `POST /api/visit` records one; "since" is the previous visit, with choices of 7 and 30 days.
- **Home (phone first):** My representatives, each with office, term, next election when known, latest story (latest vote from E4a); Since you last looked, from `changes` and from stories about the owner's representatives, grouped by kind; Coming up, empty until E6a and saying so.
- The feed gains its "mine" filter. `/` becomes Home. The PWA manifest, icons and an app-shell service worker.
- **Tests:** with visits at two times, only what lies between is listed; with no earlier visit, the last 7 days; a change and its data are written in one transaction; an invalid `home.yaml` stops startup and names the entry.

### E3b — The rest of government (2 sessions)
- **`openstates` adapter** for the `openstates/people` repository, fetched as one archive a week: each state's executive directory (governor and others) and legislature. All legislators are stored, since chamber totals need them; only California's get roster pages (PRD §13, decision 3). Whether the data marks leaders is checked in the phase; if not, leaders are curated.
- **`config/figures.yaml`:** President, Vice President, Cabinet, Supreme Court, each with a primary source and a checked date. The `stale` job reports entries past their age.
- **Pages:** `/government` (the executive and the Court); the state page gains the governor, the chamber totals and leaders, and for California the legislature; the map gains the governor's-party mode.
- **Tests:** as for the roster (no-op, history kept, changes written); a figure without a source fails validation; an entry one day past its age is reported and one day short is not.

### E4a — Votes (2 sessions)
- **`clerk` and `senate` adapters** for the two roll-call XML formats, joined to people by Bioguide id and LIS id. A position whose member is unknown is kept with the source's own name and flagged, never dropped.
- **`votes` job:** the current Congress from its start on the first run (paced), then daily. A change is written for each vote of a listed kind: passage, cloture, a veto override, a nomination, a conference report.
- **Arithmetic, in Go, each with a `learn/` note on what it measures:** the party split of a vote; how often a member votes with the majority of their party on votes where the two parties' majorities disagreed; attendance.
- **Pages:** recent votes, one vote (the split as a chart and a table, every position, the owner's members marked), recent votes on a person's page and on Home.
- **Tests:** a fixture roll call's totals equal the XML's own totals by party; the rate for one member over a small fixture matches a hand count; a new session's numbering restarts without colliding; a second run writes nothing.

### E4b — Bills, laws, orders and scores (1–2 sessions)
- **Secrets** (ADR 0009): an `ExternalSecret` for the api.data.gov key, mounted as a file; the grant in Plutus's OpenTofu (the owner applies it).
- **`congressgov` adapter:** bills that a stored vote names and bills that became law. **`fedreg` adapter:** executive orders and proclamations. **`voteview` adapter:** each member's score, joined by ICPSR id, shown with a line on what it is and is not.
- **Pages:** a bill's page (title, sponsor, status, its votes, a link to Congress.gov), laws and orders on `/government` and on Home.
- **Tests:** the key never appears in a log line, an error, a stored URL or `-h`; a bill named by two votes is fetched once; fixtures for each adapter; no-op and idempotency.

### E5 — Learning center (2 sessions, then ongoing writing)
- **`learn/`:** a page per topic of PRD §6.7, with the frontmatter `AGENTS.md` lists under Structure rules. Rendered on the server with raw HTML escaped.
- **Glossary:** built at startup from the pages' `terms` and `learn/glossary.yaml`. The server marks the terms it finds in vote questions, bill statuses and headlines; the client shows each as a link with a short definition.
- **Context links:** a fixed table from a vote's kind and a race's primary system to the page that explains it.
- **The first set:** about twelve pages and forty terms, each written from primary sources found by search in the session, dated, and read by the owner.
- **Tests:** a page without sources or `as_of` fails validation; raw HTML in a page is escaped; a term inside a longer word is not marked; the longest term wins where two overlap; every term a page declares resolves.

### E6a — Calendar and My ballot (1–2 sessions)
- **`config/calendar.yaml`:** federal dates; California's and Oakland's; every state's next statewide election as states set them, each with the election office's page as its source.
- **Pages:** `/elections` (the calendar, filter by state), `/ballot` (the owner's races and measures, with links to the official voter guides), Coming up on Home, the map's next-election mode.
- **Tests:** dates are calendar dates end to end (a test at a time zone west of UTC); a past date leaves Coming up; an entry without a source fails validation.

### E6b — Races and candidates (2 sessions)
- **`fec` adapter:** candidates for the next federal cycle and their totals, joined to people by FEC id. `races` and `candidacies`; curated entries for governors' races and the owner's own ballot.
- **Results** are entered for curated races from the certifying office, with the source; an uncertified result says so.
- **Comparing two Congresses:** load the members of the 119th who left before the first roster run from `legislators-historical.json`; show who left, who arrived and which seats changed party.
- **Pages:** a race, the candidate section of a person's page, the comparison on `/congress`.
- **Tests:** a candidate who is also a sitting member is one person; a withdrawn candidate stays, marked; the comparison's counts match a hand count on fixtures.

### E7 — The brief (2 sessions)
- ADR 0010 settles how the run is made and paid for, and how the brief is sent (Web Push or email). Load the `claude-api` skill before building.
- **`brief` job:** gather the week from the store; give headlines and summaries to a run with no tools, quoted between random markers (Plutus ADR 0012); require the answer in a fixed structure; check in Go that every reference is to an item it was given; store it with the list of what it was written from.
- **Shown** on Home, labelled as written by Claude from the listed sources, with each claim linked to its row or story.
- **Tests:** an answer that cites an item it was not given is rejected; an instruction planted in a fixture summary changes nothing but the text; with no key the job says what to set and the rest of the app is unaffected; a spend limit stops a run.

### E8 — Cards (2–3 sessions)
- ADR 0011: the frame, the vocabulary and the stats, all original (PRD §6.9).
- **Stats** are functions in Go over stored data, each tested against a hand count and each documented on a `learn/` page the card links to.
- **Collection:** a table the server may write, added to the short list of server writes with its own negative test; a card is collected by reading the person's page or passing the quiz on a related `learn/` page (quizzes are frontmatter on the page).
- **Pages:** `/cards` (the collection, decks by state, committee and Congress), the card on a person's page.
- **Tests:** two members with the same inputs get the same stats whatever their party; a missing input shows as unknown, not zero; the flip has a reduced-motion form.

## 5. Risks
| Risk | What limits it |
|---|---|
| A source changes shape or goes away | One adapter per source behind an interface, tested on a saved response; a failing job shows on the status page and leaves the stored data as it was |
| The roster source lags a resignation or a swearing-in | It is maintained by volunteers. Every page shows the fetch date; vacancies come from the apportionment, not from the file's silence alone |
| A story is tagged to the wrong person | Tagging prefers a miss; ADR 0008 lists the rules; a monthly spot check (PRD §12) |
| Curated files go stale | Each entry has an age; the `stale` job reports it; the curated set is kept small |
| District shapes are out of date after redistricting | The map is labelled with its Congress; the list beside it is the authority (gate G2) |
| The brief says something its sources do not | Every reference is checked against what the run was given; the sources are listed under it |
| Scope grows, most likely in cards | Each phase has "done when" lines and the rule not to widen it; cards are last |
| Upkeep exceeds an hour a month | No controller, no second store, no sidecar, no secret until E4b; a new Congress is data |
