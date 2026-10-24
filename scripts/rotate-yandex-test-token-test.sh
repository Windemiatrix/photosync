#!/usr/bin/env bash
# Self-test for scripts/rotate-yandex-test-token.sh with fake curl and gh on
# PATH: the order of the writes, what is kept on each failure, and that no
# token reaches the output. Run by the repo-checks job and by "make check-repo".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/rotate-yandex-test-token.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/rotate-test.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
failures=0

cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
# POST: the OAuth token request; otherwise the Disk request with the header on stdin.
if [[ " $* " == *" -X POST "* ]]; then
  cat >"$FAKE_DIR/oauth-body"
  printf '%s' "$FAKE_OAUTH_ANSWER"
else
  cat >"$FAKE_DIR/disk-header"
  printf '%s' "$FAKE_DISK_ANSWER"
fi
EOF
cat >"$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
# gh secret set NAME --env ENV --repo REPO, value on stdin.
name="$3" env="$5"
value="$(cat)"
[[ "$name" == "${FAKE_GH_FAIL:-}" ]] && exit 1
printf '%s %s %s\n' "$name" "$env" "$value" >>"$FAKE_DIR/gh-log"
EOF
chmod +x "$tmp/bin/curl" "$tmp/bin/gh"

ok_answer='{"access_token":"ACCESS-NEW","refresh_token":"1:REFRESH/NEW+=","token_type":"bearer","expires_in":31536000}'

# run <oauth answer> <disk answer> <gh name to fail>; sets rc, out, log
run() {
  rm -f "$tmp/gh-log" "$tmp/oauth-body" "$tmp/disk-header"
  set +e
  out="$(PATH="$tmp/bin:$PATH" FAKE_DIR="$tmp" FAKE_OAUTH_ANSWER="$1" FAKE_DISK_ANSWER="$2" FAKE_GH_FAIL="$3" \
    YANDEX_CLIENT_ID=cid YANDEX_CLIENT_SECRET='sec&ret' YANDEX_TEST_REFRESH_TOKEN='1:REFRESH/OLD+=' \
    YANDEX_TEST_UID=42 GH_TOKEN=gh GITHUB_REPOSITORY=o/r GITHUB_ACTIONS= bash "$script" 2>&1)"
  rc=$?
  set -e
  log="$(cat "$tmp/gh-log" 2>/dev/null || true)"
}

check() {
  if ! eval "$2"; then
    printf 'FAIL: %s\n  output: %s\n  gh log: %s\n' "$1" "$out" "$log" >&2
    failures=$((failures + 1))
  fi
}

run "$ok_answer" '{"user":{"uid":"42"}}' ""
check "ok: exit 0" '[[ $rc == 0 ]]'
check "ok: refresh saved first, access second" \
  '[[ "$log" == $'"'"'YANDEX_TEST_REFRESH_TOKEN yandex-test-token-rotation 1:REFRESH/NEW+=\nYANDEX_TEST_TOKEN integration ACCESS-NEW'"'"' ]]'
check "ok: no token in the output" '[[ "$out" != *NEW* && "$out" != *OLD* && "$out" != *sec* ]]'
check "ok: form body is URL-encoded" \
  '[[ "$(cat "$tmp/oauth-body")" == "grant_type=refresh_token&refresh_token=1%3AREFRESH%2FOLD%2B%3D&client_id=cid&client_secret=sec%26ret" ]]'
check "ok: the Disk request carries the new token" '[[ "$(cat "$tmp/disk-header")" == "Authorization: OAuth ACCESS-NEW" ]]'

run '{"error":"invalid_grant","error_description":"x"}' '' ""
check "refused: exit 1" '[[ $rc == 1 ]]'
check "refused: nothing saved" '[[ -z "$log" ]]'
check "refused: the error code is named" '[[ "$out" == *invalid_grant* ]]'

run "$ok_answer" '{"user":{"uid":"42"}}' YANDEX_TEST_REFRESH_TOKEN
check "refresh not saved: exit 1" '[[ $rc == 1 ]]'
check "refresh not saved: access not saved" '[[ -z "$log" ]]'
check "refresh not saved: asks to sign in again" '[[ "$out" == *"sign in"* ]]'

run "$ok_answer" '{"user":{"uid":"7"}}' ""
check "other account: exit 1" '[[ $rc == 1 ]]'
check "other account: only the refresh token saved" '[[ "$log" == "YANDEX_TEST_REFRESH_TOKEN "* && "$log" != *YANDEX_TEST_TOKEN\ * ]]'

run "$ok_answer" '{"user":{"uid":"42"}}' YANDEX_TEST_TOKEN
check "access not saved: exit 1" '[[ $rc == 1 ]]'
check "access not saved: refresh kept" '[[ "$log" == "YANDEX_TEST_REFRESH_TOKEN "* ]]'
check "access not saved: no token in the output" '[[ "$out" != *NEW* ]]'

run 'not json' '' ""
check "garbage answer: exit 1, nothing saved" '[[ $rc == 1 && -z "$log" ]]'

if ((failures > 0)); then
  echo "$failures rotate-yandex-test-token case(s) failed" >&2
  exit 1
fi
echo "rotate-yandex-test-token: all cases passed"
