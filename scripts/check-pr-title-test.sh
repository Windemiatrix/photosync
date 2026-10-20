#!/usr/bin/env bash
# Self-test for scripts/check-pr-title.sh: feeds it valid and invalid titles
# and checks the verdict. Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(dirname "${BASH_SOURCE[0]}")"
checker="$here/check-pr-title.sh"
failures=0

# expect <ok|fail> <title>
expect() {
  local want="$1" title="$2" got
  if bash "$checker" "$title" >/dev/null 2>&1; then got=ok; else got=fail; fi
  if [[ "$got" != "$want" ]]; then
    printf 'FAIL: want %s, got %s: %q\n' "$want" "$got" "$title" >&2
    failures=$((failures + 1))
  fi
}

# 70 and 71 characters long ("feat(plan): " is 12 characters), both otherwise valid.
pad70="feat(plan): $(printf 'x%.0s' $(seq 1 58))"
pad71="feat(plan): $(printf 'x%.0s' $(seq 1 59))"
# The same with "!" ("feat(plan)!: " is 13 characters).
pad70bang="feat(plan)!: $(printf 'x%.0s' $(seq 1 57))"
pad71bang="feat(plan)!: $(printf 'x%.0s' $(seq 1 58))"

expect ok "feat(photosdb): read albums and folders"
expect ok "fix(webdav): retry on 503"
expect ok "chore(deps): bump the go-minor-patch group with 2 updates"
expect ok "docs(architecture): describe the plan phases"
expect ok "refactor(internal/plan): split the matcher"
expect ok "$pad70"
expect ok "feat(config)!: rename the remotes key"
expect ok "refactor(internal/state)!: change the schema"
expect ok "chore(deps)!: drop the support of macOS 12"
expect ok "$pad70bang"

expect fail ""
expect fail "$pad71"
expect fail "$pad71bang"
# "!" needs a scope, stands right after it, and only once
expect fail "feat!: rename the remotes key"
expect fail "feat(config)!!: rename the remotes key"
expect fail "feat(config) !: rename the remotes key"
expect fail "feat(config!): rename the remotes key"
expect fail "feat(config)!:rename the remotes key"
expect fail "feat(config)!: rename the remotes key."
expect fail "!feat(config): rename the remotes key"
expect fail "add match levels"
expect fail "feat: add match levels"
expect fail "feat(): add match levels"
expect fail "Feat(plan): add match levels"
expect fail "feature(plan): add match levels"
expect fail "feat(Plan): add match levels"
expect fail "feat(plan):add match levels"
expect fail "feat(plan): add match levels."
expect fail "feat(plan): add match levels #123"$'\n'"second line"
expect fail "feat(plan): $(printf 'add \xd0\xbf\xd0\xbb\xd0\xb0\xd0\xbd')"
expect fail "feat(plan): add$(printf '\033')[31m match"
expect fail "feat(plan): add $(printf '\xe2\x80\xae') match"
expect fail "feat(plan): add$(printf '\t')match"

if ((failures > 0)); then
  echo "$failures check-pr-title case(s) failed" >&2
  exit 1
fi
echo "check-pr-title: all cases passed"
