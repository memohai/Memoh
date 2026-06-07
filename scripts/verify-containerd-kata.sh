#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEMOH_VERIFY_BASE_URL:-http://127.0.0.1:${MEMOH_DEV_SERVER_PORT:-18080}}"
USERNAME="${MEMOH_VERIFY_ADMIN_USERNAME:-admin}"
PASSWORD="${MEMOH_VERIFY_ADMIN_PASSWORD:-admin123}"
EXPECTED_RUNTIME="${MEMOH_VERIFY_EXPECTED_RUNTIME:-io.containerd.kata.v2}"
EXPECTED_BACKEND="${MEMOH_VERIFY_EXPECTED_BACKEND:-containerd}"
EXPECTED_WORKSPACE_BACKEND="${MEMOH_VERIFY_EXPECTED_WORKSPACE_BACKEND:-container}"
EXPECTED_STORAGE_HARD_LIMIT="${MEMOH_VERIFY_EXPECT_STORAGE_HARD_LIMIT:-false}"
EXPECTED_STORAGE_SOFT_LIMIT="${MEMOH_VERIFY_EXPECT_STORAGE_SOFT_LIMIT:-true}"
VERIFY_DATA_PRESERVATION="${MEMOH_VERIFY_DATA_PRESERVATION:-true}"
CPU_MILLICORES="${MEMOH_VERIFY_CPU_MILLICORES:-500}"
MEMORY_BYTES="${MEMOH_VERIFY_MEMORY_BYTES:-134217728}"
STORAGE_BYTES="${MEMOH_VERIFY_STORAGE_BYTES:-33554432}"
BOT_PREFIX="${MEMOH_VERIFY_BOT_PREFIX:-kata-runtime-verify}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

json_field() {
  jq -er "$1" "$2"
}

curl_json() {
  curl -fsS "$@"
}

assert_json() {
  local file="$1"
  local filter="$2"
  local message="$3"
  if ! jq -e "$filter" "$file" >/dev/null; then
    echo "ERROR: $message" >&2
    echo "JSON:" >&2
    jq . "$file" >&2 || cat "$file" >&2
    exit 1
  fi
}

read_sse_payloads() {
  sed -n 's/^data: //p' "$1"
}

assert_no_sse_error() {
  local file="$1"
  if read_sse_payloads "$file" | jq -e 'select(.type == "error")' >/dev/null; then
    echo "ERROR: SSE stream reported an error" >&2
    read_sse_payloads "$file" | jq . >&2
    exit 1
  fi
}

assert_sse_data_restored() {
  local file="$1"
  if ! read_sse_payloads "$file" | jq -e 'select(.type == "complete") | .container.data_restored == true' >/dev/null; then
    echo "ERROR: container recreate did not report restored data" >&2
    read_sse_payloads "$file" | jq . >&2
    exit 1
  fi
}

assert_file_content() {
  local file="$1"
  local expected="$2"
  local got
  got="$(json_field '.content' "$file")"
  if [ "$got" != "$expected" ]; then
    echo "ERROR: restored file content mismatch" >&2
    echo "Expected:" >&2
    printf '%s\n' "$expected" >&2
    echo "Got:" >&2
    printf '%s\n' "$got" >&2
    exit 1
  fi
}

validate_bool() {
  case "$2" in
    true|false)
      ;;
    *)
      echo "ERROR: $1 must be true or false, got: $2" >&2
      exit 1
      ;;
  esac
}

cleanup() {
  if [ -n "${BOT_ID:-}" ]; then
    if [ "${PRESERVED_DATA_CREATED:-0}" = "1" ]; then
      curl -fsS -N -X POST "$BASE_URL/bots/$BOT_ID/container" \
        -H "Authorization: Bearer $TOKEN" \
        -H 'Content-Type: application/json' \
        -H 'Accept: text/event-stream' \
        -d '{"restore_data":true}' >/dev/null 2>&1 || true
      PRESERVED_DATA_CREATED=0
    fi
    curl -fsS -X DELETE "$BASE_URL/bots/$BOT_ID/container?preserve_data=false" \
      -H "Authorization: Bearer $TOKEN" >/dev/null 2>&1 || true
    curl -fsS -X DELETE "$BASE_URL/bots/$BOT_ID" \
      -H "Authorization: Bearer $TOKEN" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR"
}

require_cmd curl
require_cmd jq
validate_bool MEMOH_VERIFY_EXPECT_STORAGE_HARD_LIMIT "$EXPECTED_STORAGE_HARD_LIMIT"
validate_bool MEMOH_VERIFY_EXPECT_STORAGE_SOFT_LIMIT "$EXPECTED_STORAGE_SOFT_LIMIT"
validate_bool MEMOH_VERIFY_DATA_PRESERVATION "$VERIFY_DATA_PRESERVATION"

TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/memoh-kata-verify.XXXXXX")"
TOKEN=""
BOT_ID=""
PRESERVED_DATA_CREATED=0
trap cleanup EXIT

if [[ "$EXPECTED_RUNTIME" == *kata* ]]; then
  if [ "$(uname -s)" != "Linux" ]; then
    echo "WARN: local host is not Linux; continuing because $BASE_URL may point at a remote Linux/KVM server." >&2
  elif [ ! -e /dev/kvm ]; then
    echo "WARN: /dev/kvm is not present on this host; Kata verification will fail unless the target server has KVM elsewhere." >&2
  fi
fi

echo "Logging in to $BASE_URL as $USERNAME..."
LOGIN_JSON="$TMPDIR/login.json"
curl_json -X POST "$BASE_URL/auth/login" \
  -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg username "$USERNAME" --arg password "$PASSWORD" '{username: $username, password: $password}')" \
  >"$LOGIN_JSON"
TOKEN="$(json_field '.access_token' "$LOGIN_JSON")"

BOT_NAME="$BOT_PREFIX-$(date +%s)-$$"
CREATE_STREAM="$TMPDIR/create-bot.sse"
echo "Creating temporary bot $BOT_NAME..."
curl -fsS -N -X POST "$BASE_URL/bots" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d "$(jq -cn --arg name "$BOT_NAME" '{name: $name, display_name: $name}')" \
  >"$CREATE_STREAM"
assert_no_sse_error "$CREATE_STREAM"
BOT_ID="$(read_sse_payloads "$CREATE_STREAM" | jq -sr 'map(select(.type == "bot_created"))[0].bot.id // empty')"
if [ -z "$BOT_ID" ]; then
  echo "ERROR: bot_created event did not include a bot id" >&2
  read_sse_payloads "$CREATE_STREAM" | jq . >&2
  exit 1
fi
if ! read_sse_payloads "$CREATE_STREAM" | jq -e 'select(.type == "ready")' >/dev/null; then
  echo "ERROR: bot create stream did not reach ready" >&2
  read_sse_payloads "$CREATE_STREAM" | jq . >&2
  exit 1
fi

SENTINEL_PATH="/data/$BOT_NAME.txt"
SENTINEL_CONTENT="memoh kata data preservation $BOT_ID"

METRICS_JSON="$TMPDIR/metrics.initial.json"
curl_json "$BASE_URL/bots/$BOT_ID/container/metrics" \
  -H "Authorization: Bearer $TOKEN" \
  >"$METRICS_JSON"
assert_json "$METRICS_JSON" ".supported == true" "container metrics must be supported"
assert_json "$METRICS_JSON" ".backend == \"$EXPECTED_BACKEND\"" "metrics backend must be $EXPECTED_BACKEND"
assert_json "$METRICS_JSON" ".status.exists == true" "container must exist"
assert_json "$METRICS_JSON" ".status.task_running == true" "container task must be running"
assert_json "$METRICS_JSON" ".resource_limits.backend == \"$EXPECTED_BACKEND\"" "resource limit backend must be $EXPECTED_BACKEND"
assert_json "$METRICS_JSON" ".resource_limits.workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "workspace backend must be $EXPECTED_WORKSPACE_BACKEND"
assert_json "$METRICS_JSON" ".resource_limits.runtime_backend == \"$EXPECTED_RUNTIME\"" "runtime_backend must be $EXPECTED_RUNTIME"
assert_json "$METRICS_JSON" ".resource_limits.capabilities.cpu.hard_limit_supported == true" "CPU hard limit must be supported"
assert_json "$METRICS_JSON" ".resource_limits.capabilities.memory.hard_limit_supported == true" "memory hard limit must be supported"
assert_json "$METRICS_JSON" ".resource_limits.capabilities.storage.hard_limit_supported == $EXPECTED_STORAGE_HARD_LIMIT" "storage hard limit capability must be $EXPECTED_STORAGE_HARD_LIMIT"
assert_json "$METRICS_JSON" ".resource_limits.capabilities.storage.soft_limit_supported == $EXPECTED_STORAGE_SOFT_LIMIT" "storage soft limit capability must be $EXPECTED_STORAGE_SOFT_LIMIT"

if [ "$VERIFY_DATA_PRESERVATION" = "true" ]; then
  WRITE_JSON="$TMPDIR/fs.write.json"
  curl_json -X POST "$BASE_URL/bots/$BOT_ID/container/fs/write" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$(jq -cn --arg path "$SENTINEL_PATH" --arg content "$SENTINEL_CONTENT" '{path: $path, content: $content}')" \
    >"$WRITE_JSON"
  assert_json "$WRITE_JSON" ".ok == true" "failed to write sentinel file"

  READ_JSON="$TMPDIR/fs.read.initial.json"
  curl_json --get "$BASE_URL/bots/$BOT_ID/container/fs/read" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "path=$SENTINEL_PATH" \
    >"$READ_JSON"
  assert_file_content "$READ_JSON" "$SENTINEL_CONTENT"
fi

echo "Applying resource limits and recreating the workspace..."
UPDATE_JSON="$TMPDIR/metrics.update.json"
curl_json -X PUT "$BASE_URL/bots/$BOT_ID/container/metrics" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -cn \
    --argjson cpu "$CPU_MILLICORES" \
    --argjson memory "$MEMORY_BYTES" \
    --argjson storage "$STORAGE_BYTES" \
    '{resource_limits: {cpu_millicores: $cpu, memory_bytes: $memory, storage_bytes: $storage}}')" \
  >"$UPDATE_JSON"
assert_json "$UPDATE_JSON" ".resource_limits.desired.cpu_millicores == $CPU_MILLICORES" "desired CPU limit was not saved"
assert_json "$UPDATE_JSON" ".resource_limits.desired.memory_bytes == $MEMORY_BYTES" "desired memory limit was not saved"
assert_json "$UPDATE_JSON" ".resource_limits.desired.storage_bytes == $STORAGE_BYTES" "desired storage limit was not saved"

PRESERVE_QUERY="preserve_data=false"
if [ "$VERIFY_DATA_PRESERVATION" = "true" ]; then
  PRESERVE_QUERY="preserve_data=true"
fi

curl_json -X DELETE "$BASE_URL/bots/$BOT_ID/container?$PRESERVE_QUERY" \
  -H "Authorization: Bearer $TOKEN" >/dev/null
if [ "$VERIFY_DATA_PRESERVATION" = "true" ]; then
  PRESERVED_DATA_CREATED=1
fi

RECREATE_STREAM="$TMPDIR/recreate-container.sse"
curl -fsS -N -X POST "$BASE_URL/bots/$BOT_ID/container" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d "$(jq -cn --argjson restore "$VERIFY_DATA_PRESERVATION" '{restore_data: $restore}')" \
  >"$RECREATE_STREAM"
assert_no_sse_error "$RECREATE_STREAM"
if ! read_sse_payloads "$RECREATE_STREAM" | jq -e 'select(.type == "complete")' >/dev/null; then
  echo "ERROR: container recreate stream did not complete" >&2
  read_sse_payloads "$RECREATE_STREAM" | jq . >&2
  exit 1
fi
if [ "$VERIFY_DATA_PRESERVATION" = "true" ]; then
  assert_sse_data_restored "$RECREATE_STREAM"
  PRESERVED_DATA_CREATED=0
fi

FINAL_METRICS_JSON="$TMPDIR/metrics.final.json"
curl_json "$BASE_URL/bots/$BOT_ID/container/metrics" \
  -H "Authorization: Bearer $TOKEN" \
  >"$FINAL_METRICS_JSON"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.runtime_backend == \"$EXPECTED_RUNTIME\"" "runtime_backend changed after recreate"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.status == \"applied\"" "resource limits must be applied after recreate"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.applied.cpu_millicores == $CPU_MILLICORES" "CPU limit was not applied"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.applied.memory_bytes == $MEMORY_BYTES" "memory limit was not applied"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.desired.storage_bytes == $STORAGE_BYTES" "storage soft limit was not preserved"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.capabilities.storage.hard_limit_supported == $EXPECTED_STORAGE_HARD_LIMIT" "storage hard limit capability changed after recreate"
assert_json "$FINAL_METRICS_JSON" ".resource_limits.capabilities.storage.soft_limit_supported == $EXPECTED_STORAGE_SOFT_LIMIT" "storage soft limit capability changed after recreate"
if [ "$EXPECTED_STORAGE_HARD_LIMIT" = "true" ]; then
  assert_json "$FINAL_METRICS_JSON" ".resource_limits.applied.storage_bytes == $STORAGE_BYTES" "storage limit was not applied"
fi
if [ "$VERIFY_DATA_PRESERVATION" = "true" ]; then
  RESTORED_READ_JSON="$TMPDIR/fs.read.restored.json"
  curl_json --get "$BASE_URL/bots/$BOT_ID/container/fs/read" \
    -H "Authorization: Bearer $TOKEN" \
    --data-urlencode "path=$SENTINEL_PATH" \
    >"$RESTORED_READ_JSON"
  assert_file_content "$RESTORED_READ_JSON" "$SENTINEL_CONTENT"
fi

echo "Verified $EXPECTED_RUNTIME workspace runtime for bot $BOT_ID."
echo "Final resource limit state:"
jq '{runtime_backend: .resource_limits.runtime_backend, status: .resource_limits.status, desired: .resource_limits.desired, applied: .resource_limits.applied, capabilities: .resource_limits.capabilities}' "$FINAL_METRICS_JSON"
