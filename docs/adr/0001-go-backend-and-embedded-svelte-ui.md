# ADR 0001 — A Go backend with an embedded Svelte UI

**Date:** 2026-11-22 · **Status:** accepted (owner, 2026-11-22, with the PRD and plan) · **Relates to:** Plutus ADRs 0001 and 0004

## Context
Eunomia fetches public data on a schedule, stores it, and shows it in a browser on a phone and a laptop. The owner writes Go and already runs a Go and Svelte app (Plutus) on the same cluster, with conventions, a reviewer and a deploy path that work.

## Decision
- **One Go binary** with subcommands: `serve`, `job <name>`, `version`. Stdlib `net/http` and `flag`, zerolog, the Prometheus client, testify.
- **A Svelte 5 and TypeScript frontend,** built by Vite to `web/dist` and embedded with `embed.FS`. The server answers an extension-less path with the app shell so routes deep-link, and keeps unknown assets and API paths as 404s.
- **A thin client over a JSON API.** The server computes; the browser formats.
- `web/dist/.gitkeep` keeps `go build` and `go test` free of Node. The frontend is built by `make web` with a host Node 22, or by `make web-docker` in the Dockerfile's `web` stage.

## Rejected
- **Server-rendered templates with HTMX:** fewer moving parts, but the map, seat charts and later cards are interactive, and the owner's existing frontend conventions are Svelte's.
- **SvelteKit with server rendering:** a second server to run for one user.
- **A native app:** a trap for a personal tool.

## Consequences
- One artifact to build, scan and deploy.
- Node 22 is needed to build the frontend; on neptune it runs only in Docker.
