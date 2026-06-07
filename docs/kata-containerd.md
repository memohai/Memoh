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

## Static Validation

This can run on macOS, Docker Desktop, or GitHub-hosted Ubuntu. It does not
prove that Kata works, but it checks the shell scripts, evidence validators,
Kata config templates, compose overrides, and the `server-kata` Dockerfile
target:

```bash
mise run test:kata:static
```

The GitHub `Kata Runtime` workflow uses the same script for its static job.
Because GitHub cannot manually dispatch a newly added workflow until that file
exists on the default branch, the already-registered `Docker` workflow also has
a `Kata static validation` job for PR-stage remote validation. Use this task
directly to reproduce the same static checks locally.

## Development E2E

Use this on a dedicated Linux/KVM development host:

```bash
mise run test:kata:runner
mise run test:kata:e2e
```

The task performs the full dev validation:

`test:kata:runner` is a lightweight readiness check for the runner or
development host. It writes `tmp/kata-evidence/environment.txt`, verifies
Docker and Docker Compose are usable, then checks Linux, `/dev/kvm`, the Kata
shim, Kata config, and Kata runtime asset directories before any Memoh stack is
started.

`test:kata:e2e` performs the full dev validation:

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
containerd smoke evidence under `tmp/kata-evidence/` by default, and writes the
same `environment.txt` summary used by the E2E tasks.

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
one `.smoke.json` file for the direct `ctr run --runtime ...` smoke check, plus
`environment.txt` from `scripts/write-kata-evidence-environment.sh`. Set
`MEMOH_KATA_EVIDENCE_DIR`, `MEMOH_VERIFY_EVIDENCE_FILE`, or
`MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE` to choose another location.
When an E2E task fails, it also writes `failure-context.txt` and, if the stack
started, `compose-logs.txt` into the same evidence directory so the uploaded
artifact contains the basic failure context. The compose logs are redacted for
common password, JWT secret, and bearer-token patterns before they are saved.
The evidence validator itself can be regression-tested locally with
`mise run test:kata:evidence`. The running-stack verifier and both E2E tasks
validate their own evidence bundles before reporting success; the GitHub
workflow also validates the full uploaded artifact directory after all selected
E2E tasks finish.

## GitHub Actions Test Environment

The `Kata Runtime` workflow always runs static validation on GitHub-hosted
Ubuntu. Real Kata verification is opt-in because it needs a self-hosted
Linux/KVM runner with Docker, Docker Compose v2, Kata Containers, `curl`, and
`jq` installed.

When the `Kata Runtime` workflow is being introduced in a PR, GitHub will not
accept manual dispatch for it until the workflow file has landed on the default
branch. During that phase, rely on the `Docker` workflow's `Kata static
validation` job for GitHub-hosted Ubuntu coverage, plus the local E2E tasks on
a Linux/KVM host.

Register the runner with these labels:

```text
self-hosted, linux, x64, kvm, kata
```

To check a newly registered runner without starting the Memoh stack, run the
workflow manually with `run_runner_readiness=true` and `run_kata_e2e=false`.
This runs only `scripts/check-kata-runner-ready.sh` and uploads a
`kata-runner-readiness` artifact containing the environment summary. The job
also runs `scripts/validate-kata-runner-readiness.sh` against that artifact so
the uploaded evidence can be rechecked independently.

For full verification, run the workflow manually with `run_kata_e2e=true`. The
E2E job executes `scripts/check-kata-runner-ready.sh` first so runner, Docker,
KVM, and Kata installation failures are reported before the Memoh stack starts.
It then executes `scripts/test-containerd-kata-e2e.sh`; if `run_compose_e2e` is
enabled, it also executes `scripts/test-containerd-kata-compose-e2e.sh`. Both
runs upload their API and smoke evidence JSON files as the `kata-evidence`
artifact. The job clears `tmp/kata-evidence/` before each run on the
self-hosted runner and also uploads `environment.txt` with the runner, Docker,
KVM, and Kata shim summary. Before uploading, it runs
`scripts/validate-kata-evidence-dir.sh` to ensure the artifact has the expected
number of API evidence files, matching `.smoke.json` files, and a Linux/KVM
environment summary.

To audit whether a PR head has actually reached the full Kata verification
bar, run:

```bash
scripts/audit-kata-github-verification.sh <pr-number>
```

The audit exits successfully only when the Kata static check is successful and
the `Linux/KVM E2E` check has succeeded for that PR head. A PR where static
checks are green but `Linux/KVM E2E` is skipped or missing is still unverified.
Use `mise run test:kata:github -- <pr-number>` as the task equivalent.

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
scripts/validate-kata-evidence-run-dir.sh \
  tmp/kata-evidence/kata-compose-manual.json \
  tmp/kata-evidence/kata-compose-manual.smoke.json
```

## Evidence Required To Call Kata Verified

A valid test run must prove all of the following:

- `scripts/check-kata-runner-ready.sh` passes on the Linux/KVM runner and writes
  an environment summary proving Linux and `/dev/kvm`.
- `scripts/validate-kata-runner-readiness.sh` passes against the readiness
  artifact.
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
- `POST /bots` creation SSE emits a container `complete` event with
  `container.runtime_backend = "io.containerd.kata.v2"`.
- `POST /bots/{id}/container` recreate SSE completes with
  `container.runtime_backend = "io.containerd.kata.v2"`.
- CPU and memory resource limits become `status = "applied"` after recreate.
- Storage remains a soft limit for this containerd/Kata path.
- After the verifier deletes the workspace for resource-limit recreate,
  `GET /bots/{id}/container` returns 404 before the workspace is recreated.
- A file written under `/data` survives delete/recreate when
  `preserve_data=true` and `restore_data=true` are used.

The `test:kata:e2e` and `test:kata:compose:e2e` tasks perform these checks.
Their evidence JSON records the target runtime, container IDs, direct
`ctr containers info` runtime names, delete-before-recreate proof, final
resource-limit state, create/recreate SSE runtime reporting, and data restore
result without storing the admin password or access token.
The running-stack verifier and E2E tasks also run
`scripts/validate-kata-evidence.sh` against the saved API evidence and
`scripts/validate-containerd-smoke-evidence.sh` against the saved smoke
evidence, then validate the API evidence, paired smoke evidence, and
environment summary together with `scripts/validate-kata-evidence-run-dir.sh`
before reporting success.

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
