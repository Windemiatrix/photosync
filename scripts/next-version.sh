#!/usr/bin/env bash
# Prints the next release tag, computed from the titles of the commits made
# since the last tag. The title of a squash commit on main is the title of its
# pull request, so the rule is the same one scripts/check-pr-title.sh enforces:
#
#   <type>(<scope>)[!]: <description>
#
# Titles since the last tag decide the next version:
#
#   "!" after the scope  before 1.0.0: minor (semver 0.x); from 1.0.0: major
#   type "feat"          minor
#   anything else        patch (a title outside the rule counts as patch, too)
#   no commits           no release: the script fails
#
# 1.0.0 is never inferred from commits: pass --major to move from 0.x to 1.0.0.
#
# Only the tag goes to stdout. The class of every title and the warnings go to
# stderr, so that release.sh can do tag="$(scripts/next-version.sh)" and still
# show them. Titles are untrusted input: every byte outside printable ASCII is
# printed as "?", and they never reach eval or a command line.
#
# Usage: scripts/next-version.sh [--major]
set -euo pipefail

# In the C locale a bracket range is a plain byte range.
export LC_ALL=C

die() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

# The same expression as in scripts/check-pr-title.sh, without the length limit
# (a squash commit title is longer than the PR title by " (#N)").
re_rule='^(feat|fix|docs|style|refactor|test|chore)\([a-z0-9._/-]+\)!?: [ -~]+$'
re_break='^[a-z]+\([a-z0-9._/-]+\)!: '
re_feat='^feat\('
re_suffix='^(.*) \(#[0-9]+\)$'
re_tag='^v(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$'

force_major=0
for arg in "$@"; do
  case "$arg" in
    --major) force_major=1 ;;
    *)
      printf 'error: unknown argument: %s\nusage: scripts/next-version.sh [--major]\n' "${arg//[^ -~]/?}" >&2
      exit 2
      ;;
  esac
done

# The last tag reachable from HEAD; no tag at all means the first release.
last="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [[ -z "$last" ]]; then
  major=0 minor=0 patch=0
  range=HEAD
  base="none (first release)"
elif [[ "$last" =~ $re_tag ]]; then
  major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}" patch="${BASH_REMATCH[3]}"
  range="$last..HEAD"
  base="$last"
else
  die "the last tag is not vMAJOR.MINOR.PATCH: ${last//[^ -~]/?}"
fi

count="$(git rev-list --count "$range")"
if ((count == 0)); then
  die "no commits since $base: nothing to release"
fi
printf 'Last tag: %s; %d commit(s) since it:\n' "$base" "$count" >&2

breaking=0 feats=0 outside=0
while IFS= read -r title; do
  stripped="$title"
  if [[ "$stripped" =~ $re_suffix ]]; then
    stripped="${BASH_REMATCH[1]}"
  fi
  if [[ "$stripped" =~ $re_rule ]]; then
    if [[ "$stripped" =~ $re_break ]]; then
      class=breaking
      breaking=$((breaking + 1))
    elif [[ "$stripped" =~ $re_feat ]]; then
      class=feat
      feats=$((feats + 1))
    else
      class=patch
    fi
  else
    class=other
    outside=$((outside + 1))
  fi
  printf '  %-9s %s\n' "$class" "${title//[^ -~]/?}" >&2
done < <(git log --format=%s "$range")

if ((outside > 0)); then
  printf 'warning: %d title(s) marked "other" do not follow "<type>(<scope>): <description>"; they count as patch changes and go to the release notes as they are\n' "$outside" >&2
fi

if ((force_major)); then
  ((major == 0)) || die "--major only moves 0.x to 1.0.0, the last tag is $base"
  next_major=1 next_minor=0 next_patch=0
elif ((breaking > 0)); then
  if ((major == 0)); then
    next_major=0 next_minor=$((minor + 1)) next_patch=0
  else
    next_major=$((major + 1)) next_minor=0 next_patch=0
  fi
elif ((feats > 0)); then
  next_major=$major next_minor=$((minor + 1)) next_patch=0
else
  next_major=$major next_minor=$minor next_patch=$((patch + 1))
fi

tag="v$next_major.$next_minor.$next_patch"
printf 'Next tag: %s\n' "$tag" >&2
printf '%s\n' "$tag"
