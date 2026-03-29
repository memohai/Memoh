# Docker Installation

Docker is the recommended way to run Memoh. The stack includes PostgreSQL, the main server (with embedded Containerd and in-process AI agent), and the web UI — all orchestrated via Docker Compose. You do not need to install containerd, nerdctl, or buildkit on your host; everything runs inside containers.

## Service Architecture

The Docker Compose stack consists of multiple services. Some are always started, others are optional and enabled via `--profile`:

| Service | Profile | Description |
|---------|---------|-------------|
| **server** | *(core)* | Main Memoh server with embedded Containerd and in-process AI agent |
| **web** | *(core)* | Web UI (Vue 3) |
| **postgres** | *(core)* | PostgreSQL database |
| **qdrant** | `qdrant` | Qdrant vector database for memory search (sparse and dense modes) |
| **browser** | `browser` | Playwright-based browser gateway for bot web automation |
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
docker compose --profile qdrant --profile sparse --profile browser up -d
```

For more details on memory modes, see [Built-in Memory Provider](/memory-providers/builtin.md).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose v2](https://docs.docker.com/compose/install/)
- Git

## One-Click Install (Recommended)

Run the official install script (requires Docker and Docker Compose):

```bash
curl -fsSL https://memoh.sh | sudo sh
```

The script will:

1. Check for Docker and Docker Compose
2. Prompt for configuration (workspace, data directory, admin credentials, JWT secret, Postgres password, sparse service toggle, browser core selection)
3. Fetch the latest release tag from GitHub and clone the repository
4. Generate `config.toml` from the Docker template with your settings
5. Pin Docker image versions to the release
6. Build the browser image with selected cores and start all services

**Silent install** (use all defaults, no prompts):

```bash
curl -fsSL https://memoh.sh | sudo sh -s -- -y
```

Defaults when running silently:

- Workspace: `~/memoh`
- Data directory: `~/memoh/data`
- Admin: `admin` / `admin123`
- JWT secret: auto-generated
- Postgres password: `memoh123`

**Install a specific version:**

```bash
curl -fsSL https://memoh.sh | sudo sh -s -- --version v0.6.0
```

Or using the environment variable:

```bash
curl -fsSL https://memoh.sh | sudo MEMOH_VERSION=v0.6.0 sh
```

**Use China mainland mirror** (for slow image pulls):

```bash
curl -fsSL https://memoh.sh | sudo USE_CN_MIRROR=true sh
```

> Environment variables can be combined, e.g. `curl -fsSL https://memoh.sh | sudo MEMOH_VERSION=v0.6.0 USE_CN_MIRROR=true sh`

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

Then start (recommended — with Qdrant, Browser, and Sparse):

```bash
sudo POSTGRES_PASSWORD=your-db-password docker compose --profile qdrant --profile browser --profile sparse up -d
```

Or start core services only (no vector DB or browser automation):

```bash
sudo POSTGRES_PASSWORD=your-db-password docker compose up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

> **Important**: `docker-compose.yml` mounts `./config.toml` by default. You must create this file before starting — running without it will fail.

### China Mainland Mirror

For users in mainland China who cannot access Docker Hub directly, uncomment the `registry` line in `config.toml`:

```toml
[workspace]
registry = "memoh.cn"
```

And add the China mirror compose overlay:

```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml \
  --profile qdrant --profile browser up -d
```

The install script handles this automatically when you set `USE_CN_MIRROR=true`.

## Access Points

After startup:

| Service         | URL                    |
|-----------------|------------------------|
| Web UI          | http://localhost:8082  |
| API             | http://localhost:8080  |
| Browser Gateway | http://localhost:8083  |

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
| `[containerd]` | Containerd socket path and namespace |
| `[workspace]` | Container image, snapshotter, data paths, CNI config, optional registry mirror |
| `[postgres]` | PostgreSQL connection (host, port, user, password, database, sslmode) |
| `[qdrant]` | Qdrant vector database connection (base_url, api_key, timeout) |
| `[sparse]` | Sparse encoding service URL |
| `[registry]` | Provider definitions directory |
| `[browser_gateway]` | Browser Gateway host, port, and server address |
| `[web]` | Web frontend host and port |

## Common Commands

> Prefix with `sudo` on Linux if your user is not in the `docker` group.

```bash
docker compose up -d           # Start
docker compose down            # Stop
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
| `USE_CN_MIRROR`    | `false`            | Set to `true` to use China mainland image mirrors |
| `BROWSER_CORES`    | `chromium,firefox`  | Browser engines to include in the browser image |
| `BROWSER_TAG`      | `latest`           | Docker tag for the browser image |
