# Memoh Deployment Guide

## One-Click Install

```bash
curl -fsSL https://memoh.sh | sudo sh
```

The script prompts for configuration, generates `config.toml`, and starts all services.

## Manual Install

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
nano config.toml   # Change passwords and JWT secret
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

> **Important**: You must create `config.toml` before starting. `docker-compose.yml` mounts `./config.toml` into the containers — running without it will fail.

### Standard startup (with Qdrant + Browser)

```bash
sudo docker compose \
  -f docker-compose.yml \
  -f docker/docker-compose.qdrant.yml \
  -f docker/docker-compose.browser.yml \
  up -d
```

### Minimal startup (core only)

```bash
sudo docker compose up -d
```

Access:
- Web UI: http://localhost:8082
- API: http://localhost:8080
- Agent: http://localhost:8081

Default credentials: `admin` / `admin123` (change in `config.toml`)

## Docker Compose Overlays

The base `docker-compose.yml` contains only essential services (postgres, server, agent, web). Optional capabilities are added via overlay files:

| Overlay | File | Description |
|---------|------|-------------|
| Qdrant | `docker/docker-compose.qdrant.yml` | Vector database for memory semantic search |
| Browser | `docker/docker-compose.browser.yml` | Browser automation gateway (Playwright) |
| Mem0 | `docker/docker-compose.mem0.yml` | Self-hosted Mem0 memory provider |
| OpenViking | `docker/docker-compose.openviking.yml` | Self-hosted OpenViking memory provider |

### Supported combinations

```bash
# Core + Qdrant + Browser (recommended default)
docker compose -f docker-compose.yml \
  -f docker/docker-compose.qdrant.yml \
  -f docker/docker-compose.browser.yml up -d

# Core + Qdrant + Mem0 (self-hosted)
docker compose -f docker-compose.yml \
  -f docker/docker-compose.qdrant.yml \
  -f docker/docker-compose.mem0.yml up -d

# Core + Qdrant + OpenViking (self-hosted)
docker compose -f docker-compose.yml \
  -f docker/docker-compose.qdrant.yml \
  -f docker/docker-compose.openviking.yml up -d
```

### SaaS / external providers

For Mem0 or OpenViking SaaS, no overlay is needed. Configure the provider directly in the Memoh admin UI with the external `base_url` and API key.

### China Mainland Mirror

Uncomment `registry = "memoh.cn"` in `config.toml` under `[mcp]`, then add:

```bash
sudo docker compose -f docker-compose.yml \
  -f docker/docker-compose.qdrant.yml \
  -f docker/docker-compose.browser.yml \
  -f docker/docker-compose.cn.yml up -d
```

## Prerequisites

- Docker (with Docker Compose v2)
- Git

## Configuration

`config.toml` is generated from `conf/app.docker.toml` and should live in the project root. It is mounted into all containers at startup and is **not** tracked by git.

Recommended changes for production:
- `admin.password` — Admin password
- `auth.jwt_secret` — JWT secret (generate with `openssl rand -base64 32`)
- `postgres.password` — Database password (also set `POSTGRES_PASSWORD` env var)

## Common Commands

> Prefix with `sudo` on Linux if your user is not in the `docker` group.

```bash
docker compose up -d          # Start
docker compose down           # Stop
docker compose logs -f        # View logs
docker compose ps             # Status
docker compose pull && docker compose up -d  # Update images
```

## Production

1. Change all default passwords and secrets
2. Configure HTTPS (reverse proxy or `docker-compose.override.yml` with SSL)
3. Configure firewall
4. Set resource limits
5. Regular backups

## Troubleshooting

```bash
docker compose logs server    # View service logs
docker compose config         # Check configuration
docker compose build --no-cache && docker compose up -d  # Full rebuild
```

## Security Warnings

- Main service has privileged container access — only run in trusted environments
- Must change all default passwords and secrets
- Use HTTPS in production
