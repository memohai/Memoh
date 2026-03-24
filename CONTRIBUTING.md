# Contributing Guide

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (for containerized dev environment)
- [mise](https://mise.jdx.dev/) (task runner & toolchain manager)

### Install mise

```bash
# macOS / Linux
curl https://mise.run | sh
# or
brew install mise

# Windows
winget install jdx.mise
```

## Quick Start

```bash
mise install       # Install toolchains (Go, Node, Bun, pnpm, sqlc)
./docker/toolkit/install.sh  # Install toolkit used by the nested workspace runtime
mise run setup     # Install deps and prepare local tooling
mise run dev       # Start full containerized dev environment
```

That's it. `dev` launches everything in Docker containers:
1. PostgreSQL + Qdrant (infrastructure)
2. Database migrations (auto-run on startup)
3. Go server with containerd (hot-reload via `go run`)
4. Agent Gateway (Bun, hot-reload)
5. Web frontend (Vite, hot-reload)

The dev stack uses `devenv/app.dev.toml` directly and no longer overwrites the repo root `config.toml`.
Default host ports are shifted away from the production compose stack: Web `18082`, API `18080`, Agent `18081`, Postgres `15432`, Qdrant `16333`/`16334`, Sparse `18085`.

## Daily Development

```bash
mise run dev             # Start all services
mise run dev:down        # Stop all services
mise run dev:logs        # View logs
mise run dev:restart -- server  # Restart a specific service
```

## More Commands

| Command | Description |
| ------- | ----------- |
| `mise run dev` | Start containerized dev environment |
| `mise run dev:down` | Stop dev environment |
| `mise run dev:logs` | View dev logs |
| `mise run dev:restart` | Restart a service (e.g. `-- server`) |
| `mise run setup` | Install deps and prepare local tooling |
| `mise run db-up` | Run database migrations |
| `mise run db-down` | Roll back database migrations |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sdk-generate` | Generate TypeScript SDK |
| `mise run sqlc-generate` | Generate SQL code |

## Project Layout

```
conf/       — Configuration templates (app.example.toml, app.docker.toml)
devenv/     — Dev environment (docker-compose, dev Dockerfiles, app.dev.toml, bridge-build.sh)
docker/     — Production Docker build & runtime (Dockerfiles, entrypoints)
cmd/        — Go application entry points
internal/   — Go backend core code
apps/       — Application services (Agent Gateway, etc.)
  agent/    — Agent Gateway (Bun/Elysia)
packages/   — Frontend monorepo (web, ui, sdk, config)
db/         — Database migrations and queries
scripts/    — Utility scripts
```
