#!/usr/bin/env bash
set -euo pipefail

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

require_cmd bash
require_cmd docker
require_cmd grep
require_cmd awk
require_cmd jq

echo "Checking Kata shell scripts..."
bash -n \
  scripts/audit-kata-github-verification.sh \
  scripts/check-kata-dev-env.sh \
  scripts/check-kata-runner-ready.sh \
  scripts/diagnose-kata-dev-stack.sh \
  scripts/prepare-kata-github-runner.sh \
  scripts/run-kata-github-e2e.sh \
  scripts/smoke-containerd-runtime.sh \
  scripts/test-containerd-kata-compose-e2e.sh \
  scripts/test-containerd-kata-e2e.sh \
  scripts/test-containerd-kata-running.sh \
  scripts/test-kata-evidence-validator.sh \
  scripts/validate-containerd-smoke-evidence.sh \
  scripts/validate-kata-evidence-dir.sh \
  scripts/validate-kata-evidence-run-dir.sh \
  scripts/validate-kata-evidence.sh \
  scripts/validate-kata-runner-readiness.sh \
  scripts/verify-containerd-kata.sh \
  scripts/write-kata-compose-failure-context.sh \
  scripts/write-kata-evidence-environment.sh

echo "Validating Kata evidence checks..."
scripts/test-kata-evidence-validator.sh

echo "Validating Kata workflow wiring..."
grep -F 'name: Kata Runtime' .github/workflows/kata-runtime.yml
grep -F 'run_runner_readiness:' .github/workflows/kata-runtime.yml
grep -F 'run_kata_e2e:' .github/workflows/kata-runtime.yml
grep -F 'scripts/audit-kata-github-verification.sh' .github/workflows/kata-runtime.yml
grep -F 'scripts/prepare-kata-github-runner.sh' .github/workflows/kata-runtime.yml
grep -F 'scripts/run-kata-github-e2e.sh' .github/workflows/kata-runtime.yml
grep -F 'runs-on: [self-hosted, linux, x64, kvm, kata]' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/test-kata-static.sh' .github/workflows/kata-runtime.yml
grep -F 'name: Linux/KVM runner readiness' .github/workflows/kata-runtime.yml
grep -F 'name: kata-runner-readiness' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/validate-kata-runner-readiness.sh tmp/kata-runner-readiness' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/check-kata-runner-ready.sh tmp/kata-evidence' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/validate-kata-runner-readiness.sh tmp/kata-evidence' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/test-containerd-kata-e2e.sh' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/test-containerd-kata-compose-e2e.sh' .github/workflows/kata-runtime.yml
grep -F 'scripts/validate-kata-evidence-dir.sh tmp/kata-evidence' .github/workflows/kata-runtime.yml
grep -F 'uses: actions/upload-artifact@v4' .github/workflows/kata-runtime.yml
grep -F 'kata-static:' .github/workflows/docker.yml
grep -F 'needs.detect-changes.outputs.kata' .github/workflows/docker.yml
grep -F "'scripts/audit-kata-github-verification.sh'" .github/workflows/docker.yml
grep -F "'scripts/check-kata-runner-ready.sh'" .github/workflows/docker.yml
grep -F "'scripts/diagnose-kata-dev-stack.sh'" .github/workflows/docker.yml
grep -F "'devenv/docker-compose.yml'" .github/workflows/docker.yml
grep -F "'scripts/prepare-kata-github-runner.sh'" .github/workflows/docker.yml
grep -F "'scripts/run-kata-github-e2e.sh'" .github/workflows/docker.yml
grep -F "'scripts/validate-kata-runner-readiness.sh'" .github/workflows/docker.yml
grep -F '"devenv/docker-compose.yml"' .github/workflows/kata-runtime.yml
grep -F '"scripts/diagnose-kata-dev-stack.sh"' .github/workflows/kata-runtime.yml
grep -F 'run: scripts/test-kata-static.sh' .github/workflows/docker.yml
grep -F 'scripts/audit-kata-github-verification.sh' docs/kata-containerd.md
grep -F 'scripts/prepare-kata-github-runner.sh' docs/kata-containerd.md
grep -F 'scripts/run-kata-github-e2e.sh' docs/kata-containerd.md
grep -F '[tasks."test:kata:github"]' mise.toml
grep -F '[tasks."test:kata:github:e2e"]' mise.toml
grep -F '[tasks."test:kata:github:runner"]' mise.toml
grep -F '[tasks."dev:kata:status"]' mise.toml
grep -F 'check_runner_capabilities()' scripts/prepare-kata-github-runner.sh
grep -F 'runner host must be x86_64/amd64' scripts/prepare-kata-github-runner.sh
grep -F 'do not register this host with the kvm label' scripts/prepare-kata-github-runner.sh
grep -F 'Kata uses container_backend=containerd' scripts/diagnose-kata-dev-stack.sh
grep -F 'docker_args=(' scripts/check-kata-dev-env.sh
grep -F '    docker' scripts/check-kata-dev-env.sh
grep -F '    run' scripts/check-kata-dev-env.sh
grep -F '/var/lib/kubelet' scripts/check-kata-dev-env.sh

if awk '
  /^  kata-static:/ { in_job = 1; next }
  in_job && /^  [A-Za-z0-9_-]+:/ { in_job = 0 }
  in_job && /docker\/setup-buildx-action@v3/ { found = 1 }
  END { exit found ? 0 : 1 }
' .github/workflows/docker.yml; then
  echo "ERROR: Docker workflow kata-static job should not depend on docker/setup-buildx-action." >&2
  exit 1
fi

if awk '
  /^  static:/ { in_job = 1; next }
  in_job && /^  [A-Za-z0-9_-]+:/ { in_job = 0 }
  in_job && /docker\/setup-buildx-action@v3/ { found = 1 }
  END { exit found ? 0 : 1 }
' .github/workflows/kata-runtime.yml; then
  echo "ERROR: Kata Runtime static job should not depend on docker/setup-buildx-action." >&2
  exit 1
fi

echo "Validating containerd runtime wiring..."
if ! awk '
  /func \(s \*DefaultService\) CreateContainer\(/,/^}/ {
    if (index($0, "containerd.WithRuntime(s.runtimeTypeOrDefault(), nil)") > 0) found = 1
  }
  END { exit found ? 0 : 1 }
' internal/container/containerd/service.go; then
  echo "ERROR: CreateContainer must pass configured runtime_type to containerd.WithRuntime" >&2
  exit 1
fi

if ! awk '
  /func \(s \*DefaultService\) RestoreContainer\(/,/^}/ {
    if (index($0, "containerd.WithRuntime(s.runtimeTypeOrDefault(), nil)") > 0) found = 1
  }
  END { exit found ? 0 : 1 }
' internal/container/containerd/service.go; then
  echo "ERROR: RestoreContainer must pass configured runtime_type to containerd.WithRuntime" >&2
  exit 1
fi
grep -F 'func TestNewDefaultServiceUsesConfiguredRuntimeType' internal/container/containerd/service_test.go
grep -F 'RuntimeType: "io.containerd.kata.v2"' internal/container/containerd/service_test.go

echo "Validating Kata bridge routing..."
grep -F 'func (m *Manager) usesKataRuntime() bool' internal/workspace/manager.go
grep -F 'BRIDGE_TCP_ADDR' internal/workspace/manager.go
grep -F 'passthrough:///%s:%d' internal/workspace/manager.go
grep -F 'grpc.WithNoProxy()' internal/workspace/bridge/client.go
grep -F 'Kata workspaces use the bridge TCP listener' docs/kata-containerd.md

echo "Validating Kata config templates..."
grep -F 'backend = "containerd"' devenv/app.kata.dev.toml
grep -F 'runtime_type = "io.containerd.kata.v2"' devenv/app.kata.dev.toml
grep -F 'backend = "containerd"' conf/app.kata.docker.toml
grep -F 'runtime_type = "io.containerd.kata.v2"' conf/app.kata.docker.toml
grep -F 'x-kata-build-proxy:' devenv/docker-compose.kata.yml
grep -F 'args: *kata-build-proxy' devenv/docker-compose.kata.yml
grep -F 'x-kata-build-proxy:' docker-compose.kata.yml
grep -F 'args: *kata-build-proxy' docker-compose.kata.yml
grep -F 'MEMOH_KATA_BUILD_HTTP_PROXY' docs/kata-containerd.md
grep -F 'MEMOH_KATA_SYSLOG_SOCKET' docs/kata-containerd.md
grep -F 'cgroup: host' docs/kata-containerd.md
grep -F 'shm_size: 1gb' docs/kata-containerd.md
grep -F 'cgroup: host' devenv/docker-compose.kata.yml
grep -F 'shm_size: 1gb' devenv/docker-compose.kata.yml
grep -F 'cgroup: host' docker-compose.kata.yml
grep -F 'shm_size: 1gb' docker-compose.kata.yml
grep -F 'target: /dev/log' devenv/docker-compose.kata.yml
grep -F 'target: /dev/log' docker-compose.kata.yml
grep -F 'host.docker.internal:host-gateway' devenv/docker-compose.yml
grep -F 'HTTP_PROXY: "${HTTP_PROXY:-${http_proxy:-}}"' devenv/docker-compose.yml

dev_compose="$(mktemp "${TMPDIR:-/tmp}/memoh-kata-dev-compose.XXXXXX.yml")"
prod_compose="$(mktemp "${TMPDIR:-/tmp}/memoh-kata-compose.XXXXXX.yml")"
cleanup() {
  rm -f "$dev_compose" "$prod_compose"
}
trap cleanup EXIT

echo "Rendering Kata compose configs..."
docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml config >"$dev_compose"
docker compose -f docker-compose.yml -f docker-compose.kata.yml config >"$prod_compose"

grep -F 'target: server-kata' "$prod_compose"
grep -F 'image: memohai/server:kata' "$prod_compose"
grep -F 'source: /dev/kvm' "$prod_compose"
grep -F 'cgroup: host' "$prod_compose"
grep -F 'shm_size:' "$prod_compose"
grep -F 'target: /dev/kvm' "$prod_compose"
grep -F 'target: /usr/local/bin/containerd-shim-kata-v2' "$prod_compose"
grep -F 'target: /etc/kata-containers' "$prod_compose"
grep -F 'target: /usr/share/kata-containers' "$prod_compose"
grep -F 'target: /opt/kata' "$prod_compose"
grep -F 'target: /dev/log' "$prod_compose"

grep -F 'image: memoh-dev-server-kata' "$dev_compose"
grep -F 'source: /dev/kvm' "$dev_compose"
grep -F 'cgroup: host' "$dev_compose"
grep -F 'shm_size:' "$dev_compose"
grep -F 'target: /dev/kvm' "$dev_compose"
grep -F 'target: /usr/local/bin/containerd-shim-kata-v2' "$dev_compose"
grep -F 'target: /etc/kata-containers' "$dev_compose"
grep -F 'target: /usr/share/kata-containers' "$dev_compose"
grep -F 'target: /opt/kata' "$dev_compose"
grep -F 'target: /dev/log' "$dev_compose"

if [ "$(grep -cF 'create_host_path: false' docker-compose.kata.yml)" -lt 5 ]; then
  echo "ERROR: docker-compose.kata.yml must disable host path creation for Kata host mounts" >&2
  exit 1
fi

if [ "$(grep -cF 'create_host_path: false' devenv/docker-compose.kata.yml)" -lt 5 ]; then
  echo "ERROR: devenv/docker-compose.kata.yml must disable host path creation for Kata host mounts" >&2
  exit 1
fi

echo "Checking server-kata Dockerfile target..."
docker build --check --target server-kata -f docker/Dockerfile.server .

echo "Kata static validation passed."
