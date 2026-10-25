#!/usr/bin/env bash
# Checks that a pull request title follows the repository rule. The title of a
# PR becomes the title of its squash commit on main, and the release job of
# ci.yml computes the next version from those titles (.svu.yml), so a bad title
# is stopped before the merge.
#
# Run:    make pr-title PR_TITLE='feat(plan): add match levels'
#         scripts/check-pr-title.sh 'feat(config)!: rename the remotes key'
# Input:  the title as the first argument, otherwise the PR_TITLE variable. CI
#         passes it through the environment, never through ${{ }} inside a run
#         script: the title of a pull request from a fork is untrusted input.
# Exit:   0 the title is valid; 1 it is not, the reason is on stderr.
#
# The rule:
#
#   <type>(<scope>)[!]: <description>
#
#   type         feat, fix, docs, style, refactor, test or chore
#   scope        required: lowercase letters, digits and . _ / -
#   !            optional, right after the scope: an incompatible change of
#                commands, flags, config keys or the state schema; it decides
#                the next version and puts the title in the "Breaking changes"
#                section of the release notes
#   description  English, printable ASCII only, which also keeps out control
#                characters, ESC and bidi marks
#   length       at most 70 characters, no period at the end
set -euo pipefail

# In the C locale [ -~] is the printable ASCII range and ${#title} counts bytes.
export LC_ALL=C

title_re='^(feat|fix|docs|style|refactor|test|chore)\([a-z0-9._/-]+\)!?: [ -~]+$'
max_length=70

die() {
  echo "error: $1" >&2
  exit 1
}

die_with_rule() {
  cat >&2 <<'EOF'
error: PR title does not match "<type>(<scope>)[!]: <description>"
  type:        feat, fix, docs, style, refactor, test or chore
  scope:       required; lowercase letters, digits and . _ / -
  !            optional, right after the scope: an incompatible change
  description: English, printable ASCII only
examples:
  feat(photosdb): read albums and folders
  feat(config)!: rename the remotes key
EOF
  exit 1
}

if (($# > 0)); then
  title="$1"
else
  title="${PR_TITLE-}"
fi

if [[ -z "$title" ]]; then
  die "PR title is empty"
fi
if ! [[ "$title" =~ $title_re ]]; then
  die_with_rule
fi
if ((${#title} > max_length)); then
  die "PR title is ${#title} characters long, the limit is $max_length"
fi
if [[ "$title" == *. ]]; then
  die "PR title must not end with a period"
fi

echo "PR title is valid: $title"
