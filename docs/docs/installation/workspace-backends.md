# Workspace Backends

Each Memoh bot works inside a workspace. A workspace may be an isolated container or, in trusted desktop/local scenarios, a host local directory. The backend decides where that workspace runs and which isolation, networking, snapshot, and display features are available.

## Container backends

Configure container workspaces in `config.toml`:

```toml
[container]
backend = "containerd" # containerd, docker, or apple
```

| Backend | Best fit | Notes |
|---------|----------|-------|
| `containerd` | Docker Compose server deploys, Linux servers, development | Default for the official server image. Supports CNI networking, snapshots, CDI devices, provider sidecars, and the broadest local workspace feature set. |
| `docker` | Host/binary deployments with Docker Engine | Uses the host Docker API. Runtime bind-mount paths such as `container.runtime_dir` must exist on the Docker host. |
| `apple` | macOS local testing | Uses socktainer and Apple Containerization. Overlay provider sidecars are not supported. |

The one-click Docker Compose server deploy uses `containerd`. That is intentional: the server image starts an embedded containerd and mounts the runtime files needed by bot workspaces. Use the other backends for manual deployments where you control the host runtime paths.

## Trusted local workspaces

Desktop and local development can enable trusted local workspaces alongside the configured container backend:

```toml
[local]
enabled = true
```

Trusted local workspaces run on the host with the server process permissions. They are useful for personal desktop workflows and local development, but they are **not container-isolated**. Do not enable local workspaces for untrusted server deployments.

When a bot uses a local workspace, UI affordances that require a container desktop or container display session may be hidden or unavailable.

## containerd

```toml
[container]
backend = "containerd"
default_image = "debian:bookworm-slim"
image_pull_policy = "if_not_present"
snapshotter = "overlayfs"
data_root = "/opt/memoh/data"
runtime_dir = "/opt/memoh/runtime"
cni_bin_dir = "/opt/cni/bin"
cni_conf_dir = "/etc/cni/net.d"

[containerd]
socket_path = "/run/containerd/containerd.sock"
namespace = "default"
```

Use this backend for the official Docker Compose stack and for hosts where Memoh can talk directly to containerd.

## Docker

```toml
[container]
backend = "docker"
default_image = "debian:bookworm-slim"
runtime_dir = "/opt/memoh/runtime"
data_root = "/opt/memoh/data"

[docker]
# Empty means Docker's standard environment discovery: DOCKER_HOST,
# DOCKER_TLS_VERIFY, DOCKER_CERT_PATH, or the platform default socket.
host = ""
```

The Docker backend talks to Docker Engine through the standard Docker environment. It is meant for a Memoh server running on the host or in an environment where Docker bind-mount source paths refer to real host paths.

Avoid switching a stock Docker Compose install from `containerd` to `docker` unless you also provide host-valid paths for `runtime_dir` and Docker socket access. Otherwise the workspace containers can be created without the bridge runtime files they need.

## Apple

```toml
[container]
backend = "apple"

[apple]
socket_path = ""
binary_path = ""
```

This backend is for local macOS testing through socktainer and Apple Containerization. It is experimental and does not support provider-backed network sidecars.

## Display, Browser Use, and Computer Use

Container workspaces can provide a display runtime: Xvnc/RFB for the workspace desktop, a headed Chrome/Chromium browser with CDP, and WebRTC sessions for the Web UI display pane.

Use this path when a site needs a real graphical browser. Headless Playwright remains available as an ordinary workspace command, but Browser Use and Computer Use target the headed workspace desktop.

Local workspaces do not provide the same container desktop isolation. Use [Browser / Computer Use](/getting-started/browser-computer-use) for the tool-level model.

## Networking and overlays

Bot networking has two layers:

- Runtime networking connects the workspace to the base container or pod network.
- Overlay providers such as Tailscale and NetBird attach optional per-bot private networking.

Runtime capabilities differ by backend:

| Backend | Runtime network | Overlay sidecars | CDI devices | Container display |
|---------|-----------------|------------------|-------------|-------------------|
| `containerd` | CNI | Yes | Yes | Yes |
| `docker` | Join Docker container network | Limited by Docker runtime capabilities | No | Yes, when runtime files and image packages are available |
| `apple` | Basic local runtime | No | No | Limited |
| `local` | Host network | No | Host-level only | No container desktop |

Overlay provider settings are configured per bot in the web UI, not in the global TOML file. The global backend still matters because it decides which overlay driver can run.
