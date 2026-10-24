#!/usr/bin/env bash
# PR metrics: prints rows of the metrics table for the changes between the
# merge base of --base and HEAD. CI posts them as a PR comment; locally run
# "make pr-metrics" before asking for a push.
#
#   01 production code lines (blocking: more than 200, or a bypass of the count)
#
# Output: one line per row, "<NN>|<metric>|<value>|<verdict>". Values never
# contain "|" or "`": file names pass a whitelist first.
# Exit code: 0 no blocking row failed, 1 a blocking row failed, 2 error.
#
# Usage, from the repository root:
#   scripts/pr-metrics.sh [--base origin/main] [--rows 01] [--out rows.txt]
#
# Plain POSIX awk and bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

# Repository specifics (the copy in go-libs differs here only).
max_lines=200
testkit_is_test=1   # internal/testkit/** counts as tests (depguard keeps it out of production)

base="origin/main"
rows="01"
out=""
while (($# > 0)); do
  case "$1" in
    --base) base="${2:-}"; shift 2 ;;
    --rows) rows="${2:-}"; shift 2 ;;
    --out) out="${2:-}"; shift 2 ;;
    *) echo "usage: $0 [--base <rev>] [--rows NN,NN] [--out <file>]" >&2; exit 2 ;;
  esac
done
[[ -n "$base" ]] || { echo "pr-metrics: --base is empty" >&2; exit 2; }
[[ -n "$rows" ]] || rows="01"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/pr-metrics.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

# No textconv and no external diff: .gitattributes of the PR cannot rewrite the
# count. A file git still takes as binary has no line count ("-"); in production
# code that is a bypass. NUL-separated output is turned into lines; a newline
# inside a name becomes \001 and fails the name whitelist.
diff_z() {
  git -c core.quotePath=false diff --text --no-textconv --no-ext-diff -z "$@" "$base...HEAD" |
    LC_ALL=C tr '\012\000' '\001\012'
}
diff_z --numstat --no-renames >"$tmp/plain" || { echo "pr-metrics: git diff failed for base $base" >&2; exit 2; }
diff_z --numstat -M >"$tmp/renames" || exit 2
diff_z --raw -M >"$tmp/raw" || exit 2

if [[ "${GITHUB_ACTIONS:-}" == true ]]; then gha=1; else gha=0; fi
[[ -n "$out" ]] || out=/dev/stdout

LC_ALL=C awk \
  -v max="$max_lines" -v testkit="$testkit_is_test" -v rows=",$rows," -v gha="$gha" \
  -v plain="$tmp/plain" -v renames="$tmp/renames" -v raw="$tmp/raw" '
function category(p) {
  if (p ~ /(^|\/)testdata\//) return "testdata"
  if (p ~ /(^|\/)go\.sum$/) return "gosum"
  if (p ~ /_test\.go$/) return "test"
  if (p ~ /^scripts\/[^\/]*-test\.sh$/) return "test"
  if (testkit && p ~ /(^|\/)internal\/testkit\//) return "test"
  if (p ~ /\.(go|c|cc|cpp|m|mm|h|s|S)$/) return "prod"
  if (p ~ /^scripts\// || p ~ /^\.github\/workflows\// || p == "Makefile") return "prod"
  if (p ~ /\.md$/) return "docs"
  return "other"
}
# A name chosen by the PR author reaches the table only through this whitelist.
function name(p, id) {
  if (p ~ /^[A-Za-z0-9._\/@+-]+$/) return p
  return "<unprintable name #" id ">"
}
function cell(s) {
  if (length(s) > 200) s = substr(s, 1, 199) "..."
  return s
}
function row(id, metric, value, verdict) {
  if (index(rows, "," id ",")) printf "%s|%s|%s|%s\n", id, metric, cell(value), verdict
}
# "<added>\t<deleted>\t<path>"; the path may itself contain tabs.
function numstat(line, f,    i, rest) {
  i = index(line, "\t"); f["add"] = substr(line, 1, i - 1); rest = substr(line, i + 1)
  i = index(rest, "\t"); f["del"] = substr(rest, 1, i - 1); f["path"] = substr(rest, i + 1)
}

BEGIN {
  n = 0
  while ((getline line < plain) > 0) {
    if (line == "") continue
    numstat(line, f)
    n++; path[n] = f["path"]; add[n] = f["add"]; del[n] = f["del"]
  }
  close(plain)

  # -M: a rename is "<added>\t<deleted>\t" followed by the old and the new path.
  # Inside one category its own numbers count (an unedited move is 0).
  pairs = 0
  while ((getline line < renames) > 0) {
    if (line !~ /^[0-9-]+\t[0-9-]+\t$/) continue
    numstat(line, f)
    if ((getline old < renames) <= 0 || (getline new < renames) <= 0) break
    if (category(old) != category(new)) continue
    skip[old] = 1; skip[new] = 1
    pairs++; padd[pairs] = f["add"]; pdel[pairs] = f["del"]; ppath[pairs] = new
  }
  close(renames)

  # --raw: ":<old mode> <new mode> <old sha> <new sha> <status>", then one path,
  # or two for a rename or copy. A symbolic link in production code is a bypass:
  # the link counts as one line, the build reads the whole target.
  while ((getline line < raw) > 0) {
    if (substr(line, 1, 1) != ":") continue
    split(line, m, " ")
    paths = (m[5] ~ /^[RC]/) ? 2 : 1
    for (k = 1; k <= paths; k++) if ((getline p < raw) <= 0) break
    if (m[2] == "120000" && category(p) == "prod") bypass = bypass " link " name(p, "raw")
  }
  close(raw)

  for (i = 1; i <= n; i++) {
    if (path[i] in skip) continue
    count(path[i], add[i], del[i], i)
  }
  for (i = 1; i <= pairs; i++) count(ppath[i], padd[i], pdel[i], "rename " i)

  # Row 01: production code lines.
  prod = padd_total["prod"] + pdel_total["prod"]
  value = prod " (+" (padd_total["prod"] + 0) " -" (pdel_total["prod"] + 0) ")"
  value = value " · tests " total("test") " · testdata " total("testdata") " · go.sum " total("gosum") " · docs " total("docs") " · other " total("other")
  if (bypass != "") {
    row("01", "Production code lines", "bypass:" bypass, "❌")
    blocked = blocked " 01"
  } else if (prod > max) {
    row("01", "Production code lines", value " (max " max ")", "❌")
    blocked = blocked " 01"
  } else {
    row("01", "Production code lines", value, "✅")
  }

  # Only a printed row can block: the macOS job prints rows that never do.
  if (blocked != "" && index(rows, ",01,")) {
    printf "%sblocking rows failed:%s\n", (gha ? "::error::pr-metrics: " : "pr-metrics: "), blocked > "/dev/stderr"
    exit 1
  }
}
function total(c) { return padd_total[c] + pdel_total[c] }
# Lines of one diff entry. "-" (binary) on a production path is a bypass.
function count(p, a, d, id,    c) {
  c = category(p)
  if (a == "-" || d == "-") {
    if (c == "prod") bypass = bypass " binary " name(p, id)
    a = 0; d = 0
  }
  padd_total[c] += a; pdel_total[c] += d
}
' >"$out"
