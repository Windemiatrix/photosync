#!/usr/bin/env bash
# Self-test for scripts/check-coverage.sh: runs it on synthetic profiles in a
# temporary module and checks the exit code and the changed-lines report.
# Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$here/check-coverage.sh"
work="$(mktemp -d "${TMPDIR:-/tmp}/check-coverage-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
printf 'module example.com/m\n\ngo 1.26\n' >"$work/go.mod"
failures=0

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

# run <want exit code> [changed-lines file]: runs the checker on $work/cover.out.
# Runs of spaces in the output are squeezed to one.
run() {
  local want="$1" got=0
  shift
  (cd "$work" && LC_ALL=C bash "$checker" cover.out "$@") >"$work/raw" 2>&1 || got=$?
  tr -s ' ' <"$work/raw" >"$work/out"
  if [[ "$got" != "$want" ]]; then
    fail "want exit $want, got $got: $(cat "$work/out")"
  fi
}

# expect_line <fixed string>: the last run printed this line (spaces squeezed).
expect_line() {
  grep -qxF -- "$1" "$work/out" || fail "missing line '$1' in: $(cat "$work/out")"
}

# expect_no <fixed string>: the last run did not print this text.
expect_no() {
  if grep -qF -- "$1" "$work/out"; then fail "unexpected '$1' in: $(cat "$work/out")"; fi
}

# internal/app: 10 statements, 8 covered (80 %); internal/domain: 2 of 2.
cat >"$work/cover.out" <<'EOF'
mode: atomic
example.com/m/internal/app/a.go:10.2,12.3 4 1
example.com/m/internal/app/a.go:14.2,14.9 1 0
example.com/m/internal/app/a.go:20.2,25.3 4 3
example.com/m/internal/app/a.go:30.2,30.9 1 0
example.com/m/internal/domain/d.go:5.2,6.3 2 1
EOF

# Without the second argument: the old output only.
run 0
expect_line "ok all other packages (total) 80.0% (min 80%)"
expect_line "ok internal/domain 100.0% (min 95%)"
expect_no "changed lines"

# Each changed line with statements counts once. Line 11 is inside the covered
# block 10-12, lines 14 and 30 are uncovered, line 99 has no statements, b.go
# is not in the profile.
printf 'internal/app/a.go:11\ninternal/app/a.go:14\ninternal/app/a.go:30\ninternal/app/a.go:99\ninternal/app/b.go:1\ninternal/domain/d.go:6\n' >"$work/changed"
run 0 changed
expect_line "low changed lines: other 33.3% (1/3, min 80%)"
expect_line "ok changed lines: domain 100.0% (1/1, min 95%)"
expect_line " uncovered: internal/app/a.go:14"
expect_line " uncovered: internal/app/a.go:30"

# Only covered lines changed.
printf 'internal/app/a.go:21\n' >"$work/changed"
run 0 changed
expect_line "ok changed lines: other 100.0% (1/1, min 80%)"
expect_no "uncovered"
expect_no "changed lines: domain"

# Nothing measurable changed (docs, tests): n/a, still exit 0.
: >"$work/changed"
run 0 changed
expect_line "info changed lines: n/a (no statements on changed lines)"

# A line in a covered and an uncovered block is uncovered; the changed lines
# never change the exit code: a failing total stays 1.
cat >"$work/cover.out" <<'EOF'
mode: atomic
example.com/m/internal/app/a.go:10.2,12.3 4 0
example.com/m/internal/app/a.go:14.2,14.9 1 1
example.com/m/internal/app/a.go:15.2,15.20 1 1
example.com/m/internal/app/a.go:15.21,15.30 1 0
EOF
printf 'internal/app/a.go:14\ninternal/app/a.go:15\n' >"$work/changed"
run 1 changed
expect_line "low changed lines: other 50.0% (1/2, min 80%)"
expect_line " uncovered: internal/app/a.go:15"

# Excluded packages stay out; names outside the whitelist are replaced; at
# most five uncovered lines, by path, then by line number.
{
  echo "mode: atomic"
  echo "example.com/m/cmd/x/main.go:5.2,5.9 1 0"
  echo "cmd/x/main.go:5" >>"$work/changed7"
  echo "example.com/m/internal/app/b.go:1.2,1.9 1 0"
  echo "internal/app/b.go:1" >>"$work/changed7"
  for line in 9 10 100 11 12 13 200; do
    echo "example.com/m/internal/app/c.go:$line.2,$line.9 1 0"
    echo "internal/app/c.go:$line" >>"$work/changed7"
  done
  echo 'example.com/m/internal/a`b/x.go:1.2,1.9 1 0'
  echo 'internal/a`b/x.go:1' >>"$work/changed7"
} >"$work/cover.out"
run 1 changed7
expect_line "low changed lines: other 0.0% (0/9, min 80%)"
expect_line " uncovered: <unprintable name>:1"
expect_line " uncovered: internal/app/b.go:1"
expect_line " uncovered: internal/app/c.go:9"
expect_line " uncovered: internal/app/c.go:10"
expect_line " uncovered: internal/app/c.go:11"
expect_no "c.go:12"
expect_no "c.go:100"
expect_no "cmd/x"
expect_no '`'

# A missing file, a directory or a malformed line is a usage error.
run 2 missing
mkdir "$work/dir"
run 2 dir
printf 'internal/app/a.go\n' >"$work/bad"
run 2 bad


if ((failures > 0)); then
  echo "$failures check-coverage case(s) failed" >&2
  exit 1
fi
echo "check-coverage: all cases passed"
