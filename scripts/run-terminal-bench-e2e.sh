#!/usr/bin/env bash
set -euo pipefail

SERVER="${MEMOH_TBENCH_SERVER:-http://127.0.0.1:18080}"
USERNAME="${MEMOH_TBENCH_USERNAME:-admin}"
PASSWORD="${MEMOH_TBENCH_PASSWORD:-admin123}"
TIMEOUT_SEC="${MEMOH_TBENCH_TIMEOUT_SEC:-1800}"
VERIFIER_TIMEOUT_SEC="${MEMOH_TBENCH_VERIFIER_TIMEOUT_SEC:-}"
OUT_DIR="${MEMOH_TBENCH_OUT_DIR:-.memoh-tbench-runs}"
BOT_PREFIX="${MEMOH_TBENCH_BOT_PREFIX:-tbench}"
WORKSPACE_BACKEND="${MEMOH_TBENCH_WORKSPACE_BACKEND:-container}"
CONTAINER_WORKDIR="${MEMOH_TBENCH_CONTAINER_WORKDIR:-/data/task}"
CHAT_CHANNEL="${MEMOH_TBENCH_CHAT_CHANNEL:-web}"
CONTAINER_IMAGE="${MEMOH_TBENCH_CONTAINER_IMAGE:-}"
TASK_DIR=""
INSTRUCTION_FILE=""
MODEL_REF="${MEMOH_TBENCH_MODEL_REF:-}"
REASONING_EFFORT="${MEMOH_TBENCH_REASONING_EFFORT:-}"
VERIFY_CMD=""
TASK_UPLOAD=true
VERIFIER_DIR="${MEMOH_TBENCH_VERIFIER_DIR:-}"
VERIFIER_CONTAINER_DIR="${MEMOH_TBENCH_VERIFIER_CONTAINER_DIR:-/tests}"
DELETE_BOT_ON_SUCCESS=false

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

warn() {
  echo "WARN: $*" >&2
}

usage() {
  cat <<'EOF'
Usage:
  scripts/run-terminal-bench-e2e.sh \
    --task-dir /path/to/task \
    --instruction-file /path/to/instruction.txt \
    --model-ref <model-uuid-or-model_id-slug> \
    --verify-cmd '<command>'

Options:
  --server URL              Memoh server URL. Default: MEMOH_TBENCH_SERVER or http://127.0.0.1:18080
  --username USERNAME       Login username. Default: MEMOH_TBENCH_USERNAME or admin
  --password PASSWORD       Login password. Default: MEMOH_TBENCH_PASSWORD or admin123
  --task-dir DIR            Existing local task directory used as the bot local workspace. Required.
  --instruction-file FILE   Instruction text file to send to Memoh. Required.
  --model-ref REF           Chat model UUID or models.model_id slug. Required.
  --model-id REF            Alias for --model-ref.
  --reasoning-effort VALUE  Optional reasoning effort override.
  --verify-cmd CMD          Command run in --task-dir after the agent ends. Required.
  --workspace-backend MODE  container or local. Default: MEMOH_TBENCH_WORKSPACE_BACKEND or container.
  --container-image IMAGE   Optional container workspace image for the generated bot.
  --container-workdir DIR   Container task directory. Default: MEMOH_TBENCH_CONTAINER_WORKDIR or /data/task.
  --skip-task-upload        Do not upload --task-dir before the agent runs. Useful when the image already contains the task state.
  --verifier-dir DIR        Local hidden verifier directory to upload after the agent ends.
  --verifier-container-dir DIR
                            Container destination for --verifier-dir. Default: MEMOH_TBENCH_VERIFIER_CONTAINER_DIR or /tests.
  --chat-channel CHANNEL    Local chat route/channel. Default: MEMOH_TBENCH_CHAT_CHANNEL or web.
  --timeout-sec SECONDS     Agent WebSocket timeout. Default: MEMOH_TBENCH_TIMEOUT_SEC or 1800.
  --verifier-timeout-sec SECONDS
                            Verifier timeout. Default: MEMOH_TBENCH_VERIFIER_TIMEOUT_SEC or --timeout-sec.
  --out-dir DIR             Output directory. Default: MEMOH_TBENCH_OUT_DIR or .memoh-tbench-runs.
  --bot-prefix PREFIX       Prefix for the generated bot name. Default: MEMOH_TBENCH_BOT_PREFIX or tbench.
  --delete-bot-on-success   Delete the created bot only when the verifier passes.
  -h, --help                Show this help.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

json_get() {
  node -e '
const fs = require("fs");
const key = process.argv[1];
const data = JSON.parse(fs.readFileSync(0, "utf8"));
const value = key.split(".").reduce((obj, part) => obj && obj[part], data);
if (value === undefined || value === null || value === "") process.exit(1);
process.stdout.write(String(value));
' "$1"
}

abs_path() {
  node -e 'console.log(require("path").resolve(process.argv[1]))' "$1"
}

url_encode() {
  node -e 'process.stdout.write(encodeURIComponent(process.argv[1]))' "$1"
}

write_json_file() {
  local output_file="$1"
  shift
  "$@" >"$output_file"
}

sanitize_bot_prefix() {
  local sanitized
  sanitized="$(printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
  if [ -z "$sanitized" ]; then
    sanitized="tbench"
  fi
  printf '%s' "$sanitized"
}

for arg in "$@"; do
  case "$arg" in
    --help|-h)
      usage
      exit 0
      ;;
  esac
done

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server)
      [ "$#" -ge 2 ] || fail "--server requires a value"
      SERVER="$2"
      shift 2
      ;;
    --username)
      [ "$#" -ge 2 ] || fail "--username requires a value"
      USERNAME="$2"
      shift 2
      ;;
    --password)
      [ "$#" -ge 2 ] || fail "--password requires a value"
      PASSWORD="$2"
      shift 2
      ;;
    --task-dir)
      [ "$#" -ge 2 ] || fail "--task-dir requires a value"
      TASK_DIR="$2"
      shift 2
      ;;
    --instruction-file)
      [ "$#" -ge 2 ] || fail "--instruction-file requires a value"
      INSTRUCTION_FILE="$2"
      shift 2
      ;;
    --model-ref|--model-id)
      [ "$#" -ge 2 ] || fail "$1 requires a value"
      MODEL_REF="$2"
      shift 2
      ;;
    --reasoning-effort)
      [ "$#" -ge 2 ] || fail "--reasoning-effort requires a value"
      REASONING_EFFORT="$2"
      shift 2
      ;;
    --verify-cmd)
      [ "$#" -ge 2 ] || fail "--verify-cmd requires a value"
      VERIFY_CMD="$2"
      shift 2
      ;;
    --workspace-backend)
      [ "$#" -ge 2 ] || fail "--workspace-backend requires a value"
      WORKSPACE_BACKEND="$2"
      shift 2
      ;;
    --container-image)
      [ "$#" -ge 2 ] || fail "--container-image requires a value"
      CONTAINER_IMAGE="$2"
      shift 2
      ;;
    --container-workdir)
      [ "$#" -ge 2 ] || fail "--container-workdir requires a value"
      CONTAINER_WORKDIR="$2"
      shift 2
      ;;
    --skip-task-upload)
      TASK_UPLOAD=false
      shift
      ;;
    --verifier-dir)
      [ "$#" -ge 2 ] || fail "--verifier-dir requires a value"
      VERIFIER_DIR="$2"
      shift 2
      ;;
    --verifier-container-dir)
      [ "$#" -ge 2 ] || fail "--verifier-container-dir requires a value"
      VERIFIER_CONTAINER_DIR="$2"
      shift 2
      ;;
    --chat-channel)
      [ "$#" -ge 2 ] || fail "--chat-channel requires a value"
      CHAT_CHANNEL="$2"
      shift 2
      ;;
    --timeout-sec)
      [ "$#" -ge 2 ] || fail "--timeout-sec requires a value"
      TIMEOUT_SEC="$2"
      shift 2
      ;;
    --verifier-timeout-sec)
      [ "$#" -ge 2 ] || fail "--verifier-timeout-sec requires a value"
      VERIFIER_TIMEOUT_SEC="$2"
      shift 2
      ;;
    --out-dir)
      [ "$#" -ge 2 ] || fail "--out-dir requires a value"
      OUT_DIR="$2"
      shift 2
      ;;
    --bot-prefix)
      [ "$#" -ge 2 ] || fail "--bot-prefix requires a value"
      BOT_PREFIX="$2"
      shift 2
      ;;
    --delete-bot-on-success)
      DELETE_BOT_ON_SUCCESS=true
      shift
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

require_cmd curl
require_cmd node
require_cmd tar

[ -n "$TASK_DIR" ] || fail "--task-dir is required"
[ -n "$INSTRUCTION_FILE" ] || fail "--instruction-file is required"
[ -n "$MODEL_REF" ] || fail "--model-ref is required"
[ -n "$VERIFY_CMD" ] || fail "--verify-cmd is required"
[ -n "$CHAT_CHANNEL" ] || fail "--chat-channel is required"

case "$WORKSPACE_BACKEND" in
  container|local)
    ;;
  *)
    fail "--workspace-backend must be container or local"
    ;;
esac
if [ "$WORKSPACE_BACKEND" = "local" ]; then
  require_cmd perl
fi

case "$CONTAINER_WORKDIR" in
  /*)
    ;;
  *)
    fail "--container-workdir must be an absolute container path"
    ;;
esac

case "$VERIFIER_CONTAINER_DIR" in
  /*)
    ;;
  *)
    fail "--verifier-container-dir must be an absolute container path"
    ;;
esac

case "$TIMEOUT_SEC" in
  ''|*[!0-9]*)
    fail "--timeout-sec must be a positive integer"
    ;;
esac
[ "$TIMEOUT_SEC" -gt 0 ] || fail "--timeout-sec must be greater than zero"
if [ -z "$VERIFIER_TIMEOUT_SEC" ]; then
  VERIFIER_TIMEOUT_SEC="$TIMEOUT_SEC"
fi
case "$VERIFIER_TIMEOUT_SEC" in
  ''|*[!0-9]*)
    fail "--verifier-timeout-sec must be a positive integer"
    ;;
esac
[ "$VERIFIER_TIMEOUT_SEC" -gt 0 ] || fail "--verifier-timeout-sec must be greater than zero"

[ -d "$TASK_DIR" ] || fail "task directory does not exist: $TASK_DIR"
[ -f "$INSTRUCTION_FILE" ] || fail "instruction file does not exist: $INSTRUCTION_FILE"
if [ -n "$VERIFIER_DIR" ]; then
  [ "$WORKSPACE_BACKEND" = "container" ] || fail "--verifier-dir is only supported with --workspace-backend container"
  [ -d "$VERIFIER_DIR" ] || fail "verifier directory does not exist: $VERIFIER_DIR"
fi

TASK_DIR="$(abs_path "$TASK_DIR")"
INSTRUCTION_FILE="$(abs_path "$INSTRUCTION_FILE")"
if [ -n "$VERIFIER_DIR" ]; then
  VERIFIER_DIR="$(abs_path "$VERIFIER_DIR")"
fi
OUT_DIR="$(abs_path "$OUT_DIR")"
SERVER="${SERVER%/}"

case "$SERVER" in
  http://*|https://*)
    ;;
  *)
    fail "--server must start with http:// or https://"
    ;;
esac

RUN_STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"
BOT_PREFIX="$(sanitize_bot_prefix "$BOT_PREFIX")"
BOT_NAME="$BOT_PREFIX-$RUN_STAMP-$$"
BOT_DISPLAY_NAME="Terminal Bench $RUN_STAMP"
RUN_DIR="$OUT_DIR/$RUN_STAMP-$BOT_NAME"
TEMP_FILES=()

cleanup() {
  for f in "${TEMP_FILES[@]:-}"; do
    if [ -n "$f" ]; then
      rm -f "$f"
    fi
  done
}
trap cleanup EXIT

mkdir -p "$RUN_DIR"
cp "$INSTRUCTION_FILE" "$RUN_DIR/instruction.txt"
AGENT_INSTRUCTION_FILE="$RUN_DIR/agent-instruction.txt"

LOGIN_PAYLOAD="$RUN_DIR/login-payload.json"
LOGIN_RESPONSE="$RUN_DIR/login-response.json"
PING_RESPONSE="$RUN_DIR/ping-response.json"
MODELS_RESPONSE="$RUN_DIR/models-response.json"
RESOLVED_MODEL_FILE="$RUN_DIR/resolved-model.json"
CREATE_BOT_PAYLOAD="$RUN_DIR/create-bot-payload.json"
CREATE_BOT_RESPONSE="$RUN_DIR/create-bot-response.json"
CREATE_SESSION_PAYLOAD="$RUN_DIR/create-session-payload.json"
CREATE_SESSION_RESPONSE="$RUN_DIR/create-session-response.json"
TASK_ARCHIVE_PAYLOAD="$RUN_DIR/task-archive-upload-response.json"
EXTRACT_PAYLOAD="$RUN_DIR/extract-payload.json"
EXTRACT_RESPONSE="$RUN_DIR/extract-response.json"
EVENTS_FILE="$RUN_DIR/events.jsonl"
AGENT_LOG="$RUN_DIR/agent.log"
VERIFIER_STDOUT="$RUN_DIR/verifier.stdout"
VERIFIER_STDERR="$RUN_DIR/verifier.stderr"
SUMMARY_FILE="$RUN_DIR/summary.json"
LAST_API_ERROR="$RUN_DIR/last-api-error.txt"

START_EPOCH="$(date +%s)"
STARTED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
BOT_ID=""
SESSION_ID=""
STREAM_ID=""
TOKEN=""
USER_ID=""
VERIFY_EXIT=""
AGENT_EXIT=""
DELETED_BOT=false

write_summary() {
  local status="$1"
  local score="$2"
  local passed="$3"
  local message="$4"
  local ended_at
  local end_epoch
  ended_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  end_epoch="$(date +%s)"
  SUMMARY_STATUS="$status" \
  SUMMARY_SCORE="$score" \
  SUMMARY_PASSED="$passed" \
  SUMMARY_MESSAGE="$message" \
  SUMMARY_ENDED_AT="$ended_at" \
  SUMMARY_DURATION_SEC="$((end_epoch - START_EPOCH))" \
  SUMMARY_STARTED_AT="$STARTED_AT" \
  SUMMARY_SERVER="$SERVER" \
  SUMMARY_TASK_DIR="$TASK_DIR" \
  SUMMARY_INSTRUCTION_FILE="$INSTRUCTION_FILE" \
  SUMMARY_MODEL_REF="$MODEL_REF" \
  SUMMARY_REASONING_EFFORT="$REASONING_EFFORT" \
  SUMMARY_WORKSPACE_BACKEND="$WORKSPACE_BACKEND" \
  SUMMARY_CONTAINER_IMAGE="$CONTAINER_IMAGE" \
  SUMMARY_CONTAINER_WORKDIR="$CONTAINER_WORKDIR" \
  SUMMARY_TASK_UPLOAD="$TASK_UPLOAD" \
  SUMMARY_VERIFIER_DIR="$VERIFIER_DIR" \
  SUMMARY_VERIFIER_CONTAINER_DIR="$VERIFIER_CONTAINER_DIR" \
  SUMMARY_CHAT_CHANNEL="$CHAT_CHANNEL" \
  SUMMARY_VERIFY_CMD="$VERIFY_CMD" \
  SUMMARY_VERIFIER_TIMEOUT_SEC="$VERIFIER_TIMEOUT_SEC" \
  SUMMARY_BOT_NAME="$BOT_NAME" \
  SUMMARY_BOT_ID="$BOT_ID" \
  SUMMARY_SESSION_ID="$SESSION_ID" \
  SUMMARY_STREAM_ID="$STREAM_ID" \
  SUMMARY_AGENT_EXIT="$AGENT_EXIT" \
  SUMMARY_VERIFIER_EXIT="$VERIFY_EXIT" \
  SUMMARY_DELETED_BOT="$DELETED_BOT" \
  SUMMARY_RUN_DIR="$RUN_DIR" \
  SUMMARY_EVENTS_FILE="$EVENTS_FILE" \
  SUMMARY_AGENT_INSTRUCTION_FILE="$AGENT_INSTRUCTION_FILE" \
  SUMMARY_VERIFIER_STDOUT="$VERIFIER_STDOUT" \
  SUMMARY_VERIFIER_STDERR="$VERIFIER_STDERR" \
  SUMMARY_RESOLVED_MODEL_FILE="$RESOLVED_MODEL_FILE" \
  node <<'NODE' >"$SUMMARY_FILE"
const fs = require("fs");

function emptyToNull(value) {
  return value === undefined || value === "" ? null : value;
}

function numberOrNull(value) {
  if (value === undefined || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

let resolvedModel = null;
const resolvedModelFile = process.env.SUMMARY_RESOLVED_MODEL_FILE;
if (resolvedModelFile && fs.existsSync(resolvedModelFile)) {
  try {
    resolvedModel = JSON.parse(fs.readFileSync(resolvedModelFile, "utf8"));
  } catch {
    resolvedModel = null;
  }
}

const summary = {
  status: process.env.SUMMARY_STATUS,
  score: Number(process.env.SUMMARY_SCORE || "0"),
  passed: process.env.SUMMARY_PASSED === "true",
  message: emptyToNull(process.env.SUMMARY_MESSAGE),
  started_at: process.env.SUMMARY_STARTED_AT,
  ended_at: process.env.SUMMARY_ENDED_AT,
  duration_sec: numberOrNull(process.env.SUMMARY_DURATION_SEC),
  server: process.env.SUMMARY_SERVER,
  task_dir: process.env.SUMMARY_TASK_DIR,
  instruction_file: process.env.SUMMARY_INSTRUCTION_FILE,
  model_ref: process.env.SUMMARY_MODEL_REF,
  reasoning_effort: emptyToNull(process.env.SUMMARY_REASONING_EFFORT),
  workspace_backend: process.env.SUMMARY_WORKSPACE_BACKEND,
  container_image: emptyToNull(process.env.SUMMARY_CONTAINER_IMAGE),
  container_workdir: emptyToNull(process.env.SUMMARY_CONTAINER_WORKDIR),
  task_upload: process.env.SUMMARY_TASK_UPLOAD === "true",
  verifier_dir: emptyToNull(process.env.SUMMARY_VERIFIER_DIR),
  verifier_container_dir: emptyToNull(process.env.SUMMARY_VERIFIER_CONTAINER_DIR),
  chat_channel: process.env.SUMMARY_CHAT_CHANNEL,
  resolved_model: resolvedModel,
  verify_cmd: process.env.SUMMARY_VERIFY_CMD,
  verifier_timeout_sec: numberOrNull(process.env.SUMMARY_VERIFIER_TIMEOUT_SEC),
  bot_name: process.env.SUMMARY_BOT_NAME,
  bot_id: emptyToNull(process.env.SUMMARY_BOT_ID),
  session_id: emptyToNull(process.env.SUMMARY_SESSION_ID),
  stream_id: emptyToNull(process.env.SUMMARY_STREAM_ID),
  agent_exit_code: numberOrNull(process.env.SUMMARY_AGENT_EXIT),
  verifier_exit_code: numberOrNull(process.env.SUMMARY_VERIFIER_EXIT),
  deleted_bot: process.env.SUMMARY_DELETED_BOT === "true",
  artifacts: {
    run_dir: process.env.SUMMARY_RUN_DIR,
    agent_instruction: process.env.SUMMARY_AGENT_INSTRUCTION_FILE,
    events_jsonl: process.env.SUMMARY_EVENTS_FILE,
    verifier_stdout: process.env.SUMMARY_VERIFIER_STDOUT,
    verifier_stderr: process.env.SUMMARY_VERIFIER_STDERR
  }
};

process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
NODE
}

fail_run() {
  local code="$1"
  local status="$2"
  local message="$3"
  AGENT_EXIT="${AGENT_EXIT:-}"
  VERIFY_EXIT="${VERIFY_EXIT:-}"
  write_summary "$status" 0 false "$message"
  echo "ERROR: $message" >&2
  echo "Summary: $SUMMARY_FILE" >&2
  exit "$code"
}

api_request() {
  local method="$1"
  local path="$2"
  local payload_file="$3"
  local out_file="$4"
  local label="$5"
  local fail_code="$6"
  local http_code
  local curl_status
  local curl_args

  curl_args=(
    curl
    -sS
    -o "$out_file"
    -w "%{http_code}"
    -X "$method"
    -H "Accept: application/json"
  )
  if [ -n "$TOKEN" ]; then
    curl_args+=(-H "Authorization: Bearer $TOKEN")
  fi
  if [ -n "$payload_file" ]; then
    curl_args+=(-H "Content-Type: application/json" --data-binary "@$payload_file")
  fi
  curl_args+=("$SERVER$path")

  set +e
  http_code="$("${curl_args[@]}")"
  curl_status="$?"
  set -e

  if [ "$curl_status" -ne 0 ]; then
    fail_run "$fail_code" "${label}_failed" "$label request failed with curl exit $curl_status"
  fi
  case "$http_code" in
    2*)
      ;;
    *)
      {
        printf '%s HTTP %s\n' "$label" "$http_code"
        sed -n '1,120p' "$out_file"
      } >"$LAST_API_ERROR"
      fail_run "$fail_code" "${label}_failed" "$label request failed with HTTP $http_code; see $LAST_API_ERROR"
      ;;
  esac
}

write_summary "initialized" 0 false "run initialized"

write_json_file "$LOGIN_PAYLOAD" env LOGIN_USERNAME="$USERNAME" LOGIN_PASSWORD="$PASSWORD" node <<'NODE'
const payload = {
  username: process.env.LOGIN_USERNAME,
  password: process.env.LOGIN_PASSWORD
};
process.stdout.write(JSON.stringify(payload));
NODE
api_request POST "/auth/login" "$LOGIN_PAYLOAD" "$LOGIN_RESPONSE" "login" 11

if ! TOKEN="$(json_get access_token <"$LOGIN_RESPONSE")"; then
  fail_run 11 "login_failed" "login response did not include access_token"
fi
if ! USER_ID="$(json_get user_id <"$LOGIN_RESPONSE")"; then
  fail_run 11 "login_failed" "login response did not include user_id"
fi

api_request GET "/ping" "" "$PING_RESPONSE" "ping" 12
LOCAL_WORKSPACE_ENABLED="$(node -e 'const fs=require("fs"); const j=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); process.stdout.write(String(j.local_workspace_enabled === true));' "$PING_RESPONSE")"
if [ "$WORKSPACE_BACKEND" = "local" ] && [ "$LOCAL_WORKSPACE_ENABLED" != "true" ]; then
  fail_run 12 "local_workspace_disabled" "server reports local_workspace_enabled=false"
fi

api_request GET "/models?type=chat" "" "$MODELS_RESPONSE" "list_models" 13
if ! MODEL_REF="$MODEL_REF" MODELS_RESPONSE="$MODELS_RESPONSE" RESOLVED_MODEL_FILE="$RESOLVED_MODEL_FILE" node <<'NODE'
const fs = require("fs");
const ref = (process.env.MODEL_REF || "").trim();
const models = JSON.parse(fs.readFileSync(process.env.MODELS_RESPONSE, "utf8"));
if (!Array.isArray(models)) {
  console.error("/models?type=chat did not return an array");
  process.exit(2);
}
const matches = models.filter((model) => model && (model.id === ref || model.model_id === ref));
if (matches.length !== 1) {
  console.error(`model ref ${JSON.stringify(ref)} matched ${matches.length} chat models; use the model UUID when model_id is duplicated`);
  for (const model of matches) {
    console.error(`  id=${model.id} model_id=${model.model_id} name=${model.name || ""} enable=${model.enable}`);
  }
  process.exit(3);
}
const model = matches[0];
if (model.type !== "chat") {
  console.error(`model ref ${JSON.stringify(ref)} resolved to non-chat model type ${JSON.stringify(model.type)}`);
  process.exit(4);
}
if (model.enable !== true) {
  console.error(`model ref ${JSON.stringify(ref)} resolved to disabled model ${model.model_id}`);
  process.exit(5);
}
fs.writeFileSync(process.env.RESOLVED_MODEL_FILE, JSON.stringify(model, null, 2) + "\n");
NODE
then
  fail_run 13 "model_ref_invalid" "model ref did not uniquely resolve to an enabled chat model"
fi

write_json_file "$CREATE_BOT_PAYLOAD" \
  env BOT_NAME="$BOT_NAME" BOT_DISPLAY_NAME="$BOT_DISPLAY_NAME" TASK_DIR="$TASK_DIR" WORKSPACE_BACKEND="$WORKSPACE_BACKEND" CONTAINER_IMAGE="$CONTAINER_IMAGE" \
  node <<'NODE'
const payload = {
  name: process.env.BOT_NAME,
  display_name: process.env.BOT_DISPLAY_NAME,
  is_active: true,
  wait_for_ready: true
};
if (process.env.WORKSPACE_BACKEND === "local") {
  payload.metadata = {
    workspace: {
      backend: "local",
      local_workspace_path: process.env.TASK_DIR
    }
  };
} else {
  const image = (process.env.CONTAINER_IMAGE || "").trim();
  if (image) {
    payload.metadata = {
      workspace: {
        backend: "container",
        image
      }
    };
  }
}
process.stdout.write(JSON.stringify(payload));
NODE
api_request POST "/bots" "$CREATE_BOT_PAYLOAD" "$CREATE_BOT_RESPONSE" "create_bot" 14
if ! BOT_ID="$(json_get id <"$CREATE_BOT_RESPONSE")"; then
  fail_run 14 "create_bot_failed" "create bot response did not include id"
fi

upload_extract_dir() {
  local source_dir="$1"
  local target_dir="$2"
  local label="$3"
  local archive_file
  local archive_path
  local upload_response
  local extract_payload
  local extract_response
  local http_code
  local curl_status
  archive_file="$(mktemp "${TMPDIR:-/tmp}/memoh-tbench-task.XXXXXX")"
  TEMP_FILES+=("$archive_file")
  archive_path="${target_dir%/}.tgz"
  upload_response="$RUN_DIR/$label-archive-upload-response.json"
  extract_payload="$RUN_DIR/$label-extract-payload.json"
  extract_response="$RUN_DIR/$label-extract-response.json"

  if ! tar -czf "$archive_file" -C "$source_dir" .; then
    fail_run 17 "${label}_archive_failed" "failed to create $label archive from $source_dir"
  fi

  set +e
  http_code="$(curl -sS -o "$upload_response" -w "%{http_code}" \
    -X POST \
    -H "Accept: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -F "path=$archive_path" \
    -F "file=@$archive_file;filename=$label.tgz" \
    "$SERVER/bots/$BOT_ID/container/fs/upload")"
  curl_status="$?"
  set -e

  if [ "$curl_status" -ne 0 ]; then
    fail_run 17 "${label}_upload_failed" "failed to upload $label archive with curl exit $curl_status"
  fi
  case "$http_code" in
    2*)
      ;;
    *)
      {
        printf '%s_upload HTTP %s\n' "$label" "$http_code"
        sed -n '1,120p' "$upload_response"
      } >"$LAST_API_ERROR"
      fail_run 17 "${label}_upload_failed" "failed to upload $label archive with HTTP $http_code; see $LAST_API_ERROR"
      ;;
  esac

  write_json_file "$extract_payload" env ARCHIVE_PATH="$archive_path" node <<'NODE'
const payload = { path: process.env.ARCHIVE_PATH };
process.stdout.write(JSON.stringify(payload));
NODE
  api_request POST "/bots/$BOT_ID/container/fs/extract" "$extract_payload" "$extract_response" "extract_$label" 17
}

if [ "$WORKSPACE_BACKEND" = "container" ]; then
  if [ "$TASK_UPLOAD" = "true" ]; then
    upload_extract_dir "$TASK_DIR" "$CONTAINER_WORKDIR" "task"
  fi
  {
    if [ "$TASK_UPLOAD" = "true" ]; then
      printf 'The benchmark task files have been prepared in your container workspace at `%s`.\n' "$CONTAINER_WORKDIR"
    else
      printf 'Use the benchmark container workspace at `%s`.\n' "$CONTAINER_WORKDIR"
    fi
    printf 'Work from that directory. When you run shell commands, start by changing into `%s`.\n\n' "$CONTAINER_WORKDIR"
    cat "$INSTRUCTION_FILE"
  } >"$AGENT_INSTRUCTION_FILE"
else
  cp "$INSTRUCTION_FILE" "$AGENT_INSTRUCTION_FILE"
fi

write_json_file "$CREATE_SESSION_PAYLOAD" \
  env SESSION_TITLE="Terminal Bench $RUN_STAMP" CHAT_CHANNEL="$CHAT_CHANNEL" \
  node <<'NODE'
const payload = {
  type: "chat",
  title: process.env.SESSION_TITLE,
  channel_type: process.env.CHAT_CHANNEL || "web"
};
process.stdout.write(JSON.stringify(payload));
NODE
api_request POST "/bots/$BOT_ID/sessions" "$CREATE_SESSION_PAYLOAD" "$CREATE_SESSION_RESPONSE" "create_session" 15
if ! SESSION_ID="$(json_get id <"$CREATE_SESSION_RESPONSE")"; then
  fail_run 15 "create_session_failed" "create session response did not include id"
fi

STREAM_ID="tbench-$RUN_STAMP-$$"
case "$SERVER" in
  http://*)
    WS_BASE="ws://${SERVER#http://}"
    ;;
  https://*)
    WS_BASE="wss://${SERVER#https://}"
    ;;
esac
WS_URL="$WS_BASE/bots/$BOT_ID/$CHAT_CHANNEL/ws?token=$(url_encode "$TOKEN")"

echo "Running Memoh Terminal-Bench E2E:"
printf '  server=%s\n' "$SERVER"
printf '  bot_id=%s\n' "$BOT_ID"
printf '  session_id=%s\n' "$SESSION_ID"
printf '  model_ref=%s\n' "$MODEL_REF"
printf '  task_dir=%s\n' "$TASK_DIR"
printf '  run_dir=%s\n' "$RUN_DIR"

set +e
WS_URL="$WS_URL" \
SESSION_ID="$SESSION_ID" \
STREAM_ID="$STREAM_ID" \
INSTRUCTION_FILE="$AGENT_INSTRUCTION_FILE" \
MODEL_REF="$MODEL_REF" \
REASONING_EFFORT="$REASONING_EFFORT" \
EVENTS_FILE="$EVENTS_FILE" \
AGENT_LOG="$AGENT_LOG" \
TIMEOUT_SEC="$TIMEOUT_SEC" \
node <<'NODE' 2>>"$AGENT_LOG"
const fs = require("fs");

const {
  WS_URL,
  SESSION_ID,
  STREAM_ID,
  INSTRUCTION_FILE,
  MODEL_REF,
  REASONING_EFFORT,
  EVENTS_FILE,
  AGENT_LOG,
  TIMEOUT_SEC
} = process.env;

function log(message) {
  fs.appendFileSync(AGENT_LOG, `[${new Date().toISOString()}] ${message}\n`);
}

async function eventDataToString(data) {
  if (typeof data === "string") return data;
  if (Buffer.isBuffer(data)) return data.toString("utf8");
  if (data instanceof ArrayBuffer) return Buffer.from(data).toString("utf8");
  if (ArrayBuffer.isView(data)) {
    return Buffer.from(data.buffer, data.byteOffset, data.byteLength).toString("utf8");
  }
  if (data && typeof data.arrayBuffer === "function") {
    return Buffer.from(await data.arrayBuffer()).toString("utf8");
  }
  return String(data);
}

if (typeof WebSocket !== "function") {
  log("global WebSocket is not available; use Node 22+");
  process.exit(23);
}

const instruction = fs.readFileSync(INSTRUCTION_FILE, "utf8");
const timeoutMs = Number(TIMEOUT_SEC) * 1000;
let finished = false;
let opened = false;

function finish(code, message) {
  if (finished) return;
  finished = true;
  clearTimeout(timer);
  log(message);
  try {
    ws.close();
  } catch {
    // Ignore close errors while exiting.
  }
  setTimeout(() => process.exit(code), 25);
}

const timer = setTimeout(() => {
  if (opened) {
    try {
      ws.send(JSON.stringify({
        type: "abort",
        session_id: SESSION_ID,
        stream_id: STREAM_ID
      }));
    } catch {
      // Ignore abort send failures; timeout is already terminal for this runner.
    }
  }
  finish(21, `agent timed out after ${TIMEOUT_SEC}s`);
}, timeoutMs);

const ws = new WebSocket(WS_URL);

ws.addEventListener("open", () => {
  opened = true;
  log("websocket opened");
  ws.send(JSON.stringify({
    type: "message",
    session_id: SESSION_ID,
    stream_id: STREAM_ID,
    text: instruction,
    model_id: MODEL_REF,
    reasoning_effort: REASONING_EFFORT || undefined
  }));
  log("instruction sent");
});

ws.addEventListener("message", async (event) => {
  const raw = await eventDataToString(event.data);
  fs.appendFileSync(EVENTS_FILE, raw + "\n");

  let payload;
  try {
    payload = JSON.parse(raw);
  } catch {
    log(`received non-json event: ${raw.slice(0, 160)}`);
    return;
  }

  const type = payload.type || "unknown";
  const streamID = payload.stream_id || payload.streamId || "";
  const sessionID = payload.session_id || payload.sessionId || "";
  log(`event type=${type} stream_id=${streamID} session_id=${sessionID}`);

  if (streamID && streamID !== STREAM_ID) return;
  if (sessionID && sessionID !== SESSION_ID) return;

  if (type === "end") {
    finish(0, "agent ended");
  } else if (type === "error") {
    const message = payload.message || "agent websocket error";
    finish(20, `agent returned error: ${message}`);
  }
});

ws.addEventListener("error", () => {
  finish(23, "websocket error");
});

ws.addEventListener("close", () => {
  if (!finished) {
    finish(opened ? 22 : 23, opened ? "websocket closed before end" : "websocket closed before open");
  }
});
NODE
AGENT_EXIT="$?"
set -e

if [ "$AGENT_EXIT" -ne 0 ]; then
  fail_run "$AGENT_EXIT" "agent_failed" "agent run failed with exit code $AGENT_EXIT"
fi

if [ "$WORKSPACE_BACKEND" = "container" ] && [ -n "$VERIFIER_DIR" ]; then
  echo "Uploading verifier files:"
  printf '  source=%s\n' "$VERIFIER_DIR"
  printf '  destination=%s\n' "$VERIFIER_CONTAINER_DIR"
  upload_extract_dir "$VERIFIER_DIR" "$VERIFIER_CONTAINER_DIR" "verifier"
fi

echo "Running verifier:"
printf '  command=%s\n' "$VERIFY_CMD"
if [ "$WORKSPACE_BACKEND" = "container" ]; then
  printf '  backend=container\n'
  printf '  cwd=%s\n' "$CONTAINER_WORKDIR"
else
  printf '  backend=local\n'
  printf '  cwd=%s\n' "$TASK_DIR"
fi

set +e
if [ "$WORKSPACE_BACKEND" = "container" ]; then
  TERMINAL_WS_URL="$WS_BASE/bots/$BOT_ID/container/terminal/ws?token=$(url_encode "$TOKEN")&cols=160&rows=48"
  TERMINAL_WS_URL="$TERMINAL_WS_URL" \
  VERIFY_CMD="$VERIFY_CMD" \
  CONTAINER_WORKDIR="$CONTAINER_WORKDIR" \
  TIMEOUT_SEC="$VERIFIER_TIMEOUT_SEC" \
  node <<'NODE' >"$VERIFIER_STDOUT" 2>"$VERIFIER_STDERR"
const {
  TERMINAL_WS_URL,
  VERIFY_CMD,
  CONTAINER_WORKDIR,
  TIMEOUT_SEC
} = process.env;

function shQuote(value) {
  return `'${String(value).replace(/'/g, `'\"'\"'`)}'`;
}

async function eventDataToString(data) {
  if (typeof data === "string") return data;
  if (Buffer.isBuffer(data)) return data.toString("utf8");
  if (data instanceof ArrayBuffer) return Buffer.from(data).toString("utf8");
  if (ArrayBuffer.isView(data)) {
    return Buffer.from(data.buffer, data.byteOffset, data.byteLength).toString("utf8");
  }
  if (data && typeof data.arrayBuffer === "function") {
    return Buffer.from(await data.arrayBuffer()).toString("utf8");
  }
  return String(data);
}

if (typeof WebSocket !== "function") {
  console.error("global WebSocket is not available; use Node 22+");
  process.exit(125);
}

const sentinel = `__MEMOH_TBENCH_EXIT_${Date.now()}_${process.pid}__`;
const timeoutMs = Number(TIMEOUT_SEC) * 1000;
let output = "";
let finished = false;
let opened = false;

function finish(code) {
  if (finished) return;
  finished = true;
  clearTimeout(timer);
  try {
    ws.close();
  } catch {
    // Ignore close errors while exiting.
  }
  setTimeout(() => process.exit(code), 25);
}

const timer = setTimeout(() => {
  console.error(`verifier timed out after ${TIMEOUT_SEC}s`);
  finish(124);
}, timeoutMs);

const verifierScript = [
  `cd ${shQuote(CONTAINER_WORKDIR)}`,
  `cd_status=$?`,
  `if [ "$cd_status" -eq 0 ]; then sh -lc ${shQuote(VERIFY_CMD)}; status=$?; else status=$cd_status; fi`,
  `printf '\\n${sentinel}:%s\\n' "$status"`
].join("\n");
const command = `sh -lc ${shQuote(verifierScript)}; exit\n`;

const ws = new WebSocket(TERMINAL_WS_URL);

ws.addEventListener("open", () => {
  opened = true;
  ws.send(Buffer.from(command, "utf8"));
});

ws.addEventListener("message", async (event) => {
  const raw = await eventDataToString(event.data);
  output += raw;
  process.stdout.write(raw);
  const match = output.match(new RegExp(`${sentinel}:(\\d+)`));
  if (match) {
    finish(Number(match[1]));
  }
});

ws.addEventListener("error", () => {
  console.error("terminal websocket error");
  finish(125);
});

ws.addEventListener("close", () => {
  if (!finished) {
    console.error(opened ? "terminal websocket closed before verifier exit sentinel" : "terminal websocket closed before open");
    finish(126);
  }
});
NODE
else
  export MEMOH_TBENCH_LOCAL_VERIFIER_TIMEOUT="$VERIFIER_TIMEOUT_SEC"
  (
    cd "$TASK_DIR"
    perl -e 'alarm shift; exec @ARGV' "$MEMOH_TBENCH_LOCAL_VERIFIER_TIMEOUT" bash -lc "$VERIFY_CMD"
  ) >"$VERIFIER_STDOUT" 2>"$VERIFIER_STDERR"
fi
VERIFY_EXIT="$?"
set -e

if [ "$VERIFY_EXIT" -eq 0 ]; then
  if [ "$DELETE_BOT_ON_SUCCESS" = "true" ]; then
    DELETE_RESPONSE="$RUN_DIR/delete-bot-response.json"
    set +e
    DELETE_HTTP_CODE="$(curl -sS -o "$DELETE_RESPONSE" -w "%{http_code}" \
      -X DELETE \
      -H "Accept: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      "$SERVER/bots/$BOT_ID")"
    DELETE_CURL_STATUS="$?"
    set -e
    if [ "$DELETE_CURL_STATUS" -eq 0 ] && [ "${DELETE_HTTP_CODE#2}" != "$DELETE_HTTP_CODE" ]; then
      DELETED_BOT=true
    else
      warn "failed to delete bot $BOT_ID after successful verifier; HTTP=$DELETE_HTTP_CODE curl_status=$DELETE_CURL_STATUS"
    fi
  fi
  write_summary "passed" 1 true "verifier passed"
  echo "PASS score=1"
  echo "Summary: $SUMMARY_FILE"
  exit 0
fi

write_summary "verifier_failed" 0 false "verifier failed with exit code $VERIFY_EXIT"
echo "FAIL score=0"
echo "Summary: $SUMMARY_FILE"
exit 30
