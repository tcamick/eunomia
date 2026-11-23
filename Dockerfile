# Multi-stage: build the frontend, check and build a static Go binary, run it
# on distroless/static as non-root. No CGO (ADR 0002), so no C runtime.
#
# Every base is pinned by digest (docs/go-conventions.md, Security); the tag
# is kept for the reader. To update one:
#   docker buildx imagetools inspect <image>:<tag>   # copy the Digest line
# The golang tag must match the toolchain line in go.mod.
#
# Stages:
#   web       package.json and the lockfile → npm ci; the source → tests, svelte-check, build
#   web-dist  only web/dist, for `make web-docker` to copy out
#   deps      go.mod and go.sum → the module downloads
#   tools     the Makefile → golangci-lint and govulncheck, compiled
#   check     the source → fmt, vet, race tests, lint, govulncheck
#   build     the source, after check → the binary
# Each slow step is a layer whose inputs rarely change, so a code change
# recompiles only the code.

FROM node:22-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM scratch AS web-dist
COPY --from=web /src/web/dist /

FROM golang:1.26.11-bookworm@sha256:dc40a13b61304647ed7e5d581d246b86413dfc29d63c6d018310feb93cd98fc0 AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS tools
COPY Makefile ./
RUN make golangci-lint govulncheck

# check: the race detector needs CGO, which this image has; the binary below
# is built without it. The build stage depends on check, so a red check
# builds nothing.
FROM deps AS check
COPY --from=tools /src/bin ./bin
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN make test lint && touch /checked

FROM deps AS build
ARG VERSION=dev
COPY --from=check /checked /tmp/checked
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X github.com/tcamick/eunomia/internal/version.Version=${VERSION}" \
      -o /out/eunomia ./cmd/eunomia
# An empty directory to copy into the runtime image as /data (distroless has
# no shell to mkdir with).
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/eunomia /eunomia
# /data must belong to nonroot (65532): a new volume is seeded with the image
# directory's ownership.
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME ["/data"]
EXPOSE 8080 8081
ENTRYPOINT ["/eunomia"]
CMD ["serve", "-listen", "0.0.0.0:8080", "-internal-listen", "0.0.0.0:8081", "-data-dir", "/data"]
