# Workspace backend

每个 Memoh 机器人都在一个 workspace 里工作。workspace 可以是隔离容器，也可以是在 Desktop/local 场景下明确受信任的本机目录。Backend 决定 workspace 跑在哪里，以及隔离、网络、快照和 display 能力是否可用。

## 容器后端

容器 workspace 在 `config.toml` 里配置：

```toml
[container]
backend = "containerd" # containerd、docker 或 apple
```

| Backend | 适合 | 说明 |
|---------|------|------|
| `containerd` | Docker Compose Server Deploy、Linux 服务器、开发环境 | 官方 server 镜像默认用它。支持 CNI 网络、快照、CDI 设备、provider sidecar，本地 workspace 功能最完整。 |
| `docker` | Memoh 直接跑在宿主机，宿主机有 Docker Engine | 走宿主机 Docker API。`container.runtime_dir` 这类 bind mount 源路径必须在 Docker 宿主机上真实存在。 |
| `apple` | macOS 本地测试 | 通过 socktainer 和 Apple Containerization。provider sidecar 不支持。 |

一键 Docker Compose Server Deploy 固定使用 `containerd`。这是有意的：server 镜像会启动内置 containerd，并挂好机器人 workspace 需要的 runtime 文件。`docker`、`apple` 更适合你能控制宿主机 runtime 路径的手动部署。

## Trusted local workspace

Desktop 和本地开发可以在容器后端之外启用 trusted local workspace：

```toml
[local]
enabled = true
```

Trusted local workspace 直接用 server 进程权限在宿主机上运行。它适合个人桌面工作流和本地开发，但**没有容器隔离**。不要在不受信任的 server 部署里打开 local workspace。

当机器人使用 local workspace 时，依赖容器桌面或容器 display session 的界面能力可能会隐藏或不可用。

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

官方 Docker Compose 栈和直接连接 containerd 的 Linux 主机都用它。

## Docker

```toml
[container]
backend = "docker"
default_image = "debian:bookworm-slim"
runtime_dir = "/opt/memoh/runtime"
data_root = "/opt/memoh/data"

[docker]
# 留空时使用 Docker 标准环境发现：DOCKER_HOST、DOCKER_TLS_VERIFY、
# DOCKER_CERT_PATH 或平台默认 socket。
host = ""
```

Docker backend 通过标准 Docker 环境连接 Docker Engine。它更适合 Memoh 服务直接跑在宿主机上的部署，或者你能保证 Docker bind mount 的源路径就是宿主机真实路径的环境。

不要把官方 Docker Compose 安装里的 `containerd` 直接改成 `docker`，除非你同时处理好 Docker socket 和 `runtime_dir` 的宿主机路径。否则 workspace 容器可能建出来，但拿不到 bridge runtime 文件。

## Apple

```toml
[container]
backend = "apple"

[apple]
socket_path = ""
binary_path = ""
```

Apple backend 用于通过 socktainer 和 Apple Containerization 做 macOS 本地测试。它仍是实验性能力，不支持 provider sidecar。

## Display、Browser Use 与 Computer Use

容器 workspace 可以提供 display runtime：Xvnc/RFB 作为 workspace 桌面显示与输入基础，有头 Chrome/Chromium 提供 CDP，网页端 Display pane 通过 WebRTC 打开会话。

当网站需要真实图形浏览器时，用这条路径。Headless Playwright 仍然可以作为普通 workspace 命令运行，但 Browser Use 和 Computer Use 面向的是有头 workspace 桌面。

Local workspace 不提供同样的容器桌面隔离。工具层面的区别见 [Browser / Computer Use](/zh/getting-started/browser-computer-use)。

## 网络和 overlay

机器人网络有两层：

- runtime network：把 workspace 接到基础容器或 Pod 网络。
- overlay provider：如 Tailscale、NetBird，可给单个机器人附加私有网络。

不同后端能力不同：

| Backend | Runtime network | Overlay sidecar | CDI 设备 | 容器 display |
|---------|-----------------|-----------------|----------|--------------|
| `containerd` | CNI | 支持 | 支持 | 支持 |
| `docker` | 加入 Docker 容器网络 | 受 Docker runtime 能力限制 | 不支持 | runtime 文件和镜像组件齐全时支持 |
| `apple` | 基础本地 runtime | 不支持 | 不支持 | 有限 |
| `local` | 宿主机网络 | 不支持 | 宿主机级别 | 无容器桌面 |

Overlay provider 在机器人界面里配置，不在全局 TOML 里配。全局 backend 仍然重要，因为它决定能跑哪类 overlay driver。
