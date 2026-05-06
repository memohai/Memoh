# Docker 安装

推荐用 Docker 跑 Memoh。默认编排里包含 PostgreSQL、主服务（显式配置 workspace backend、智能体也在同一进程）和网页前端。单机轻量部署也可以用 SQLite，见 [SQLite 部署](/zh/installation/sqlite.md)。

官方 Compose 栈使用 `containerd` workspace backend。server 镜像会启动内置 containerd，并挂好机器人 workspace 需要的 runtime 文件。Docker Engine、Kubernetes 和 Apple 后端见 [Workspace backend](/zh/installation/workspace-backends.md)。

## 服务结构

Compose 里有多组服务。有的默认就起，有的通过 `--profile` 打开：

| 服务 | Profile | 说明 |
|------|---------|------|
| **server** | *（核心）* | 主服务，使用配置中的容器运行时后端，智能体同进程 |
| **web** | *（核心）* | 网页端（Vue 3） |
| **postgres** | *（核心）* | PostgreSQL |
| **qdrant** | `qdrant` | 向量库，给记忆检索用（稀疏/稠密） |
| **sparse** | `sparse` | 神经稀疏编码，给记忆检索（见下） |

### sparse 服务

**sparse** 容器跑神经稀疏向量，给记忆检索用。里面是一个轻量 Python（Flask）服务，端口 8085，模型是 OpenSearch 项目放出来的 [`opensearch-neural-sparse-encoding-multilingual-v1`](https://huggingface.co/opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1)。

**它做什么：**

- 把文档压成稀疏向量（一批 token 下标 + 权重），基于掩码语言模型。
- 查询端用 IDF 加权词表，检索快。
- 和 Qdrant 一起用，可以在**不另接外部 embedding API** 的情况下做语义级记忆搜索。

**什么时候值得开：**

- 不想为 embedding 花钱，模型在容器里本地跑。
- 多语言模型现成的。
- 比纯关键词（BM25）强一截，又比大稠密向量省资源。

**何时启用：**

打算用内置记忆提供方的 **sparse** 模式时，把 sparse profile 打开。镜像构建时会预下模型，启动不用临时拉权重。

```bash
docker compose --profile qdrant --profile sparse up -d
```

模式细节见 [内置记忆提供方](/zh/memory-providers/builtin.md)。

## 先决条件

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose v2](https://docs.docker.com/compose/install/)
- Git

## 一键安装（推荐）

官方脚本（本机已装好 Docker 与 Compose）：

```bash
curl -fsSL https://memoh.sh | sh
```

请用普通用户运行安装脚本，不要给整个脚本套 `sudo`。如果 Docker
需要提权，脚本会只对 `docker` 命令使用 `sudo`。如果确实要以 root
运行整个安装脚本，需要显式设置 `MEMOH_ALLOW_ROOT_INSTALL=true`。

脚本会：检查 Docker/Compose；交互问配置（工作区、数据目录、管理员、JWT、数据库后端、Postgres 密码、workspace backend 提示、是否开 sparse、浏览器核等）；从 GitHub 取最新发布并克隆；按 Docker 模板生成 `config.toml`；按数据库后端选择 compose 文件；钉死镜像版本；按选的浏览器核拉齐服务。

**静默安装**（全默认、无提问）：

```bash
curl -fsSL https://memoh.sh | sh -s -- -y
```

静默时默认大概：工作区 `~/memoh`；数据 `~/memoh/data`；管理员 `admin` / `admin123`；JWT 随机；数据库后端 PostgreSQL；Postgres 密码 `memoh123`。

**使用 SQLite**（单机轻量部署）：

```bash
curl -fsSL https://memoh.sh | MEMOH_DATABASE_DRIVER=sqlite sh
```

也可以用参数：

```bash
curl -fsSL https://memoh.sh | sh -s -- --database-driver sqlite
```

**指定版本：**

```bash
curl -fsSL https://memoh.sh | sh -s -- --version v0.6.0
```

或：

```bash
curl -fsSL https://memoh.sh | MEMOH_VERSION=v0.6.0 sh
```

**大陆镜像**（拉镜像慢时）：

```bash
curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
```

> 环境变量可组合，例如 `MEMOH_VERSION=v0.6.0 USE_CN_MIRROR=true`。

## 手动安装

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
```

至少改 `config.toml` 里：

- `admin.password`
- `auth.jwt_secret`（可 `openssl rand -base64 32`）
- `postgres.password`（环境变量 `POSTGRES_PASSWORD` 要一致）

如果用 SQLite，把 `database.driver` 改成 `"sqlite"`，并使用 `docker-compose.sqlite.yml`。详细步骤见 [SQLite 部署](/zh/installation/sqlite.md)。

然后（推荐开 Qdrant、浏览器、sparse）：

```bash
POSTGRES_PASSWORD=你的库密码 docker compose --profile qdrant --profile sparse up -d
```

只跑核心（无向量、无浏览器）：

```bash
POSTGRES_PASSWORD=你的库密码 docker compose up -d
```

> macOS 或用户已在 `docker` 组里，一般不必 `sudo`。

> **重要**：`docker-compose.yml` 默认挂 `./config.toml`，先建好文件再 `up`，否则起不来。

### 大陆镜像源

拉 Docker Hub 困难时，在 `config.toml` 里取消 `registry` 一行的注释：

```toml
[container]
registry = "memoh.cn"
image_pull_policy = "if_not_present" # if_not_present、always 或 never
```

并叠加国内 overlay：

```bash
docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml \
  --profile qdrant up -d
```

一键脚本在 `USE_CN_MIRROR=true` 时会处理这套。

## 访问地址

起来之后：

| 服务 | 地址 |
|------|------|
| 网页 | http://localhost:8082 |
| API | http://localhost:8080 |

默认登录 `admin` / `admin123`（请在 `config.toml` 改掉）。首次拉镜像、初始化可能要一两分钟。

## 配置总览

`config.toml` 主段落大致如下：

| 段落 | 含义 |
|------|------|
| `[log]` | 等级与格式（`info`/`debug`；`text`/`json`） |
| `[server]` | 监听，默认 `:8080` |
| `[admin]` | 管理员账号 |
| `[auth]` | JWT 与过期时间 |
| `timezone` | 服时区，默认 `UTC` |
| `[database]` | 数据库后端，`postgres` 或 `sqlite` |
| `[container]` | Workspace backend 选择，以及通用 workspace 镜像、拉取策略、数据路径、runtime 路径、CNI 设置 |
| `[containerd]` | socket 与 namespace |
| `[docker]` | Docker Engine host 覆盖；留空时用 Docker 环境变量或默认 socket |
| `[kubernetes]` | namespace、kubeconfig/in-cluster、PVC、镜像拉取 secret、bridge 端口 |
| `[apple]` | Apple backend 的 socktainer socket 和 binary 覆盖 |
| `[postgres]` | PostgreSQL 连接 |
| `[sqlite]` | SQLite 文件路径、WAL、锁等待时间 |
| `[qdrant]` | Qdrant 地址、密钥、超时 |
| `[sparse]` | 稀疏服务 URL |
| `[registry]` | 供应商定义目录 |
| `[web]` | 前端 host/port |

## 常用命令

> Linux 上若用户不在 `docker` 组，命令前加 `sudo`。

```bash
docker compose up -d           # 起
docker compose down            # 停
docker compose logs -f         # 看日志
docker compose ps              # 状态
docker compose pull && docker compose up -d  # 更新镜像再起
```

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `POSTGRES_PASSWORD` | `memoh123` | 须与 `config.toml` 里 `postgres.password` 一致 |
| `MEMOH_CONFIG` | `./config.toml` | 配置文件路径 |
| `MEMOH_VERSION` | 最新发版 | 要装的 git 标签，也用于钉死镜像 |
| `MEMOH_INSTALL_MODE` | `auto` | 安装模式：`auto`、`fresh`、`upgrade` 或 `reinstall` |
| `MEMOH_DATABASE_DRIVER` | `postgres` | 新安装时使用的数据库后端：`postgres` 或 `sqlite` |
| `MEMOH_CONTAINER_BACKEND` | `containerd` | Workspace backend。一键 Docker Compose 安装只支持 `containerd`；`docker`、`kubernetes`、`apple` 请走手动部署。 |
| `MEMOH_ALLOW_ROOT_INSTALL` | `false` | 允许以 root 运行安装脚本本身。建议保持未设置，用普通用户运行安装脚本。 |
| `USE_CN_MIRROR` | `false` | 是否用大陆镜像 |
