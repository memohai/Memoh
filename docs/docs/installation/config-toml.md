# config.toml Reference

Memoh uses a TOML configuration file (`config.toml`) in the project root. For Docker deployments, copy the template first: `cp conf/app.docker.toml config.toml`. See [Docker installation](./docker) for details.

## Full Example

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
# registry = "memoh.cn"  # Uncomment for China mainland mirror
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

## Section Reference

### `[log]`

| Field   | Type   | Default | Description                                      |
|---------|--------|---------|--------------------------------------------------|
| `level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error`     |
| `format`| string | `"text"` | Log format: `text` or `json`                    |

### `[server]`

| Field  | Type   | Default | Description                                      |
|--------|--------|---------|--------------------------------------------------|
| `addr` | string | `":8080"` | HTTP listen address. Use `:8080` for all interfaces, or `host:port` (e.g. `server:8080` in Docker). |

### `[admin]`

| Field      | Type   | Default | Description                          |
|------------|--------|---------|--------------------------------------|
| `username` | string | `"admin"` | Admin login username                |
| `password` | string | —       | Admin login password. **Change in production.** |
| `email`    | string | —       | Admin email (for display)            |

### `[auth]`

| Field          | Type   | Default | Description                                      |
|----------------|--------|---------|--------------------------------------------------|
| `jwt_secret`   | string | —       | Secret for signing JWT tokens. **Required.** Generate with `openssl rand -base64 32`. |
| `jwt_expires_in` | string | `"24h"` | JWT expiration, e.g. `"24h"`, `"168h"` (7 days) |

### `[containerd]`

| Field         | Type   | Default | Description                                      |
|---------------|--------|---------|--------------------------------------------------|
| `socket_path` | string | `"/run/containerd/containerd.sock"` | Path to containerd socket       |
| `namespace`   | string | `"default"` | Containerd namespace for bot containers      |

### `[mcp]`

MCP (Model Context Protocol) container configuration. Each bot runs in a container built from this image.

| Field         | Type   | Default | Description                                      |
|---------------|--------|---------|--------------------------------------------------|
| `registry`    | string | `""`    | Image registry mirror prefix. Set to `"memoh.cn"` for China mainland. When set, the final image ref becomes `registry/image`. |
| `image`       | string | `"memohai/mcp:latest"` | MCP container image. Short Docker Hub names are auto-normalized for containerd (e.g. `memohai/mcp:latest` → `docker.io/memohai/mcp:latest`). |
| `snapshotter` | string | `"overlayfs"` | Containerd snapshotter                      |
| `data_root`   | string | `"data"` | Host path for bot data (Docker: `/opt/memoh/data`) |
| `cni_bin_dir` | string | `"/opt/cni/bin"` | CNI plugin binary directory              |
| `cni_conf_dir`| string | `"/etc/cni/net.d"` | CNI configuration directory            |

### `[postgres]`

| Field     | Type   | Default | Description                                      |
|-----------|--------|---------|--------------------------------------------------|
| `host`    | string | `"127.0.0.1"` | PostgreSQL host                             |
| `port`    | int    | `5432`  | PostgreSQL port                                  |
| `user`    | string | `"memoh"` | Database user                                  |
| `password`| string | —       | Database password                                |
| `database`| string | `"memoh"` | Database name                                 |
| `sslmode` | string | `"disable"` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |

### `[qdrant]`

| Field            | Type   | Default | Description                                      |
|------------------|--------|---------|--------------------------------------------------|
| `base_url`       | string | `"http://127.0.0.1:6334"` | Qdrant HTTP API URL                    |
| `api_key`        | string | `""`    | Optional API key for Qdrant Cloud                 |
| `collection`     | string | `"memory"` | Vector collection name for memories           |
| `timeout_seconds`| int    | `10`    | Request timeout in seconds                       |

### `[agent_gateway]`

| Field         | Type   | Default | Description                                      |
|---------------|--------|---------|--------------------------------------------------|
| `host`        | string | `"127.0.0.1"` | Agent gateway bind host. In Docker, use `"agent"` (service name). |
| `port`        | int    | `8081`  | Agent gateway port                               |
| `server_addr` | string | `":8080"` | Address the agent uses to reach the main server. In Docker, use `"server:8080"`. |

### `[web]`

| Field  | Type   | Default | Description                                      |
|--------|--------|---------|--------------------------------------------------|
| `host` | string | `"127.0.0.1"` | Web UI bind host                              |
| `port` | int    | `8082`  | Web UI port                                      |

Web search providers (Brave, Bing, Google, Tavily, Serper, SearXNG, Jina, Exa, Bocha, DuckDuckGo, Yandex, Sogou) are configured through the web UI under **Search Providers**, not in `config.toml`.
