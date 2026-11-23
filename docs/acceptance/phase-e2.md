# Phase E2 — News acceptance

Branch: `feat/phase-e2-news` · Started: 2026-11-23 · Merged:

Roadmap `docs/roadmap.md`, E2; plan `docs/implementation-plan.md` §4, E2; PRD §3, §6.6, §7, §8.

## Done-when checklist
- [ ] The feed shows stories from all five sources, each labelled, within an hour or so of publication — evidence:
- [ ] A story that names a senator appears on that senator's page and their state's page — evidence:
- [ ] Marking a story read on one device shows it read on another — evidence:
- [ ] No article body is in the database (a query in the acceptance file shows the longest stored summary) — evidence:
- [ ] Fifty mentions from the first day's stories are checked by hand and none is wrong (the week-long check is gate G3) — evidence:
- [ ] A backup restores into a working copy, and a missing disk shows as a failed backup on the status page — evidence:

## Prerequisites
- The backup directory on seabass: made by the owner, 2026-11-23, with a mount check and a marker file (`/srv/seabass/eunomia-backups/.eunomia-backup-disk`, owned by 65532, mode 0770; the plan's command is amended to match).

## Sources

## Open questions found during the phase
-

## Review findings not fixed, and why
-

## Manual verifications
- <what> · <who> · <date> · <device/environment>
