# Eunomia: Go backend, Svelte frontend. Conventions: docs/go-conventions.md.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

LOCALBIN ?= $(CURDIR)/bin
# Tool binaries carry their version in the file name, so changing a version
# here makes the target missing and make installs it.
GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOVULNCHECK_VERSION ?= v1.9.0
GOVULNCHECK ?= $(LOCALBIN)/govulncheck-$(GOVULNCHECK_VERSION)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -X github.com/tcamick/eunomia/internal/version.Version=$(VERSION)

# The map's district shapes (make geo): the Census Bureau's cartographic
# boundary file for the 119th Congress, pinned by checksum.
GEO_CD_URL := https://www2.census.gov/geo/tiger/GENZ2024/shp/cb_2024_us_cd119_500k.zip
GEO_CD_ZIP := scripts/geo/input/cb_2024_us_cd119_500k.zip
GEO_CD_SHA256 := 2b4238aed7e865df75d0c9cfcd8dffa1b79007b24519ecc4b213e683078b031a
# The Node image the Dockerfile's web stage uses.
NODE_IMAGE := node:22-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

# The race detector needs CGO at test time; the binary is built without it.
.PHONY: test
test: fmt vet ## Run unit tests with the race detector.
	CGO_ENABLED=1 go test -race -coverprofile cover.out ./...

.PHONY: test-integration
test-integration: ## Run integration-tagged tests (temp SQLite, recorded fixtures; no network).
	CGO_ENABLED=1 go test -race -tags integration ./...

.PHONY: lint
lint: golangci-lint govulncheck ## Run golangci-lint and govulncheck.
	$(GOLANGCI_LINT) run $(LINT_ARGS)
	$(GOVULNCHECK) ./...

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint with fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: web
web: ## Build the frontend into web/dist with the host's Node 22 (npm ci, tests, svelte-check, build).
	cd web && npm ci && npm run build

# Neptune's host has no Node 22, so the frontend is built in the Dockerfile's
# web stage and only its output is copied back; nothing from the host is
# mounted into the container.
.PHONY: web-docker
web-docker: ## Build the frontend in Docker (Node 22) and copy web/dist out.
	docker build --target web-dist --output type=local,dest=web/dist .

.PHONY: geo
geo: ## Regenerate the map's shapes into web/src/lib/geo (Node 22 in Docker; docs/adr/0006).
	mkdir -p scripts/geo/input web/src/lib/geo
	test -f $(GEO_CD_ZIP) || curl -fsS -m 300 -o $(GEO_CD_ZIP) $(GEO_CD_URL)
	echo "$(GEO_CD_SHA256)  $(GEO_CD_ZIP)" | sha256sum -c --quiet -
	cd scripts/geo/input && python3 -I -m zipfile -e cb_2024_us_cd119_500k.zip .
	docker run --rm --user $$(id -u):$$(id -g) -e HOME=/tmp -e npm_config_cache=/tmp/.npm \
	  -v $(CURDIR)/scripts/geo:/geo -v $(CURDIR)/web/src/lib/geo:/out -w /geo $(NODE_IMAGE) \
	  sh -c 'npm ci --no-audit --no-fund && node generate.mjs input /out'

.PHONY: build
build: ## Build bin/eunomia for the host (static, no CGO).
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(LOCALBIN)/eunomia ./cmd/eunomia

# Each worktree has its own data/; give each its own ports when several run
# at once: make run PORT=8090.
PORT ?= 8080
INTERNAL_PORT ?= $(shell echo $$(( $(PORT) + 1 )))

.PHONY: run
run: build ## Run the server on 127.0.0.1:$(PORT) with this worktree's data/.
	$(LOCALBIN)/eunomia serve -listen 127.0.0.1:$(PORT) -internal-listen 127.0.0.1:$(INTERNAL_PORT) -log-format text

.PHONY: job
job: build ## Run one job against this worktree's data/: make job JOB=roster.
	@test -n "$(JOB)" || { echo "usage: make job JOB=<name>"; $(LOCALBIN)/eunomia job; exit 2; }
	$(LOCALBIN)/eunomia job -log-format text $(JOB)

.PHONY: docker-check
docker-check: ## Run vet, lint, govulncheck and race tests in the Linux build container.
	docker build --target check .

# Images are tagged by commit, so the tag names exactly what is in the image.
IMAGE_TAG ?= $(shell git rev-parse --short=12 HEAD)
REGISTRY ?= localhost:5000

# The committed tree is streamed to Docker on neptune (this machine), built
# with its layer cache, and pushed to the registry on its loopback, which k3s
# pulls from (Plutus ADR 0011). Prints the digest to pin in k8s/app.
.PHONY: image-neptune
image-neptune: ## Build and push the image on neptune, tagged by commit (needs a clean tree).
	@test -z "$$(git status --porcelain)" || { echo "commit or stash first: the image is tagged by commit"; exit 2; }
	@log=/tmp/eunomia-build-$$(id -un).log; \
	git archive --format=tar HEAD | docker build --progress=plain --build-arg VERSION=$(IMAGE_TAG) \
	  --tag $(REGISTRY)/eunomia:$(IMAGE_TAG) - >"$$log" 2>&1 || { \
	  echo "image build failed (full log: $$log); the lines that say why:" >&2; \
	  grep -E -- "--- FAIL|FAIL|panic|Vulnerability #|[.]go:[0-9]+:[0-9]+: |npm ERR|ERROR" "$$log" | tail -40 >&2; \
	  tail -15 "$$log" >&2; exit 1; }; \
	docker push --quiet $(REGISTRY)/eunomia:$(IMAGE_TAG) >/dev/null; \
	docker inspect --format "{{index .RepoDigests 0}}" $(REGISTRY)/eunomia:$(IMAGE_TAG)

##@ Deploy (from the primary checkout, on main; docs/development-workflow.md §4)

.PHONY: land
land: ## Merge BRANCH (it must contain main already) and deploy (MSG="…").
	scripts/deploy.sh land "$(BRANCH)" "$(MSG)"

.PHONY: deploy
deploy: ## Build the image if the code changed, pin it, commit, push main, nudge Flux (MSG="…").
	scripts/deploy.sh deploy "$(MSG)"

.PHONY: deploy-status
deploy-status: ## What is pinned, what main has, and what the cluster runs.
	scripts/deploy.sh status

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf $(LOCALBIN)/eunomia cover.out web/dist/*
	touch web/dist/.gitkeep

##@ Tools

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Install the pinned golangci-lint into bin/ if it is not there.
# Compiled with the project's own toolchain, so its gofmt agrees with `go fmt`.
$(GOLANGCI_LINT): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv $(LOCALBIN)/golangci-lint $(GOLANGCI_LINT)

.PHONY: govulncheck
govulncheck: $(GOVULNCHECK) ## Install the pinned govulncheck into bin/ if it is not there.
$(GOVULNCHECK): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	mv $(LOCALBIN)/govulncheck $(GOVULNCHECK)
