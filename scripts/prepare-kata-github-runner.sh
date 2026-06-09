#!/usr/bin/env bash
set -euo pipefail

REPO="${MEMOH_KATA_GITHUB_REPO:-}"
RUNNER_NAME="${MEMOH_KATA_RUNNER_NAME:-memoh-kata-$(hostname 2>/dev/null || echo runner)}"
RUNNER_DIR="${MEMOH_KATA_RUNNER_DIR:-$HOME/actions-runner-memoh-kata}"
RUNNER_WORK_DIR="${MEMOH_KATA_RUNNER_WORK_DIR:-_work}"
OUTPUT_SCRIPT="${MEMOH_KATA_RUNNER_SCRIPT:-tmp/kata-runner-register.sh}"
EVIDENCE_DIR="${MEMOH_KATA_RUNNER_EVIDENCE_DIR:-tmp/kata-runner-readiness}"
EXECUTE="${MEMOH_KATA_RUNNER_EXECUTE:-false}"
SKIP_HOST_CHECK="${MEMOH_KATA_RUNNER_SKIP_HOST_CHECK:-false}"
LABELS="kvm,kata"
REQUIRED_LABELS="self-hosted, linux, x64, kvm, kata"

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

validate_bool MEMOH_KATA_RUNNER_EXECUTE "$EXECUTE"
validate_bool MEMOH_KATA_RUNNER_SKIP_HOST_CHECK "$SKIP_HOST_CHECK"

require_cmd gh
require_cmd jq

if [ -z "$REPO" ]; then
  REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null)" || \
    fail "could not infer GitHub repo; set MEMOH_KATA_GITHUB_REPO=owner/repo"
fi

if [ "$SKIP_HOST_CHECK" != "true" ]; then
  scripts/check-kata-runner-ready.sh "$EVIDENCE_DIR"
  scripts/validate-kata-runner-readiness.sh "$EVIDENCE_DIR"
fi

mkdir -p "$(dirname "$OUTPUT_SCRIPT")"

cat >"$OUTPUT_SCRIPT" <<EOF
#!/usr/bin/env bash
set -euo pipefail

REPO="$REPO"
RUNNER_NAME="$RUNNER_NAME"
RUNNER_DIR="$RUNNER_DIR"
RUNNER_WORK_DIR="$RUNNER_WORK_DIR"
LABELS="$LABELS"
KATA_SHIM_PATH="\${MEMOH_KATA_SHIM_PATH:-/opt/kata/bin/containerd-shim-kata-v2}"
KATA_CONFIG_DIR="\${MEMOH_KATA_CONFIG_DIR:-/etc/kata-containers}"
KATA_SHARE_DIR="\${MEMOH_KATA_SHARE_DIR:-/usr/share/kata-containers}"
KATA_OPT_DIR="\${MEMOH_KATA_OPT_DIR:-/opt/kata}"

fail() {
  echo "ERROR: \$*" >&2
  exit 1
}

require_cmd() {
  if ! command -v "\$1" >/dev/null 2>&1; then
    fail "missing required command: \$1"
  fi
}

check_runner_capabilities() {
  [ "\$(uname -s)" = "Linux" ] || fail "runner host must be Linux"
  case "\$(uname -m)" in
    x86_64|amd64)
      ;;
    *)
      fail "runner host must be x86_64/amd64 because this helper installs the linux-x64 actions runner"
      ;;
  esac
  [ -e /dev/kvm ] || fail "/dev/kvm is missing; do not register this host with the kvm label"
  require_cmd docker
  docker info >/dev/null
  docker compose version >/dev/null
  [ -x "\$KATA_SHIM_PATH" ] || fail "Kata shim is missing or not executable at \$KATA_SHIM_PATH"
  [ -f "\$KATA_CONFIG_DIR/configuration.toml" ] || fail "Kata configuration missing at \$KATA_CONFIG_DIR/configuration.toml"
  [ -d "\$KATA_SHARE_DIR" ] || fail "Kata share directory missing at \$KATA_SHARE_DIR"
  [ -d "\$KATA_OPT_DIR" ] || fail "Kata opt directory missing at \$KATA_OPT_DIR"
}

if [ "\$(id -u)" = "0" ]; then
  fail "do not run the GitHub Actions runner as root; run this script as the dedicated runner user"
fi

require_cmd curl
require_cmd gh
require_cmd jq
require_cmd tar
check_runner_capabilities

repo_url="https://github.com/\$REPO"
mkdir -p "\$RUNNER_DIR"
cd "\$RUNNER_DIR"

if [ ! -x ./config.sh ]; then
  asset_url="\$(gh api repos/actions/runner/releases/latest --jq '.assets[] | select(.name | test("^actions-runner-linux-x64-.*\\\\.tar\\\\.gz$")) | .browser_download_url' | head -n 1)"
  [ -n "\$asset_url" ] || fail "could not find latest linux-x64 actions runner asset"
  curl -fsSL "\$asset_url" -o actions-runner-linux-x64.tar.gz
  tar xzf actions-runner-linux-x64.tar.gz
fi

token="\$(gh api -X POST "repos/\$REPO/actions/runners/registration-token" --jq .token)"
[ -n "\$token" ] || fail "could not obtain GitHub Actions runner registration token"

./config.sh \\
  --url "\$repo_url" \\
  --token "\$token" \\
  --name "\$RUNNER_NAME" \\
  --labels "\$LABELS" \\
  --work "\$RUNNER_WORK_DIR" \\
  --unattended \\
  --replace

echo
echo "Runner configured. Install and start the service with:"
echo "  cd \$RUNNER_DIR"
echo "  sudo ./svc.sh install"
echo "  sudo ./svc.sh start"
EOF

chmod +x "$OUTPUT_SCRIPT"

echo "Prepared Kata GitHub runner registration script:"
printf '  repo=%s\n' "$REPO"
printf '  runner_name=%s\n' "$RUNNER_NAME"
printf '  runner_dir=%s\n' "$RUNNER_DIR"
printf '  required_labels=%s\n' "$REQUIRED_LABELS"
printf '  generated_script=%s\n' "$OUTPUT_SCRIPT"
if [ "$SKIP_HOST_CHECK" != "true" ]; then
  printf '  readiness_evidence=%s\n' "$EVIDENCE_DIR"
fi
echo
echo "Run the generated script on the Linux/KVM host as the dedicated runner user:"
printf '  %s\n' "$OUTPUT_SCRIPT"
echo
echo "After the service starts, verify registration with:"
echo "  gh api repos/$REPO/actions/runners --jq '.runners[] | select((.labels | map(.name | ascii_downcase) | index(\"kata\")) or (.labels | map(.name | ascii_downcase) | index(\"kvm\"))) | {name,status,busy,labels:[.labels[].name]}'"
echo
echo "Then dispatch the Kata E2E workflow:"
echo "  scripts/run-kata-github-e2e.sh <pr-number>"

if [ "$EXECUTE" = "true" ]; then
  "$OUTPUT_SCRIPT"
fi
