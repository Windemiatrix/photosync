#!/usr/bin/env bash
# Checks that a release does not depend on an unreleased library: go.mod must
# name a tag of go-libs/osxphotos (vX.Y.Z), not a pseudo-version of some
# commit. Pseudo-versions are fine while development goes on, so the check runs
# only when a release is made: scripts/release.sh and the verify job of
# release.yml. A go.mod that does not require the library passes.
#
# Run:    scripts/check-release-deps.sh [path/to/go.mod], from the repository
#         root; scripts/release.sh and release.yml call it.
# Input:  the path of go.mod, default go.mod.
# Exit:   0 the library is required by a tag or not at all; 1 it is required
#         by anything else.
set -euo pipefail
export LC_ALL=C

lib=github.com/Windemiatrix/go-libs/osxphotos
release_version_re='^v[0-9]+\.[0-9]+\.[0-9]+$'

# required_version <go.mod>: prints the version of the library, from a
# one-line "require" or from a line inside a require block; nothing when the
# library is not required.
required_version() {
  awk -v m="$lib" '
    $1 == m { print $2; exit }
    $1 == "require" && $2 == m { print $3; exit }
  ' "$1"
}

gomod="${1:-go.mod}"
version="$(required_version "$gomod")"

if [[ -z "$version" ]]; then
  echo "$gomod does not require $lib: nothing to check"
  exit 0
fi
if ! [[ "$version" =~ $release_version_re ]]; then
  printf 'error: %s requires %s at %q: a release needs a tagged version (vX.Y.Z), not a pseudo-version\n' "$gomod" "$lib" "$version" >&2
  exit 1
fi
echo "$gomod requires $lib $version"
