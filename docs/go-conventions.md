# Go conventions

Adapted on 2026-11-22 from Plutus's `docs/go-conventions.md`, which came from the owner's reference codebases (`~/git/bifrost`, `~/git/socket-load-balancer`). Removed: the two-store rule (there is one store), CEL rules, the password and its cookie, and the rules about financial files. Added: the provenance rule and the rules for fetching from public sources. When a rule here conflicts with what you would do by default, follow the rule.

## Layout
```
cmd/eunomia/main.go         one binary; subcommands: serve, job <name>, version
internal/<package>/         one responsibility per package; no pkg/; no util/
internal/store/migrations/  plain SQL, numbered, embedded and applied at startup
web/                        TypeScript (Svelte) built to web/dist and embedded with embed.FS
config/, learn/             curated YAML and markdown, embedded with embed.FS
Makefile, .golangci.yml, go.mod, go.sum, Dockerfile
```
- Module path `github.com/tcamick/eunomia`. A pinned `toolchain` line on the latest patch release (govulncheck must report no standard-library findings); the `go` and `toolchain` directives and the Dockerfile's base image move together.
- **No CGO.** `CGO_ENABLED=0` everywhere; SQLite is `modernc.org/sqlite`. A dependency that needs CGO is the owner's decision (ADR 0002).
- Dependencies are not vendored. A new module is the owner's decision; say what it replaces and what the standard library lacks.
- Package names are short nouns: `store`, `config`, `jobs`, `httpapi`, `roster`, `news`, `tagging`, `votes`, `bills`, `elections`, `learn`, `geo`, `fetch`, `logging`, `version`. A source's adapter is named for the source (`legislators`, `congressgov`, `fec`, `openstates`, `fedreg`, `voteview`, `clerk`, `senate`).
- **One store, one writer package.** `internal/store` is the only package that imports the database driver and the only one that holds SQL. SQL lives beside the table it reads; parameters only, never string-built SQL. Other packages use `store`'s methods through interfaces they define.
- **Two processes share the file.** The server and the job pods open the same SQLite file: WAL mode, a busy timeout, short transactions. The server writes only visits and marks; jobs write everything else. A test holds this line (a server handler that writes another table fails it).

## Code standards
- **Errors:** wrap with context using `%w` (`fmt.Errorf("fetch %s: %w", source, err)`); compare with `errors.Is` and `errors.As`; sentinel errors for conditions callers branch on; never leak a dependency's error types across a package boundary.
- **Interfaces for seams:** depend on narrow interfaces defined by the consumer (`roster.Source`, `news.Feed`, `roster.Store`), with the real implementation behind a constructor. Inject time (`func() time.Time`), randomness and HTTP clients so tests control them. No package-level mutable state outside init-time metric registration.
- **Context:** every function that does I/O takes `ctx context.Context` first and honours cancellation. No `context.Background()` in request paths. HTTP requests carry a context (`noctx`).
- **Configuration fails fast.** Every setting is a flag with an env override, parsed and validated in `cmd/eunomia` at startup (`internal/config`: `Bind(fs, getenv)` registers flags with environment defaults; `Validate` checks them all and reports every problem at once). A flag is added in the phase that first reads it, with its validation. The curated files in `config/` are parsed and validated the same way: a bad entry stops the process and names the file and the entry.
- **Comments explain why.** Exported identifiers get doc comments; non-obvious code gets a comment on the invariant or the failure it prevents (cite the ADR). Do not narrate what the code does.
- **Deprecate softly.** A removed flag keeps being accepted, is marked `// Deprecated:`, and names its replacement.
- **No `fmt.Print*` outside `cmd/`;** everything else logs or returns.
- `gofmt`, `goimports` (stdlib / third-party / local groups), `go vet` clean.

## Provenance (PRD §3: every fact has a source and a date)
- Every table filled from outside has `source` and `fetched_at` columns, `NOT NULL`. A row is written only with both. A test per job asserts it.
- A value the source does not give is `NULL` and shown as unknown. Never a default that looks like data (an empty party, a zero vote count, today's date for a missing one).
- A job never deletes history because a source stopped listing something. A member who leaves gets an end date on the term and a row in `changes`; the person stays.
- Identifiers from sources (Bioguide, FEC, ICPSR, Open States, Wikidata) are the join keys. Never join people by name. Name matching exists in one place, `tagging`, where a miss is preferred to a wrong match.
- What a job found different from the last run goes to `changes` in the same transaction as the write, so "Since you last looked" cannot disagree with the data.

## Fetching from public sources
- All outbound HTTP goes through `internal/fetch`: a timeout on every request, a response size limit, a `User-Agent` of `eunomia/<version>` with nothing that identifies the owner, conditional requests (`ETag`, `If-Modified-Since`) where the source supports them, a pace between requests to one host, and backoff with jitter on 429 and 5xx. Redirects stay on `https`.
- A job asks for what changed, not for everything: since the newest stored item, or with the conditional headers. An unchanged source costs one request and writes nothing.
- A source's base URL is a flag or a `config/sources.yaml` entry, never a constant in the middle of the code, so a test points it at `httptest.Server`.
- API keys come from a file-path flag (`-api-data-gov-key-file`), never a flag value or the repository. They are never logged and never appear in a URL that is logged or stored; a test asserts neither `-h` nor an error prints one.
- A job that fails for one source reports it and lets the other sources' jobs run. A partial fetch is not written as if it were complete: a roster with fewer than a sane minimum of members is rejected, not stored.

## Logging
- **zerolog**, JSON by default, text for `-log-format text`; level from `-log-level` / `LOG_LEVEL`. The logger travels in the context (`log.Ctx(ctx)`); jobs add `job`, HTTP handlers add `requestId`, `method`, `path`.
- **Message style:** capital first letter, no trailing period, active voice, past tense, names the object (`"Fetched roster"`, `"Could not fetch feed"`). Values go in fields, never in the message.
- **Keys** lowerCamelCase and consistent: `job`, `source`, `feed`, `chamber`, `congress`, `state`, `rows`, `added`, `changed`, `reason`, `durationMs`, `status`.
- **Levels:** `Error` only for unexpected failures; expected, recoverable errors are `Info` or `Warn` with `err` as a field. `Info` for startup configuration and state changes an operator should see. A quiet steady state is the goal: a run with nothing new logs a handful of lines.
- Shared packages (everything under `internal/` except `jobs` and `httpapi`) do not log errors; they return them. Report each failure once.
- Never log an API key or a whole response body.

## Metrics
- Prometheus client; `/metrics` on the internal port (`-internal-listen`), not the one the tailnet reaches. Names are `eunomia_<area>_<thing>_<unit>`: `eunomia_fetch_requests_total{source,result}`, `eunomia_fetch_duration_seconds{source}`, `eunomia_job_last_run_ok{job}`, `eunomia_job_last_success_timestamp_seconds{job}`, `eunomia_batch_last_run_ok` (pushed, grouped by `batch`), `eunomia_roster_changes_total{kind}`, `eunomia_news_stories_added_total{feed}`, `eunomia_http_requests_total{route,code}`.
- **Labels are bounded:** every value can be listed when the code is written (`source`, `feed`, `job`, `result`, `kind`, `route`). Never a person, a state's free text, a URL, a headline or an error message.
- **Every outside source** gets a request counter by `source` and `result` and a duration histogram, from `internal/fetch`, so an adapter cannot forget them.
- A job pod lives seconds. It pushes its counters to beholder's Pushgateway once at exit (`PUT`, `job="eunomia-batch"`, grouping label `batch=<job name>`; beholder ADR 0002), and records its outcome in the `jobs` table, which the server exports at scrape time. A pushed family must not carry a `job` or `batch` label of its own (the gateway's grouping labels); `internal/jobs` leaves such a family out of the push.
- Help text says what is counted and lists the label values. Metrics are tested as behaviour: the name, the labels, that it was emitted, and that a bounded label cannot take an unbounded value.

## Testing
- **Every behaviour change ships with a test that fails without it.** A bug fix gets a regression test named for the bug.
- **Unit tests are hermetic:** no network, no real clock, no filesystem beyond `t.TempDir()`. Sources are behind interfaces with small hand-written fakes that record what they were asked. No mock generators.
- **Fixtures are real responses, trimmed.** Saved under the package's `testdata/` with a comment or README line saying where and when each was fetched. Public records only.
- **Table-driven, map-keyed cases with `t.Run`** (`tests := map[string]struct{…}`); case names read as sentences (`"a member who left keeps their past term"`).
- **Names describe behaviour:** `TestRoster_SecondRunWithSameFileWritesNothing`, `TestTagging_SurnameAloneDoesNotMatch`, not `TestRoster2`.
- **`t.Parallel()`** in every test and subtest unless it mutates global state, in which case a comment says why it is serial. `-race` always.
- **No fixed delays.** Never `time.Sleep`; inject a clock; poll with a context and a timeout when waiting on goroutines.
- **Assertions:** testify; `require` for preconditions, `assert` for checks; failure messages say what was being done, expected and observed.
- **Test the contract:** rows written, changes recorded, requests made (or not made), metrics emitted. Every job's tests include the **no-op case** (a second run with unchanged input writes nothing and records no change) and the **idempotency case** (the same input twice gives the same rows).
- **Principles get negative tests** (PRD §3): no article body is stored; a row without a source is refused; a roster shrinking past the minimum is rejected; the same page fields exist for every party; the server writes only visits and marks.
- **Integration tests** (`//go:build integration`) use a temp SQLite file and `httptest.Server` replaying fixtures. None reach the real network. **Flaky tests are bugs.**

## Security
- **Untrusted input:** every feed, API response and downloaded file. Validate at the boundary (types, ranges, lengths, enum values, a size limit on the body); never build a shell command, SQL, a file path or HTML from it. XML is parsed with the standard decoder and no external entities.
- **URLs from outside** are stored only if they parse and are `https`. The server never fetches a URL that came from a feed; it fetches only configured sources.
- **Portraits and other mirrored files** are written under the data directory by a name derived from our own id, never from the source's file name. Content type is checked before a file is served.
- HTTP server: read, write and idle timeouts; the security headers of `docs/web-conventions.md`; no authentication (the tailnet and a NetworkPolicy decide who reaches it), so **no handler does anything a stranger on the tailnet must not do**: handlers read, and write only visits and marks.
- `gosec` clean. Supply chain: `govulncheck` in `make lint`; every Dockerfile base image pinned by digest, with the tag kept for the reader; Go on a supported release.

## Git workflow
- **Never commit on `main` by hand.** Every change, including docs, lands on a branch cut from the latest `main` and merged `--no-ff` by `make land` (`docs/development-workflow.md`).
- Stage explicit paths, not `git add -A`.
- Commit messages: imperative subject, body says why, and the attribution trailer when an agent wrote it.

## Lint and build
- `make help | fmt | vet | lint | test | test-integration | build | run | job | web | web-docker | docker-check | image-neptune`. `make test` runs `go test -race ./...` after fmt and vet (the race detector needs CGO at test time only; the binary is built without it). `make lint` runs golangci-lint with Plutus's linter set plus `govulncheck`: `errcheck, errorlint, gosec, govet, staticcheck, unused, ineffassign, unconvert, unparam, prealloc, nakedret, goconst, gocyclo, dupl, lll, misspell, revive, copyloopvar, contextcheck, forbidigo (time.Sleep in tests; fmt.Print* outside cmd/), loggercheck (zerolog), noctx, promlinter, testifylint, thelper, tparallel`.
- Tool binaries are pinned in the Makefile and installed into `bin/` with the version in the file name.
- The bar is zero findings.

## Definition of done
- Behaviour matches the PRD and the phase's "done when"; anything unspecified is called out, not invented.
- Tests cover the change, the no-op case and idempotency; `make test lint` pass, or the commit says exactly what could not run and why.
- Every new outside call goes through `internal/fetch`; every new failure mode has a metric and a log line; every new job appears on the status page and has a CronJob in `k8s/`.
- Every new table filled from outside has `source` and `fetched_at`.
- New flags are documented in `README.md`.
- Done on a branch, landed with `make land`.
