# Kubernetes conventions for `k8s/`

Rules for the manifests Flux applies on neptune. Adapted on 2026-11-22 from the Security section of Plutus's `docs/controller-conventions.md` and from how beholder and Plutus are deployed (`~/git/beholder/README.md`, `~/git/plutus/deploy/k8s/README.md`). Eunomia has no controller and no CRD, so the API-design and reconciler rules are not carried over.

## Layout
```
k8s/app/kustomization.yaml     what Flux applies first (path ./k8s/app); the image's digest is pinned here
k8s/app/namespace.yaml         eunomia, Restricted, labelled for the Pushgateway
k8s/app/server.yaml            Deployment, Service, ServiceAccount
k8s/app/jobs.yaml              one CronJob per job
k8s/app/storage.yaml           the data volume; from E2 the backup volume on seabass
k8s/app/network.yaml           NetworkPolicy, in and out
k8s/app/ingress.yaml           Tailscale Ingress: https://eunomia.<tailnet>.ts.net
k8s/app/claude-ops.yaml        read-only Role for the claude-ops account
k8s/monitoring/                PodMonitor and the Grafana dashboard ConfigMap (path ./k8s/monitoring)
```
The `GitRepository` and the two `Kustomization`s that point here (`eunomia`, and `eunomia-monitoring` after it and after `beholder`) are in Plutus (`deploy/k8s/clusters/neptune/eunomia.yaml`) until the cluster has its own repository.

## Rules
1. **Every pod meets the Restricted Pod Security Standard:** `runAsNonRoot`, `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`, `seccompProfile: RuntimeDefault`, `readOnlyRootFilesystem: true`, no host namespaces, `hostPath` or `hostPort`. `automountServiceAccountToken: false`: nothing here calls the Kubernetes API. The namespace enforces `restricted` with a pinned `enforce-version`.
2. **Each workload has its own service account,** never `default`.
3. **Secrets are mounted as files** and read through `-*-file` flags, not environment variables. They come from Google Secret Manager through the External Secrets Operator (PRD §13, decision 6). No secret value is in this repository.
4. **NetworkPolicy is a real boundary** (k3s enforces it) and default-deny in both directions.
   - In: the `tailscale` namespace to the server's port; Prometheus in `monitoring` to the internal port.
   - Out: the cluster's DNS; the internet, except private and link-local ranges (`10/8`, `172.16/12`, `192.168/16`, `100.64/10`, `169.254.169.254`); for job pods, the Pushgateway in `monitoring`.
5. **Images are pinned by digest** in `k8s/app/kustomization.yaml`, with the commit as a comment after it. `make deploy` writes that line; never pin by hand from a branch or an old build.
6. **One replica, `strategy: Recreate`,** for the server: one data volume, one SQLite file.
7. **CronJobs:** `concurrencyPolicy: Forbid`, a `startingDeadlineSeconds`, an `activeDeadlineSeconds`, `backoffLimit` low, `ttlSecondsAfterFinished` so finished pods are cleaned up by Kubernetes, and a history limit. Commands are argument lists, never `sh -c`. Schedules are in UTC, staggered so two jobs do not open the same source at once.
8. **Requests and limits** on every container; a memory limit always, no CPU limit.
9. **Probes:** the server has readiness and liveness on `/healthz`.
10. **Monitoring objects need beholder's CRDs.** They live in `k8s/monitoring/`, their own Flux Kustomization depending on `beholder`, so a broken monitoring stack never holds up a deploy.
11. **`claude-ops` reads; it does not write,** and it cannot read Secrets (`k8s/app/claude-ops.yaml`, modelled on beholder's).
12. Render before pushing: `kubectl kustomize k8s/app` and `kubectl kustomize k8s/monitoring` must succeed. The reviewer runs both.

## Definition of done
Both directories render with `kubectl kustomize`; every new pod meets rule 1 or names the exception and why; a new port has a NetworkPolicy rule and nothing wider; a new job has a CronJob, appears on the status page, and pushes its counters; nothing in the diff is a secret.
