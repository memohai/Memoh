# Docker Installation

Docker is the recommended way to run Memoh. The stack includes PostgreSQL, Qdrant, Containerd, the main server, agent gateway, and web UI — all orchestrated via Docker Compose. You do not need to install containerd, nerdctl, or buildkit on your host; everything runs inside containers.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose v2](https://docs.docker.com/compose/install/)
- Git

## One-Click Install

Run the official install script (requires Docker and Docker Compose):

```bash
curl -fsSL https://raw.githubusercontent.com/memohai/Memoh/main/scripts/install.sh | sudo sh
```

The script will:

1. Check for Docker and Docker Compose
2. Prompt for configuration (workspace, data directory, admin credentials, JWT secret, Postgres password)
3. Clone the repository
4. Generate `config.toml` from the Docker template
5. Start all services with `docker compose up -d --build`

**Silent install** (use all defaults, no prompts):

```bash
curl -fsSL https://raw.githubusercontent.com/memohai/Memoh/main/scripts/install.sh | sudo sh -s -- -y
```

Defaults when running silently:

- Workspace: `~/memoh`
- Data directory: `~/memoh/data`
- Admin: `admin` / `admin123`
- JWT secret: auto-generated
- Postgres password: `memoh123`

## Manual Install

Clone the repository and start with Docker Compose:

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
sudo docker compose up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

By default, Docker Compose uses `conf/app.docker.toml`. No config file in the project root is mounted — only this built-in config is used. See [config.toml reference](./config-toml) for all configuration fields.

## Access Points

After startup:

| Service       | URL                    |
|---------------|------------------------|
| Web UI        | http://localhost:8082  |
| API           | http://localhost:8080  |
| Agent Gateway | http://localhost:8081  |

Default login: `admin` / `admin123`

First startup may take 1–2 minutes while images build and services initialize.

## Custom Configuration

To use your own config file:

1. Copy the Docker config template and edit it. See [config.toml reference](./config-toml) for field descriptions:

```bash
cp conf/app.docker.toml config.toml
nano config.toml
```

2. Point `MEMOH_CONFIG` at your config when starting (path is on the host; run `docker compose` from the project root):

```bash
sudo MEMOH_CONFIG=./config.toml docker compose up -d
```

**Recommended changes for production** (see [config.toml reference](./config-toml) for details):

- `admin.password` — Change the admin password
- `auth.jwt_secret` — Generate with `openssl rand -base64 32`
- `postgres.password` — Change the database password (and set `POSTGRES_PASSWORD` when running `docker compose`)

## Common Commands

> Prefix with `sudo` on Linux if your user is not in the `docker` group.

```bash
docker compose up -d           # Start
docker compose down            # Stop
docker compose logs -f         # View logs
docker compose ps              # Status
docker compose up -d --build   # Rebuild and restart
```

## Production Checklist

1. **HTTPS** — Configure SSL (e.g. via `docker-compose.override.yml` with certs)
2. **Passwords** — Change all default passwords and secrets
3. **Firewall** — Restrict access to necessary ports
4. **Resource limits** — Set memory/CPU limits for containers
5. **Backups** — Regular backups of Postgres and Qdrant data

## Troubleshooting

```bash
docker compose logs server      # View main service logs
docker compose logs containerd # View containerd logs
docker compose config          # Validate configuration
docker compose build --no-cache && docker compose up -d  # Full rebuild
```

## Security Warnings

- The main service runs with privileged container access — only run in trusted environments
- You must change all default passwords and secrets before production use
- Use HTTPS in production
