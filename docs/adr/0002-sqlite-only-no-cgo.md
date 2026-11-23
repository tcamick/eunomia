# ADR 0002 — SQLite only, and no CGO

**Date:** 2026-11-22 · **Status:** accepted (owner, 2026-11-22, with the plan) · **Relates to:** Plutus ADRs 0002 and 0005

## Context
Plutus keeps state in SQLite and time series in DuckDB, and DuckDB's driver makes its build need CGO. Eunomia's data is small and relational: a few thousand people with their terms, tens of thousands of roll calls and their positions, a few thousand stories a month. Its queries are joins and filters, not scans over millions of rows.

## Decision
- **SQLite is the only store,** through `modernc.org/sqlite` (pure Go). WAL, a busy timeout, foreign keys on, and every transaction taking the write lock when it begins (`_txlock=immediate`), as in Plutus.
- **`internal/store` is the only package that imports the driver or holds SQL.** Migrations are numbered SQL files, embedded and applied at startup, each once.
- **The binary is built with `CGO_ENABLED=0`** and runs on `distroless/static`. The race detector still needs CGO, so tests run with it; the image's check stage has a C compiler.

## Rejected
- **DuckDB beside SQLite, as in Plutus:** no workload here needs a columnar engine, and it would bring back CGO and a second store's rules.
- **PostgreSQL:** a server to run, back up and upgrade for one user.

## Consequences
- A static binary, a smaller image, and `go build` anywhere.
- If vote analytics outgrow SQLite (they are not expected to), this is the decision to revisit, not a reason to add a second store quietly.
