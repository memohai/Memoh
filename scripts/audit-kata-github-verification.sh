#!/usr/bin/env bash
set -euo pipefail

PR_NUMBER="${1:-${MEMOH_KATA_GITHUB_PR:-}}"
REPO="${MEMOH_KATA_GITHUB_REPO:-}"
REQUIRED_RUNNER_LABELS="self-hosted, linux, x64, kvm, kata"

fail() {
  echo "ERROR: $*" >&2
  exit 2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

require_cmd gh
require_cmd jq

if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)" || \
    fail "could not infer GitHub repo; set MEMOH_KATA_GITHUB_REPO=owner/repo"
fi

if [ -z "$PR_NUMBER" ]; then
  PR_NUMBER="$(gh pr view --repo "$REPO" --json number --jq .number 2>/dev/null)" || \
    fail "could not infer current PR; pass a PR number or set MEMOH_KATA_GITHUB_PR"
fi

PR_JSON="$(gh pr view "$PR_NUMBER" --repo "$REPO" \
  --json headRefOid,statusCheckRollup,url,isDraft,state)"

check_state() {
  local workflow="$1"
  local name="$2"
  jq -r \
    --arg workflow "$workflow" \
    --arg name "$name" \
    '
      [.statusCheckRollup[]
        | select(.workflowName == $workflow and .name == $name)][0]
      | if . == null then
          "missing"
        elif (.conclusion // "") != "" then
          (.conclusion | ascii_downcase)
        else
          (.status | ascii_downcase)
        end
    ' <<<"$PR_JSON"
}

print_check() {
  local workflow="$1"
  local name="$2"
  local state="$3"
  printf '  %-34s %s\n' "$workflow / $name:" "$state"
}

kata_static="$(check_state "Kata Runtime" "Static validation")"
docker_kata_static="$(check_state "Docker" "Kata static validation")"
runner_readiness="$(check_state "Kata Runtime" "Linux/KVM runner readiness")"
kata_e2e="$(check_state "Kata Runtime" "Linux/KVM E2E")"

runner_status="unknown"
runner_detail=""
if runners_json="$(gh api "repos/$REPO/actions/runners" 2>/dev/null)"; then
  runner_detail="$(jq -r '
    .runners[]
    | . as $runner
    | ([.labels[].name | ascii_downcase]) as $labels
    | select(
        ($labels | index("self-hosted")) and
        ($labels | index("linux")) and
        ($labels | index("x64")) and
        ($labels | index("kvm")) and
        ($labels | index("kata"))
      )
    | "\($runner.name) status=\($runner.status) busy=\($runner.busy)"
  ' <<<"$runners_json")"
  if [ -n "$runner_detail" ]; then
    runner_status="present"
  else
    runner_status="missing"
  fi
fi

url="$(jq -r .url <<<"$PR_JSON")"
head="$(jq -r .headRefOid <<<"$PR_JSON")"
state="$(jq -r .state <<<"$PR_JSON")"
is_draft="$(jq -r .isDraft <<<"$PR_JSON")"

echo "Kata GitHub verification audit:"
printf '  repo=%s\n' "$REPO"
printf '  pr=%s\n' "$PR_NUMBER"
printf '  url=%s\n' "$url"
printf '  head=%s\n' "$head"
printf '  state=%s draft=%s\n' "$state" "$is_draft"
printf '  required_runner_labels=%s\n' "$REQUIRED_RUNNER_LABELS"
printf '  matching_runner=%s\n' "$runner_status"
if [ -n "$runner_detail" ]; then
  while IFS= read -r line; do
    printf '    %s\n' "$line"
  done <<<"$runner_detail"
fi
echo
print_check "Kata Runtime" "Static validation" "$kata_static"
print_check "Docker" "Kata static validation" "$docker_kata_static"
print_check "Kata Runtime" "Linux/KVM runner readiness" "$runner_readiness"
print_check "Kata Runtime" "Linux/KVM E2E" "$kata_e2e"
echo

missing=()
if [ "$kata_static" != "success" ]; then
  missing+=("Kata Runtime / Static validation must be success")
fi
if [ "$docker_kata_static" != "missing" ] && [ "$docker_kata_static" != "success" ]; then
  missing+=("Docker / Kata static validation is present but not success")
fi
if [ "$kata_e2e" != "success" ]; then
  missing+=("Kata Runtime / Linux/KVM E2E must be success")
fi

if [ "${#missing[@]}" -gt 0 ]; then
  echo "Kata verification is incomplete:"
  for item in "${missing[@]}"; do
    printf '  - %s\n' "$item"
  done
  echo
  echo "Dispatch .github/workflows/kata-runtime.yml on this PR branch with run_kata_e2e=true after a self-hosted Linux/KVM runner with labels [$REQUIRED_RUNNER_LABELS] is registered."
  exit 1
fi

echo "Kata verification is complete for this PR head."
