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
# The optional second argument is a file of changed lines, one "<path>:<line>"
# per line, the path relative to the module root. Then the script also prints
# the share of covered lines among the changed lines with statements, per
# group, against the same threshold, and up to five uncovered lines. It is
# information for the PR metrics: the exit code still depends only on the
# thresholds above (2 for a malformed or unreadable file).
#
# Usage, from the module root (the directory with go.mod):
#   go test -race -covermode=atomic -coverprofile=coverage.out ./...
#   scripts/check-coverage.sh coverage.out [changed-lines.txt]
#
# Plain POSIX awk and bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

profile="${1:-}"
changed="${2:-}"
if [[ -z "$profile" || ! -r "$profile" || ( -n "$changed" && ! ( -f "$changed" && -r "$changed" ) ) ]]; then
  echo "usage: $0 <coverprofile> [changed-lines]   (run from the module root)" >&2
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

# The path goes through the environment: awk -v would expand backslashes.
CHANGED="$changed" awk -v module="$module" -v excl="${excluded[*]}" -v dmin="$domain_min" -v omin="$other_min" '
BEGIN {
  changed = ENVIRON["CHANGED"]
  if (changed != "") {
    while ((rc = (getline line < changed)) > 0) {
      n = match(line, /:[0-9]+$/)
      if (n > 1) chg[substr(line, 1, n - 1), substr(line, n + 1) + 0] = 1
      else if (line != "") bad = "malformed line in " changed
    }
    if (rc < 0) bad = "cannot read " changed
    close(changed)
  }
}

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
  if (bad != "") {
    print "error: " bad > "/dev/stderr"
    exit 2
  }
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
      group = "domain"
      dtot[pkg] += stmts[key]
      dcov[pkg] += covered
    } else {
      group = "other"
      otot += stmts[key]
      ocov += covered
    }

    # A changed line counts once if a block with statements spans it, and as
    # uncovered if any such block is uncovered (if err != nil { return err }).
    if (changed == "" || stmts[key] + 0 == 0) continue
    # <file>:<start line>.<col>,<end line>.<col>
    rel = file
    if (index(rel, module "/") == 1) rel = substr(rel, length(module) + 2)
    split(substr(key, length(file) + 2), ends, ",")
    for (l = int(ends[1]); l <= int(ends[2]); l++) {
      if (!((rel, l) in chg)) continue
      lgroup[rel, l] = group
      if (!covered) lunc[rel, l] = 1
    }
  }
  for (k in lgroup) {
    ctot[lgroup[k]]++
    if (!(k in lunc)) { ccov[lgroup[k]]++; continue }
    split(k, part, SUBSEP)
    # A name from the PR reaches the output only through this whitelist.
    if (part[1] !~ /^[A-Za-z0-9._\/+-]+$/) part[1] = "<unprintable name>"
    uncovered[part[1] ":" part[2]] = 1
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

  # Changed lines: information only, never part of the exit code.
  if (changed != "") {
    if (ctot["domain"] + ctot["other"] == 0) {
      print "info changed lines: n/a (no statements on changed lines)"
    }
    for (g = 1; g <= 2; g++) {
      group = (g == 1) ? "domain" : "other"
      if (ctot[group] + 0 == 0) continue
      min = (group == "domain") ? dmin : omin
      pct = 100 * ccov[group] / ctot[group]
      verdict = (pct + 0 < min + 0) ? "low" : "ok"
      printf "%-4s %-45s %5.1f%% (%d/%d, min %s%%)\n", verdict, "changed lines: " group, pct, ccov[group], ctot[group], min
    }
    cmd = "sort -t: -k1,1 -k2,2n | head -n 5 | sed \"s/^/     uncovered: /\""
    for (place in uncovered) print place | cmd
    close(cmd)
  }
  exit failed
}
' "$profile"
