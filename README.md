# Eunomia

A personal dashboard and learning center for United States politics: who holds office, what they did, which elections are coming, and what the words mean. One user, on the home server neptune, reached over the tailnet.

- **What and why:** [`docs/prd.md`](docs/prd.md)
- **Order of work and status:** [`docs/roadmap.md`](docs/roadmap.md)
- **How it is built:** [`docs/implementation-plan.md`](docs/implementation-plan.md), [`docs/adr/`](docs/adr/)
- **How to work on it (people and agents):** [`AGENTS.md`](AGENTS.md), [`docs/development-workflow.md`](docs/development-workflow.md)

```
 CronJobs ── eunomia job <name> ──► public sources
     │ write
     ▼
 data volume (eunomia.sqlite) ◄── read ── eunomia serve ◄── Tailscale Ingress ◄── tailnet
                                             └─ :8081 /metrics ◄── Prometheus (beholder)
```

## Commands
```
eunomia serve [flags]          serve the app, its API and /metrics
eunomia job [flags] [name]     run one job and exit; no name lists the jobs
eunomia version                print the build version
```

## Flags
Every flag has an environment override; a flag beats the environment. Invalid values stop the process with every problem listed.

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `-data-dir` | `EUNOMIA_DATA_DIR` | `data` | the SQLite file and mirrored files; created if missing; a relative path is resolved against the working directory |
| `-listen` | `EUNOMIA_LISTEN` | `127.0.0.1:8080` | the app and its API |
| `-internal-listen` | `EUNOMIA_INTERNAL_LISTEN` | `127.0.0.1:8081` | `/metrics`; must differ from `-listen` |
| `-log-level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `-log-format` | `LOG_FORMAT` | `json` | `json` or `text` |
| `-pushgateway-url` | `EUNOMIA_PUSHGATEWAY_URL` | empty | where a job pushes its gauges at exit (beholder ADR 0002); empty pushes nothing |
| `-legislators-url` | `EUNOMIA_LEGISLATORS_URL` | `https://unitedstates.github.io/congress-legislators/` | the congress-legislators JSON files' directory; https, ending in `/` |
| `-portraits-url` | `EUNOMIA_PORTRAITS_URL` | `https://unitedstates.github.io/images/congress/225x275/` | the members' portraits' directory; https, ending in `/` |

## Jobs
| Job | What it does | Schedule on neptune |
|---|---|---|
| `roster` | reads the `unitedstates/congress-legislators` files (members, committees, committee seats); updates people, terms, leadership and committees, and records each arrival, departure, party switch and leadership change. Rejects a file with fewer than 500 members | daily at 10:23 UTC |
| `portraits` | mirrors each member's photo from `unitedstates/images` into `data/portraits/`, with conditional requests; a missing photo is recorded as missing | daily at 10:53 UTC |

## Developing
```bash
make help             # every target
make test lint        # race tests, golangci-lint, govulncheck
make web-docker       # build the frontend in Node 22 (Docker) into web/dist
make run PORT=8090    # serve this worktree's data/ on 127.0.0.1:8090 (metrics on 8091)
make job JOB=roster   # run one job against this worktree's data/ (fetches from the source)
```
The binary embeds `web/dist`; without a frontend build the app's pages answer 404 and the API still works.

## Deploying
A deploy is a commit on `main` made from the primary checkout (`docs/development-workflow.md` §4):
```bash
make land BRANCH=<branch> MSG="one line"   # merge --no-ff, then deploy
make deploy MSG="one line"                 # build if the code changed, pin, push, nudge Flux
make deploy-status                         # pinned vs main vs what the cluster runs
```
Flux reads `k8s/app` and `k8s/monitoring` from this repository's `main`. The `GitRepository` and `Kustomization`s that point here are in the Plutus repository (`deploy/k8s/clusters/neptune/eunomia.yaml`) until the cluster has its own repository.

### Wiring (one-time, the owner)
Flux reads this repository with its own read-only deploy key; GitHub deploy keys are per repository.
```sh
ssh-keygen -t ed25519 -N '' -C flux-eunomia -f /tmp/eunomia-deploy
# GitHub → tcamick/eunomia → Settings → Deploy keys → add /tmp/eunomia-deploy.pub (read-only)
sudo k3s kubectl -n flux-system create secret generic eunomia-git-credentials \
  --from-file=identity=/tmp/eunomia-deploy \
  --from-literal=known_hosts="github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"
shred -u /tmp/eunomia-deploy
```

## Using it
- **The app:** `https://eunomia.<tailnet>.ts.net`, from any device on the tailnet. No login.
- **Its dashboard:** Grafana (beholder), "Eunomia".
