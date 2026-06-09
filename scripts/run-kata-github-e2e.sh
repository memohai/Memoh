#!/usr/bin/env bash
set -euo pipefail

PR_NUMBER="${1:-${MEMOH_KATA_GITHUB_PR:-}}"
REPO="${MEMOH_KATA_GITHUB_REPO:-}"
RUN_COMPOSE_E2E="${MEMOH_KATA_GITHUB_COMPOSE_E2E:-true}"
READINESS_ONLY="${MEMOH_KATA_GITHUB_READINESS_ONLY:-false}"
WAIT_FOR_RUN="${MEMOH_KATA_GITHUB_WAIT:-true}"
ALLOW_NO_RUNNER="${MEMOH_KATA_GITHUB_ALLOW_NO_RUNNER:-false}"
WORKFLOW_FILE="${MEMOH_KATA_GITHUB_WORKFLOW:-kata-runtime.yml}"
REQUIRED_RUNNER_LABELS="self-hosted, linux, x64, kvm, kata"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

validate_bool() {
  case "$2" in
    true|false)
      ;;
    *)
      fail "$1 must be true or false; got: $2"
      ;;
  esac
}

require_cmd gh
require_cmd jq

validate_bool MEMOH_KATA_GITHUB_COMPOSE_E2E "$RUN_COMPOSE_E2E"
validate_bool MEMOH_KATA_GITHUB_READINESS_ONLY "$READINESS_ONLY"
validate_bool MEMOH_KATA_GITHUB_WAIT "$WAIT_FOR_RUN"
validate_bool MEMOH_KATA_GITHUB_ALLOW_NO_RUNNER "$ALLOW_NO_RUNNER"

if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)" || \
    fail "could not infer GitHub repo; set MEMOH_KATA_GITHUB_REPO=owner/repo"
fi

if [ -z "$PR_NUMBER" ]; then
  PR_NUMBER="$(gh pr view --repo "$REPO" --json number --jq .number 2>/dev/null)" || \
    fail "could not infer current PR; pass a PR number or set MEMOH_KATA_GITHUB_PR"
fi

PR_JSON="$(gh pr view "$PR_NUMBER" --repo "$REPO" \
  --json headRefName,headRefOid,url,state,isDraft)"
HEAD_REF="$(jq -r .headRefName <<<"$PR_JSON")"
HEAD_SHA="$(jq -r .headRefOid <<<"$PR_JSON")"
PR_URL="$(jq -r .url <<<"$PR_JSON")"

[ -n "$HEAD_REF" ] && [ "$HEAD_REF" != "null" ] || fail "could not resolve PR head branch"
[ -n "$HEAD_SHA" ] && [ "$HEAD_SHA" != "null" ] || fail "could not resolve PR head sha"

if ! gh workflow view "$WORKFLOW_FILE" --repo "$REPO" >/dev/null 2>&1; then
  fail "workflow $WORKFLOW_FILE is not available on the default branch; GitHub requires workflow_dispatch workflows to exist on the default branch before they can be manually dispatched"
fi

if ! gh workflow view "$WORKFLOW_FILE" --repo "$REPO" --ref "$HEAD_REF" --yaml >/dev/null 2>&1; then
  fail "workflow $WORKFLOW_FILE is not available on PR ref $HEAD_REF"
fi

matching_runner_count=0
if runners_json="$(gh api "repos/$REPO/actions/runners" 2>/dev/null)"; then
  matching_runner_count="$(jq -r '
    [.runners[]
      | ([.labels[].name | ascii_downcase]) as $labels
      | select(
          ($labels | index("self-hosted")) and
          ($labels | index("linux")) and
          ($labels | index("x64")) and
          ($labels | index("kvm")) and
          ($labels | index("kata"))
        )
    ] | length
  ' <<<"$runners_json")"
fi

if [ "$matching_runner_count" = "0" ] && [ "$ALLOW_NO_RUNNER" != "true" ]; then
  fail "no visible runner has labels [$REQUIRED_RUNNER_LABELS]; set MEMOH_KATA_GITHUB_ALLOW_NO_RUNNER=true to dispatch anyway"
fi

run_runner_readiness=false
run_kata_e2e=true
if [ "$READINESS_ONLY" = "true" ]; then
  run_runner_readiness=true
  run_kata_e2e=false
  RUN_COMPOSE_E2E=false
fi

started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

echo "Dispatching Kata GitHub workflow:"
printf '  repo=%s\n' "$REPO"
printf '  pr=%s\n' "$PR_NUMBER"
printf '  url=%s\n' "$PR_URL"
printf '  ref=%s\n' "$HEAD_REF"
printf '  head=%s\n' "$HEAD_SHA"
printf '  workflow=%s\n' "$WORKFLOW_FILE"
printf '  run_runner_readiness=%s\n' "$run_runner_readiness"
printf '  run_kata_e2e=%s\n' "$run_kata_e2e"
printf '  run_compose_e2e=%s\n' "$RUN_COMPOSE_E2E"
printf '  matching_runner_count=%s\n' "$matching_runner_count"

gh workflow run "$WORKFLOW_FILE" \
  --repo "$REPO" \
  --ref "$HEAD_REF" \
  -f "run_runner_readiness=$run_runner_readiness" \
  -f "run_kata_e2e=$run_kata_e2e" \
  -f "run_compose_e2e=$RUN_COMPOSE_E2E"

if [ "$WAIT_FOR_RUN" != "true" ]; then
  echo "Kata workflow dispatched. Waiting disabled by MEMOH_KATA_GITHUB_WAIT=false."
  exit 0
fi

run_id=""
for _ in $(seq 1 30); do
  run_id="$(gh run list \
    --repo "$REPO" \
    --workflow "$WORKFLOW_FILE" \
    --branch "$HEAD_REF" \
    --event workflow_dispatch \
    --json databaseId,headSha,createdAt \
    --jq \
      ".[] | select(.headSha == \"$HEAD_SHA\" and .createdAt >= \"$started_at\") | .databaseId" \
    | head -n 1)"
  if [ -n "$run_id" ]; then
    break
  fi
  sleep 2
done

[ -n "$run_id" ] || fail "workflow was dispatched but no matching workflow_dispatch run was found for $HEAD_SHA"

echo "Watching Kata workflow run: $run_id"
gh run watch "$run_id" --repo "$REPO" --exit-status

if [ "$run_kata_e2e" = "true" ]; then
  scripts/audit-kata-github-verification.sh "$PR_NUMBER"
else
  echo "Readiness-only Kata workflow completed."
fi
