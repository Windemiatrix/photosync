#!/usr/bin/env bash
# Coverage gate: reads a Go coverage profile and checks the thresholds.
#
#   internal/domain/...  each package on its own, at least 95 %
#   everything else      one total for the group, at least 80 %
#
# Packages listed in "excluded" are left out of both groups: an entry matches
# that package only, an entry ending in "/..." also its subpackages. When
# nothing is left to check (the skeleton, or a profile without statements) the
# gate passes and says so.
#
# Run:    make cover, which runs from the module root (the directory with go.mod):
#           go test -race -covermode=atomic -coverprofile=coverage.out ./...
#           scripts/check-coverage.sh coverage.out
# Input:  the path of the coverage profile.
# Output: one line per checked group with its verdict, percentage and minimum.
# Exit:   0 every group reaches its minimum; 1 some group does not; 2 no
#         readable profile or no go.mod in the current directory.
#
# Plain POSIX awk and bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

domain_min=95
other_min=80

# Packages relative to the module root. Their code is composition-only wiring
# or a developer tool, so a percentage would say nothing about its quality.
excluded=(
  "cmd/..."          # composition root: wiring only
  "tools/anonymize"  # main package of a developer tool
)

profile="${1:-}"
if [[ -z "$profile" || ! -r "$profile" ]]; then
  echo "usage: $0 <coverprofile>   (run from the module root)" >&2
  exit 2
fi

module="$(awk '$1 == "module" { print $2; exit }' go.mod)"
if [[ -z "$module" ]]; then
  echo "error: no module line in go.mod (run from the module root)" >&2
  exit 2
fi

awk -v module="$module" -v excluded_entries="${excluded[*]}" \
  -v domain_min="$domain_min" -v other_min="$other_min" '
function file_of(block,    file) {
  file = block
  sub(/:[^:]*$/, "", file)
  return file
}

function dir_of(path,    dir) {
  dir = path
  sub(/\/[^\/]*$/, "", dir)
  return dir
}

function relative_to_module(path) {
  if (index(path, module "/") == 1) return substr(path, length(module) + 2)
  if (path == module) return "."
  return path
}

function package_of(block) {
  return relative_to_module(dir_of(file_of(block)))
}

function matches_entry(pkg, entry,    root) {
  if (pkg == entry) return 1
  if (entry !~ /\/\.\.\.$/) return 0
  root = substr(entry, 1, length(entry) - 4)
  return (pkg == root || index(pkg, root "/") == 1)
}

function is_excluded(pkg,    i, n, entries) {
  n = split(excluded_entries, entries, " ")
  for (i = 1; i <= n; i++) {
    if (matches_entry(pkg, entries[i])) return 1
  }
  return 0
}

function is_domain(pkg) {
  return (pkg == "internal/domain" || index(pkg, "internal/domain/") == 1)
}

function is_below(pct, min) {
  return pct < min + 0
}

function verdict_line(name, pct, min) {
  return sprintf("%-4s %-45s %5.1f%% (min %s%%)", is_below(pct, min) ? "FAIL" : "ok", name, pct, min)
}

NR == 1 && $1 == "mode:" { next }

# A profile line: <import path>/<file>.go:<start>,<end> <statements> <hit count>.
# The same block can be listed more than once; it counts as covered when any
# of its lines has hits.
NF == 3 {
  statements[$1] = $2
  if ($3 + 0 > hits[$1] + 0) hits[$1] = $3
}

END {
  for (block in statements) {
    pkg = package_of(block)
    if (is_excluded(pkg)) continue

    covered = (hits[block] + 0 > 0) ? statements[block] + 0 : 0
    if (is_domain(pkg)) {
      domain_statements[pkg] += statements[block]
      domain_covered[pkg] += covered
    } else {
      other_statements += statements[block]
      other_covered += covered
    }
  }

  failed = 0
  checked = 0

  # The order of "for (pkg in ...)" is unspecified in awk: the lines are
  # sorted by package to keep the output stable.
  for (pkg in domain_statements) {
    pct = 100 * domain_covered[pkg] / domain_statements[pkg]
    checked++
    if (is_below(pct, domain_min)) failed = 1
    print verdict_line(pkg, pct, domain_min) | "sort -k2"
  }
  close("sort -k2")

  if (other_statements > 0) {
    pct = 100 * other_covered / other_statements
    checked++
    if (is_below(pct, other_min)) failed = 1
    print verdict_line("all other packages (total)", pct, other_min)
  }

  if (checked == 0) {
    print "coverage: nothing to check (every package is excluded or has no statements)"
  }
  exit failed
}
' "$profile"
