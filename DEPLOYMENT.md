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
sudo docker compose up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

> **Important**: You must create `config.toml` before starting. `docker-compose.yml` mounts `./config.toml` into the containers — running without it will fail.

Access:
- Web UI: http://localhost:8082
- API: http://localhost:8080
- Agent: http://localhost:8081

Default credentials: `admin` / `admin123` (change in `config.toml`)

## Prerequisites

- Docker (with Docker Compose v2)
- Git

## Configuration

`config.toml` is generated from `conf/app.docker.toml` and should live in the project root. It is mounted into all containers at startup and is **not** tracked by git.

Recommended changes for production:
- `admin.password` — Admin password
- `auth.jwt_secret` — JWT secret (generate with `openssl rand -base64 32`)
- `postgres.password` — Database password (also set `POSTGRES_PASSWORD` env var)

### China Mainland Mirror

Uncomment `registry = "memoh.cn"` in `config.toml` under `[mcp]`, then use:

```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml up -d
```

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
