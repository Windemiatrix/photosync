#!/usr/bin/env bash
# Creates and pushes the signed release tag. Pushing the tag starts the
# "release" workflow, which builds a draft GitHub Release; the maintainer reads
# its notes and publishes it.
#
# Run:    scripts/release.sh [--major]
#         by the maintainer, on a local machine, from anywhere inside the
#         repository, in a terminal; AI agents do not run it (AGENTS.md). The
#         tag ruleset lets only the repository admin create tags.
# Input:  --major  the next major version; the only way from 0.x to 1.0.0.
#         Otherwise scripts/next-version.sh computes the version from the PR
#         titles since the last tag.
# Exit:   0 the tag is pushed; 1 a check failed, the release was cancelled or
#         the push failed; 2 wrong arguments; the status of git or gh when one
#         of them fails.
#
# Steps, in order; the first failed check stops the script:
#   1. fetch origin/main and the tags;
#   2. check that the current branch is main, HEAD equals origin/main and the
#      working tree is clean;
#   3. compute the next version and show the class of every title;
#   4. check that ci.yml succeeded on the push of this commit to main;
#   5. check that go.mod requires a tagged version of the library
#      (scripts/check-release-deps.sh);
#   6. ask the maintainer to confirm, then create the signed tag and push it.
#
# Needs: git with a signing key, gh signed in.
#
# bash 3.2 compatible: the script also runs on macOS.
set -euo pipefail
export LC_ALL=C

# Named explicitly: otherwise gh picks the repository from the git remotes,
# and one of them may be a fork.
repo_slug=Windemiatrix/photosync

major_flag=""
# The commit that passed the checks; the tag goes on it even if HEAD moves
# while the script waits for the confirmation.
sha=""
tag=""

die() {
  echo "error: $1" >&2
  exit 1
}

usage() {
  echo "usage: scripts/release.sh [--major]" >&2
  exit 2
}

parse_args() {
  local arg
  for arg in "$@"; do
    case "$arg" in
      --major) major_flag="--major" ;;
      *) usage ;;
    esac
  done
}

# Checked first, so that the script stops before any network call.
require_terminal() {
  if ! [[ -t 0 ]]; then
    die "run it in a terminal: the release needs your confirmation"
  fi
}

# The repository that holds this script, whatever the current directory.
go_to_repository_root() {
  local here root
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  root="$(git -C "$here" rev-parse --show-toplevel)" || die "not inside a git repository"
  cd "$root"
}

fetch_main_and_tags() {
  git fetch --quiet origin main --tags
}

# refs/remotes/origin/main in full: a local tag or branch named origin/main
# would otherwise take its place.
require_clean_main() {
  local branch
  branch="$(git symbolic-ref --short -q HEAD || true)"
  if [[ "$branch" != main ]]; then
    die "the current branch must be main"
  fi
  sha="$(git rev-parse HEAD)"
  if [[ "$sha" != "$(git rev-parse refs/remotes/origin/main)" ]]; then
    die "HEAD is not origin/main: pull or push first"
  fi
  if [[ -n "$(git status --porcelain)" ]]; then
    die "the work tree is not clean"
  fi
}

compute_tag() {
  tag="$(bash scripts/next-version.sh ${major_flag:+"$major_flag"})"
}

# ci-ok depends on every required job and runs with "if: always()", so a
# successful ci.yml run means ci-ok is green.
require_green_ci() {
  local runs
  runs="$(gh run list --repo "$repo_slug" --workflow ci.yml --commit "$sha" --event push --status success --limit 1 --json databaseId --jq length)"
  if [[ "$runs" != 1 ]]; then
    die "no successful ci.yml run on the push of ${sha:0:12} to main"
  fi
}

require_tagged_library() {
  bash scripts/check-release-deps.sh
}

confirm() {
  local answer
  printf '\nCreate the signed tag %s on %s and push it? [y/N] ' "$tag" "${sha:0:12}"
  read -r answer
  if [[ "$answer" != y && "$answer" != Y ]]; then
    die "cancelled"
  fi
}

create_and_push_tag() {
  git tag -s "$tag" -m "photosync $tag" "$sha"
  if ! git push origin "refs/tags/$tag"; then
    die "the push failed; the local tag $tag was created, delete it with: git tag -d $tag"
  fi
  echo "Pushed $tag. release.yml builds a draft GitHub Release: read the notes and publish it."
}

parse_args "$@"
require_terminal
go_to_repository_root
fetch_main_and_tags
require_clean_main
compute_tag
require_green_ci
require_tagged_library
confirm
create_and_push_tag
