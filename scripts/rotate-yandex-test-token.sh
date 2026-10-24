#!/usr/bin/env bash
# Refreshes the OAuth token of the Yandex test account and stores both new
# tokens in GitHub secrets. A refresh token works once: the new one is saved
# first, the access token for the integration tests second. No token is ever
# printed. Run only by rotate-yandex-test-token.yml.
#
# Environment: YANDEX_CLIENT_ID, YANDEX_CLIENT_SECRET, YANDEX_TEST_REFRESH_TOKEN,
# YANDEX_TEST_UID, GH_TOKEN (writes environment secrets), GITHUB_REPOSITORY.
set -euo pipefail

oauth_url="${YANDEX_OAUTH_URL:-https://oauth.yandex.ru/token}"
disk_url="${YANDEX_DISK_URL:-https://cloud-api.yandex.net/v1/disk/?fields=user.uid}"
rotation_env=yandex-test-token-rotation
tests_env=integration

for v in YANDEX_CLIENT_ID YANDEX_CLIENT_SECRET YANDEX_TEST_REFRESH_TOKEN YANDEX_TEST_UID GH_TOKEN GITHUB_REPOSITORY; do
  if [[ -z "${!v:-}" ]]; then
    echo "error: $v is not set" >&2
    exit 1
  fi
done

# The form body is built from the environment, so no secret reaches argv.
answer="$(jq -rn '"grant_type=refresh_token"
  + "&refresh_token=" + (env.YANDEX_TEST_REFRESH_TOKEN | @uri)
  + "&client_id=" + (env.YANDEX_CLIENT_ID | @uri)
  + "&client_secret=" + (env.YANDEX_CLIENT_SECRET | @uri)' |
  curl -sS -X POST --data-binary @- "$oauth_url")" || {
  echo "error: the OAuth request failed; the old tokens are kept" >&2
  exit 1
}
access="$(jq -r '.access_token // empty' <<<"$answer" 2>/dev/null || true)"
refresh="$(jq -r '.refresh_token // empty' <<<"$answer" 2>/dev/null || true)"
if [[ -z "$access" || -z "$refresh" ]]; then
  code="$(jq -r '.error // "no tokens in the answer"' <<<"$answer" 2>/dev/null || echo "unreadable answer")"
  echo "error: refresh refused ($code); the old tokens are kept" >&2
  exit 1
fi
if [[ "${GITHUB_ACTIONS:-}" == true ]]; then
  echo "::add-mask::$access"
  echo "::add-mask::$refresh"
fi

# The old refresh token is spent now: save the new one before anything else.
if ! printf '%s' "$refresh" | gh secret set YANDEX_TEST_REFRESH_TOKEN --env "$rotation_env" --repo "$GITHUB_REPOSITORY" >/dev/null; then
  echo "error: the new refresh token was not saved; sign in to the test account again and set YANDEX_TEST_REFRESH_TOKEN by hand" >&2
  exit 1
fi

# The token must belong to the test account before the tests get it.
uid="$(curl -sS -H @- "$disk_url" <<<"Authorization: OAuth $access" | jq -r '.user.uid // empty' 2>/dev/null || true)"
if [[ "$uid" != "$YANDEX_TEST_UID" ]]; then
  echo "error: the new token is not for the test account; YANDEX_TEST_TOKEN is kept" >&2
  exit 1
fi

if ! printf '%s' "$access" | gh secret set YANDEX_TEST_TOKEN --env "$tests_env" --repo "$GITHUB_REPOSITORY" >/dev/null; then
  echo "error: YANDEX_TEST_TOKEN was not saved; the old one still works, rerun the workflow" >&2
  exit 1
fi
echo "rotated: YANDEX_TEST_REFRESH_TOKEN ($rotation_env), YANDEX_TEST_TOKEN ($tests_env)"
