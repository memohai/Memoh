# Memoh Deployment Guide

## One-Click Install

```bash
curl -fsSL https://raw.githubusercontent.com/memohai/Memoh/main/scripts/install.sh | sh
```

Or manually:

```bash
git clone https://github.com/memohai/Memoh.git
cd Memoh
docker compose up -d
```

Access:
- Web UI: http://localhost
- API: http://localhost:8080
- Agent: http://localhost:8081

Default credentials: `admin` / `admin123`

## Prerequisites

- Docker (with Docker Compose v2)
- Git

That's it. No containerd, nerdctl, or buildkit required on the host.

## Custom Configuration

```bash
cp docker/config/config.docker.toml config.toml
nano config.toml
MEMOH_CONFIG=./config.toml docker compose up -d
```

Recommended changes for production:
- `admin.password` - Admin password
- `auth.jwt_secret` - JWT secret (generate with `openssl rand -base64 32`)
- `postgres.password` - Database password

## Common Commands

```bash
docker compose up -d          # Start
docker compose down           # Stop
docker compose logs -f        # View logs
docker compose ps             # Status
docker compose up -d --build  # Rebuild and restart
```

## Production

1. Configure HTTPS (create `docker-compose.override.yml` with SSL certs)
2. Change all default passwords
3. Configure firewall
4. Set resource limits
5. Regular backups

## Troubleshooting

```bash
docker compose logs server    # View service logs
docker compose logs containerd # View containerd logs
docker compose config         # Check configuration
docker compose build --no-cache && docker compose up -d  # Full rebuild
```

## Security Warnings

- Main service has privileged container access - only run in trusted environments
- Must change all default passwords and secrets
- Use HTTPS in production
