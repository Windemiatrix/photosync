#!/usr/bin/env bash
# Self-test for scripts/next-version.sh: builds throwaway git repositories with
# the history of each case and checks the tag the script prints and the
# messages it writes. Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/next-version.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/next-version-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

# The test must not depend on the developer's git configuration (commits there
# are usually signed, which would ask for a passphrase).
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
failures=0
repo=""
status=0 out="" err=""

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

g() {
  git -c user.name=test -c user.email=test@example.invalid \
    -c commit.gpgsign=false -c tag.gpgsign=false -c init.defaultBranch=main "$@"
}

# new_repo creates an empty repository in $repo.
new_repo() {
  repo="$(mktemp -d "$tmp/repo.XXXXXX")"
  g -C "$repo" init -q
}

# commit <title>... adds one empty commit per title.
commit() {
  local title
  for title in "$@"; do
    g -C "$repo" commit -q --allow-empty -m "$title"
  done
}

# tag <name> puts a lightweight tag on HEAD.
tag() {
  g -C "$repo" tag "$1"
}

# run [args...] runs the script in $repo and sets status, out (stdout) and err (stderr).
run() {
  status=0
  out="$(cd "$repo" && bash "$script" "$@" 2>"$tmp/stderr")" || status=$?
  err="$(cat "$tmp/stderr")"
}

# expect_tag <want> [args...]: the script succeeds and prints exactly the tag.
expect_tag() {
  local want="$1"
  shift
  run "$@"
  if [[ "$status" != 0 || "$out" != "$want" ]]; then
    fail "want $want, got status $status, stdout '$out', stderr: $err"
  fi
}

# expect_refusal <message part> [args...]: the script fails, prints nothing to
# stdout and mentions the message part on stderr.
expect_refusal() {
  local part="$1"
  shift
  run "$@"
  if [[ "$status" == 0 || -n "$out" || "$err" != *"$part"* ]]; then
    fail "want refusal with '$part', got status $status, stdout '$out', stderr: $err"
  fi
}

# --- first release, version rule before 1.0.0 -------------------------------

new_repo
commit "chore(repo): skeleton" "fix(core): handle empty input"
expect_tag v0.0.1

new_repo
commit "fix(core): handle empty input" "feat(core): add the first command"
expect_tag v0.1.0

# --- patch, minor, breaking on 0.x ------------------------------------------

new_repo
commit "feat(core): first"
tag v0.1.0
commit "fix(core): handle empty input (#3)"
expect_tag v0.1.1

commit "docs(readme): describe the install steps (#4)" "chore(deps): bump x (#5)"
expect_tag v0.1.1

commit "feat(plan): add match levels (#6)"
expect_tag v0.2.0

# breaking on 0.x is minor, not major
new_repo
commit "feat(core): first"
tag v0.1.0
commit "fix(core): a bug" "fix(config)!: rename the remotes key (#7)"
expect_tag v0.2.0

# --- only commits after the last tag count -----------------------------------

new_repo
commit "feat(core): first"
tag v0.1.0
commit "feat(core): second"
tag v0.2.0
commit "fix(core): third"
expect_tag v0.2.1

# --- 1.x: breaking is major, the module path must carry /vN ------------------

new_repo
commit "feat(core): first"
tag v1.2.3
commit "fix(core): a bug"
expect_tag v1.2.4

commit "feat(core): a feature"
expect_tag v1.3.0

commit "refactor(state)!: change the schema (#12)"
printf 'module example.com/photosync/v2\n\ngo 1.27.0\n' >"$repo/go.mod"
expect_tag v2.0.0

printf 'module example.com/photosync\n\ngo 1.27.0\n' >"$repo/go.mod"
expect_refusal "/v2"

rm "$repo/go.mod"
expect_refusal "go.mod not found"

# --- --major ----------------------------------------------------------------

new_repo
commit "feat(core): first"
tag v0.3.0
commit "fix(core): a bug"
expect_tag v1.0.0 --major

new_repo
commit "feat(core): first"
tag v1.0.0
commit "fix(core): a bug"
expect_refusal "--major only moves 0.x" --major

new_repo
commit "feat(core): first"
tag v0.3.0
expect_refusal "no commits" --major

# --- nothing to release ------------------------------------------------------

new_repo
commit "feat(core): first"
tag v0.1.0
expect_refusal "no commits since v0.1.0"

# --- titles outside the rule count as patch ----------------------------------

new_repo
commit "feat(core): first"
tag v0.1.0
commit "add some stuff"
expect_tag v0.1.1
[[ "$err" == *"do not follow"* ]] || fail "no warning about a title outside the rule: $err"
[[ "$err" == *"other"*"add some stuff"* ]] || fail "the title is not marked as other: $err"

# "!" without a scope, or with a scope outside the rule, is not a breaking change
new_repo
commit "feat(core): first"
tag v0.1.0
commit "feat!: no scope" "feat(Core)!: upper case scope" "Feat(core): upper case type"
expect_tag v0.1.1

# --- titles are untrusted: printed escaped -----------------------------------

new_repo
commit "feat(core): first"
tag v0.1.0
commit "fix(core): red $(printf '\033')[31m text" \
  "fix(core): bidi $(printf '\xe2\x80\xae') mark" \
  'fix(core): a back\slash'
expect_tag v0.1.1
[[ "$err" != *$'\033'* ]] || fail "a raw ESC reached stderr"
[[ "$err" != *$'\xe2\x80\xae'* ]] || fail "a raw bidi mark reached stderr"
[[ "$err" == *'red \x1b[31m text'* ]] || fail "ESC is not shown as \\x1b: $err"
[[ "$err" == *'bidi \xe2\x80\xae mark'* ]] || fail "the bidi mark is not shown as bytes: $err"
[[ "$err" == *'a back\x5cslash'* ]] || fail "the backslash is not escaped: $err"

# --- tags -------------------------------------------------------------------

new_repo
commit "feat(core): first"
tag v0.2.0-rc1
commit "fix(core): a bug"
expect_refusal "not vMAJOR.MINOR.PATCH"

new_repo
commit "feat(core): first"
run --unknown
[[ "$status" == 2 ]] || fail "an unknown argument must exit with 2, got $status"

if ((failures > 0)); then
  echo "$failures next-version case(s) failed" >&2
  exit 1
fi
echo "next-version: all cases passed"
