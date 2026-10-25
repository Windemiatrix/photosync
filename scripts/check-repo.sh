#!/usr/bin/env bash
# Repository hygiene checks: rules that neither the compiler nor the linter
# enforce, plus the self-test of the PR title check. Only files tracked by git are examined, so build output and local
# junk never matter.
#
# Run:    make check-repo (which also runs actionlint), or
#         bash scripts/check-repo.sh, from the repository root; CI runs it in
#         the repo-checks job.
# Input:  none.
# Exit:   0 every check passed; 1 at least one failed. Every check runs, and
#         each failure is printed as "FAIL: ..." on stderr after the offending
#         lines.
set -euo pipefail

# The Go language version in go.mod must not be newer than this minor version.
# Raise it deliberately, together with the toolchain used in CI.
max_go_minor=27

# The only package that may use cgo.
cgo_allowed_dir="cmd/photosync/"

spike_build_tag_re='^//go:build.*[[:space:](&|]spike([[:space:])&|]|$)|^// \+build.*[[:space:]]spike'
cgo_import_re='^import "C"$'
action_use_re='uses: *[^ ]+@'
pinned_to_full_sha_re='uses: *[^ @]+@[0-9a-f]{40}( |$)'
go_version_re='^1\.([0-9]+)(\.[0-9]+)?$'

failed=0

fail() {
  echo "FAIL: $1" >&2
  failed=1
}

# Spike code lives only in the unpublished spike branch and never reaches main.
check_no_spike_code() {
  if git ls-files | grep -E '(^|/)internal/spike/'; then
    fail "the internal/spike/ directory must not be committed"
  fi
  if git grep -nE "$spike_build_tag_re" -- '*.go'; then
    fail "files with the 'spike' build tag must not be committed"
  fi
}

check_no_dependency_overrides() {
  if grep -nE '^[[:space:]]*replace([[:space:](]|$)' go.mod; then
    fail "go.mod must not contain replace directives"
  fi
  if git ls-files | grep -E '(^|/)go\.work(\.sum)?$'; then
    fail "go.work must not be committed"
  fi
}

check_go_version() {
  local go_version go_minor=""
  go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
  if [[ "$go_version" =~ $go_version_re ]]; then
    go_minor="${BASH_REMATCH[1]}"
  fi
  if [[ -z "$go_minor" ]] || ((go_minor > max_go_minor)); then
    fail "go.mod has 'go $go_version', expected 1.$max_go_minor or older"
  fi
}

check_cgo_only_where_allowed() {
  if git grep -lE "$cgo_import_re" -- '*.go' ":!$cgo_allowed_dir"; then
    fail "the files above import \"C\" outside $cgo_allowed_dir"
  fi
}

check_actions_pinned() {
  if git grep -nE "$action_use_re" -- '.github/workflows' | grep -vE "$pinned_to_full_sha_re"; then
    fail "workflow actions must be pinned to a full 40-character commit SHA"
  fi
}

run_self_tests() {
  bash scripts/check-pr-title-test.sh || failed=1
}

check_no_spike_code
check_no_dependency_overrides
check_go_version
check_cgo_only_where_allowed
check_actions_pinned
run_self_tests

if ((failed)); then
  exit 1
fi
echo "repo checks passed"
