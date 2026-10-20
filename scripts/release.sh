#!/usr/bin/env bash
# Creates and pushes the release tag. Only the maintainer runs it, on their own
# machine, from a clone of the repository: pushing a tag is what starts
# release.yml, and creating tags is limited by the tag ruleset to the
# repository admin. Agents and workflows do not run it.
#
# What it does, in order; the first failed check stops it:
#   1. fetches origin/main and the tags;
#   2. checks that HEAD is main, equals origin/main, and the work tree is clean;
#   3. computes the next version from the PR titles since the last tag
#      (scripts/next-version.sh) and shows every title with its class;
#   4. checks that the tag exists neither locally nor in origin;
#   5. checks that CI passed on this very commit (push to main, ci.yml);
#   6. checks that go.mod names a tagged version of the library;
#   7. asks for confirmation, then creates the signed tag and pushes it.
#
# Usage: scripts/release.sh [--major]
#   --major   v0.x -> v1.0.0 (never inferred from commits)
#
# The release notes are not made here: GoReleaser builds a draft GitHub Release
# from the same titles in release.yml; the maintainer reads it and publishes it.
set -euo pipefail
export LC_ALL=C

repo_slug=Windemiatrix/photosync

die() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

flag=""
for arg in "$@"; do
  case "$arg" in
    --major) flag="--major" ;;
    *)
      printf 'usage: scripts/release.sh [--major]\n' >&2
      exit 2
      ;;
  esac
done

command -v gh >/dev/null || die "the GitHub CLI (gh) is needed to check CI"
[[ -t 0 ]] || die "run it in a terminal: the release needs your confirmation"
cd "$(git rev-parse --show-toplevel)"

# 1-2. Fresh state of main, nothing uncommitted.
git fetch --quiet origin main --tags
branch="$(git symbolic-ref --short -q HEAD || true)"
[[ "$branch" == main ]] || die "the current branch must be main"
sha="$(git rev-parse HEAD)"
[[ "$sha" == "$(git rev-parse refs/remotes/origin/main)" ]] || die "HEAD is not origin/main: pull or push first"
[[ -z "$(git status --porcelain)" ]] || die "the work tree is not clean"

# 3. The version and the list of titles (stderr of the script goes to the terminal).
tag="$(bash scripts/next-version.sh ${flag:+"$flag"})"

# 4. The tag is free.
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  die "the tag $tag already exists locally"
fi
if [[ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]]; then
  die "the tag $tag already exists in origin"
fi

# 5. CI is green on this commit. ci-ok aggregates all required jobs and runs
# with "if: always()", so a successful run means a green ci-ok.
runs="$(gh run list --repo "$repo_slug" --workflow ci.yml --commit "$sha" --event push --status success --limit 1 --json databaseId --jq length)"
[[ "$runs" == 1 ]] || die "no successful ci.yml run on the push of ${sha:0:12} to main"

# 6. The library is a tag, not a pseudo-version.
bash scripts/check-release-deps.sh

# 7. Confirmation, tag, push.
printf '\nCreate the signed tag %s on %s and push it? [y/N] ' "$tag" "${sha:0:12}"
read -r answer
[[ "$answer" == y || "$answer" == Y ]] || die "cancelled"

git tag -s "$tag" -m "photosync $tag" "$sha"
if ! git push origin "refs/tags/$tag"; then
  die "the push failed; the local tag $tag was created, delete it with: git tag -d $tag"
fi
echo "Pushed $tag. release.yml builds a draft GitHub Release: read the notes and publish it."
