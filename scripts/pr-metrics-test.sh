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
# The machine's git config (diff.algorithm, hooks, signing) stays out.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1

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
lines 2 Makefile
lines 3 .github/workflows/ci.yml
lines 4 internal/app/x.hpp
lines 5 internal/x/testdata/y.go
run 0
expect 01 "17 (+17 -0) · tests 17 · testdata 25 · go.sum 5 · docs 7 · other 3" "✅"

start deletions
lines 50 internal/app/a.go
lines 10 internal/app/b.go
g add -A
g commit -qm "base with code"
lines 30 internal/app/a.go
rm "$repo/internal/app/b.go"
run 0
expect 01 "30 (+0 -30)" "✅"

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
git -C "$repo" mv internal/app/moved.go internal/app/again.go
printf 'func a() {}\n' >>"$repo/internal/app/again.go"
run 0
expect 01 "1 (+1 -0)" "✅"

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

start syso
printf '\177ELF\000\001\n' >"$repo/internal/app/x.syso"
run 1
expect 01 "bypass: binary internal/app/x.syso" "❌"

start symlink
lines 300 testdata/big.txt
g add -A
g commit -qm "base with data"
ln -s ../../testdata/big.txt "$repo/internal/app/x.go"
run 1
expect 01 "bypass: link internal/app/x.go" "❌"

# A link to a directory has no extension, and the build follows it.
start symlink-dir
lines 300 testdata/impl/impl.go
g add -A
g commit -qm "base with data"
ln -s ../testdata/impl "$repo/internal/impl"
run 1
expect 01 "bypass: link internal/impl" "❌"

start submodule
g update-index --add --cacheinfo "160000,$(g rev-parse HEAD),vendor/sub"
mkdir -p "$repo/vendor/sub" # an unpopulated submodule: "add -A" keeps the entry
run 1
expect 01 "bypass: link vendor/sub" "❌"

# An unedited move to "a<LF>b.go" (0 lines) and a new "a<0x01>b.go" read the
# same after the NUL-to-newline step; the newline itself is the bypass.
start newline-collision
lines 10 internal/app/x.go
g add -A
g commit -qm "base with code"
git -C "$repo" mv internal/app/x.go "$(printf 'internal/app/a\nb.go')"
lines 300 "$(printf 'internal/app/a\001b.go')"
run 1
expect 01 "bypass: newline unprintable-name-" "❌"

# Names reach the output in bypass reasons; only whitelisted ones verbatim.
# names <name>...: each one a symbolic link in production code; the reasons
# show a plain name verbatim and every other one as unprintable-name-<id>.
names() {
  local n want=$#
  printf 'x\n' >"$repo/target.txt"
  for n in "$@"; do ln -s ../../target.txt "$repo/internal/app/$n.go"; done
  ln -s ../../target.txt "$repo/internal/app/plain-name.go"
  run 1
  expect 01 "link internal/app/plain-name.go" "❌"
  [[ "$(grep -o 'link unprintable-name-r[0-9]*' "$work/out" | sort -u | wc -l | tr -d ' ')" == "$want" ]] ||
    fail "want $want distinct unprintable links: $(cat "$work/out")"
  if grep -qE '`|caf|::|=>|a\|b|@|#' "$work/out"; then fail "a raw name reached the output: $(cat "$work/out")"; fi
  [[ "$(cut -d'|' -f1 "$work/out" | tr '\n' ' ')" == "01 02 03 " ]] ||
    fail "a name broke the row format: $(cat "$work/out")"
}
start names-1
names "$(printf 'caf\303\251')" "a => b" "a\`b" "a|b"
start names-2
names "$(printf 'a\342\200\256b')" "::warning x" "@someone"

start binary-name
printf 'package app\000\n' >"$repo/internal/app/a|b.go"
run 1
expect 01 "bypass: binary unprintable-name-1" "❌"

# A directory name and a chain file name pass the same whitelist in row 02.
start topic-names
lines 1 "internal/x\`y/a.go"
lines 1 "internal/x|y/a.go"
lines 1 "internal/::error x/a.go"
mkdir -p "$repo/scripts"
printf 'x\n' >"$repo/scripts/a|b.sh"
run 0 PR_BODY="nothing"
expect 02 "packages 3: unprintable-name-" "❌"
if grep -qE '`|::|x\|y|a\|b' "$work/out"; then fail "a raw name reached the output: $(cat "$work/out")"; fi
[[ "$(cut -d'|' -f1 "$work/out" | tr '\n' ' ')" == "01 02 03 " ]] || fail "a name broke the row format: $(cat "$work/out")"

start topic-packages
for p in a b c d e; do lines 1 "internal/$p/$p.go"; done
run 0
expect 02 "packages 5: internal/a, internal/b, internal/c, internal/d, internal/e" "⚠️"

start topic-chain
lines 1 internal/app/a.go
lines 1 Makefile
lines 1 go.mod
run 0 PR_BODY="The Makefile gets a target."
expect 02 "chain: Makefile (in description) · go.mod: changed" "⚠️"

start topic-chain-unexplained
lines 1 .golangci.yml
run 0 PR_BODY="Lint settings."
expect 02 "chain: .golangci.yml (not in description)" "❌"

start topic-chain-no-body
lines 1 .github/workflows/ci.yml
run 0
expect 02 "chain: .github/workflows/ci.yml · go.mod: -" "⚠️"

start topic-clean
lines 1 internal/app/a.go
lines 1 internal/app/a_test.go
run 0 PR_BODY=""
expect 02 "chain: - · go.mod: - · packages 1: internal/app" "✅"
expect 03 "code +1 -0 · tests +1 -0 · ratio 1.0" "n/a (no PR_TITLE)"

start topic-four-packages
for p in a b c d; do lines 1 "internal/$p/$p.go"; done
run 0
expect 02 "packages 4: " "✅"

start topic-gomod
lines 1 go.mod
run 0
expect 02 "go.mod: changed" "⚠️"

start topic-chain-basename
lines 1 .github/workflows/ci.yml
lines 1 .goreleaser.yaml
run 0 PR_BODY="ci.yml gets a step, .goreleaser.yaml a hook."
expect 02 "chain: .github/workflows/ci.yml (in description), .goreleaser.yaml (in description)" "⚠️"

start topic-env
lines 1 .env
run 0 PR_BODY="x"
expect 02 "chain: .env (not in description)" "❌"

# The checklist of the PR template names Makefile itself: no explanation.
start topic-template-checklist
lines 1 Makefile
run 0 PR_BODY="$(printf -- '- [x] Changes to `.github/`, `scripts/`, `Makefile` do not weaken any CI check.\nRefactor the parser.')"
expect 02 "chain: Makefile (not in description)" "❌"

# Long package names are cut, the chain part stays visible.
start topic-long
for p in adapters/yandexrest adapters/space/macos adapters/network/macos adapters/photokit adapters/secrets/keychain; do
  lines 1 "internal/$p/x.go"
done
lines 1 Makefile
run 0 PR_BODY="none"
expect 02 "chain: Makefile (not in description) · go.mod: - · packages 5: " "❌"

# Row 03 counts what row 01 counts: testdata is not tests, an unedited move
# is no code.
start tests-testdata-not-tests
lines 3 internal/app/a.go
lines 3 internal/app/testdata/x_test.go
run 0 PR_TITLE="feat(app): add a"
expect 03 "code +3 -0 · tests +0 -0" "❌"

start tests-unedited-move
lines 10 internal/app/big.go
g add -A
g commit -qm "base with code"
git -C "$repo" mv internal/app/big.go internal/app/moved.go
run 0 PR_TITLE="feat(app): move big"
expect 03 "code +0 -0" "✅"

start tests-feat
lines 3 internal/app/a.go
run 0 PR_TITLE="feat(app): add a"
expect 03 "code +3 -0 · tests +0 -0" "❌"

start tests-fix-breaking
lines 3 internal/app/a.go
run 0 PR_TITLE="fix(app)!: change a"
expect 03 "code +3 -0" "❌"

start tests-refactor
lines 3 internal/app/a.go
run 0 PR_TITLE="refactor(app): split a"
expect 03 "code +3 -0" "⚠️"

start tests-present
lines 3 internal/app/a.go
lines 3 internal/testkit/fake.go
run 0 PR_TITLE="fix(app): repair a"
expect 03 "tests +3 -0" "✅"

start tests-no-code
lines 3 docs/a.md
run 0 PR_TITLE="feat(app): document a"
expect 03 "code +0 -0" "✅"

start arguments
lines 300 internal/app/a.go
run 1
(cd "$repo" && bash "$metrics" --base HEAD~1 --rows 02,03) >"$work/out" 2>/dev/null || fail "a row that was not printed blocked"
[[ "$(cut -d'|' -f1 "$work/out" | tr '\n' ' ')" == "02 03 " ]] || fail "--rows not applied: $(cat "$work/out")"
for args in "--base no-such-rev" "--base" "--out $work" "--bogus x" "--rows 1" "--rows 01,"; do
  got=0
  # shellcheck disable=SC2086 # the arguments are split on purpose
  (cd "$repo" && bash "$metrics" $args) >/dev/null 2>&1 || got=$?
  [[ "$got" == 2 ]] || fail "'$args' exited $got, want 2"
done

if ((failures > 0)); then
  echo "$failures pr-metrics case(s) failed" >&2
  exit 1
fi
echo "pr-metrics: all cases passed"
