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

# Changed lines inside covered and uncovered blocks; line 11 is inside 10-12,
# line 99 has no statements, b.go is not in the profile.
printf 'internal/app/a.go:11\ninternal/app/a.go:14\ninternal/app/a.go:30\ninternal/app/a.go:99\ninternal/app/b.go:1\ninternal/domain/d.go:6\n' >"$work/changed"
run 0 changed
expect_line "low changed lines: other 66.7% (4/6, min 80%)"
expect_line "ok changed lines: domain 100.0% (2/2, min 95%)"
expect_line " uncovered: internal/app/a.go:14"
expect_line " uncovered: internal/app/a.go:30"

# Only covered lines changed.
printf 'internal/app/a.go:21\n' >"$work/changed"
run 0 changed
expect_line "ok changed lines: other 100.0% (4/4, min 80%)"
expect_no "uncovered"
expect_no "changed lines: domain"

# Nothing measurable changed (docs, tests): n/a, still exit 0.
: >"$work/changed"
run 0 changed
expect_line "info changed lines: n/a (no statements on changed lines)"

# The changed lines never change the exit code: a failing total stays 1.
cat >"$work/cover.out" <<'EOF'
mode: atomic
example.com/m/internal/app/a.go:10.2,12.3 4 0
example.com/m/internal/app/a.go:14.2,14.9 1 1
EOF
printf 'internal/app/a.go:14\n' >"$work/changed"
run 1 changed
expect_line "ok changed lines: other 100.0% (1/1, min 80%)"

# At most five uncovered places, in path and line order.
{
  echo "mode: atomic"
  for line in 70 10 60 20 50 30 40; do
    echo "example.com/m/internal/app/c.go:$line.2,$line.9 1 0"
    echo "internal/app/c.go:$line" >>"$work/changed7"
  done
} >"$work/cover.out"
run 1 changed7
expect_line " uncovered: internal/app/c.go:10"
expect_line " uncovered: internal/app/c.go:50"
expect_no "c.go:60"

# A missing changed-lines file is a usage error.
run 2 missing

if ((failures > 0)); then
  echo "$failures check-coverage case(s) failed" >&2
  exit 1
fi
echo "check-coverage: all cases passed"
