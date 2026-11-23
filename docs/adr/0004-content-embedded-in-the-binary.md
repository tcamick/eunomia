# ADR 0004 — Curated content is embedded in the binary

**Date:** 2026-11-22 · **Status:** accepted (owner, 2026-11-22, with the plan) · **Amends:** PRD §9 ("a `git-sync` sidecar")

## Context
Two kinds of content live in the repository and are written by hand: `config/` (the owner's districts, the news sources, curated figures, the election calendar) and `learn/` (the explainers and glossary). Plutus reads its markdown through a `git-sync` sidecar so an edit appears within a minute, at the cost of a sidecar, a deploy key the pod holds, and an init container in every pod.

## Decision
- **`config/` and `learn/` are embedded** with `embed.FS` and parsed and validated at startup, like flags. A bad entry stops the process and names the file and the entry.
- **An edit reaches the cluster with the next deploy:** `make land`, or `make deploy` on `main`. `scripts/deploy.sh` counts both directories as image inputs, so a content change builds a new image.

## Rejected
- **`git-sync`, as in Plutus:** a second container per pod, an SSH key in the namespace, and content that can change under a running server without passing the checks a deploy runs.
- **Content in the database, edited from the app:** a write path the server otherwise does not have, and content outside version control.

## Consequences
- Every content change passes `make test lint` and the reviewer before it is live, including the validation of sources and dates.
- A change is a few minutes from commit to cluster, not one.
