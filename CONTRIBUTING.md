# Contributing Guide

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (for dev infrastructure)
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
mise run setup     # Start infra + copy config + migrate DB + install deps
mise run dev       # Start server + agent + web (all hot-reload)
```

That's it. `setup` handles everything automatically:
1. Copies `conf/app.dev.toml` → `config.toml` (if not exists)
2. Starts PostgreSQL + Qdrant via `devenv/docker-compose.yml`
3. Runs database migrations
4. Installs Go/Node/pnpm dependencies

## Daily Development

```bash
mise run dev       # Start all services with hot-reload
```

## Infrastructure

```bash
mise run infra        # Start dev postgres + qdrant
mise run infra-down   # Stop dev infrastructure
mise run infra-logs   # View infrastructure logs
```

## More Commands

| Command | Description |
| ------- | ----------- |
| `mise run dev` | Start development environment |
| `mise run setup` | Full setup (infra + config + migrate + deps) |
| `mise run infra` | Start dev infrastructure only |
| `mise run infra-down` | Stop dev infrastructure |
| `mise run db-up` | Run database migrations |
| `mise run db-down` | Roll back database migrations |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sdk-generate` | Generate TypeScript SDK |
| `mise run sqlc-generate` | Generate SQL code |
| `mise run //agent:dev` | Start agent gateway only |
| `mise run //cmd/agent:start` | Start main server only |
| `mise run //packages/web:dev` | Start web dev server only |

## Project Layout

```
conf/       — Configuration templates (app.example.toml, app.dev.toml, app.docker.toml)
devenv/     — Development infrastructure (docker-compose for postgres + qdrant)
docker/     — Production Docker build & runtime (Dockerfiles, entrypoints)
cmd/        — Go application entry points
internal/   — Go backend core code
agent/      — Agent Gateway (Bun/Elysia)
packages/   — Frontend monorepo (web, ui, sdk, cli, config)
db/         — Database migrations and queries
scripts/    — Utility scripts
```
