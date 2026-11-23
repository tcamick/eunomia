# Phase E0 — Skeleton and deploy acceptance

Branch: `feat/phase-e0-skeleton` · Started: 2026-11-22 · Merged:

Roadmap `docs/roadmap.md`, E0; plan `docs/implementation-plan.md` §4, E0; PRD §8, §9.

## Done-when checklist
- [ ] `make test lint` pass on neptune and in the image's check stage — evidence:
- [ ] The Status page opens at `https://eunomia.<tailnet>.ts.net` and shows the `noop` job's last run — evidence:
- [ ] The `noop` CronJob has run on the cluster, written its row through the shared volume, and its pushed counters are in Prometheus — evidence:
- [ ] The server's target is up in Prometheus — evidence:
- [ ] `make land` merged a branch and deployed it; `make deploy-status` shows pinned, `main` and the cluster in agreement — evidence:
- [ ] A request from a pod outside the `tailscale` namespace to the server's port is refused — evidence:

## Open questions found during the phase
-

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
