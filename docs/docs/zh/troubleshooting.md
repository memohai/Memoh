# 故障排除

## MCP 容器：`no running task found: task mcp-xxx not found`

### 症状

当 Bot 尝试使用容器工具（如执行命令）时，服务端日志显示：

```
level=WARN msg="exec failed" provider=container_tool bot_id=xxx command=date error="no running task found: task mcp-xxx not found"
```

containerd 容器日志中也可能出现：

```
level=error msg="failed to delete task" error="rpc error: code = NotFound desc = container not created: not found"
```

### 原因

`config.toml` 中的 `[mcp] data_root` 设置为**宿主机路径**（例如 `/Users/you/Code/Memoh/data`），但 server 和 containerd 容器使用的是挂载在 `/opt/memoh/data` 的 Docker 命名卷。

当 server 在 containerd 内创建 MCP 容器时，会使用 `data_root` 作为挂载源。由于该宿主机路径在 containerd 容器内不存在，`runc` 会报错：

```
failed to fulfil mount request: open /Users/you/Code/Memoh/data/bots/xxx: no such file or directory
```

### 解决方案

1. 在配置中将 `data_root` 设为容器内路径：

```toml
[mcp]
data_root = "/opt/memoh/data"
```

2. 清理残留的 containerd 容器（如存在）：

```bash
docker exec memoh-containerd ctr -n default containers rm mcp-<bot-id>
```

3. 重启 server：

```bash
docker compose restart server
```

> **注意**：如果你同时在本地（Docker 外）运行 server，请将 Docker 配置（`conf/app.docker.toml`）与本地 `config.toml` 分开维护，并在 `docker-compose.yml` 中挂载 Docker 专用配置。

## MCP 容器：重新构建后镜像更新未生效

### 症状

更新 `Dockerfile.containerd`（例如添加 Node.js/Python 到 MCP 镜像）后，重建并重启 containerd 容器，MCP 工具仍报错：

```
exec: "npx": executable file not found in $PATH
```

### 原因

containerd 入口脚本（`containerd-entrypoint.sh`）在 containerd 的镜像存储中已存在该镜像时会跳过导入：

```sh
if ! ctr -n default images check "name==${MCP_IMAGE}" ...; then
  # import
fi
```

由于 `containerd_data` 是持久化的 Docker 卷，旧的 MCP 镜像在容器重启后仍然存在。重建 Docker 镜像中嵌入的新 MCP 镜像永远不会被导入。

### 解决方案

1. 从 containerd 中删除旧的 MCP 镜像：

```bash
docker exec memoh-containerd ctr -n default images rm docker.io/library/memoh-mcp:latest
```

2. 重启 containerd 容器以触发重新导入：

```bash
docker compose restart containerd
```

3. 验证新镜像已导入（如果添加了 Node.js/Python，大小应明显增大）：

```bash
docker exec memoh-containerd ctr -n default images ls
```

4. 删除 Bot 的 MCP 容器，并从 Bot 详情页重新创建，以使用新镜像。
