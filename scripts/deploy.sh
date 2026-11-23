#!/usr/bin/env bash
# Deploy Eunomia to neptune: make the cluster match main. Ported from Plutus's
# scripts/deploy.sh for one image and one pin file.
#
#   scripts/deploy.sh deploy ["one line for the commit"]   (make deploy MSG="…")
#   scripts/deploy.sh land <branch> "one line"            (make land BRANCH=… MSG="…")
#   scripts/deploy.sh status                              (make deploy-status)
#
# land, from the checkout that has main: refuses unless the branch already
# contains main (merge main into it and rerun the checks in its worktree
# first) and its worktree is clean; then, under the lock, merges it --no-ff
# and runs deploy.
#
# deploy, from the checkout that has main (never a feature worktree):
#   1. refuses unless on main, not behind origin/main, with the image's
#      inputs and the pin file clean;
#   2. holds a lock in the shared .git directory, so two sessions on this
#      machine cannot interleave a build and a pin;
#   3. builds the image only if its inputs changed since the commit the pin
#      names (make image-neptune; its check stage runs the tests and linters);
#   4. rewrites the pin, commits "Deploy <sha> on neptune: …" with only the
#      pin file, pushes main and nudges Flux.
# With no image to build it still pushes and nudges, so a manifest-only
# change deploys the same way.
#
# Needs ~/.kube/neptune.yaml for the nudge and the cluster half of status;
# without it those are skipped. Extra trailers for the commit message
# (Co-Authored-By: …) come from DEPLOY_TRAILERS, one per line.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

kc="${NEPTUNE_KUBECONFIG:-$HOME/.kube/neptune.yaml}"
pin=k8s/app/kustomization.yaml
image=eunomia
# What goes into the image: a change anywhere else (docs, manifests, agent
# files) deploys without a build.
inputs=(cmd internal web config learn go.mod go.sum Makefile Dockerfile .dockerignore .golangci.yml)
head=$(git rev-parse --short=12 HEAD)

die() { echo "deploy: $*" >&2; exit 1; }

# existing_inputs: the inputs that exist at HEAD (config/ and learn/ arrive
# in later phases), so git diff is never given a path it does not know.
existing_inputs() {
  local p
  for p in "${inputs[@]}"; do
    if git cat-file -e "HEAD:$p" 2>/dev/null; then echo "$p"; fi
  done
}

# pinned: the commit in the comment after the image's digest.
pinned() {
  awk -v name="$image" '
    $0 == "  - name: " name { inblk = 1; next }
    inblk && /^    digest:/ { if (sub(/.*# */, "")) print; exit }
  ' "$pin"
}

# changed <pinned commit>: true when the image's inputs differ between the
# pin and HEAD, or the pin names no commit this repository knows.
changed() {
  [[ -n "$1" ]] && git rev-parse -q --verify "$1^{commit}" >/dev/null || return 0
  local paths
  mapfile -t paths < <(existing_inputs)
  ! git diff --quiet "$1" HEAD -- "${paths[@]}"
}

# repin <digest> <commit>
repin() {
  local tmp
  tmp=$(mktemp)
  awk -v name="$image" -v d="$1" -v c="$2" '
    $0 == "  - name: " name { inblk = 1 }
    inblk && /^    digest:/ { sub(/digest: .*/, "digest: " d "  # " c); inblk = 0; done++ }
    { print }
    END { exit done != 1 }
  ' "$pin" >"$tmp" || { rm -f "$tmp"; die "could not rewrite the $image pin in $pin"; }
  cat "$tmp" >"$pin" && rm -f "$tmp"
}

# build: build and push the image on neptune; prints the digest.
build() {
  local out
  out=$(make --no-print-directory image-neptune | tee /dev/stderr) ||
    die "the image did not build (its check stage runs the tests and linters)"
  grep -o '@sha256:[0-9a-f]\{64\}' <<<"$out" | tail -1 | cut -c2-
}

kube() { kubectl --kubeconfig "$kc" "$@"; }

# fetch_main: update origin/main; an origin with no main yet (before the
# first push) is not an error.
fetch_main() {
  if git ls-remote --exit-code -q origin refs/heads/main >/dev/null 2>&1; then
    git fetch -q origin main || die "could not fetch origin"
  else
    git ls-remote -q origin >/dev/null 2>&1 || die "could not reach origin"
  fi
}

nudge() {
  if [[ ! -f "$kc" ]]; then
    echo "no $kc: Flux will pick up main on its own within ten minutes"
    return
  fi
  local now
  now=$(date +%s)
  kube -n flux-system annotate gitrepository eunomia reconcile.fluxcd.io/requestedAt="$now" --overwrite >/dev/null &&
    kube -n flux-system annotate kustomization eunomia reconcile.fluxcd.io/requestedAt="$now" --overwrite >/dev/null &&
    echo "nudged Flux; it applies main within a minute or two (make deploy-status)" ||
    echo "the Flux nudge failed; Flux picks up main on its own within ten minutes"
}

status() {
  git fetch -q origin main 2>/dev/null || echo "(could not fetch origin main; comparing with the last fetched origin/main)"
  local branch ahead behind p
  branch=$(git symbolic-ref -q --short HEAD || echo "detached")
  if git rev-parse -q --verify origin/main >/dev/null; then
    read -r behind ahead < <(git rev-list --left-right --count origin/main...HEAD)
    echo "checkout:   $branch at $head ($ahead ahead, $behind behind origin/main)"
  else
    echo "checkout:   $branch at $head (origin has no main yet)"
  fi
  p=$(pinned)
  if changed "$p"; then echo "image:      pinned ${p:-nothing}; its inputs have changed since, a deploy builds it"; else echo "image:      pinned $p, current"; fi
  if [[ ! -f "$kc" ]]; then
    echo "cluster:    no $kc, skipped"
    return
  fi
  local rev running want
  rev=$(kube -n flux-system get gitrepository eunomia -o jsonpath='{.status.artifact.revision}' 2>/dev/null || true)
  echo "flux:       has ${rev:-unknown}; origin/main is $(git rev-parse -q --short=12 --verify origin/main || echo absent)"
  running=$(kube -n eunomia get deploy eunomia -o jsonpath='{.spec.template.spec.containers[?(@.name=="eunomia")].image}' 2>/dev/null || true)
  want=$(grep -A2 "^  - name: $image\$" "$pin" | grep -o 'sha256:[0-9a-f]*' || true)
  if [[ -n "$want" && "$running" == *"$want" ]]; then
    echo "server:     runs the pinned digest"
  else
    echo "server:     runs ${running:-unknown}, not the pinned digest (Flux has not applied it yet, or the pin is unpushed)"
  fi
}

# take_lock: one land or deploy at a time on this machine. The lock is a
# directory in the shared .git, so every worktree sees the same one.
take_lock() {
  [[ -n "${lock:-}" ]] && return
  local l
  l="$(git rev-parse --git-common-dir)/eunomia-deploy.lock"
  mkdir "$l" 2>/dev/null ||
    die "another land or deploy holds $l ($(cat "$l/owner" 2>/dev/null || echo unknown)); remove that directory if none is running"
  lock="$l"
  trap 'rm -rf "$lock"' EXIT
  echo "pid $$ at $(date -u +%FT%TZ)" >"$lock/owner"
}

# worktree_of <branch>: the checkout that has the branch, if any.
worktree_of() {
  git worktree list --porcelain | awk -v b="branch refs/heads/$1" '/^worktree /{ w = substr($0, 10) } $0 == b { print w; exit }'
}

# land <branch> <message>: merge a feature branch into main, then deploy.
land() {
  local branch="$1" msg="$2" bwt where
  [[ -n "$branch" && -n "$msg" ]] || die 'usage: make land BRANCH=<branch> MSG="one line for the merge"'
  where=$(worktree_of main)
  [[ "$(git symbolic-ref -q --short HEAD || true)" == main ]] ||
    die "land from the checkout that has main (${where:-none has it}); a session in a worktree leaves it first (ExitWorktree, keep)"
  [[ "$branch" != main ]] || die "land a feature branch, not main"
  git rev-parse -q --verify "refs/heads/$branch" >/dev/null || die "no branch $branch"
  bwt=$(worktree_of "$branch")
  [[ -z "$bwt" || -z "$(git -C "$bwt" status --porcelain)" ]] ||
    die "$bwt has uncommitted changes, which would not land; commit them first"

  take_lock
  fetch_main
  if git rev-parse -q --verify origin/main >/dev/null; then
    git merge-base --is-ancestor origin/main HEAD ||
      die "main is behind origin/main; git pull --no-rebase, then land again"
  fi
  git merge-base --is-ancestor HEAD "$branch" ||
    die "$branch does not contain main ($head); in its worktree: git merge main, rerun the checks, then land again"

  local trailers=()
  [[ -n "${DEPLOY_TRAILERS:-}" ]] && trailers=(-m "$DEPLOY_TRAILERS")
  git merge -q --no-ff "$branch" -m "Merge $branch: $msg" "${trailers[@]}" ||
    die "the merge failed (git says why above); most likely uncommitted edits here touch the same files"
  head=$(git rev-parse --short=12 HEAD)
  echo "merged $branch into main ($head)"
  (deploy "$msg") ||
    die "merged into main but not deployed or pushed (see above); fix it, then make deploy"
}

deploy() {
  local msg="${1:-}" p d paths
  [[ "$(git symbolic-ref -q --short HEAD || true)" == main ]] ||
    die "deploy from the checkout that has main (here: $(git symbolic-ref -q --short HEAD || echo detached))"
  mapfile -t paths < <(existing_inputs)
  [[ -z "$(git status --porcelain -- "${paths[@]}" "$pin")" ]] ||
    die "commit or stash the changes to the image's inputs or $pin first: the image is built from the commit"

  take_lock
  fetch_main
  # A first deploy has no origin/main yet.
  if git rev-parse -q --verify origin/main >/dev/null; then
    git merge-base --is-ancestor origin/main HEAD ||
      die "main is behind or diverged from origin/main; git pull --no-rebase, then deploy again"
  fi

  p=$(pinned)
  if changed "$p"; then
    echo "building $image $head (its inputs changed since ${p:-nothing}; a few minutes)"
    d=$(build) && [[ -n "$d" ]] || die "no digest from the build"
    repin "$d" "$head"
    [[ -n "$msg" ]] || msg=$(git log -1 --format=%s HEAD | sed -E 's/^Merge [^:]*: //')
    local body="Image from $head."
    [[ -n "${DEPLOY_TRAILERS:-}" ]] && body+=$'\n\n'"$DEPLOY_TRAILERS"
    git commit -q -m "Deploy $head on neptune: $msg" -m "$body" -- "$pin"
    echo "committed $(git log -1 --format='%h %s')"
  else
    echo "image current: its inputs match $p"
  fi

  if git rev-parse -q --verify origin/main >/dev/null && [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]]; then
    echo "main is already on origin; nothing to push"
  else
    git push -q origin main || die "push refused (origin moved?): git pull --no-rebase, then deploy again; the image is cached"
    echo "pushed main ($(git rev-parse --short=12 HEAD))"
  fi
  nudge
}

case "${1:-}" in
  deploy) shift; deploy "${1:-}" ;;
  land) shift; land "${1:-}" "${2:-}" ;;
  status) status ;;
  *) echo "usage: scripts/deploy.sh deploy [message] | land <branch> <message> | status" >&2; exit 2 ;;
esac
