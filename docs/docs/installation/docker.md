# Docker Installation

Docker is the recommended way to run Memoh. The stack includes PostgreSQL, Qdrant, the main server (with embedded Containerd), agent gateway, and web UI — all orchestrated via Docker Compose. You do not need to install containerd, nerdctl, or buildkit on your host; everything runs inside containers.

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
2. Prompt for configuration (workspace, data directory, admin credentials, JWT secret, Postgres password, China mirror)
3. Clone the repository
4. Generate `config.toml` from the Docker template with your settings
5. Pull images and start all services

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

Then start:

```bash
sudo POSTGRES_PASSWORD=your-db-password docker compose up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

> **Important**: `docker-compose.yml` mounts `./config.toml` by default. You must create this file before starting — running without it will fail.

### China Mainland Mirror

For users in mainland China who cannot access Docker Hub directly, uncomment the `registry` line in `config.toml`:

```toml
[mcp]
registry = "memoh.cn"
```

And use the China mirror compose overlay:

```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml up -d
```

The install script handles this automatically when you answer "yes" to the China mirror prompt.

## Access Points

After startup:

| Service       | URL                    |
|---------------|------------------------|
| Web UI        | http://localhost:8082  |
| API           | http://localhost:8080  |
| Agent Gateway | http://localhost:8081  |

Default login: `admin` / `admin123` (change this in `config.toml`).

First startup may take 1–2 minutes while images are pulled and services initialize.

## Common Commands

> Prefix with `sudo` on Linux if your user is not in the `docker` group.

```bash
docker compose up -d           # Start
docker compose down            # Stop
docker compose logs -f         # View logs
docker compose ps              # Status
docker compose pull && docker compose up -d  # Update to latest images
```

## Production Checklist

1. **Passwords** — Change all default passwords and secrets in `config.toml`
2. **HTTPS** — Configure SSL (e.g. via `docker-compose.override.yml` with certs or a reverse proxy)
3. **Firewall** — Restrict access to necessary ports
4. **Resource limits** — Set memory/CPU limits for containers
5. **Backups** — Regular backups of Postgres and Qdrant data

## Troubleshooting

```bash
docker compose logs server      # View main service logs
docker compose config           # Validate configuration
docker compose build --no-cache && docker compose up -d  # Full rebuild
```

## Security Warnings

- The main service runs with privileged container access — only run in trusted environments
- You must change all default passwords and secrets before production use
- Use HTTPS in production
