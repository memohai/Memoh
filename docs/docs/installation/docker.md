# Docker Installation

Docker is the recommended way to run Memoh. The default stack includes PostgreSQL, the main server (with an explicit workspace backend and in-process AI agent), and the web UI, all orchestrated via Docker Compose. SQLite is also available for single-node installs; see [SQLite deployment](/installation/sqlite.md).

The official Compose stack uses the `containerd` workspace backend. The server image starts an embedded containerd and mounts the runtime files needed by bot workspaces. For Docker Engine, Kubernetes, and Apple backends, see [Workspace backends](/installation/workspace-backends.md).

## Service Architecture

The Docker Compose stack consists of multiple services. Some are always started, others are optional and enabled via `--profile`:

| Service | Profile | Description |
|---------|---------|-------------|
| **server** | *(core)* | Main Memoh server with the configured container runtime backend and in-process AI agent |
| **web** | *(core)* | Web UI (Vue 3) |
| **postgres** | *(core)* | PostgreSQL database |
| **qdrant** | `qdrant` | Qdrant vector database for memory search (sparse and dense modes) |
| **sparse** | `sparse` | Neural sparse encoding service for memory retrieval (see below) |

### Sparse Service

The **sparse** container provides neural sparse vector encoding for memory retrieval. It runs a lightweight Python (Flask) service on port 8085 that uses the [`opensearch-neural-sparse-encoding-multilingual-v1`](https://huggingface.co/opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1) model from OpenSearch.

**What it does:**

- Converts document text into sparse vectors (a compact list of token indices + importance weights) using a masked language model
- Encodes queries using IDF-weighted term lookup for fast, efficient retrieval
- Works with Qdrant to enable semantic memory search without requiring an external embedding API

**Why use it:**

- **No embedding API costs** — The model runs locally inside the container, so you don't need an OpenAI/Cohere/etc. embedding API key
- **Multilingual** — The underlying model supports multiple languages out of the box
- **Good retrieval quality** — Neural sparse encoding provides significantly better results than keyword-only search (BM25), while being lighter than dense embedding models

**When to enable it:**

Enable the sparse profile (`--profile sparse`) if you plan to use the built-in memory provider in **sparse mode**. The model is pre-downloaded during the Docker image build, so the container starts quickly without needing to fetch weights at runtime.

```bash
docker compose --profile qdrant --profile sparse up -d
```

For more details on memory modes, see [Built-in Memory Provider](/memory-providers/builtin.md).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose v2](https://docs.docker.com/compose/install/)
- Git

## One-Click Install (Recommended)

Run the official install script (requires Docker and Docker Compose):

```bash
curl -fsSL https://memoh.sh | sh
```

Run the installer as your normal user. Do not wrap the whole script in `sudo`;
the script will use `sudo docker` internally only if Docker requires it. If you
intentionally need to run the whole installer as root, set
`MEMOH_ALLOW_ROOT_INSTALL=true` explicitly.

The script will:

1. Check for Docker and Docker Compose
2. Detect whether this is a first-time install, an upgrade, or a reinstall
3. Prompt for configuration (workspace, data directory, admin credentials, JWT secret, database backend, Postgres password when needed, workspace backend notice, and sparse service toggle)
4. Reuse the existing `config.toml` automatically during upgrades so database credentials stay aligned with the persisted PostgreSQL volume
5. Offer a clean reinstall mode that removes Memoh Docker containers, volumes, and network before starting again
6. Fetch the latest release tag from GitHub and clone the repository
7. Generate `config.toml` from the Docker template with your settings when needed
8. Select `docker-compose.yml` for PostgreSQL or `docker-compose.sqlite.yml` for SQLite
9. Pin Docker image versions to the release
10. Start all services
11. Print recent database, migration, and server logs automatically if startup fails

**Silent install** (use all defaults, no prompts):

```bash
curl -fsSL https://memoh.sh | sh -s -- -y
```

Defaults when running silently:

- Workspace: `~/memoh`
- Data directory: `~/memoh/data`
- Admin: `admin` / `admin123`
- JWT secret: auto-generated
- Database: PostgreSQL
- Postgres password: `memoh123`

If the script detects an existing Memoh installation in silent mode, it defaults to **upgrade** and reuses the previous `config.toml`. If Docker state exists but no reusable `config.toml` can be found, the script exits and asks you to choose an explicit reinstall.

**Force a clean reinstall** (removes Memoh Docker data before starting again):

```bash
curl -fsSL https://memoh.sh | MEMOH_INSTALL_MODE=reinstall sh
```

You can also pass the install mode as an argument:

```bash
curl -fsSL https://memoh.sh | sh -s -- --install-mode reinstall
```

**Install a specific version:**

```bash
curl -fsSL https://memoh.sh | sh -s -- --version v0.6.0
```

Or using the environment variable:

```bash
curl -fsSL https://memoh.sh | MEMOH_VERSION=v0.6.0 sh
```

**Use China mainland mirror** (for slow image pulls):

```bash
curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
```

> Environment variables can be combined, e.g. `curl -fsSL https://memoh.sh | MEMOH_VERSION=v0.6.0 USE_CN_MIRROR=true sh`

**Use SQLite instead of PostgreSQL** (single-node installs):

```bash
curl -fsSL https://memoh.sh | MEMOH_DATABASE_DRIVER=sqlite sh
```

Or:

```bash
curl -fsSL https://memoh.sh | sh -s -- --database-driver sqlite
```

## Manual Install

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
```

Edit `config.toml` — at minimum change:

- `admin.password` — Admin password
- `auth.jwt_secret` — Generate with `openssl rand -base64 32`
- `postgres.password` — Database password (also set `POSTGRES_PASSWORD` env var to match)

For SQLite, set `database.driver = "sqlite"` and use `docker-compose.sqlite.yml`. Details are in [SQLite deployment](/installation/sqlite.md).

Then start (recommended — with Qdrant and Sparse):

```bash
POSTGRES_PASSWORD=your-db-password docker compose --profile qdrant --profile sparse up -d
```

Or start core services only (no vector DB or sparse memory service):

```bash
POSTGRES_PASSWORD=your-db-password docker compose up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

> **Important**: `docker-compose.yml` mounts `./config.toml` by default. You must create this file before starting — running without it will fail.

### China Mainland Mirror

For users in mainland China who cannot access Docker Hub directly, uncomment the `registry` line in `config.toml`:

```toml
[container]
registry = "memoh.cn"
image_pull_policy = "if_not_present" # if_not_present, always, or never
```

And add the China mirror compose overlay:

```bash
docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml \
  --profile qdrant up -d
```

The install script handles this automatically when you set `USE_CN_MIRROR=true`.

## Access Points

After startup:

| Service         | URL                    |
|-----------------|------------------------|
| Web UI          | http://localhost:8082  |
| API             | http://localhost:8080  |

Default login: `admin` / `admin123` (change this in `config.toml`).

First startup may take 1–2 minutes while images are pulled and services initialize.

## Configuration Reference

The `config.toml` file controls all server behavior. Here is a summary of the available sections:

| Section | Description |
|---------|-------------|
| `[log]` | Logging level and format (`info`, `debug`; `text`, `json`) |
| `[server]` | HTTP listen address (default `:8080`) |
| `[admin]` | Admin account credentials (username, password, email) |
| `[auth]` | JWT secret and token expiration |
| `timezone` | Server timezone (default `UTC`) |
| `[database]` | Database backend selection (`postgres` or `sqlite`) |
| `[container]` | Workspace backend selection plus common workspace image, pull policy, data path, runtime path, and CNI settings |
| `[containerd]` | Containerd socket path and namespace |
| `[docker]` | Docker Engine host override; empty uses Docker environment/default socket |
| `[kubernetes]` | Kubernetes namespace, kubeconfig/in-cluster mode, PVC, image pull secret, bridge port |
| `[apple]` | socktainer socket and binary overrides for the Apple backend |
| `[postgres]` | PostgreSQL connection (host, port, user, password, database, sslmode) |
| `[sqlite]` | SQLite file path, WAL mode, and busy timeout |
| `[qdrant]` | Qdrant vector database connection (base_url, api_key, timeout) |
| `[sparse]` | Sparse encoding service URL |
| `[registry]` | Provider definitions directory |
| `[web]` | Web frontend host and port |

## Common Commands

> Prefix with `sudo` on Linux if your user is not in the `docker` group.

```bash
docker compose up -d           # Start
docker compose down            # Stop
docker compose down -v         # Stop and remove Memoh Docker data
docker compose logs -f         # View logs
docker compose ps              # Status
docker compose pull && docker compose up -d  # Update to latest images
```

## Environment Variables

| Variable           | Default            | Description                                  |
|--------------------|--------------------|----------------------------------------------|
| `POSTGRES_PASSWORD`| `memoh123`         | PostgreSQL password (must match `postgres.password` in `config.toml`) |
| `MEMOH_CONFIG`     | `./config.toml`    | Path to the configuration file               |
| `MEMOH_VERSION`    | *(latest release)* | Git tag to install (e.g. `v0.6.0`). Also pins Docker image versions. |
| `MEMOH_INSTALL_MODE` | `auto`           | Install mode: `auto`, `fresh`, `upgrade`, or `reinstall` |
| `MEMOH_DATABASE_DRIVER` | `postgres`    | Database backend for fresh installs: `postgres` or `sqlite` |
| `MEMOH_CONTAINER_BACKEND` | `containerd` | Workspace backend. One-click Docker Compose installs support `containerd`; use manual deployment for `docker`, `kubernetes`, or `apple`. |
| `MEMOH_ALLOW_ROOT_INSTALL` | `false` | Allow running the installer shell itself as root. Prefer leaving this unset and running the installer as a normal user. |
| `USE_CN_MIRROR`    | `false`            | Set to `true` to use China mainland image mirrors |
