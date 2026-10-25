#!/usr/bin/env bash
# Creates or deletes the local development token for the Yandex Disk REST API.
# It is separate from the token in GitHub secrets. Sign in with the test
# account, not your own.
#
# Run:    make yandex-token-create   sign in with a device code, save the token
#         make yandex-token-delete   revoke the token and delete the file
# Input:  the command, create or delete, and the environment:
#           YANDEX_CLIENT_ID, YANDEX_CLIENT_SECRET   required; the Makefile
#                                                    exports them from .env
#           YANDEX_TOKEN_FILE   default ~/.config/photosync-dev/yandex-token.json
# Exit:   0 done; 1 a variable is missing, the token file already exists, or
#         the sign-in failed or expired; 2 unknown command; the status of curl,
#         jq or openssl when one of them fails.
#
# Secrets go only in request bodies built from the environment, never in argv
# or output.
set -euo pipefail

oauth_url=https://oauth.yandex.ru
token_file="${YANDEX_TOKEN_FILE:-$HOME/.config/photosync-dev/yandex-token.json}"
client_credentials='"&client_id=" + (env.YANDEX_CLIENT_ID | @uri) + "&client_secret=" + (env.YANDEX_CLIENT_SECRET | @uri)'

# The last answer of the OAuth server.
answer=""

die() {
  echo "$1" >&2
  exit 1
}

# post <path> <jq expression>: sends the form body that the expression builds
# from the environment and prints the answer. jq -j: a trailing newline would
# become part of the last value.
post() {
  jq -jn "$2" | curl -sS -X POST --data-binary @- "$oauth_url$1"
}

# post_with_client <path> <jq expression>: the same, with the client id and
# secret added to the body.
post_with_client() {
  post "$1" "$2 + $client_credentials"
}

# field <json> <jq filter>: prints one value of an answer.
field() {
  jq -r "$2" <<<"$1"
}

require_client_credentials() {
  if [[ -z "${YANDEX_CLIENT_ID:-}" ]]; then
    die "YANDEX_CLIENT_ID is not set in .env"
  fi
  if [[ -z "${YANDEX_CLIENT_SECRET:-}" ]]; then
    die "YANDEX_CLIENT_SECRET is not set in .env"
  fi
}

request_device_code() {
  DEVICE_ID="$(openssl rand -hex 16)"
  export DEVICE_ID
  answer="$(post /device/code '"client_id=" + (env.YANDEX_CLIENT_ID | @uri) + "&device_id=" + env.DEVICE_ID
    + "&device_name=photosync-dev&scope=" + ("cloud_api:disk.read cloud_api:disk.write" | @uri)')"
  DEVICE_CODE="$(field "$answer" '.device_code // empty')"
  export DEVICE_CODE
  if [[ -z "$DEVICE_CODE" ]]; then
    die "device code refused: $(field "$answer" '.error // "unreadable answer"')"
  fi
}

show_sign_in_instructions() {
  echo "Open $(field "$answer" .verification_url) in a private window, sign in with the TEST account"
  echo "and enter the code $(field "$answer" .user_code). Waiting..."
}

# wait_for_sign_in: polls until the sign-in is confirmed; answer then holds
# the tokens.
wait_for_sign_in() {
  local interval expires_in deadline
  interval="$(field "$answer" '.interval // 5')"
  expires_in="$(field "$answer" '.expires_in // 300')"
  deadline=$((SECONDS + expires_in))

  while ((SECONDS < deadline)); do
    sleep "$interval"
    answer="$(post_with_client /token '"grant_type=device_code&code=" + (env.DEVICE_CODE | @uri)')"
    case "$(field "$answer" '.error // "ok"')" in
      ok) break ;;
      authorization_pending) continue ;;
      slow_down) interval=$((interval + 5)) ;;
      *) die "sign-in failed: $(field "$answer" .error)" ;;
    esac
  done

  if [[ -z "$(field "$answer" '.access_token // empty')" ]]; then
    die "the code expired before the sign-in was confirmed"
  fi
}

save_tokens() {
  local dir days
  dir="$(dirname "$token_file")"
  mkdir -p "$dir"
  chmod 700 "$dir"
  # The file holds the tokens: readable by the owner only from the start.
  (umask 077 && jq '{access_token, refresh_token}' <<<"$answer" >"$token_file.tmp")
  mv "$token_file.tmp" "$token_file"
  days=$(($(field "$answer" '.expires_in // 0') / 86400))
  echo "saved $token_file (0600), valid for $days days"
}

create() {
  if [[ -e "$token_file" ]]; then
    die "$token_file already exists: run 'make yandex-token-delete' first"
  fi
  request_device_code
  show_sign_in_instructions
  wait_for_sign_in
  save_tokens
}

# The file is deleted even when the revoke fails: the token then has to be
# revoked by hand.
delete() {
  if [[ ! -e "$token_file" ]]; then
    echo "no $token_file: nothing to delete"
    return
  fi
  ACCESS_TOKEN="$(jq -r '.access_token // empty' "$token_file")"
  export ACCESS_TOKEN
  answer="$(post_with_client /revoke_token '"access_token=" + (env.ACCESS_TOKEN | @uri)')" || answer='{}'
  rm -f "$token_file"
  if [[ "$(field "$answer" '.status // empty' 2>/dev/null)" == ok ]]; then
    echo "revoked and deleted $token_file"
  else
    echo "deleted $token_file; revoke failed ($(field "$answer" '.error // "no answer"' 2>/dev/null)):" \
      "revoke it at https://id.yandex.ru/security" >&2
  fi
}

require_client_credentials
case "${1:-}" in
  create) create ;;
  delete) delete ;;
  *)
    echo "usage: $0 create|delete" >&2
    exit 2
    ;;
esac
