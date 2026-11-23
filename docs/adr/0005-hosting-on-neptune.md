# ADR 0005 — Hosting on neptune, deployed from this repository

**Date:** 2026-11-22 · **Status:** accepted (owner, 2026-11-22: go-ahead for the change in Plutus) · **Relates to:** Plutus ADRs 0011 and 0015, beholder ADR 0001

## Context
Neptune is the owner's home server: single-node k3s with Flux, the Tailscale operator, the External Secrets Operator, a loopback image registry, and Prometheus and Grafana from beholder. Plutus and beholder run there, each from its own repository. Plutus ADR 0015's direction is that the cluster's Flux configuration moves to a repository of its own; until then it lives in Plutus.

## Decision
- **Eunomia runs in its own namespace, `eunomia`,** enforcing the Restricted Pod Security Standard, from manifests in this repository's `k8s/app` and `k8s/monitoring`.
- **Flux follows this repository's `main`** with its own read-only deploy key (`eunomia-git-credentials` in `flux-system`, made once by the owner). The `GitRepository` and two `Kustomization`s live in Plutus's `deploy/k8s/clusters/neptune/eunomia.yaml`, beside `beholder.yaml`:
  - `eunomia` applies `./k8s/app` and depends on `tailscale` (the ingress class);
  - `eunomia-monitoring` applies `./k8s/monitoring` and depends on `eunomia` and `beholder` (the PodMonitor CRD), so a broken monitoring stack never holds up a deploy.
- **Access is the tailnet alone:** a Tailscale Ingress at `https://eunomia.<tailnet>.ts.net` and a NetworkPolicy that admits only the `tailscale` namespace to the app's port and Prometheus to the metrics port. There is no login; nothing the server does needs one (its only writes are the owner's visits and reading marks, and migrations at startup).
- **Images are built on neptune** by `make image-neptune`, pushed to its loopback registry, and pinned by digest in `k8s/app/kustomization.yaml` by `make deploy`.
- **`claude-ops` reads the namespace** through a Role here, as it reads `monitoring` through beholder's.
- **Backups** (from E2) go to the seabass disk with Plutus's method.

## Rejected
- **The manifests in Plutus:** the investing project would own a politics app's deployment.
- **A login in the app:** one user, already identified by the tailnet.
- **Public exposure:** nothing here is meant for anyone else.

## Consequences
- One more namespace and about 100 MB of memory on neptune.
- Landing Eunomia's Flux wiring is a change in Plutus, made with the owner's go-ahead (given 2026-11-22), and moves with the rest when the cluster gets its own repository.
