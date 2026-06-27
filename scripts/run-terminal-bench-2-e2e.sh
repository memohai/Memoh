#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run-terminal-bench-e2e.sh"

SERVER="${MEMOH_TBENCH_SERVER:-http://127.0.0.1:18080}"
USERNAME="${MEMOH_TBENCH_USERNAME:-admin}"
PASSWORD="${MEMOH_TBENCH_PASSWORD:-admin123}"
DATASET_DIR="${MEMOH_TBENCH2_DATASET_DIR:-.terminal-bench-2}"
OUT_DIR="${MEMOH_TBENCH2_OUT_DIR:-.memoh-tbench2-runs}"
MODEL_REF="${MEMOH_TBENCH_MODEL_REF:-}"
REASONING_EFFORT="${MEMOH_TBENCH_REASONING_EFFORT:-}"
CHAT_CHANNEL="${MEMOH_TBENCH_CHAT_CHANNEL:-web}"
CONTAINER_WORKDIR="${MEMOH_TBENCH2_CONTAINER_WORKDIR:-/app}"
VERIFIER_CONTAINER_DIR="${MEMOH_TBENCH2_VERIFIER_CONTAINER_DIR:-/tests}"
VERIFY_CMD="${MEMOH_TBENCH2_VERIFY_CMD:-mkdir -p /logs/verifier && bash /tests/test.sh}"
LIMIT=""
DOWNLOAD=false
FAIL_FAST=false
DELETE_BOT_ON_SUCCESS=false
TASK_FILTERS=()

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage:
  scripts/run-terminal-bench-2-e2e.sh \
    --dataset-dir /path/to/terminal-bench-2 \
    --model-ref glm-5.2

Options:
  --download               Clone the official Terminal-Bench 2 dataset when --dataset-dir is missing.
  --dataset-dir DIR        Terminal-Bench 2 dataset directory. Default: MEMOH_TBENCH2_DATASET_DIR or .terminal-bench-2
  --task NAME              Run one task slug or full task name. Can be repeated. Omit to run all tasks.
  --limit N                Run at most N selected tasks, useful for smoke tests.
  --server URL             Memoh server URL. Default: MEMOH_TBENCH_SERVER or http://127.0.0.1:18080
  --username USERNAME      Login username. Default: MEMOH_TBENCH_USERNAME or admin
  --password PASSWORD      Login password. Default: MEMOH_TBENCH_PASSWORD or admin123
  --model-ref REF          Chat model UUID or models.model_id slug. Required.
  --model-id REF           Alias for --model-ref.
  --reasoning-effort VALUE Optional reasoning effort override.
  --chat-channel CHANNEL   Chat route/channel. Default: MEMOH_TBENCH_CHAT_CHANNEL or web.
  --container-workdir DIR  Official task working directory in the container. Default: /app.
  --out-dir DIR            Output directory. Default: MEMOH_TBENCH2_OUT_DIR or .memoh-tbench2-runs.
  --fail-fast              Stop after the first task failure.
  --delete-bot-on-success  Delete created Memoh bots when their verifier passes.
  -h, --help               Show this help.

Examples:
  scripts/run-terminal-bench-2-e2e.sh --download --model-ref glm-5.2 --limit 5
  scripts/run-terminal-bench-2-e2e.sh --dataset-dir /tmp/terminal-bench-2 --task regex-log --model-ref glm-5.2
EOF
}

abs_path() {
  node -e 'console.log(require("path").resolve(process.argv[1]))' "$1"
}

sanitize_task_slug() {
  local value="$1"
  value="${value#terminal-bench/}"
  printf '%s' "$value" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g'
}

parse_task_toml() {
  local task_dir="$1"
  node - "$task_dir" <<'NODE'
const fs = require("fs");
const path = require("path");

const taskDir = process.argv[2];
const toml = fs.readFileSync(path.join(taskDir, "task.toml"), "utf8");

function section(name) {
  const re = new RegExp(`\\[${name.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\$&")}\\]([\\s\\S]*?)(?=\\n\\[|$)`);
  const match = toml.match(re);
  return match ? match[1] : "";
}

function stringValue(body, key) {
  const match = body.match(new RegExp(`^\\s*${key}\\s*=\\s*"([^"]+)"`, "m"));
  return match ? match[1].trim() : "";
}

function numberValue(body, key) {
  const match = body.match(new RegExp(`^\\s*${key}\\s*=\\s*([0-9]+(?:\\.[0-9]+)?)`, "m"));
  return match ? Number(match[1]) : null;
}

const taskName = stringValue(section("task"), "name") || `terminal-bench/${path.basename(taskDir)}`;
const dockerImage = stringValue(section("environment"), "docker_image");
const agentTimeout = numberValue(section("agent"), "timeout_sec") || 1800;
const verifierTimeout = numberValue(section("verifier"), "timeout_sec") || agentTimeout;

if (!dockerImage) {
  console.error(`missing [environment].docker_image in ${path.join(taskDir, "task.toml")}`);
  process.exit(2);
}

const slug = taskName.startsWith("terminal-bench/")
  ? taskName.slice("terminal-bench/".length)
  : path.basename(taskDir);

process.stdout.write([
  taskName,
  slug,
  dockerImage,
  String(Math.ceil(agentTimeout)),
  String(Math.ceil(verifierTimeout))
].join("\t"));
NODE
}

task_selected() {
  local task_name="$1"
  local slug="$2"
  local filter

  if [ "${#TASK_FILTERS[@]}" -eq 0 ]; then
    return 0
  fi

  for filter in "${TASK_FILTERS[@]}"; do
    if [ "$filter" = "$slug" ] || [ "$filter" = "$task_name" ] || [ "$filter" = "terminal-bench/$slug" ]; then
      return 0
    fi
  done
  return 1
}

append_result() {
  local summary_file="$1"
  local task_name="$2"
  local slug="$3"
  local docker_image="$4"
  local runner_exit="$5"

  node - "$summary_file" "$task_name" "$slug" "$docker_image" "$runner_exit" <<'NODE' >>"$RESULTS_JSONL"
const fs = require("fs");

const [summaryFile, taskName, slug, dockerImage, runnerExit] = process.argv.slice(2);
let summary = null;
if (summaryFile && fs.existsSync(summaryFile)) {
  try {
    summary = JSON.parse(fs.readFileSync(summaryFile, "utf8"));
  } catch {
    summary = null;
  }
}

const result = {
  task_name: taskName,
  slug,
  docker_image: dockerImage,
  runner_exit_code: Number(runnerExit),
  summary_file: summaryFile || null,
  status: summary ? summary.status : "missing_summary",
  passed: summary ? summary.passed === true : false,
  score: summary ? Number(summary.score || 0) : 0,
  bot_id: summary ? summary.bot_id : null,
  session_id: summary ? summary.session_id : null,
  duration_sec: summary ? summary.duration_sec : null
};

process.stdout.write(JSON.stringify(result) + "\n");
NODE
}

write_aggregate_summary() {
  node - "$RESULTS_JSONL" "$AGGREGATE_SUMMARY" "$RUN_ROOT" "$DATASET_DIR" "$MODEL_REF" <<'NODE'
const fs = require("fs");

const [resultsFile, outputFile, runRoot, datasetDir, modelRef] = process.argv.slice(2);
const lines = fs.existsSync(resultsFile)
  ? fs.readFileSync(resultsFile, "utf8").split(/\n/).filter(Boolean)
  : [];
const results = lines.map((line) => JSON.parse(line));
const total = results.length;
const passed = results.filter((result) => result.passed).length;
const score = total === 0 ? 0 : results.reduce((sum, result) => sum + Number(result.score || 0), 0) / total;
const summary = {
  status: total === 0 ? "no_tasks" : (passed === total ? "passed" : "failed"),
  score,
  passed,
  total,
  model_ref: modelRef,
  dataset_dir: datasetDir,
  run_root: runRoot,
  results
};

fs.writeFileSync(outputFile, JSON.stringify(summary, null, 2) + "\n");
NODE
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
    --download)
      DOWNLOAD=true
      shift
      ;;
    --dataset-dir)
      [ "$#" -ge 2 ] || fail "--dataset-dir requires a value"
      DATASET_DIR="$2"
      shift 2
      ;;
    --task)
      [ "$#" -ge 2 ] || fail "--task requires a value"
      TASK_FILTERS+=("$2")
      shift 2
      ;;
    --limit)
      [ "$#" -ge 2 ] || fail "--limit requires a value"
      LIMIT="$2"
      shift 2
      ;;
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
    --chat-channel)
      [ "$#" -ge 2 ] || fail "--chat-channel requires a value"
      CHAT_CHANNEL="$2"
      shift 2
      ;;
    --container-workdir)
      [ "$#" -ge 2 ] || fail "--container-workdir requires a value"
      CONTAINER_WORKDIR="$2"
      shift 2
      ;;
    --out-dir)
      [ "$#" -ge 2 ] || fail "--out-dir requires a value"
      OUT_DIR="$2"
      shift 2
      ;;
    --fail-fast)
      FAIL_FAST=true
      shift
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

[ -x "$RUNNER" ] || fail "single-task runner is not executable: $RUNNER"
[ -n "$MODEL_REF" ] || fail "--model-ref is required"

case "$LIMIT" in
  ""|*[!0-9]*)
    if [ -n "$LIMIT" ]; then
      fail "--limit must be a positive integer"
    fi
    ;;
esac
if [ -n "$LIMIT" ] && [ "$LIMIT" -le 0 ]; then
  fail "--limit must be greater than zero"
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
    fail "MEMOH_TBENCH2_VERIFIER_CONTAINER_DIR must be an absolute container path"
    ;;
esac

if [ ! -d "$DATASET_DIR" ]; then
  if [ "$DOWNLOAD" = "true" ]; then
    command -v git >/dev/null 2>&1 || fail "missing required command: git"
    git clone https://github.com/harbor-framework/terminal-bench-2.git "$DATASET_DIR"
  else
    fail "dataset directory does not exist: $DATASET_DIR (use --download to clone it)"
  fi
fi

DATASET_DIR="$(abs_path "$DATASET_DIR")"
OUT_DIR="$(abs_path "$OUT_DIR")"
RUN_STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"
RUN_ROOT="$OUT_DIR/$RUN_STAMP-terminal-bench-2"
RESULTS_JSONL="$RUN_ROOT/results.jsonl"
AGGREGATE_SUMMARY="$RUN_ROOT/summary.json"

mkdir -p "$RUN_ROOT/tasks" "$RUN_ROOT/staging"
: >"$RESULTS_JSONL"

echo "Running Terminal-Bench 2 through Memoh:"
printf '  dataset=%s\n' "$DATASET_DIR"
printf '  model_ref=%s\n' "$MODEL_REF"
printf '  run_root=%s\n' "$RUN_ROOT"

selected_count=0
failed_count=0

while IFS= read -r task_toml; do
  task_dir="$(dirname "$task_toml")"
  task_info="$(parse_task_toml "$task_dir")"
  IFS=$'\t' read -r task_name raw_slug docker_image timeout_sec verifier_timeout_sec <<EOF
$task_info
EOF
  slug="$(sanitize_task_slug "$raw_slug")"

  if ! task_selected "$task_name" "$raw_slug"; then
    continue
  fi
  if [ -n "$LIMIT" ] && [ "$selected_count" -ge "$LIMIT" ]; then
    break
  fi

  instruction_file="$task_dir/instruction.md"
  verifier_dir="$task_dir/tests"
  [ -f "$instruction_file" ] || fail "missing instruction.md for $task_name"
  [ -d "$verifier_dir" ] || fail "missing tests directory for $task_name"

  selected_count=$((selected_count + 1))
  task_out_dir="$RUN_ROOT/tasks/$slug"
  staging_dir="$RUN_ROOT/staging/$slug"
  task_log="$task_out_dir/runner.log"
  mkdir -p "$task_out_dir" "$staging_dir"

  echo
  printf '[%s] %s\n' "$selected_count" "$task_name"
  printf '  image=%s\n' "$docker_image"
  printf '  agent_timeout=%ss\n' "$timeout_sec"
  printf '  verifier_timeout=%ss\n' "$verifier_timeout_sec"

  runner_args=(
    "$RUNNER"
    --server "$SERVER"
    --username "$USERNAME"
    --password "$PASSWORD"
    --task-dir "$staging_dir"
    --instruction-file "$instruction_file"
    --model-ref "$MODEL_REF"
    --workspace-backend container
    --container-image "$docker_image"
    --container-workdir "$CONTAINER_WORKDIR"
    --skip-task-upload
    --verifier-dir "$verifier_dir"
    --verifier-container-dir "$VERIFIER_CONTAINER_DIR"
    --verify-cmd "$VERIFY_CMD"
    --timeout-sec "$timeout_sec"
    --verifier-timeout-sec "$verifier_timeout_sec"
    --out-dir "$task_out_dir"
    --bot-prefix "tbench2-$slug"
    --chat-channel "$CHAT_CHANNEL"
  )
  if [ -n "$REASONING_EFFORT" ]; then
    runner_args+=(--reasoning-effort "$REASONING_EFFORT")
  fi
  if [ "$DELETE_BOT_ON_SUCCESS" = "true" ]; then
    runner_args+=(--delete-bot-on-success)
  fi

  set +e
  "${runner_args[@]}" 2>&1 | tee "$task_log"
  runner_exit="${PIPESTATUS[0]}"
  set -e

  summary_file="$(find "$task_out_dir" -name summary.json -type f | sort | tail -n 1)"
  append_result "$summary_file" "$task_name" "$slug" "$docker_image" "$runner_exit"

  if [ "$runner_exit" -ne 0 ]; then
    failed_count=$((failed_count + 1))
    if [ "$FAIL_FAST" = "true" ]; then
      break
    fi
  fi
done < <(find "$DATASET_DIR" -mindepth 2 -maxdepth 2 -name task.toml -type f | sort)

write_aggregate_summary

echo
echo "Terminal-Bench 2 aggregate summary:"
node - "$AGGREGATE_SUMMARY" <<'NODE'
const fs = require("fs");
const summary = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
console.log(`  score=${summary.score}`);
console.log(`  passed=${summary.passed}/${summary.total}`);
console.log(`  summary=${process.argv[2]}`);
NODE

if [ "$selected_count" -eq 0 ]; then
  fail "no tasks selected"
fi

if [ "$failed_count" -ne 0 ]; then
  exit 30
fi
exit 0
