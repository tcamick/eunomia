# ADR 0003 — Jobs run as CronJobs that share the server's data volume

**Date:** 2026-11-22 · **Status:** accepted (owner, 2026-11-22: PRD §13, decision 5) · **Relates to:** Plutus ADR 0009, beholder ADR 0002

## Context
Eunomia fetches on several schedules: news hourly, the roster and votes daily, slower sources weekly. Something must start each fetch, record how it went, and make a failure visible.

## Decision
- **Each job is `eunomia job <name>`,** run by its own Kubernetes CronJob in `k8s/app/jobs.yaml`, in UTC, staggered.
- **The job pods mount the server's data volume** (`eunomia-data`, local-path on neptune's one node; ReadWriteOnce is per node, so the pods share it) and open the same SQLite file. WAL lets the server read while a job writes; both retry a busy file.
- **Jobs write the data; the server writes only the owner's visits and reading marks.** A test holds the line from E2.
- **Both apply migrations when they open the file** (found in E0's review, 2026-11-22). Whichever starts first creates the new tables, each migration in one write transaction that rechecks it is still pending, so two processes starting together cannot both apply it. This is the server's only write outside visits and marks, and it happens before it serves a request.
- **Each run records its outcome** in the `jobs` table. The server exports those outcomes at scrape time and shows them on the status page, since a job's pod is gone before Prometheus could scrape it.
- **Each run pushes its gauges once at exit** to beholder's Pushgateway (`job="eunomia-batch"`, `batch=<name>`), with `PUT`, so the gateway holds each batch's last run.
- CronJobs use `concurrencyPolicy: Forbid`, deadlines, a low `backoffLimit` and `ttlSecondsAfterFinished` (`docs/k8s-conventions.md`, rule 7).

## Rejected
- **An in-process scheduler in the server,** as Plutus v1 had: one process, but a fetch shares the server's memory and lifetime, and a deploy interrupts a run.
- **Plutus's freshness controller:** it exists for one nightly chain with a cutoff; Eunomia's jobs are independent and frequent, and a controller is a second codebase to maintain.

## Consequences
- A job's schedule, deadline and resources are a manifest edit.
- Two processes write one SQLite file (the server rarely). The store's transaction rules exist for this.
- Each new job needs a CronJob and a row on the status page (the definition of done says so).
