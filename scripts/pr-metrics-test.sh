#!/usr/bin/env bash
# Self-test for scripts/pr-metrics.sh: builds a temporary git repository with a
# base commit and a head commit per case and checks the rows and the exit code.
# Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
metrics="$here/pr-metrics.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/pr-metrics-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
failures=0
repo=""

fail() {
  printf 'FAIL [%s]: %s\n' "$case_name" "$1" >&2
  failures=$((failures + 1))
}

g() {
  git -C "$repo" -c user.name=test -c user.email=test@example.com \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null "$@"
}

# start <name>: a fresh repository with one base commit.
start() {
  case_name="$1"
  repo="$work/$1"
  mkdir -p "$repo/internal/app"
  git init -q "$repo"
  printf 'package app\n' >"$repo/internal/app/app.go"
  printf '# Readme\n' >"$repo/README.md"
  g add -A
  g commit -qm base
}

# lines <n> <file>: writes n distinct lines.
lines() {
  mkdir -p "$(dirname "$repo/$2")"
  seq 1 "$1" | sed 's|^|// line |' >"$repo/$2"
}

# run <want exit code> [env assignments...]: commits the work tree as head and
# runs the metrics against the base commit.
run() {
  local want="$1" got=0
  shift
  g add -A
  g commit -qm head
  (cd "$repo" && env -u PR_TITLE -u PR_BODY -u GITHUB_ACTIONS "$@" bash "$metrics" --base HEAD~1) >"$work/out" 2>"$work/err" || got=$?
  if [[ "$got" != "$want" ]]; then
    fail "want exit $want, got $got: $(cat "$work/out" "$work/err")"
  fi
}

# expect <row> <fixed text> [verdict]: the row contains the text and ends with the verdict.
expect() {
  local line
  line="$(grep "^$1|" "$work/out" || true)"
  [[ "$line" == *"$2"* ]] || fail "row $1 has no '$2': $line"
  if [[ -n "${3:-}" && "$line" != *"|$3" ]]; then fail "row $1 verdict is not $3: $line"; fi
}

start limit-200
lines 200 internal/app/a.go
run 0
expect 01 "200 (+200 -0)" "✅"

start limit-201
lines 201 internal/app/a.go
run 1
expect 01 "201 (+201 -0) · tests 0" "❌"
grep -q "blocking rows failed: 01" "$work/err" || fail "no blocking message: $(cat "$work/err")"

start categories
lines 10 internal/app/a_test.go
lines 20 internal/app/testdata/x.go
lines 5 go.sum
lines 7 docs/a.md
lines 3 locales/en.json
lines 4 scripts/check-x-test.sh
lines 6 scripts/check-x.sh
lines 2 internal/app/bridge.m
lines 3 internal/testkit/fake.go
run 0
expect 01 "8 (+8 -0) · tests 17 · testdata 20 · go.sum 5 · docs 7 · other 3" "✅"

start test-renamed-to-code
lines 150 internal/app/a_test.go
g add -A
g commit -qm "base with a test"
git -C "$repo" mv internal/app/a_test.go internal/app/a.go
lines 150 internal/app/b.go
run 1
expect 01 "300 (+300 -0)" "❌"

start testdata-moved-into-code
lines 150 testdata/x.go
g add -A
g commit -qm "base with testdata"
mkdir -p "$repo/internal/x"
git -C "$repo" mv testdata/x.go internal/x/x.go
run 0
expect 01 "150 (+150 -0)" "✅"

start rename-inside-code
lines 300 internal/app/big.go
g add -A
g commit -qm "base with code"
git -C "$repo" mv internal/app/big.go internal/app/moved.go
run 0
expect 01 "0 (+0 -0)" "✅"

start gitattributes
printf '*.go binary\n*.m -diff\n' >"$repo/.gitattributes"
g add -A
g commit -qm "base with attributes"
lines 120 internal/app/a.go
lines 120 internal/app/b.m
lines 120 docs/a.md
run 1
expect 01 "bypass: binary internal/app/a.go" "❌"
expect 01 "binary internal/app/b.m|" "❌"

# Git counts no lines of a binary file even with --text: in production code
# that is a bypass, outside of it the file counts as 0.
start binary-code
printf 'package app\000\001\002\n' >"$repo/internal/app/blob.go"
run 1
expect 01 "bypass: binary internal/app/blob.go" "❌"

start binary-data
mkdir -p "$repo/testdata"
printf 'x\000\001\002\n' >"$repo/testdata/blob.bin"
run 0
expect 01 "0 (+0 -0) · tests 0 · testdata 0" "✅"

start symlink
lines 300 testdata/big.txt
g add -A
g commit -qm "base with data"
ln -s ../../testdata/big.txt "$repo/internal/app/x.go"
run 1
expect 01 "bypass: link internal/app/x.go" "❌"

# Names reach the output in bypass reasons; only whitelisted ones verbatim.
# names <name>...: each one a symbolic link in production code; the reasons
# show a plain name verbatim and every other one as <unprintable name #...>.
names() {
  local n want=$#
  printf 'x\n' >"$repo/target.txt"
  for n in "$@"; do ln -s ../../target.txt "$repo/internal/app/$n.go"; done
  ln -s ../../target.txt "$repo/internal/app/plain-name.go"
  run 1
  expect 01 "link internal/app/plain-name.go" "❌"
  [[ "$(grep -o '<unprintable name #[a-z0-9]*>' "$work/out" | wc -l | tr -d ' ')" == "$want" ]] ||
    fail "want $want unprintable names: $(cat "$work/out")"
  if grep -qE '`|caf|::|=>|a\|b' "$work/out"; then fail "a raw name reached the output: $(cat "$work/out")"; fi
  [[ "$(wc -l <"$work/out" | tr -d ' ')" == 1 && "$(cut -d'|' -f1 "$work/out")" == 01 ]] ||
    fail "a name broke the row format: $(cat "$work/out")"
}
start names-1
names "$(printf 'caf\303\251')" "a => b" "a\`b" "a|b"
start names-2
names "$(printf 'a\342\200\256b')" "::warning x" "$(printf 'new\nline')"

start binary-name
printf 'package app\000\n' >"$repo/internal/app/a|b.go"
run 1
expect 01 "bypass: binary <unprintable name #1>" "❌"

start rows
lines 300 internal/app/a.go
run 1
(cd "$repo" && bash "$metrics" --base HEAD~1 --rows 02) >"$work/out" 2>/dev/null || fail "a row that was not printed blocked"
[[ ! -s "$work/out" ]] || fail "--rows not applied: $(cat "$work/out")"
got=0
(cd "$repo" && bash "$metrics" --base no-such-rev) >/dev/null 2>&1 || got=$?
[[ "$got" == 2 ]] || fail "a bad base exited $got, want 2"


if ((failures > 0)); then
  echo "$failures pr-metrics case(s) failed" >&2
  exit 1
fi
echo "pr-metrics: all cases passed"
