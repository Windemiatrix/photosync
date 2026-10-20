#!/usr/bin/env bash
# A release of the application must not depend on an unreleased library: the
# go.mod of a tagged commit has to name a tag of go-libs/osxphotos, not a
# pseudo-version of some commit. While development goes on, pseudo-versions are
# fine; this check runs only when a release is made (scripts/release.sh and the
# verify job of release.yml). A go.mod that does not require the library at all
# passes.
#
# Usage: scripts/check-release-deps.sh [path/to/go.mod]
set -euo pipefail
export LC_ALL=C

lib=github.com/Windemiatrix/go-libs/osxphotos
gomod="${1:-go.mod}"

# The module is a "require" line or a line inside a require block.
version="$(awk -v m="$lib" '
  $1 == m { print $2; exit }
  $1 == "require" && $2 == m { print $3; exit }
' "$gomod")"

if [[ -z "$version" ]]; then
  echo "$gomod does not require $lib: nothing to check"
  exit 0
fi

re_release='^v[0-9]+\.[0-9]+\.[0-9]+$'
if ! [[ "$version" =~ $re_release ]]; then
  printf 'error: %s requires %s at %q: a release needs a tagged version (vX.Y.Z), not a pseudo-version\n' "$gomod" "$lib" "$version" >&2
  exit 1
fi
echo "$gomod requires $lib $version"
