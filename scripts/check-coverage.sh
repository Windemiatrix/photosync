#!/usr/bin/env bash
# Coverage gate: reads a Go coverage profile and checks the thresholds.
#
#   internal/domain/...  each package on its own, at least DOMAIN_MIN (95)
#   everything else      one total for the group, at least OTHER_MIN (80)
#
# Packages listed in "excluded" are left out of both groups: an entry matches
# that package only, an entry ending in "/..." also its subpackages.
# If nothing is left to check (the skeleton, or a profile without statements)
# the gate passes and says so.
#
# Usage, from the module root (the directory with go.mod):
#   go test -race -covermode=atomic -coverprofile=coverage.out ./...
#   scripts/check-coverage.sh coverage.out
#
# Plain POSIX awk and bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

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

domain_min="${DOMAIN_MIN:-95}"
other_min="${OTHER_MIN:-80}"

# Packages relative to the module root, matched as path prefixes. Their code
# runs only against real system APIs or is composition-only wiring, so a
# percentage would say nothing about its quality.
excluded=(
  "cmd/..."                             # composition root: wiring only
  "tools/anonymize"                     # main package of a developer tool
  "internal/adapters/secrets/keychain"  # cgo, Security.framework
  "internal/adapters/space/macos"       # cgo, volume capacity API
  "internal/adapters/network/macos"     # cgo, NWPathMonitor
)

awk -v module="$module" -v excl="${excluded[*]}" -v dmin="$domain_min" -v omin="$other_min" '
function is_excluded(pkg,    i, n, parts, root) {
  n = split(excl, parts, " ")
  for (i = 1; i <= n; i++) {
    if (pkg == parts[i]) return 1
    if (parts[i] ~ /\/\.\.\.$/) {
      root = substr(parts[i], 1, length(parts[i]) - 4)
      if (pkg == root || index(pkg, root "/") == 1) return 1
    }
  }
  return 0
}

# Profile line: <import path>/<file>.go:<start>,<end> <statements> <count>
NR == 1 && $1 == "mode:" { next }
NF == 3 {
  key = $1
  stmts[key] = $2
  if ($3 + 0 > hits[key] + 0) hits[key] = $3
}

END {
  for (key in stmts) {
    file = key
    sub(/:[^:]*$/, "", file)
    pkg = file
    sub(/\/[^\/]*$/, "", pkg)
    if (index(pkg, module "/") == 1) pkg = substr(pkg, length(module) + 2)
    else if (pkg == module) pkg = "."
    if (is_excluded(pkg)) continue

    covered = (hits[key] + 0 > 0) ? stmts[key] + 0 : 0
    if (pkg == "internal/domain" || index(pkg, "internal/domain/") == 1) {
      dtot[pkg] += stmts[key]
      dcov[pkg] += covered
    } else {
      otot += stmts[key]
      ocov += covered
    }
  }

  failed = 0
  checked = 0

  for (pkg in dtot) {
    checked++
    pct = 100 * dcov[pkg] / dtot[pkg]
    verdict = (pct + 0 < dmin + 0) ? "FAIL" : "ok"
    if (verdict == "FAIL") failed = 1
    printf "%-4s %-45s %5.1f%% (min %s%%)\n", verdict, pkg, pct, dmin | "sort -k2"
  }
  close("sort -k2")

  if (otot > 0) {
    checked++
    pct = 100 * ocov / otot
    verdict = (pct + 0 < omin + 0) ? "FAIL" : "ok"
    if (verdict == "FAIL") failed = 1
    printf "%-4s %-45s %5.1f%% (min %s%%)\n", verdict, "all other packages (total)", pct, omin
  }

  if (checked == 0) {
    print "coverage: nothing to check (every package is excluded or has no statements)"
  }
  exit failed
}
' "$profile"
