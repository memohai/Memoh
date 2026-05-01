# Workspace backends

Each Memoh bot runs in an isolated workspace. The workspace backend decides where those containers or pods are created and which networking features are available.

Configure it in `config.toml`:

```toml
[container]
backend = "containerd" # containerd, docker, kubernetes, or apple
```

## Choosing a backend

| Backend | Best fit | Notes |
|---------|----------|-------|
| `containerd` | Docker Compose installs, Linux servers, development | Default for the official Docker image. Supports CNI networking, snapshots, CDI devices, and provider sidecars. |
| `docker` | Host/binary deployments with Docker Engine | Uses the host Docker API. Runtime bind-mount paths such as `container.runtime_dir` must exist on the Docker host. |
| `kubernetes` | Cluster deployments | Creates one Pod/PVC per bot workspace. Uses Kubernetes-native networking and storage. |
| `apple` | macOS local testing | Uses socktainer and Apple Containerization. Overlay provider sidecars are not supported. |

The one-click Docker Compose installer uses `containerd`. That is intentional: the server image starts an embedded containerd and mounts the runtime files needed by bot workspaces. Use the other backends for manual deployments where you control the host or cluster runtime paths.

For a ready-to-edit Kubernetes starter deployment, see [Kubernetes deployment](/installation/kubernetes.md).

## containerd

```toml
[container]
backend = "containerd"

[containerd]
socket_path = "/run/containerd/containerd.sock"
namespace = "default"

[container]
backend = "containerd"
default_image = "debian:bookworm-slim"
image_pull_policy = "if_not_present"
snapshotter = "overlayfs"
data_root = "/opt/memoh/data"
runtime_dir = "/opt/memoh/runtime"
cni_bin_dir = "/opt/cni/bin"
cni_conf_dir = "/etc/cni/net.d"
```

Use this backend for the official Docker Compose stack and for hosts where Memoh can talk directly to containerd. It provides the fullest local workspace feature set.

## Docker

```toml
[container]
backend = "docker"

[docker]
# Empty means Docker's standard environment discovery: DOCKER_HOST,
# DOCKER_TLS_VERIFY, DOCKER_CERT_PATH, or the platform default socket.
host = ""

[container]
backend = "docker"
default_image = "debian:bookworm-slim"
runtime_dir = "/opt/memoh/runtime"
data_root = "/opt/memoh/data"
```

The Docker backend talks to Docker Engine through the standard Docker environment. It is meant for a Memoh server running on the host or in an environment where Docker bind-mount source paths refer to real host paths.

Avoid switching a stock Docker Compose install from `containerd` to `docker` unless you also provide host-valid paths for `runtime_dir` and Docker socket access. Otherwise the workspace containers can be created without the bridge runtime files they need.

## Kubernetes

```toml
[container]
backend = "kubernetes"

[kubernetes]
namespace = "memoh"
in_cluster = true
kubeconfig = ""
service_account_name = ""
image_pull_secret = ""
pvc_storage_class = ""
pvc_size = "10Gi"
bridge_port = 9090
```

When `in_cluster = true`, Memoh uses the pod ServiceAccount. For external control, set `in_cluster = false` and provide `kubeconfig`.

Kubernetes workspaces use Pods and PVCs. Make sure the namespace, RBAC, storage class, and image pull access are ready before starting Memoh.

## Apple

```toml
[container]
backend = "apple"

[apple]
socket_path = ""
binary_path = ""
```

This backend is for local macOS testing through socktainer and Apple Containerization. It is experimental and does not support provider-backed network sidecars.

## Networking and overlays

Bot networking has two layers:

- Runtime networking connects the workspace to the base container or pod network.
- Overlay providers such as Tailscale and NetBird attach optional per-bot private networking.

Runtime capabilities differ by backend:

| Backend | Runtime network | Overlay sidecars | Kubernetes-native overlay | CDI devices |
|---------|-----------------|------------------|---------------------------|-------------|
| `containerd` | CNI | Yes | No | Yes |
| `docker` | Join Docker container network | Limited by Docker runtime capabilities | No | No |
| `kubernetes` | Pod network | Provider-dependent | Yes | No |
| `apple` | Basic local runtime | No | No | No |

Overlay provider settings are configured per bot in the web UI, not in the global TOML file. The global backend still matters because it decides which overlay driver can run.
