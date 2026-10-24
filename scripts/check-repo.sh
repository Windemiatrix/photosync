#!/usr/bin/env bash
# Repository hygiene checks. Run from the repository root; CI runs them in the
# repo-checks job, "make check-repo" runs them locally. Only files tracked by
# git are examined, so build output and local junk never matter.
set -euo pipefail

# The Go language version in go.mod must not be newer than this minor version.
# Raise it deliberately, together with the toolchain used in CI.
max_go_minor=27

# Packages that may use cgo ('import "C"'), as path prefixes.
cgo_allowed=(
  "cmd/photosync/"
  "internal/adapters/space/macos/"
  "internal/adapters/network/macos/"
)

failed=0
fail() {
  echo "FAIL: $1" >&2
  failed=1
}

# 1. Spike code lives only in the unpublished spike branch and never reaches main.
if git ls-files | grep -E '(^|/)internal/spike/'; then
  fail "the internal/spike/ directory must not be committed"
fi
if git grep -nE '^//go:build.*[[:space:](&|]spike([[:space:])&|]|$)|^// \+build.*[[:space:]]spike' -- '*.go'; then
  fail "files with the 'spike' build tag must not be committed"
fi

# 2. No local overrides of dependencies.
if grep -nE '^[[:space:]]*replace([[:space:](]|$)' go.mod; then
  fail "go.mod must not contain replace directives"
fi
if git ls-files | grep -E '(^|/)go\.work(\.sum)?$'; then
  fail "go.work must not be committed"
fi
# .env is included by the Makefile: a committed one could change any target.
if git ls-files | grep -E '(^|/)\.env$'; then
  fail ".env must not be committed"
fi
# testdata is not counted as code by the PR metrics, so code never imports it.
if git grep -nE '"[^"]*/testdata(/[^"]*)?"' -- '*.go' ':!*_test.go' ':!**/testdata/**'; then
  fail "production code must not import packages under testdata/"
fi

# 3. The Go language version is not newer than the agreed one.
go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
go_major="${go_version%%.*}"
go_rest="${go_version#*.}"
go_minor="${go_rest%%.*}"
if [[ "$go_major" != 1 || ! "$go_minor" =~ ^[0-9]+$ || "$go_minor" -gt "$max_go_minor" ]]; then
  fail "go.mod has 'go $go_version', expected 1.$max_go_minor or older"
fi

# 4. cgo only where it is allowed.
while IFS= read -r file; do
  [[ -n "$file" ]] || continue
  allowed=0
  for prefix in "${cgo_allowed[@]}"; do
    [[ "$file" == "$prefix"* ]] && allowed=1
  done
  if [[ "$allowed" == 0 ]]; then
    fail "$file imports \"C\" outside the packages allowed to use cgo"
  fi
done < <(git grep -lE '^import "C"$' -- '*.go' || true)

# 5. Every third-party action is pinned to a full commit SHA.
if git grep -nE 'uses: *[^ ]+@' -- '.github/workflows' | grep -vE '@[0-9a-f]{40}( |$)'; then
  fail "workflow actions must be pinned to a full 40-character commit SHA"
fi

# 6. Self-tests of the scripts that decide titles, versions and releases.
bash scripts/check-pr-title-test.sh || failed=1
bash scripts/next-version-test.sh || failed=1
bash scripts/check-release-deps-test.sh || failed=1
bash scripts/pr-metrics-test.sh || failed=1

if ((failed)); then
  exit 1
fi
echo "repo checks passed"
