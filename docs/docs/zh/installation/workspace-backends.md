# Workspace backend

每个 Memoh 机器人都有自己的 workspace。`workspace backend` 决定这些容器或 Pod 由谁创建，也决定可用的网络能力。

在 `config.toml` 里配置：

```toml
[container]
backend = "containerd" # containerd、docker、kubernetes 或 apple
```

## 怎么选

| Backend | 适合场景 | 说明 |
|---------|----------|------|
| `containerd` | Docker Compose 安装、Linux 服务器、开发环境 | 官方 Docker 镜像默认用这个。支持 CNI 网络、快照、CDI 设备和 provider sidecar。 |
| `docker` | Memoh 直接跑在宿主机，宿主机有 Docker Engine | 走宿主机 Docker API。`container.runtime_dir` 这类 bind mount 源路径必须在 Docker 宿主机上真实存在。 |
| `kubernetes` | 集群部署 | 每个机器人一个 Pod/PVC，使用 Kubernetes 原生网络和存储。 |
| `apple` | macOS 本地测试 | 通过 socktainer 和 Apple Containerization 运行。暂不支持 overlay provider sidecar。 |

一键 Docker Compose 安装固定使用 `containerd`。这是有意的：server 镜像会启动内置 containerd，并挂好机器人 workspace 需要的 runtime 文件。`docker`、`kubernetes`、`apple` 更适合手动部署，你需要自己保证宿主机或集群路径可用。

可直接改造的 Kubernetes 起步方案见 [Kubernetes 部署](/zh/installation/kubernetes.md)。

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

官方 Docker Compose 栈和直接连接 containerd 的 Linux 主机都用它。它是本地 workspace 功能最完整的后端。

## Docker

```toml
[container]
backend = "docker"

[docker]
# 留空时使用 Docker 标准环境发现：DOCKER_HOST、DOCKER_TLS_VERIFY、
# DOCKER_CERT_PATH 或平台默认 socket。
host = ""

[container]
backend = "docker"
default_image = "debian:bookworm-slim"
runtime_dir = "/opt/memoh/runtime"
data_root = "/opt/memoh/data"
```

Docker backend 通过标准 Docker 环境连接 Docker Engine。它更适合 Memoh 服务直接跑在宿主机上的部署，或者你能保证 Docker bind mount 的源路径就是宿主机真实路径的环境。

不要把官方 Docker Compose 安装里的 `containerd` 直接改成 `docker`，除非你同时处理好 Docker socket 和 `runtime_dir` 的宿主机路径。否则 workspace 容器可能建出来，但拿不到 bridge runtime 文件。

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

`in_cluster = true` 时，Memoh 使用 Pod 的 ServiceAccount。若从集群外控制，设为 `false` 并提供 `kubeconfig`。

Kubernetes workspace 会创建 Pod 和 PVC。启动前请确认 namespace、RBAC、StorageClass 和镜像拉取权限都准备好了。

## Apple

```toml
[container]
backend = "apple"

[apple]
socket_path = ""
binary_path = ""
```

这个后端用于 macOS 本地测试，依赖 socktainer 和 Apple Containerization。它还比较实验性，不支持 provider-backed network sidecar。

## 网络与 overlay

机器人网络分两层：

- runtime network：把 workspace 接到基础容器或 Pod 网络。
- overlay provider：比如 Tailscale、NetBird，给单个机器人挂私有网络。

不同 backend 的能力不一样：

| Backend | Runtime network | Overlay sidecar | Kubernetes 原生 overlay | CDI 设备 |
|---------|-----------------|-----------------|--------------------------|----------|
| `containerd` | CNI | 支持 | 不适用 | 支持 |
| `docker` | 加入 Docker 容器网络 | 受 Docker runtime 能力限制 | 不适用 | 不支持 |
| `kubernetes` | Pod 网络 | 取决于 provider | 支持 | 不支持 |
| `apple` | 基础本地 runtime | 不支持 | 不适用 | 不支持 |

Overlay provider 是按机器人在 Web UI 里配置的，不在全局 TOML 里配。全局 backend 仍然重要，因为它决定具体能跑哪类 overlay driver。
