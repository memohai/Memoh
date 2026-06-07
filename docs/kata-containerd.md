# Containerd Kata Workspace Runtime

Memoh can run bot workspaces through containerd's Kata runtime on Linux/KVM
hosts. This keeps the Memoh workspace API and lifecycle model the same while
asking containerd to create each workspace with `io.containerd.kata.v2`.

This path is Linux-only. A macOS or Docker Desktop host can validate compose
syntax and runc regressions, but it cannot prove the Kata runtime works.

## What This Enables

- `[container].backend = "containerd"` remains the workspace backend.
- `[containerd].runtime_type = "io.containerd.kata.v2"` selects Kata for bot
  workspace containers.
- The `server-kata` image target uses a Debian/glibc runtime because host Kata
  shims are commonly glibc-linked.
- The compose overrides mount `/dev/kvm`, the host Kata shim, Kata config, and
  Kata runtime assets into the Memoh server container.

Kata is still driven through containerd snapshots in this implementation. CPU
and memory limits are hard limits. Storage is saved and reported as a soft
limit until a VM disk quota or block-device quota implementation is added.

## Host Requirements

- Linux host with KVM available at `/dev/kvm`.
- Nested virtualization enabled if the host itself is a VM.
- Docker with Docker Compose v2.
- Kata Containers installed on the host.
- `curl` and `jq` on the host for the API verifier.

Default host paths:

```bash
MEMOH_KATA_SHIM_PATH=/opt/kata/bin/containerd-shim-kata-v2
MEMOH_KATA_CONFIG_DIR=/etc/kata-containers
MEMOH_KATA_SHARE_DIR=/usr/share/kata-containers
MEMOH_KATA_OPT_DIR=/opt/kata
```

If your Kata install uses different paths, export those variables before
running the dev or production compose commands.

## Development E2E

Use this on a dedicated Linux/KVM development host:

```bash
mise run test:kata:e2e
```

The task performs the full dev validation:

1. Checks the host has Linux, `/dev/kvm`, Kata shim/config/assets.
2. Builds the Kata dev server image.
3. Checks the same shim/config/assets are visible inside the server image.
4. Starts the dev compose server with `devenv/app.kata.dev.toml`.
5. Runs a direct `ctr run --runtime io.containerd.kata.v2` smoke test.
6. Creates a temporary bot and validates the Memoh API reports the Kata runtime.
7. Applies CPU, memory, and storage resource values.
8. Recreates the workspace and verifies data-preserving restore.

For an already-running dev stack, use:

```bash
mise run dev:kata
mise run test:kata
```

`test:kata` uses the running dev stack instead of rebuilding it, but it still
generates and validates both the API verifier evidence and the direct
containerd smoke evidence under `tmp/kata-evidence/` by default.

## Production Compose E2E

Use this on a dedicated Linux/KVM host because the root compose file uses fixed
container names such as `memoh-server` and `memoh-postgres`:

```bash
mise run test:kata:compose:e2e
```

The script refuses to run if the root compose containers already exist, then
builds and starts:

```bash
docker compose -f docker-compose.yml -f docker-compose.kata.yml up --build
```

It verifies the same direct containerd runtime path and Memoh API workflow as
the dev E2E. By default it tears the stack down when it exits. Set
`MEMOH_KATA_COMPOSE_E2E_KEEP=true` to keep the stack for inspection.

Both E2E tasks write machine-readable verification evidence under
`tmp/kata-evidence/` by default: one JSON file for the Memoh API verifier and
one `.smoke.json` file for the direct `ctr run --runtime ...` smoke check. Set
`MEMOH_KATA_EVIDENCE_DIR`, `MEMOH_VERIFY_EVIDENCE_FILE`, or
`MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE` to choose another location.
The evidence validator itself can be regression-tested locally with
`mise run test:kata:evidence`.

## GitHub Actions Test Environment

The `Kata Runtime` workflow always runs static validation on GitHub-hosted
Ubuntu. Real Kata verification is opt-in because it needs a self-hosted
Linux/KVM runner with Docker, Docker Compose v2, Kata Containers, `curl`, and
`jq` installed.

Register the runner with these labels:

```text
self-hosted, linux, x64, kvm, kata
```

Then run the workflow manually and enable `run_kata_e2e`. The job executes
`scripts/test-containerd-kata-e2e.sh`; if `run_compose_e2e` is enabled, it also
executes `scripts/test-containerd-kata-compose-e2e.sh`. Both runs upload their
API and smoke evidence JSON files as the `kata-evidence` artifact. The job
clears `tmp/kata-evidence/` before each run on the self-hosted runner and also
uploads `environment.txt` with the runner, Docker, KVM, and Kata shim summary.
Before uploading, it runs `scripts/validate-kata-evidence-dir.sh` to ensure the
artifact has the expected number of API evidence files, matching `.smoke.json`
files, and a Linux/KVM environment summary.

For manual production deployment, copy and edit the Kata config first:

```bash
cp conf/app.kata.docker.toml config.kata.toml
# Change admin password, JWT secret, and database password.
MEMOH_CONFIG=./config.kata.toml \
  docker compose -f docker-compose.yml -f docker-compose.kata.yml up --build -d
```

Then verify the running stack:

```bash
MEMOH_CONTAINERD_SMOKE_CTR_COMMAND='docker compose -f docker-compose.yml -f docker-compose.kata.yml exec -T server ctr' \
MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE=tmp/kata-evidence/kata-compose-manual.smoke.json \
  scripts/smoke-containerd-runtime.sh
MEMOH_VERIFY_BASE_URL=http://127.0.0.1:8080 \
MEMOH_VERIFY_CONTAINERD_RUNTIME=true \
MEMOH_VERIFY_CTR_COMMAND='docker compose -f docker-compose.yml -f docker-compose.kata.yml exec -T server ctr' \
MEMOH_VERIFY_EVIDENCE_FILE=tmp/kata-evidence/kata-compose-manual.json \
  scripts/verify-containerd-kata.sh
scripts/validate-containerd-smoke-evidence.sh tmp/kata-evidence/kata-compose-manual.smoke.json
scripts/validate-kata-evidence.sh tmp/kata-evidence/kata-compose-manual.json
```

## Evidence Required To Call Kata Verified

A valid test run must prove all of the following:

- `scripts/check-kata-dev-env.sh` passes on the Linux/KVM host.
- `scripts/check-kata-dev-env.sh` passes with `MEMOH_KATA_CHECK_CONTAINER=1`
  after the target server image is built.
- `scripts/smoke-containerd-runtime.sh` starts Alpine with
  `--runtime io.containerd.kata.v2`.
- `GET /ping` returns `container_backend = "containerd"`.
- `GET /bots/{id}/container` returns
  `runtime_backend = "io.containerd.kata.v2"`.
- `ctr containers info <workspace-id>` returns
  `Runtime.Name = "io.containerd.kata.v2"`.
- `GET /bots/{id}/container/metrics` reports the same runtime backend.
- CPU and memory resource limits become `status = "applied"` after recreate.
- Storage remains a soft limit for this containerd/Kata path.
- After the verifier deletes the workspace for resource-limit recreate,
  `GET /bots/{id}/container` returns 404 before the workspace is recreated.
- A file written under `/data` survives delete/recreate when
  `preserve_data=true` and `restore_data=true` are used.

The `test:kata:e2e` and `test:kata:compose:e2e` tasks perform these checks.
Their evidence JSON records the target runtime, container IDs, direct
`ctr containers info` runtime names, delete-before-recreate proof, final
resource-limit state, and data restore result without storing the admin
password or access token.
The E2E tasks also run `scripts/validate-kata-evidence.sh` against the saved
API evidence and `scripts/validate-containerd-smoke-evidence.sh` against the
saved smoke evidence before reporting success.

## Troubleshooting

- `Kata validation requires a Linux host with KVM`: run the E2E on a Linux host
  with KVM. Docker Desktop is not enough.
- `/dev/kvm is missing`: enable KVM or nested virtualization, then make sure
  Docker can pass `/dev/kvm` through.
- `Kata shim not found`: set `MEMOH_KATA_SHIM_PATH` to the host
  `containerd-shim-kata-v2` path.
- Missing paths from `configuration.toml`: mount the referenced Kata assets or
  set `MEMOH_KATA_SHARE_DIR` / `MEMOH_KATA_OPT_DIR` to the correct host paths.
- Runtime mismatch in `ctr containers info`: confirm the server config uses
  `runtime_type = "io.containerd.kata.v2"` and that the Kata compose override is
  included.
