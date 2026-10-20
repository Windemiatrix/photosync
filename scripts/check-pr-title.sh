#!/usr/bin/env bash
# Checks a pull request title against the repository rule.
#
# The title of a PR becomes the title of the squash commit on main, so it is
# checked before merging:
#
#   <type>(<scope>): <description>
#
# - type is one of feat, fix, docs, style, refactor, test, chore;
# - scope is lowercase letters, digits and . _ / -;
# - the description is printable ASCII only (this also rejects control
#   characters, ESC and bidi marks, and enforces "English");
# - the whole title is at most 70 characters and does not end with a period.
#
# Usage:
#   PR_TITLE='feat(plan): add match levels' scripts/check-pr-title.sh
#   scripts/check-pr-title.sh 'feat(plan): add match levels'
#
# In CI the title comes from the environment, never from ${{ }} inside a run
# script: the title of a pull request from a fork is untrusted input.
set -euo pipefail

# In the C locale a bracket range is a plain byte range and ${#title} counts
# bytes, which equals characters for ASCII-only input.
export LC_ALL=C

title="${1-${PR_TITLE-}}"

re='^(feat|fix|docs|style|refactor|test|chore)\([a-z0-9._/-]+\): [ -~]+$'
max=70

if [[ -z "$title" ]]; then
  echo "error: PR title is empty" >&2
  exit 1
fi

if ! [[ "$title" =~ $re ]]; then
  cat >&2 <<'EOF'
error: PR title does not match "<type>(<scope>): <description>"
  type:        feat, fix, docs, style, refactor, test or chore
  scope:       lowercase letters, digits and . _ / -
  description: English, printable ASCII only
example: feat(photosdb): read albums and folders
EOF
  exit 1
fi

if ((${#title} > max)); then
  echo "error: PR title is ${#title} characters long, the limit is ${max}" >&2
  exit 1
fi

if [[ "$title" == *. ]]; then
  echo "error: PR title must not end with a period" >&2
  exit 1
fi

echo "PR title is valid: $title"
