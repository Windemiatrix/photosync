#!/usr/bin/env bash
# Self-test for scripts/check-release-deps.sh: feeds it go.mod files and checks
# the verdict. Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$here/check-release-deps.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/check-release-deps-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
failures=0

# expect <ok|fail> <go.mod content>
expect() {
  local want="$1" content="$2" got
  printf '%s\n' "$content" >"$tmp/go.mod"
  if bash "$checker" "$tmp/go.mod" >/dev/null 2>&1; then got=ok; else got=fail; fi
  if [[ "$got" != "$want" ]]; then
    printf 'FAIL: want %s, got %s for: %s\n' "$want" "$got" "$content" >&2
    failures=$((failures + 1))
  fi
}

lib=github.com/Windemiatrix/go-libs/osxphotos
head=$'module github.com/Windemiatrix/photosync\n\ngo 1.27.0'

expect ok "$head"
expect ok "$head"$'\n\nrequire golang.org/x/text v0.1.0'
expect ok "$head"$'\n\nrequire '"$lib"' v0.2.0'
expect ok "$head"$'\n\nrequire (\n\t'"$lib"$' v0.2.0\n\tgolang.org/x/text v0.1.0\n)'
expect ok "$head"$'\n\nrequire (\n\tgolang.org/x/text v0.1.0 // indirect\n\t'"$lib"' v1.10.3 // indirect\n)'

expect fail "$head"$'\n\nrequire '"$lib"' v0.2.1-0.20261020123456-abcdef123456'
expect fail "$head"$'\n\nrequire (\n\t'"$lib"' v0.0.0-20261020123456-abcdef123456\n)'
expect fail "$head"$'\n\nrequire '"$lib"' v0.2.0-rc1'

if ((failures > 0)); then
  echo "$failures check-release-deps case(s) failed" >&2
  exit 1
fi
echo "check-release-deps: all cases passed"
