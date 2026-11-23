---
name: phase
description: Start work on a phase of the Eunomia roadmap. Use for "/phase E1a", "start phase E2", "let's build E3b". Enters a worktree, cuts the branch, loads the roadmap, plan and conventions, writes the acceptance checklist, then implements tests-first per docs/development-workflow.md.
---

# /phase <id>

Follow `docs/development-workflow.md` §0–§2 exactly.

1. Work in your own worktree (`docs/development-workflow.md`, Parallel sessions). If this session is in the primary checkout (`/home/cole/git/eunomia`, on `main`), enter one first with EnterWorktree, named after the phase; never switch the primary's branch. Verify `git status --short` is empty in the worktree.
2. Check the phase's prerequisites in `docs/roadmap.md`. If one is the owner's to provide (an API key in Secret Manager, a directory on seabass, the home districts, a go-ahead for a change in the Plutus repository), stop and say exactly what is needed, with the command where there is one.
3. `git branch -m feat/phase-<id>-<short-name>`.
4. Read the phase in `docs/roadmap.md`, its section of `docs/implementation-plan.md`, the PRD sections it cites (always §3), `AGENTS.md`'s rules for facts and outside text, `docs/go-conventions.md`, `docs/web-conventions.md` if the phase touches `web/`, `docs/k8s-conventions.md` if it touches `k8s/`, the relevant ADRs, and the packages you will touch.
5. Create `docs/acceptance/phase-<id>.md` from the template in the workflow doc, with every "done when" line as an unchecked item. Commit it.
6. For a new outside source: fetch one real response, read the source's terms of use and rate limits, record both in the acceptance file, and save a trimmed fixture before writing the adapter.
7. Implement tests-first in small commits. `make test lint` clean before each commit. For a UI change, run the app and look at the page before calling it done. Record unspecified behaviour as open questions in the acceptance file rather than inventing it.
8. When the phase's scope is complete, run `/ship`.

Stop and ask when the workflow doc says to. Do not widen the phase.
