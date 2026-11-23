# Development workflow (humans and agents)

How a change goes from a roadmap phase to `main` and onto neptune. Applies to every change to code, manifests, `config/` and `learn/`; a change to `docs/` alone follows the git steps and skips the checks. Agents run this through `/phase <id>` and `/ship`.

Adapted on 2026-11-22 from Plutus's `docs/development-workflow.md`. Differences: there is no seed data (the sources are public, so a worktree fetches its own), and the review covers manifests and `learn/` content as well as code.

## Parallel sessions
Several sessions may build at once, each in its own git worktree. They meet only when a branch lands.
- **The primary checkout (`/home/cole/git/eunomia`) stays on `main`.** It is the only place merges and deploys happen, through `make land` and `make deploy`. Never switch its branch or commit feature work in it. A Claude session in a worktree is isolated from it, so to land, the session leaves its worktree (ExitWorktree, keep) and runs `make land` from the primary.
- **One worktree per session:** `claude -w <name>`, or EnterWorktree. It lands in `.claude/worktrees/<name>` on `worktree-<name>`, based on `origin/main`. Rename the branch to the convention below.
- **State is per worktree:** `make run` and `make job JOB=<name>` use the worktree's own `data/` (gitignored). Never point a worktree at the cluster's database. Frontend builds need Node 22, which neptune's host does not have: they run in Docker (`make web-docker`, the Dockerfile's `web` stage, no host mount).
- **Land one at a time:** `make land` holds a lock and refuses a branch that does not contain the latest `main`.
- When the session ends, offer to remove the worktree and its branch; a landed branch is safe to remove.

## 0. Preconditions
- You are in your own worktree, on a branch that is not `main`, with a clean tree.
- The phase's prerequisites in `docs/roadmap.md` are met. Where a prerequisite is the owner's (a key, a directory on seabass, a go-ahead for a change in Plutus), ask before starting, not at the end.

## 1. Start a phase or a change
1. `git branch -m <type>/<short-name>`, where type is `feat`, `fix`, `docs`, `chore` or `test`. One branch per phase, or per independent change inside a large phase.
2. Read, in this order: the phase in `docs/roadmap.md` and its section of `docs/implementation-plan.md`; the PRD sections it cites, and always §3; `docs/go-conventions.md`; `docs/web-conventions.md` if `web/` will change; `docs/k8s-conventions.md` if `k8s/` will change; the code and tests in the packages you will touch; the relevant ADRs.
3. Write the phase's "done when" lines into `docs/acceptance/phase-<id>.md` as a checklist before writing code. Anything the plan left unspecified goes there as an open question; do not invent it silently.

## 2. Build
- **Tests first for behaviour.** For each behaviour in scope, write the table-driven test that fails, then make it pass. Every job gets the no-op case and the idempotency case. Every PRD principle the change touches (§3) gets a negative test.
- **Sources are met through fixtures.** Record a real response once, trim it, save it under the package's `testdata/`, and test against it. No test reaches the network.
- **For a UI change, look at it.** Run the server, open the page, and check it at phone and laptop widths, in light and dark, with the keyboard alone. A page that passes its tests and has not been looked at is not done.
- Keep commits small and buildable; subject in the imperative, body says why. Stage explicit paths.
- Run `make test lint` before every commit; both clean. Do not commit with a failing or skipped test unless the message says what and why.
- When the plan or the PRD turns out to be wrong, change the document in the same branch and say so in the commit body. The documents are the spec; drift is a bug.
- **Stop and ask the owner when:** a decision changes the data model or a PRD principle; a new dependency is needed (Go module or npm package); a new outside source is added or a source's terms are unclear; anything must change in the Plutus repository; a check cannot run and the reason is unclear; the phase is clearly larger than planned.

## 3. Review (separate from the author)
1. Run `make fmt vet lint test`, and `make web-docker` if `web/` changed. Fix everything.
2. Invoke the `reviewer` subagent (`.claude/agents/reviewer.md`) on the branch. It runs with fresh context and must not be told the author's conclusions; give it the branch name and the phase id only.
3. Resolve every **blocking** and **should fix** finding, or record in the acceptance file why a finding is wrong, with the owner's agreement for blocking ones. Nits are the author's call.
4. If the fixes were more than trivial, run the reviewer again.
5. Walk `docs/acceptance/phase-<id>.md`: mark each line met with evidence (a test name, command output, a screenshot's description), or manual with who verified it and when, or not met. A phase does not merge with a "not met" line unless the plan is amended to move it.

## 4. Land (merge and deploy)
```bash
# in the worktree
git merge main                                # settle any conflicts here, in the branch
make test lint                                # recheck the merged result; commit
# leave the worktree (ExitWorktree, keep), then in the primary checkout
make land BRANCH=<branch> MSG="<one line>"    # merges --no-ff into main, then deploys
```
Update the status table in `docs/roadmap.md` in the branch, after merging `main` and before landing. `make land` refuses if `main` moved since your merge: merge again, recheck, land again. If the deploy half fails after the merge, the merge is on `main` but not pushed; fix forward and run `make deploy` in the primary.

`make deploy` builds the image on neptune (its check stage runs the tests and linters, so a red check builds nothing), pushes it to the registry on neptune's loopback, rewrites the digest in `k8s/app/kustomization.yaml`, commits only that file, and pushes `main`. Flux applies it within a minute or two. `make deploy-status` shows what is pinned, what `main` has and what the cluster runs.

**Before `make land` exists** (phase E0 builds it): E0 is merged by hand in the primary with `git merge --no-ff`, and its first deploy is the acceptance check for the script.

## 5. When CI exists
Add `.github/workflows/ci.yml` running `make test lint` and the Docker build, and a review workflow running the `reviewer` subagent on every pull request. Merges then happen through pull requests with no other change to this procedure.

## Severity and tone of findings
Reviews cite the rule, show the failure concretely, and propose the fix. "I would have done it differently" is not a finding. A finding without a rule or a failure scenario is dropped.

## Acceptance file template (`docs/acceptance/phase-<id>.md`)
```
# Phase <id> — <title> acceptance

Branch: · Started: · Merged:

## Done-when checklist
- [ ] <line from the roadmap> — evidence:
- [ ] ...

## Open questions found during the phase
-

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
```
