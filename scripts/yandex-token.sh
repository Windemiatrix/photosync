#!/usr/bin/env bash
# Local development token for the Yandex Disk REST API, separate from the one
# in GitHub secrets. Sign in with the test account, not your own.
#   make yandex-token-create   sign in with a device code and save the token
#   make yandex-token-delete   revoke the token and delete the file
# Needs YANDEX_CLIENT_ID and YANDEX_CLIENT_SECRET (the Makefile exports .env).
# Secrets go in request bodies built from the environment, never in argv or output.
set -euo pipefail

file="${YANDEX_TOKEN_FILE:-$HOME/.config/photosync-dev/yandex-token.json}"
: "${YANDEX_CLIENT_ID:?is not set in .env}"
: "${YANDEX_CLIENT_SECRET:?is not set in .env}"

# post <path> <jq expression that builds the form body from the environment>;
# -j: no trailing newline, it would stick to the last value.
post() { jq -jn "$2" | curl -sS -X POST --data-binary @- "https://oauth.yandex.ru$1"; }
client='"&client_id=" + (env.YANDEX_CLIENT_ID | @uri) + "&client_secret=" + (env.YANDEX_CLIENT_SECRET | @uri)'

create() {
  if [[ -e "$file" ]]; then
    echo "$file already exists: run 'make yandex-token-delete' first" >&2
    exit 1
  fi
  DEVICE_ID="$(openssl rand -hex 16)" && export DEVICE_ID
  answer="$(post /device/code '"client_id=" + (env.YANDEX_CLIENT_ID | @uri) + "&device_id=" + env.DEVICE_ID
    + "&device_name=photosync-dev&scope=" + ("cloud_api:disk.read cloud_api:disk.write" | @uri)')"
  DEVICE_CODE="$(jq -r '.device_code // empty' <<<"$answer")" && export DEVICE_CODE
  if [[ -z "$DEVICE_CODE" ]]; then
    echo "device code refused: $(jq -r '.error // "unreadable answer"' <<<"$answer")" >&2
    exit 1
  fi
  interval="$(jq -r '.interval // 5' <<<"$answer")"
  deadline=$((SECONDS + $(jq -r '.expires_in // 300' <<<"$answer")))
  echo "Open $(jq -r .verification_url <<<"$answer") in a private window, sign in with the TEST account"
  echo "and enter the code $(jq -r .user_code <<<"$answer"). Waiting..."

  while ((SECONDS < deadline)); do
    sleep "$interval"
    answer="$(post /token '"grant_type=device_code&code=" + (env.DEVICE_CODE | @uri) + '"$client")"
    case "$(jq -r '.error // "ok"' <<<"$answer")" in
      ok) break ;;
      authorization_pending) continue ;;
      slow_down) interval=$((interval + 5)) ;;
      *) echo "sign-in failed: $(jq -r .error <<<"$answer")" >&2; exit 1 ;;
    esac
  done
  if [[ -z "$(jq -r '.access_token // empty' <<<"$answer")" ]]; then
    echo "the code expired before the sign-in was confirmed" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$file")"
  chmod 700 "$(dirname "$file")"
  (umask 077 && jq '{access_token, refresh_token}' <<<"$answer" >"$file.tmp")
  mv "$file.tmp" "$file"
  echo "saved $file (0600), valid for $(($(jq -r '.expires_in // 0' <<<"$answer") / 86400)) days"
}

delete() {
  if [[ ! -e "$file" ]]; then
    echo "no $file: nothing to delete"
    return
  fi
  ACCESS_TOKEN="$(jq -r '.access_token // empty' "$file")" && export ACCESS_TOKEN
  answer="$(post /revoke_token '"access_token=" + (env.ACCESS_TOKEN | @uri) + '"$client")" || answer='{}'
  rm -f "$file"
  if [[ "$(jq -r '.status // empty' <<<"$answer" 2>/dev/null)" == ok ]]; then
    echo "revoked and deleted $file"
  else
    echo "deleted $file; revoke failed ($(jq -r '.error // "no answer"' <<<"$answer" 2>/dev/null)):" \
      "revoke it at https://id.yandex.ru/security" >&2
  fi
}

case "${1:-}" in
  create) create ;;
  delete) delete ;;
  *) echo "usage: $0 create|delete" >&2; exit 2 ;;
esac
