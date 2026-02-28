# config.toml 参考

Memoh 使用项目根目录下的 TOML 配置文件（`config.toml`）。Docker 部署时，先复制模板：`cp conf/app.docker.toml config.toml`。详见 [Docker 安装](./docker)。

## 完整示例

```toml
[log]
level = "info"
format = "text"

[server]
addr = ":8080"

[admin]
username = "admin"
password = "change-your-password"
email = "admin@example.com"

[auth]
jwt_secret = "your-secret-from-openssl-rand-base64-32"
jwt_expires_in = "168h"

[containerd]
socket_path = "/run/containerd/containerd.sock"
namespace = "default"

[mcp]
# registry = "memoh.cn"  # 中国大陆镜像，取消注释即可启用
image = "memohai/mcp:latest"
snapshotter = "overlayfs"
data_root = "data"

[postgres]
host = "127.0.0.1"
port = 5432
user = "memoh"
password = "your-password"
database = "memoh"
sslmode = "disable"

[qdrant]
base_url = "http://127.0.0.1:6334"
api_key = ""
collection = "memory"
timeout_seconds = 10

[agent_gateway]
host = "127.0.0.1"
port = 8081
server_addr = ":8080"

[web]
host = "127.0.0.1"
port = 8082
```

## 配置项参考

### `[log]`

| 字段    | 类型   | 默认值   | 说明                                              |
|---------|--------|----------|--------------------------------------------------|
| `level` | string | `"info"` | 日志级别：`debug`、`info`、`warn`、`error`         |
| `format`| string | `"text"` | 日志格式：`text` 或 `json`                        |

### `[server]`

| 字段   | 类型   | 默认值     | 说明                                              |
|--------|--------|-----------|--------------------------------------------------|
| `addr` | string | `":8080"` | HTTP 监听地址。使用 `:8080` 监听所有接口，或 `host:port`（如 Docker 中使用 `server:8080`）。 |

### `[admin]`

| 字段       | 类型   | 默认值     | 说明                                |
|------------|--------|-----------|-------------------------------------|
| `username` | string | `"admin"` | 管理员登录用户名                      |
| `password` | string | —         | 管理员登录密码。**生产环境务必修改。**    |
| `email`    | string | —         | 管理员邮箱（用于显示）                  |

### `[auth]`

| 字段             | 类型   | 默认值   | 说明                                              |
|------------------|--------|---------|--------------------------------------------------|
| `jwt_secret`     | string | —       | JWT 令牌签名密钥。**必填。** 使用 `openssl rand -base64 32` 生成。 |
| `jwt_expires_in` | string | `"24h"` | JWT 过期时间，如 `"24h"`、`"168h"`（7 天）          |

### `[containerd]`

| 字段          | 类型   | 默认值                                | 说明                          |
|---------------|--------|--------------------------------------|-------------------------------|
| `socket_path` | string | `"/run/containerd/containerd.sock"`  | containerd socket 路径         |
| `namespace`   | string | `"default"`                          | Bot 容器使用的 containerd 命名空间 |

### `[mcp]`

MCP (Model Context Protocol) 容器配置。每个 Bot 运行在基于此镜像构建的容器中。

| 字段          | 类型   | 默认值                    | 说明                                              |
|---------------|--------|--------------------------|--------------------------------------------------|
| `registry`    | string | `""`                     | 镜像仓库镜像前缀。中国大陆设为 `"memoh.cn"`。设置后最终镜像引用变为 `registry/image`。 |
| `image`       | string | `"memohai/mcp:latest"`   | MCP 容器镜像。Docker Hub 短名称会自动为 containerd 标准化（如 `memohai/mcp:latest` → `docker.io/memohai/mcp:latest`）。 |
| `snapshotter` | string | `"overlayfs"`            | Containerd snapshotter                            |
| `data_root`   | string | `"data"`                 | Bot 数据的宿主机路径（Docker 中为 `/opt/memoh/data`） |
| `cni_bin_dir` | string | `"/opt/cni/bin"`         | CNI 插件二进制目录                                  |
| `cni_conf_dir`| string | `"/etc/cni/net.d"`       | CNI 配置目录                                       |

### `[postgres]`

| 字段      | 类型   | 默认值          | 说明                                              |
|-----------|--------|----------------|--------------------------------------------------|
| `host`    | string | `"127.0.0.1"`  | PostgreSQL 主机                                    |
| `port`    | int    | `5432`         | PostgreSQL 端口                                    |
| `user`    | string | `"memoh"`      | 数据库用户                                          |
| `password`| string | —              | 数据库密码                                          |
| `database`| string | `"memoh"`      | 数据库名称                                          |
| `sslmode` | string | `"disable"`    | SSL 模式：`disable`、`require`、`verify-ca`、`verify-full` |

### `[qdrant]`

| 字段             | 类型   | 默认值                          | 说明                          |
|------------------|--------|---------------------------------|-------------------------------|
| `base_url`       | string | `"http://127.0.0.1:6334"`      | Qdrant HTTP API 地址            |
| `api_key`        | string | `""`                            | 可选的 Qdrant Cloud API 密钥    |
| `collection`     | string | `"memory"`                      | 记忆向量集合名称                  |
| `timeout_seconds`| int    | `10`                            | 请求超时时间（秒）                |

### `[agent_gateway]`

| 字段          | 类型   | 默认值          | 说明                                              |
|---------------|--------|----------------|--------------------------------------------------|
| `host`        | string | `"127.0.0.1"`  | Agent 网关绑定主机。Docker 中使用 `"agent"`（服务名）。 |
| `port`        | int    | `8081`         | Agent 网关端口                                      |
| `server_addr` | string | `":8080"`      | Agent 连接主服务的地址。Docker 中使用 `"server:8080"`。 |

### `[web]`

| 字段   | 类型   | 默认值          | 说明              |
|--------|--------|----------------|-------------------|
| `host` | string | `"127.0.0.1"`  | Web UI 绑定主机    |
| `port` | int    | `8082`         | Web UI 端口       |

Web 搜索 Provider（Brave、Bing、Google、Tavily、Serper、SearXNG、Jina、Exa、Bocha、DuckDuckGo、Yandex、Sogou）通过 Web UI 的 **Search Providers** 配置，不在 `config.toml` 中。
