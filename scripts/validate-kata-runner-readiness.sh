#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-}"
ENVIRONMENT_FILE=""

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

if [ -z "$EVIDENCE_DIR" ]; then
  echo "usage: scripts/validate-kata-runner-readiness.sh <evidence-dir>" >&2
  exit 1
fi

[ -d "$EVIDENCE_DIR" ] || fail "readiness evidence directory not found: $EVIDENCE_DIR"
ENVIRONMENT_FILE="$EVIDENCE_DIR/environment.txt"
[ -s "$ENVIRONMENT_FILE" ] || fail "missing environment summary: $ENVIRONMENT_FILE"

grep -q '^run_id=' "$ENVIRONMENT_FILE" || fail "environment summary missing run_id"
grep -q '^runner_name=' "$ENVIRONMENT_FILE" || fail "environment summary missing runner_name"
grep -q '^runner_os=Linux$' "$ENVIRONMENT_FILE" || fail "environment summary does not prove this is a Linux runner"
grep -q '^runner_arch=' "$ENVIRONMENT_FILE" || fail "environment summary missing runner_arch"
grep -q '^docker=' "$ENVIRONMENT_FILE" || fail "environment summary missing docker version"
grep -q '^docker_compose=' "$ENVIRONMENT_FILE" || fail "environment summary missing docker compose version"
grep -q '^kvm_present=true$' "$ENVIRONMENT_FILE" || fail "environment summary does not prove /dev/kvm was present"
grep -q '^kata_shim=.' "$ENVIRONMENT_FILE" || fail "environment summary missing kata_shim"

if grep -q '^docker=missing$' "$ENVIRONMENT_FILE"; then
  fail "environment summary reports docker=missing"
fi
if grep -q '^docker_compose=missing$' "$ENVIRONMENT_FILE"; then
  fail "environment summary reports docker_compose=missing"
fi
if grep -Eiq '(access_token|authorization|bearer|password|secret|jwt)' "$ENVIRONMENT_FILE"; then
  fail "environment summary contains sensitive-looking text"
fi

echo "Kata runner readiness evidence validated:"
printf '  directory=%s\n' "$EVIDENCE_DIR"
