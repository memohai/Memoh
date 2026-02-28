# Docker 安装

Docker 是运行 Memoh 的推荐方式。技术栈包括 PostgreSQL、Qdrant、主服务（内嵌 Containerd）、Agent 网关和 Web UI——全部通过 Docker Compose 编排。你无需在宿主机上安装 containerd、nerdctl 或 buildkit；一切都在容器内运行。

## 前提条件

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose v2](https://docs.docker.com/compose/install/)
- Git

## 一键安装（推荐）

运行官方安装脚本（需要 Docker 和 Docker Compose）：

```bash
curl -fsSL https://memoh.sh | sudo sh
```

脚本将会：

1. 检查 Docker 和 Docker Compose
2. 提示配置（工作目录、数据目录、管理员凭据、JWT 密钥、Postgres 密码、中国镜像）
3. 克隆仓库
4. 根据 Docker 模板和你的设置生成 `config.toml`
5. 拉取镜像并启动所有服务

**静默安装**（使用所有默认值，无交互提示）：

```bash
curl -fsSL https://memoh.sh | sudo sh -s -- -y
```

静默安装的默认值：

- 工作目录：`~/memoh`
- 数据目录：`~/memoh/data`
- 管理员：`admin` / `admin123`
- JWT 密钥：自动生成
- Postgres 密码：`memoh123`

## 手动安装

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
```

编辑 `config.toml`——至少修改以下内容：

- `admin.password` — 管理员密码
- `auth.jwt_secret` — 使用 `openssl rand -base64 32` 生成
- `postgres.password` — 数据库密码（同时设置 `POSTGRES_PASSWORD` 环境变量保持一致）

然后启动：

```bash
sudo POSTGRES_PASSWORD=your-db-password docker compose up -d
```

> 在 macOS 上或你的用户属于 `docker` 组时，不需要 `sudo`。

> **重要**：`docker-compose.yml` 默认挂载 `./config.toml`。启动前必须创建此文件——没有此文件运行会失败。

### 中国大陆镜像

中国大陆用户如果无法直接访问 Docker Hub，请取消 `config.toml` 中 `registry` 行的注释：

```toml
[mcp]
registry = "memoh.cn"
```

并使用中国镜像 compose 叠加配置：

```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml up -d
```

安装脚本在你选择中国镜像时会自动处理此步骤。

## 访问地址

启动后：

| 服务          | URL                    |
|---------------|------------------------|
| Web UI        | http://localhost:8082  |
| API           | http://localhost:8080  |
| Agent 网关    | http://localhost:8081  |

默认登录：`admin` / `admin123`（请在 `config.toml` 中修改）。

首次启动可能需要 1–2 分钟，等待镜像拉取和服务初始化。

## 常用命令

> 在 Linux 上如果你的用户不在 `docker` 组中，请加 `sudo` 前缀。

```bash
docker compose up -d           # 启动
docker compose down            # 停止
docker compose logs -f         # 查看日志
docker compose ps              # 查看状态
docker compose pull && docker compose up -d  # 更新到最新镜像
```

## 生产环境检查清单

1. **密码** — 修改 `config.toml` 中的所有默认密码和密钥
2. **HTTPS** — 配置 SSL（例如通过 `docker-compose.override.yml` 配置证书或使用反向代理）
3. **防火墙** — 限制必要端口的访问
4. **资源限制** — 为容器设置内存/CPU 限制
5. **备份** — 定期备份 Postgres 和 Qdrant 数据

## 故障排除

```bash
docker compose logs server      # 查看主服务日志
docker compose config           # 验证配置
docker compose build --no-cache && docker compose up -d  # 完整重建
```

## 安全警告

- 主服务以特权容器访问权限运行——仅在可信环境中运行
- 生产环境使用前必须修改所有默认密码和密钥
- 生产环境请使用 HTTPS
