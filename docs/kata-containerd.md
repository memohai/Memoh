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

Kata workspaces use the bridge TCP listener (`BRIDGE_TCP_ADDR=:9090`) instead
of the Unix socket bridge. The Unix socket can appear on the host through the
Kata shared filesystem, but it is not a usable connection boundary across the
guest VM. Memoh routes Kata bridge traffic to the workspace CNI IP and disables
HTTP proxy use for bridge gRPC dials so proxy settings on the server container
do not intercept private workspace addresses.

For shared deployments, enable `[bridge_tls].mode = "strict"`. The default
`disabled` mode keeps local installs simple, but it does not protect a shared
Kata/CNI network from workspace-to-workspace bridge traffic. In a multi-team
Kata deployment, strict bridge mTLS is the required isolation layer unless your
networking stack independently blocks workspace-to-workspace TCP access.

## Bridge mTLS Material

Generate strict bridge mTLS material with the repository tool instead of
hand-writing OpenSSL commands:

```bash
INSTANCE_ID="$(uuidgen | tr '[:upper:]' '[:lower:]')"
scripts/gen-bridge-mtls.sh \
  -instance-id "$INSTANCE_ID" \
  -out /opt/memoh/bridge-mtls
```

The `mise` equivalent is:

```bash
mise run bridge:mtls:gen -- \
  -instance-id "$INSTANCE_ID" \
  -out /opt/memoh/bridge-mtls
```

The generator creates two independent CAs, signs one `ClientAuth` certificate
for the Memoh server, signs one `ServerAuth` certificate for the workspace
bridge, writes only the required leaf keys and CA bundles, and discards both CA
private keys. It prints a config snippet like this:

```toml
instance_id = "11111111-1111-1111-1111-111111111111"

[bridge_tls]
mode = "strict"
server_dir = "/opt/memoh/bridge-mtls/server"
bridge_dir = "/opt/memoh/bridge-mtls/bridge"
server_name = ""
```

`server_dir` is read only by the Memoh server and contains:

```text
server-client.crt
server-client.key
bridge-server-ca.crt
```

`bridge_dir` is mounted read-only into workspace containers and must contain
only:

```text
bridge-server.crt
bridge-server.key
server-client-ca.crt
```

Do not reuse `server_dir` for `bridge_dir`, and do not copy
`server-client.crt` or `server-client.key` into `bridge_dir`. Strict mode
rejects a shared directory, symlinked shared directory, missing material, or
unexpected files in `bridge_dir` before mounting anything into a workspace.

If you enable strict bridge mTLS on an existing deployment, recreate or restart
all existing workspace containers. Old containers do not have the TLS material
mount or `BRIDGE_TLS_*` environment variables and will fail closed once the
server requires mTLS.

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
MEMOH_KATA_SYSLOG_SOCKET=/run/systemd/journal/dev-log
```

If your Kata install uses different paths, export those variables before
running the dev or production compose commands. The syslog socket is mounted
into the server container as `/dev/log` so the Kata shim can initialize logging.
On non-systemd hosts, set `MEMOH_KATA_SYSLOG_SOCKET` to the host's syslog
socket path.

The Dockerized Memoh server runs a nested containerd for workspace containers.
The Kata compose overrides therefore set `cgroup: host` and `shm_size: 1gb` on
the server service. The host cgroup namespace lets the nested Kata runtime
create sandbox cgroup controllers, and the larger shared-memory segment avoids
QEMU/KVM boot failures caused by Docker's small default `/dev/shm`.

If the Linux/KVM host needs a proxy during Docker image builds, set the build
proxy variables before running the Kata compose tasks:

```bash
export MEMOH_KATA_BUILD_HTTP_PROXY=http://172.17.0.1:7890
export MEMOH_KATA_BUILD_HTTPS_PROXY=http://172.17.0.1:7890
export MEMOH_KATA_BUILD_NO_PROXY=127.0.0.1,localhost
```

Use a host address that is reachable from Docker build containers. On Linux,
`localhost` inside a Dockerfile `RUN` step is the build container, not the
host.

## Development Stack

Use this on a dedicated Linux/KVM development host:

```bash
mise run kata:runner
mise run dev:kata
mise run dev:kata:status
```

`kata:runner` is a lightweight readiness check for the runner or
development host. It writes `tmp/kata-evidence/environment.txt`, verifies
Docker and Docker Compose are usable, then checks Linux, `/dev/kvm`, the Kata
shim, Kata config, and Kata runtime asset directories before any Memoh stack is
started.

`dev:kata:status` is a lightweight diagnostic for the current dev server. Use
it when `http://127.0.0.1:18082` is already open and you need to check whether
the backing server is the Kata dev stack or the normal dev stack. It inspects
`/ping`, the `memoh-dev-server` container `CONFIG_PATH`, `/dev/kvm`, and the
Kata shim/config mounts. `container_backend = "containerd"` is expected for
Kata; the value that proves the workspace runtime is Kata is
`runtime_backend = "io.containerd.kata.v2"` on a bot workspace, or
`Runtime.Name = "io.containerd.kata.v2"` from `ctr containers info`.

## Production Compose

Use this on a dedicated Linux/KVM host because the root compose file uses fixed
container names such as `memoh-server` and `memoh-postgres`:

```bash
docker compose -f docker-compose.yml -f docker-compose.kata.yml up --build
```

## GitHub Actions Runner

Register the runner with these labels:

```text
self-hosted, linux, x64, kvm, kata
```

On the Linux/KVM host, this command can run the readiness preflight and generate
a runner registration script that adds the required `kvm,kata` labels without
writing the short-lived GitHub registration token into the generated file:

```bash
scripts/prepare-kata-github-runner.sh
```

Set `MEMOH_KATA_RUNNER_NAME`, `MEMOH_KATA_RUNNER_DIR`, or
`MEMOH_KATA_RUNNER_SCRIPT` to override the generated runner name, install
directory, or output script. The `mise` equivalent is
`mise run kata:github:runner`. The generated registration script rechecks
Linux, x86_64/amd64, `/dev/kvm`, Docker Compose, and the Kata shim/config paths
before registering the runner, so a copied script cannot silently register the
wrong host with the `kvm,kata` labels.

To check a newly registered runner without starting the Memoh stack, run the
workflow manually with `run_runner_readiness=true`.
This runs only `scripts/check-kata-runner-ready.sh` and uploads a
`kata-runner-readiness` artifact containing the environment summary.

Once a runner with the required labels is registered, this command can dispatch
the readiness workflow from the PR branch, wait for it to finish, and audit the PR
checks:

```bash
scripts/run-kata-github-e2e.sh <pr-number>
```

The `mise` equivalent is
`mise run kata:github:readiness -- <pr-number>`. Because GitHub requires
`workflow_dispatch` workflows to be registered before they can be manually run,
this command checks that the workflow is available before dispatching.

To audit whether a PR head has completed runner readiness verification, run:

```bash
scripts/audit-kata-github-verification.sh <pr-number>
```

Use `mise run kata:github -- <pr-number>` as the task equivalent.

For manual production deployment, copy and edit the Kata config first:

```bash
cp conf/app.kata.docker.toml config.kata.toml
# Change admin password, JWT secret, and database password.
MEMOH_CONFIG=./config.kata.toml \
  docker compose -f docker-compose.yml -f docker-compose.kata.yml up --build -d
```

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
