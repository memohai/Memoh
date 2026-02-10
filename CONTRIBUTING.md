# Contributing Guide

## Mise

You need to install mise first.

### Linux/macOS

```bash
curl https://mise.run | sh
```

Or use homebrew:

```bash
brew install mise
```

### Windows

```bash
winget install jdx.mise
```

## Initialize

Install toolchains and dependencies:

```bash
mise install
```

Setup project:

```bash
mise run setup
```

## Configure

Copy config.toml.example to config.toml and configure:

```bash
cp config.toml.example config.toml
```

## Containerd (macOS)

本项目依赖 containerd 运行容器。macOS 上通过 [Lima](https://lima-vm.io/) 在虚拟机中运行 containerd。

### 安装与启动 Lima VM

```bash
# 安装 Lima（如果尚未安装）
brew install lima

# 启动默认 VM（脚本也会自动执行）
mise run containerd-install
```

### 启动 containerd 服务

Lima VM 启动后，containerd 不一定会自动运行，需要手动启用：

```bash
limactl shell default -- sudo systemctl enable --now containerd
```

### Socket 转发

Go 应用运行在 macOS 宿主机上，但 containerd socket (`/run/containerd/containerd.sock`) 位于 Lima VM 内部，宿主机无法直接访问。

由于 containerd socket 权限为 `root:root rw-rw----`，SSH 转发以普通用户身份无法直接访问。需要先在 VM 内安装 `socat` 并以 root 权限创建代理 socket，再通过 SSH 转发到宿主机：

```bash
# 1. 安装 socat（仅首次需要）
limactl shell default -- sudo apt-get install -y socat

# 2. 在 VM 内创建代理 socket（以 root 权限运行，普通用户可访问）
limactl shell default -- sudo bash -c \
  'rm -f /tmp/containerd-proxy.sock; nohup socat UNIX-LISTEN:/tmp/containerd-proxy.sock,fork,mode=0666 UNIX-CONNECT:/run/containerd/containerd.sock > /dev/null 2>&1 &'

# 3. SSH 转发代理 socket 到宿主机
rm -f /tmp/containerd-lima.sock
ssh -nNT -L /tmp/containerd-lima.sock:/tmp/containerd-proxy.sock \
  -F ~/.lima/default/ssh.config lima-default &
```

然后在 `config.toml` 中配置转发后的 socket 路径：

```toml
[containerd]
socket_path = "/tmp/containerd-lima.sock"
```

### 常见问题

- **Lima VM 状态为 Broken**：运行 `limactl stop default && limactl start default` 重启 VM。
- **连接超时 (`dial unix:///run/containerd/containerd.sock: timeout`)**：检查 VM 是否运行、containerd 是否启动、socket 转发是否建立。
- **gRPC EOF 错误 (`error reading server preface: EOF`)**：通常是 socket 权限问题，确认使用了 socat 代理（步骤 2），而非直接转发 `/run/containerd/containerd.sock`。
- **转发断开**：socat 代理和 SSH 转发均为后台进程，重启电脑或 VM 后需要重新执行步骤 2 和 3。

## Development

Start development environment:

```bash
mise run dev
```

## More Commands

| Command | Description |
| ------- | ----------- |
| `mise run dev` | Start development environment |
| `mise run setup` | Setup development environment |
| `mise run db-up` | Initialize and Migrate Database |
| `mise run db-down` | Drop Database |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sqlc-generate` | Generate SQL code |
| `mise run pnpm-install` | Install dependencies |
| `mise run go-install` | Install Go dependencies |
| `mise run //agent:dev` | Start agent gateway development server |
| `mise run //cmd/agent:start` | Start main server |
| `mise run //packages/web:dev` | Start web development server |
| `mise run //packages/web:build` | Build web |
| `mise run //packages/web:start` | Start web preview |