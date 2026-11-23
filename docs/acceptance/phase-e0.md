# Phase E0 — Skeleton and deploy acceptance

Branch: `feat/phase-e0-skeleton` · Started: 2026-11-22 · Merged: 2026-11-22

Roadmap `docs/roadmap.md`, E0; plan `docs/implementation-plan.md` §4, E0; PRD §8, §9.

## Done-when checklist
- [x] `make test lint` pass on neptune and in the image's check stage — evidence: `make test lint` on neptune, 2026-11-22 (all packages pass with `-race`; golangci-lint 0 issues; govulncheck no vulnerabilities); `docker build .` ran the check stage green and built a 22 MB image; the reviewer reran both.
- [ ] The Status page opens at `https://eunomia.<tailnet>.ts.net` and shows the `noop` job's last run — evidence: (waits on the deploy key and the push to GitHub). Locally: `make run` served the page and `/api/status` showed `noop`'s run.
- [ ] The `noop` CronJob has run on the cluster, written its row through the shared volume, and its pushed counters are in Prometheus — evidence: (cluster). Locally: `TestRun_JobRecordsItsRunAndPushes` (one PUT to `/metrics/job/eunomia-batch/batch/noop`, the row in the file); `TestOpen_TwoHandlesShareTheFile`; the image ran `job noop` read-only as uid 65532 on a tmpfs `/data`.
- [ ] The server's target is up in Prometheus — evidence: (cluster).
- [ ] `make deploy` deployed `main`, and `make deploy-status` shows pinned, `main` and the cluster in agreement — evidence: (cluster). *Amended 2026-11-22 (review): was "`make land` merged a branch and deployed it". E0 itself is merged by hand (development-workflow §4), so `make land` is first used, and checked, when E1a lands; the roadmap carries that line to E1a.*
- [ ] A request from a pod outside the `tailscale` namespace to the server's port is refused — evidence: (cluster, after the deploy).

## Open questions found during the phase
- Outside this repository, the plan's E0 step "in Plutus, with the owner's go-ahead: the Flux objects and the `claude-ops` grant" is on Plutus branch `chore/eunomia-flux` (go-ahead given 2026-11-22); every cluster line above waits on it landing.
- The fifth done-when line was rewritten by the author (moving the `make land` check to E1a). The owner agreed, 2026-11-22.
- Both processes apply migrations when they open the file, so the server writes `schema_migrations` and creates tables at startup. Decided as an exception and recorded in ADR 0003 and go-conventions, Layout (review finding 5).

## Review findings not fixed, and why
Reviewer pass 1, 2026-11-22: no blocking findings; seven should-fix, all fixed (the fetch client's error text, User-Agent and cancellation test; the Pushgateway exemption and migrations written into the conventions; the UI checks below; failure messages on bare `require` calls). Nits fixed: a `rate_limited` result for 429; cutting a stored message on a character boundary; the listen-port clash check for `:8080` against `0.0.0.0:8080`; the "Refused an unknown job" log message; internet egress limited to the job pods; `temp_store=MEMORY` for pods with no `/tmp`.
- **Not fixed: CSP `style-src 'unsafe-inline'` also allows `<style>` elements, where `style-src-attr` would allow only attributes.** Whether `style-src-attr` is Baseline in every target browser was not verified here; the change waits until it is checked, since a wrong CSP breaks every page.
Reviewer pass 2, 2026-11-22: no blocking findings; three should-fix, all fixed: tests that read back every connection pragma, that a second `BEGIN` waits under `_txlock=immediate` (checked to fail without it), and that two handles migrating at once apply each file once; messages on the remaining bare `require` calls; the 200% zoom check looked at (it found a long failure message overflowing the table at 640 px, fixed with `overflow-wrap: anywhere`). Nits fixed: the check stage runs `make lint test`, so `go fmt` cannot hide unformatted code from the linter; `.dockerignore` excludes `.env*`; ADR 0005 names every write the server makes.
- **Carried to E1a:** when `noop` is removed, its row stays in `jobs`; E1a shows only registered jobs on the status page.
- The go-conventions edit replacing `eunomia_job_runs_total` with the job-history gauges landed in commit 41b11b1 without saying why: a job pod runs one job and exits, so counters by job would be pushed with a `job` label the Pushgateway refuses; the outcomes come from the job history at scrape time instead.

## Manual verifications
- Status page at 360 px wide (light) and 1280 px (dark), headless Chrome screenshots of `make run` on neptune; the table becomes label and value rows below 34rem · Claude · 2026-11-22 · Chrome headless on neptune
- Status page at 360 px with a device scale factor of 2, nothing clipped · Claude · 2026-11-22 · Chrome headless (a scale factor, not browser zoom; 200% zoom on a 1280 px window is 640 CSS px, wider than the 360 px checked)
- Keyboard: the page has no interactive elements yet; focus styles are defined for E1a's links · Claude · 2026-11-22
- Reduced motion: the page has no motion · Claude · 2026-11-22
- Status page at 640 CSS px with a device scale factor of 2 (200% zoom on a 1280 px window), with a failed job whose message holds a URL and a 150-character token with no spaces: the full table fits, the message wraps inside its cell, nothing clipped; 360 px rechecked the same way · Claude · 2026-11-22 · Chrome headless on neptune
- The owner's phone: not yet · owner · — · —
