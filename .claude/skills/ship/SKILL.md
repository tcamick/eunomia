---
name: ship
description: Pre-merge procedure for an Eunomia branch: run the checks, get an independent reviewer pass, resolve findings, verify phase acceptance, then land it on main and deploy with make land. Use for "/ship", "merge this", "ready to merge", "land this".
---

# /ship [branch]

Follow `docs/development-workflow.md` §3–§4 exactly. Steps 1–7 run in the branch's worktree; only step 8 runs in the primary checkout.

1. `git merge main` into the branch first, so the checks and the review see what will land; settle any conflicts here.
2. `make fmt vet lint test`; for `web/` changes, `make web-docker` (Node 22 in Docker, no host mount); for `k8s/` changes, `kubectl kustomize k8s/app` and `kubectl kustomize k8s/monitoring`. If anything fails, fix it first; do not proceed with a red check.
3. Spawn the `reviewer` subagent with only: the branch name, the phase id (if any), and "review against the conventions, the PRD's principles and the phase; run the checks". Do not pass it your own assessment.
4. Resolve every blocking and should-fix finding. For a finding you believe is wrong, write the reason in `docs/acceptance/phase-<id>.md`; blocking disagreements go to the owner before merging.
5. If fixes were more than renames or comments, run the reviewer again.
6. For a phase branch, complete `docs/acceptance/phase-<id>.md`: each line met with evidence, manual with who, when and device, or not met. Do not land with a not-met line; amend the roadmap first if the scope moved.
7. Update the status table in `docs/roadmap.md` and commit.
8. Leave the worktree with ExitWorktree (`keep`), then in the primary checkout run `make land BRANCH=<branch> MSG="<one line>"` with `DEPLOY_TRAILERS` set to the session's Co-Authored-By line. It merges `--no-ff` into `main` and deploys. If it says `main` moved, re-enter the worktree and go back to step 1. If the deploy half fails, the merge is on `main` unpushed: report it and fix forward. Once landed, `git worktree remove .claude/worktrees/<name>` and `git branch -d <branch>` clean up.
9. Report: checks run and results, reviewer findings and how each was resolved, acceptance status, the merge and deploy commits, and `make deploy-status` once.

Never land with a failing check, an unresolved blocking finding, or uncommitted changes. Never merge or commit in the primary checkout by hand, switch its branch, force-push or rewrite `main`. The one exception is phase E0, which builds `make land` and is merged by hand as the workflow doc says.
