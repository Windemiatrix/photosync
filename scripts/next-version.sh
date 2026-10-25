#!/usr/bin/env bash
# Prints the next release tag, computed from the PR titles since the last tag.
# The title of a PR becomes the title of the squash commit on main
# (scripts/check-pr-title.sh), so the history says what changed.
#
# Run:    scripts/next-version.sh [--major]      (scripts/release.sh calls it)
#         from the repository root; also by hand, to see the next release.
# Input:  --major  the next major version; the only way from 0.x to 1.0.0.
# Output: stdout has the next tag and nothing else, for example v0.2.0; stderr
#         lists every title with its class, the warnings and the tag.
# Exit:   0 done; 1 nothing to release, or a check failed; 2 wrong arguments.
#
# Version rule, by the titles since the last tag:
#
#   type(scope)!: ...   minor before 1.0.0, major from 1.0.0
#   feat(scope): ...    minor
#   anything else       patch, a title outside the rule too
#
# A title is checked against the PR title rule once the " (#N)" that GitHub
# appends is cut off. The length limit is not checked: it applies to the PR
# title, and the squash commit is longer by the suffix.
#
# Titles are untrusted data: they are never executed, and every byte outside
# printable ASCII is printed as "?".
#
# bash 3.2 compatible: the script also runs on macOS.
set -euo pipefail

# In the C locale a bracket range and a tr class are plain byte ranges.
export LC_ALL=C

# The rule of scripts/check-pr-title.sh with the type and the "!" captured:
# change both together.
title_re='^(feat|fix|docs|style|refactor|test|chore)\([a-z0-9._/-]+\)(!?): [ -~]+$'
pr_number_suffix_re='^(.*) \(#[0-9]+\)$'
tag_re='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'

# Arguments.
force_major=0

# The last tag and the commits after it; set by find_last_tag.
major=0
minor=0
patch=0
range=HEAD
base="none (first release)"

# The titles since the last tag, counted by classify_titles.
breaking=0
feats=0
outside=0

die() {
  echo "error: $1" >&2
  exit 1
}

usage() {
  echo "usage: scripts/next-version.sh [--major]" >&2
  exit 2
}

# esc <text>: the text with every byte outside printable ASCII shown as "?".
esc() {
  printf '%s' "$1" | tr -c '[:print:]' '?'
}

parse_args() {
  local arg
  for arg in "$@"; do
    case "$arg" in
      --major) force_major=1 ;;
      *) usage ;;
    esac
  done
}

# Without a tag the first-release values stay.
find_last_tag() {
  local last
  last="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
  if [[ -z "$last" ]]; then
    return
  fi
  if ! [[ "$last" =~ $tag_re ]]; then
    die "the last tag is not vMAJOR.MINOR.PATCH: $(esc "$last")"
  fi
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  range="$last..HEAD"
  base="$last"
}

require_commits() {
  local count
  count="$(git rev-list --count "$range")"
  if ((count == 0)); then
    die "no commits since $base: nothing to release"
  fi
  echo "Last tag: $base; $count commit(s) since it:" >&2
}

# title_class <title>: prints breaking, feat, patch, or other for a title
# outside the rule.
title_class() {
  local title="$1"
  if [[ "$title" =~ $pr_number_suffix_re ]]; then
    title="${BASH_REMATCH[1]}"
  fi
  if ! [[ "$title" =~ $title_re ]]; then
    echo other
    return
  fi

  local commit_type="${BASH_REMATCH[1]}" breaking_mark="${BASH_REMATCH[2]}"
  if [[ -n "$breaking_mark" ]]; then
    echo breaking
  elif [[ "$commit_type" == feat ]]; then
    echo feat
  else
    echo patch
  fi
}

# Prints every title with its class on stderr.
classify_titles() {
  local title class
  while IFS= read -r title; do
    class="$(title_class "$title")"
    case "$class" in
      breaking) breaking=$((breaking + 1)) ;;
      feat) feats=$((feats + 1)) ;;
      other) outside=$((outside + 1)) ;;
    esac
    printf '  %-9s %s\n' "$class" "$(esc "$title")" >&2
  done < <(git log --format=%s "$range")
}

warn_about_titles_outside_rule() {
  if ((outside > 0)); then
    echo "warning: $outside title(s) marked \"other\" do not follow \"<type>(<scope>): <description>\"; they count as patch changes and go to the release notes as they are" >&2
  fi
}

allow_major_flag_only_on_0x() {
  if ((force_major && major != 0)); then
    die "--major only moves 0.x to 1.0.0, the last tag is $base"
  fi
}

next_tag() {
  if ((force_major)); then
    echo "v1.0.0"
  elif ((breaking > 0 && major == 0)); then
    # Before 1.0.0 an incompatible change is a minor version (semver 0.x).
    echo "v0.$((minor + 1)).0"
  elif ((breaking > 0)); then
    echo "v$((major + 1)).0.0"
  elif ((feats > 0)); then
    echo "v$major.$((minor + 1)).0"
  else
    echo "v$major.$minor.$((patch + 1))"
  fi
}

parse_args "$@"
find_last_tag
require_commits
classify_titles
warn_about_titles_outside_rule
allow_major_flag_only_on_0x
tag="$(next_tag)"
echo "Next tag: $tag" >&2
echo "$tag"
