#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-}"
EXPECTED_RUNS="${MEMOH_KATA_EVIDENCE_EXPECTED_RUNS:-1}"
REQUIRE_ENVIRONMENT="${MEMOH_KATA_EVIDENCE_REQUIRE_ENVIRONMENT:-true}"
EXPECT_KVM="${MEMOH_KATA_EVIDENCE_EXPECT_KVM:-true}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
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

validate_positive_int() {
  case "$2" in
    ''|*[!0-9]*)
      echo "ERROR: $1 must be a positive integer, got: $2" >&2
      exit 1
      ;;
    *)
      if [ "$2" -lt 1 ]; then
        echo "ERROR: $1 must be at least 1, got: $2" >&2
        exit 1
      fi
      ;;
  esac
}

assert_environment() {
  local file="$1"

  [ -s "$file" ] || { echo "ERROR: missing environment summary: $file" >&2; exit 1; }
  grep -q '^run_id=' "$file" || { echo "ERROR: environment summary missing run_id" >&2; exit 1; }
  grep -q '^runner_os=' "$file" || { echo "ERROR: environment summary missing runner_os" >&2; exit 1; }
  grep -q '^docker=' "$file" || { echo "ERROR: environment summary missing docker version" >&2; exit 1; }
  grep -q '^docker_compose=' "$file" || { echo "ERROR: environment summary missing docker compose version" >&2; exit 1; }
  grep -q '^kata_shim=' "$file" || { echo "ERROR: environment summary missing kata_shim" >&2; exit 1; }
  if [ "$EXPECT_KVM" = "true" ]; then
    grep -q '^kvm_present=true$' "$file" || { echo "ERROR: environment summary does not prove /dev/kvm was present" >&2; exit 1; }
  fi

  if grep -Eiq '(access_token|authorization|bearer|password|secret|jwt)' "$file"; then
    echo "ERROR: environment summary contains sensitive-looking text" >&2
    exit 1
  fi
}

if [ -z "$EVIDENCE_DIR" ]; then
  echo "usage: scripts/validate-kata-evidence-dir.sh <evidence-dir>" >&2
  exit 1
fi

require_cmd jq
validate_positive_int MEMOH_KATA_EVIDENCE_EXPECTED_RUNS "$EXPECTED_RUNS"
validate_bool MEMOH_KATA_EVIDENCE_REQUIRE_ENVIRONMENT "$REQUIRE_ENVIRONMENT"
validate_bool MEMOH_KATA_EVIDENCE_EXPECT_KVM "$EXPECT_KVM"

[ -d "$EVIDENCE_DIR" ] || { echo "ERROR: evidence directory not found: $EVIDENCE_DIR" >&2; exit 1; }

if [ "$REQUIRE_ENVIRONMENT" = "true" ]; then
  assert_environment "$EVIDENCE_DIR/environment.txt"
fi

api_files=()
while IFS= read -r -d '' file; do
  api_files+=("$file")
done < <(find "$EVIDENCE_DIR" -maxdepth 1 -type f -name '*.json' ! -name '*.smoke.json' -print0)

smoke_count=0
while IFS= read -r -d '' _; do
  smoke_count=$((smoke_count + 1))
done < <(find "$EVIDENCE_DIR" -maxdepth 1 -type f -name '*.smoke.json' -print0)

if [ "${#api_files[@]}" -ne "$EXPECTED_RUNS" ]; then
  echo "ERROR: expected $EXPECTED_RUNS API evidence file(s), found ${#api_files[@]} in $EVIDENCE_DIR" >&2
  find "$EVIDENCE_DIR" -maxdepth 1 -type f -print >&2
  exit 1
fi
if [ "$smoke_count" -ne "$EXPECTED_RUNS" ]; then
  echo "ERROR: expected $EXPECTED_RUNS smoke evidence file(s), found $smoke_count in $EVIDENCE_DIR" >&2
  find "$EVIDENCE_DIR" -maxdepth 1 -type f -print >&2
  exit 1
fi

for api_file in "${api_files[@]}"; do
  smoke_file="${api_file%.json}.smoke.json"
  [ -f "$smoke_file" ] || { echo "ERROR: missing paired smoke evidence for $api_file" >&2; exit 1; }

  scripts/validate-kata-evidence.sh "$api_file" >/dev/null
  scripts/validate-containerd-smoke-evidence.sh "$smoke_file" >/dev/null

  api_runtime="$(jq -er '.target.expected_runtime' "$api_file")"
  smoke_runtime="$(jq -er '.target.runtime' "$smoke_file")"
  if [ "$api_runtime" != "$smoke_runtime" ]; then
    echo "ERROR: runtime mismatch between $api_file and $smoke_file: $api_runtime != $smoke_runtime" >&2
    exit 1
  fi
done

echo "Kata evidence directory validated:"
printf '  directory=%s\n' "$EVIDENCE_DIR"
printf '  api_evidence=%s\n' "${#api_files[@]}"
printf '  smoke_evidence=%s\n' "$smoke_count"
