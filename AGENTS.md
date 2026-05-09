# AGENTS.md

## Project Overview

Memoh is a multi-member, structured long-memory, containerized AI agent system platform. Users can create AI bots and chat with them via Telegram, Discord, Lark (Feishu), DingTalk, WeChat, Matrix, Email, and more. Every bot has an independent container and memory system, allowing it to edit files, execute commands, and build itself — providing a secure, flexible, and scalable solution for multi-bot management.

## Architecture Overview

The system consists of two core services:

| Service | Tech Stack | Port | Description |
|---------|-----------|------|-------------|
| **Server** (Backend) | Go + Echo | 8080 | Main service: REST API, auth, database, container management, **in-process AI agent** |
| **Web** (Frontend) | Vue 3 + Vite | 8082 | Management UI: visual configuration for Bots, Models, Channels, etc. |

Infrastructure dependencies:
- **PostgreSQL or SQLite** — Relational data storage
- **Qdrant** — Vector database for memory semantic search
- **Container runtime** — Isolated workspace containers per bot (Docker, Kubernetes, containerd, Apple Virtualization)

## Tech Stack

### Backend (Go)
- **Framework**: Echo (HTTP)
- **Dependency Injection**: Uber FX
- **AI SDK**: [Twilight AI](https://github.com/memohai/twilight-ai) (Go LLM SDK — OpenAI, Anthropic, Google)
- **Database Drivers**: pgx/v5 (PostgreSQL), modernc.org/sqlite (SQLite)
- **Code Generation**: sqlc (SQL → Go)
- **API Docs**: Swagger/OpenAPI (swaggo)
- **MCP**: modelcontextprotocol/go-sdk
- **Containers**: Docker / Kubernetes / containerd v2 / Apple Virtualization adapters
- **TUI**: Charm libraries (bubbletea, glamour, lipgloss) for CLI interactive mode

### Frontend (TypeScript)
- **Framework**: Vue 3 (Composition API)
- **Build Tool**: Vite 8
- **State Management**: Pinia 3 + Pinia Colada
- **UI**: Tailwind CSS 4 + custom component library (`@memohai/ui`) + Reka UI
- **Icons**: lucide-vue-next + `@memohai/icon` (brand/provider icons)
- **i18n**: vue-i18n
- **Markdown**: markstream-vue + Shiki + Mermaid + KaTeX
- **Desktop**: Electron + [electron-vite](https://electron-vite.github.io/) (thin shell whose renderer imports `@memohai/web`'s bootstrap)
- **Package Manager**: pnpm monorepo

### Tooling
- **Task Runner**: mise
- **Package Managers**: pnpm (frontend monorepo), Go modules (backend)
- **Linting**: golangci-lint (Go), ESLint + typescript-eslint + vue-eslint-parser (TypeScript)
- **Testing**: Vitest
- **Version Management**: bumpp
- **SDK Generation**: @hey-api/openapi-ts (with `@hey-api/client-fetch` + `@pinia/colada` plugins)

## Project Structure

```
Memoh/
├── cmd/                        # Go application entry points
│   ├── agent/                  #   Main backend server (main.go, FX wiring)
│   ├── bridge/                 #   In-container gRPC bridge (UDS-based, runs inside bot containers)
│   │   └── template/           #     Prompt templates for bridge (TOOLS.md, SOUL.md, IDENTITY.md, etc.)
│   ├── mcp/                    #   MCP stdio transport binary
│   └── memoh/                  #   Desktop companion CLI (Cobra: chat, tui, bots, start/stop/restart/status/logs, version) — bundled into Memoh.app, talks to the local 18731 server
├── internal/                   # Go backend core code (domain packages)
│   ├── accounts/               #   User account management (CRUD, password hashing)
│   ├── acl/                    #   Access control list (source-aware chat trigger ACL)
│   ├── agent/                  #   In-process AI agent (Twilight AI SDK integration)
│   │   ├── agent.go            #     Core agent: Stream() / Generate() via Twilight SDK
│   │   ├── stream.go           #     Streaming event assembly
│   │   ├── sential.go          #     Sential (sentinel) loop detection logic
│   │   ├── prompt.go           #     Prompt assembly (system, heartbeat, schedule, subagent, discuss)
│   │   ├── config.go           #     Agent service dependencies
│   │   ├── types.go            #     Shared types (StreamEvent, GenerateResult, FileAttachment)
│   │   ├── fs.go               #     Filesystem utilities
│   │   ├── guard_state.go      #     Guard state management
│   │   ├── retry.go            #     Retry logic
│   │   ├── read_media.go       #     Media reading utilities
│   │   ├── spawn_adapter.go    #     Spawn adapter for sub-processes
│   │   ├── prompts/            #     Prompt templates (Markdown, with partials prefixed by _)
│   │   │   ├── system_chat.md, system_discuss.md, system_heartbeat.md, system_schedule.md, system_subagent.md
│   │   │   ├── _tools.md, _memory.md, _contacts.md, _schedule_task.md, _subagent.md
│   │   │   ├── heartbeat.md, schedule.md
│   │   │   └── memory_extract.md, memory_update.md
│   │   └── tools/              #     Tool providers (ToolProvider interface)
│   │       ├── message.go      #       Send message tool
│   │       ├── contacts.go     #       Contact list tool
│   │       ├── schedule.go     #       Schedule management tool
│   │       ├── memory.go       #       Memory read/write tool
│   │       ├── web.go          #       Web search tool
│   │       ├── webfetch.go     #       Web page fetch tool
│   │       ├── container.go    #       Container file/exec tools
│   │       ├── fsops.go        #       Filesystem operations tool
│   │       ├── email.go        #       Email send tool
│   │       ├── subagent.go     #       Sub-agent invocation tool
│   │       ├── skill.go        #       Skill activation tool
│   │       ├── tts.go          #       Text-to-speech tool
│   │       ├── federation.go   #       MCP federation tool
│   │       ├── image_gen.go    #       Image generation tool
│   │       ├── prune.go        #       Pruning tool
│   │       ├── history.go      #       History access tool
│   │       └── read_media.go   #       Media reading tool
│   ├── attachment/             #   Attachment normalization (MIME types, base64)
│   ├── auth/                   #   JWT authentication middleware and utilities
│   ├── bind/                   #   Channel identity-to-user binding code management
│   ├── boot/                   #   Runtime configuration provider (container backend detection)
│   ├── bots/                   #   Bot management (CRUD, lifecycle)
│   ├── channel/                #   Channel adapter system
│   │   ├── adapters/           #     Platform adapters: telegram, discord, feishu, qq, dingtalk, weixin, wecom, wechatoa, matrix, misskey, local
│   │   └── identities/        #     Channel identity service
│   ├── command/                #   Slash command system (extensible command handlers)
│   ├── compaction/             #   Message history compaction service (LLM summarization)
│   ├── config/                 #   Configuration loading and parsing (TOML + YAML providers)
│   ├── container/              #   Container runtime abstraction + adapters (containerd, Apple, Docker, Kubernetes)
│   ├── conversation/           #   Conversation management and flow resolver
│   │   ├── service.go          #     Conversation CRUD and routing
│   │   └── flow/               #     Chat orchestration (resolver, streaming, memory, triggers)
│   ├── copilot/                #   GitHub Copilot client integration
│   ├── db/                     #   Database connection and migration utilities
│   │   └── sqlc/               #   ⚠️ Auto-generated by sqlc — DO NOT modify manually
│   ├── email/                  #   Email provider and outbox management (Mailgun, generic SMTP, OAuth)
│   ├── embedded/               #   Embedded filesystem assets (web only)
│   ├── handlers/               #   HTTP request handlers (REST API endpoints)
│   ├── healthcheck/            #   Health check adapter system (MCP, channel checkers)
│   ├── heartbeat/              #   Heartbeat scheduling service (cron-based)
│   ├── identity/               #   Identity type utilities (human vs bot)
│   ├── logger/                 #   Structured logging (slog)
│   ├── mcp/                    #   MCP protocol manager (connections, OAuth, tool gateway)
│   ├── media/                  #   Content-addressed media asset service
│   ├── memory/                 #   Long-term memory system (multi-provider: Qdrant, BM25, LLM extraction)
│   ├── message/                #   Message persistence and event publishing
│   ├── messaging/              #   Outbound message executor
│   ├── models/                 #   LLM model management (CRUD, variants, client types, probe)
│   ├── oauthctx/               #   OAuth context helpers
│   ├── pipeline/               #   Discuss/chat pipeline (adapt, projection, rendering, driver)
│   ├── policy/                 #   Access policy resolution (guest access)
│   ├── providers/              #   LLM provider management (OpenAI, Anthropic, etc.)
│   ├── prune/                  #   Text pruning utilities (truncation with head/tail)
│   ├── registry/               #   Provider registry service (YAML provider templates)
│   ├── schedule/               #   Scheduled task service (cron)
│   ├── searchproviders/        #   Search engine provider management (Brave, etc.)
│   ├── server/                 #   HTTP server wrapper (Echo setup, middleware, shutdown)
│   ├── session/                #   Bot session management service
│   ├── settings/               #   Bot settings management
│   ├── storage/                #   Storage provider interface (filesystem, container FS)
│   ├── textutil/               #   UTF-8 safe text utilities
│   ├── timezone/               #   Timezone utilities
│   ├── tts/                    #   Text-to-speech provider management
│   ├── tui/                    #   Terminal UI (Charm stack for CLI interactive mode)
│   ├── version/                #   Build-time version information
│   └── workspace/              #   Workspace container lifecycle management
│       ├── manager.go          #     Container reconciliation, gRPC connection pool
│       ├── manager_lifecycle.go #    Container create/start/stop operations
│       ├── bridge/             #     gRPC client for in-container bridge service
│       └── bridgepb/           #     Protobuf definitions (bridge.proto)
├── apps/                       # Application services
│   ├── desktop/                #   Electron desktop app (@memohai/desktop, electron-vite; renderer imports @memohai/web)
│   └── web/                    #   Main web app (@memohai/web, Vue 3) — see apps/web/AGENTS.md
├── packages/                   # Shared TypeScript libraries
│   ├── ui/                     #   Shared UI component library (@memohai/ui)
│   ├── sdk/                    #   TypeScript SDK (@memohai/sdk, auto-generated from OpenAPI)
│   ├── icons/                  #   Brand/provider icon library (@memohai/icon)
│   └── config/                 #   Shared configuration utilities (@memohai/config)
├── spec/                       # OpenAPI specifications (swagger.json, swagger.yaml)
├── db/                         # Database
│   ├── postgres/               #   PostgreSQL SQL resources
│   │   ├── migrations/         #   SQL migration files (0001–0067+)
│   │   └── queries/            #   SQL query files (sqlc input)
│   └── sqlite/                 #   SQLite SQL resources (parallel backend track)
│       ├── migrations/         #   SQLite migration files
│       └── queries/            #   SQLite query files (sqlc input)
├── conf/                       # Configuration
│   ├── providers/              #   Provider YAML templates (openai, anthropic, codex, github-copilot, etc.)
│   ├── app.example.toml        #   Default config template
│   ├── app.docker.toml         #   Docker deployment config
│   ├── app.apple.toml          #   macOS (Apple Virtualization) config
│   └── app.windows.toml        #   Windows config
├── devenv/                     # Dev environment
│   ├── docker-compose.yml      #   Main dev compose
│   ├── docker-compose.minify.yml #  Minified services compose
│   ├── docker-compose.selinux.yml # SELinux overlay compose
│   └── app.dev.toml            #   Dev config (connects to devenv docker-compose)
├── deploy/
│   ├── kubernetes/             # Kubernetes kustomize starter deployment
│   └── kubernetes-local/       # Local K8s overlay using k8s-dev image tags
├── docker/                     # Production Docker (Dockerfiles, entrypoints, nginx.conf, toolkit/)
├── docs/                       # Documentation site (VitePress)
├── scripts/                    # Utility scripts (db-up, db-drop, release, install, sync-openrouter-models)
├── docker-compose.yml          # Docker Compose orchestration (production)
├── mise.toml                   # mise tasks and tool version definitions
├── sqlc.yaml                   # sqlc code generation config
├── openapi-ts.config.ts        # SDK generation config (@hey-api/openapi-ts)
├── bump.config.ts              # Version bumping config (bumpp)
├── vitest.config.ts            # Test framework config (Vitest)
├── tsconfig.json               # TypeScript monorepo config
└── eslint.config.mjs           # ESLint config
```

## Development Guide

### Prerequisites

1. Install [mise](https://mise.jdx.dev/)
2. Install toolchains and dependencies: `mise install`
3. Initialize the project: `mise run setup`
4. Start the dev environment: `mise run dev`
5. Dev web UI: `http://localhost:18082` (server: `18080`)

### Common Commands

| Command | Description |
|---------|-------------|
| `mise run dev` | Start the containerized dev environment (all services) |
| `mise run dev:minify` | Start dev environment with minified services |
| `mise run dev:sqlite` | Start SQLite-backed development environment |
| `mise run dev:sqlite:minify` | Start SQLite-backed development environment with minified services |
| `mise run dev:selinux` | Start dev environment on SELinux systems |
| `mise run dev:down` | Stop the dev environment |
| `mise run dev:down:sqlite` | Stop SQLite development environment |
| `mise run dev:logs` | View dev environment logs |
| `mise run dev:logs:sqlite` | View SQLite development logs |
| `mise run dev:restart` | Restart a service (e.g. `-- server`) |
| `mise run dev:restart:sqlite` | Restart a SQLite dev service (e.g. `-- server`) |
| `mise run setup` | Install dependencies + workspace toolkit |
| `mise run sqlc-generate` | Regenerate Go code after modifying SQL files |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sdk-generate` | Generate TypeScript SDK (depends on swagger-generate) |
| `mise run icons-generate` | Generate icon Vue components from SVG sources |
| `mise run db-up` | Initialize and migrate the database |
| `mise run db-down` | Drop the database |
| `mise run docs` | Start documentation dev server |
| `mise run build-embedded-assets` | Build and stage embedded web assets |
| `mise run build-unified` | Build memoh CLI locally |
| `mise run bridge:build` | Rebuild bridge binary in dev container |
| `mise run desktop:dev` | Start Electron desktop app in dev mode (renderer reuses @memohai/web) |
| `mise run desktop:build` | Build Electron desktop app for release (electron-builder) |
| `mise run lint` | Run all linters (Go + ESLint) |
| `mise run lint:fix` | Run all linters with auto-fix |
| `mise run release` | Release new version (bumpp) |
| `mise run install-socktainer` | Install socktainer (macOS container backend) |
| `mise run install-workspace-toolkit` | Install workspace toolkit (bridge binary etc.) |

### Docker Deployment

```bash
docker compose up -d        # Start all services
# Visit http://localhost:8082
```

Production services: `postgres`, `migrate`, `server`, `web`.
Optional profiles: `qdrant` (vector DB), `sparse` (BM25 search).

## Key Development Rules

### Database, sqlc & Migrations

1. **PostgreSQL SQL queries** are defined in `db/postgres/queries/*.sql`; **SQLite SQL queries** live in `db/sqlite/queries/*.sql`.
2. All Go files under `internal/db/postgres/sqlc/` and `internal/db/sqlite/sqlc/` are auto-generated by sqlc. **DO NOT modify them manually.**
3. **Always update both database backends together.** Any schema or query change must update the PostgreSQL and SQLite equivalents in the same change unless the code path is explicitly backend-specific and documented.
4. After modifying any SQL files (migrations or queries), run `mise run sqlc-generate` to update both generated Go packages.

#### Migration Rules

PostgreSQL migrations live in `db/postgres/migrations/` and follow a dual-update convention:

- **PostgreSQL `0001_init.up.sql` is the canonical full PostgreSQL schema.** It always contains the complete, up-to-date PostgreSQL database definition (all tables, indexes, constraints, etc.). When adding PostgreSQL schema changes, you must **also update `db/postgres/migrations/0001_init.up.sql`** to reflect the final state.
- **SQLite `0001_init.up.sql` is the canonical full SQLite schema.** SQLite currently uses a single baseline migration at `db/sqlite/migrations/0001_init.up.sql`; when adding schema changes, update this file and its paired down migration.
- **Incremental PostgreSQL migration files** (`0002_`, `0003_`, ...) contain only the diff needed to upgrade an existing PostgreSQL database. They exist for environments that already have the schema and need to apply only the delta.
- **Both PostgreSQL and SQLite must be kept in sync**: every schema change requires updating PostgreSQL `0001_init.up.sql`, adding the next PostgreSQL incremental migration pair, and updating SQLite `0001_init.up.sql` / `0001_init.down.sql` to the equivalent final schema.
- **Both query sets must be kept in sync**: every query change in `db/postgres/queries/*.sql` must have an equivalent SQLite query change in `db/sqlite/queries/*.sql`, with dialect differences handled deliberately (`jsonb` vs JSON1, casts, `ILIKE`, `FOR UPDATE`, date/time functions, arrays).
- **Naming**: `{NNNN}_{description}.up.sql` and `{NNNN}_{description}.down.sql`, where `{NNNN}` is a zero-padded sequential number (e.g., `0005`). Always use the next available number.
- **Paired files**: Every incremental migration **must** have both an `.up.sql` (apply) and a `.down.sql` (rollback) file.
- **Header comment**: Each file should start with a comment indicating the migration name and a brief description:
  ```sql
  -- 0005_add_feature_x
  -- Add feature_x column to bots table for ...
  ```
- **Idempotent DDL**: Use `IF NOT EXISTS` / `IF EXISTS` guards (e.g., `CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `DROP TABLE IF EXISTS`) so migrations are safe to re-run.
- **Down migration must fully reverse up**: The `.down.sql` must cleanly undo everything its `.up.sql` does, in reverse order.
- **After creating or modifying migrations**, run `mise run sqlc-generate` to regenerate both Go SQLC packages, then validate both migration tracks (`mise run db-up` for PostgreSQL and SQLite migration/dev tasks where relevant).

### API Development Workflow

1. Write handlers in `internal/handlers/` with swaggo annotations.
2. Run `mise run swagger-generate` to update the OpenAPI docs (output in `spec/`).
3. Run `mise run sdk-generate` to update the frontend TypeScript SDK (`packages/sdk/`).
4. The frontend calls APIs via the auto-generated `@memohai/sdk`.

### Agent Development

- The AI agent runs **in-process** within the Go server — there is no separate agent gateway service.
- Core agent logic lives in `internal/agent/`, powered by the [Twilight AI](https://github.com/memohai/twilight-ai) Go SDK.
- `internal/agent/agent.go` provides `Stream()` (SSE streaming) and `Generate()` (non-streaming) methods.
- Model/client types are defined in `internal/models/types.go`: `openai-completions`, `openai-responses`, `anthropic-messages`, `google-generative-ai`, `openai-codex`, `github-copilot`, `edge-speech`.
- Model types: `chat`, `embedding`, `speech`.
- Tools are implemented as `ToolProvider` instances in `internal/agent/tools/`, loaded via setter injection to avoid FX dependency cycles.
- Prompt templates are embedded Go Markdown files in `internal/agent/prompts/`. Partials (reusable fragments) are prefixed with `_` (e.g., `_tools.md`, `_memory.md`). System prompts include `system_chat.md` (standard chat) and `system_discuss.md` (discuss mode).
- The conversation flow resolver (`internal/conversation/flow/`) orchestrates message assembly, memory injection, history trimming, and agent invocation.
- The discuss/chat pipeline (`internal/pipeline/`) provides an alternative orchestration path with adaptation, projection, rendering, and driver layers.
- The compaction service (`internal/compaction/`) handles LLM-based conversation summarization.
- Loop detection (text and tool loops) is built into the agent with configurable thresholds.
- Tag extraction system processes inline tags in streaming output (attachments, reactions, speech/TTS).

### Frontend Development

- Use Vue 3 Composition API with `<script setup>` style.
- Shared components belong in `packages/ui/`.
- API calls use the auto-generated `@memohai/sdk`.
- State management uses Pinia; data fetching uses Pinia Colada.
- i18n via vue-i18n.
- See `apps/web/AGENTS.md` for detailed frontend conventions.

### Desktop App

- `apps/desktop/` is an [electron-vite](https://electron-vite.github.io/) project (`@memohai/desktop`).
- The renderer is intentionally a **thin shell**: `src/renderer/src/main.ts` is a single-line `import '@memohai/web/main'` that defers the full bootstrap (router, Pinia, api-client, `App.vue`) to `@memohai/web`.
- `@memohai/web`'s `package.json` exposes an `exports` map (`./main`, `./App.vue`, `./style.css`, `./*`) so downstream consumers can reuse web modules.
- `electron.vite.config.ts` mirrors `apps/web/vite.config.ts`: same `@` / `#` path aliases, same `/api` proxy (driven by `MEMOH_WEB_PROXY_TARGET` / `config.toml` via `@memohai/config`).
- Packaging is handled by `electron-builder` (config in `apps/desktop/electron-builder.yml`); output lands in `apps/desktop/dist/`.
- The Memoh CLI (`cmd/memoh/`) is bundled into the app at `Resources/cli/memoh` next to `Resources/server/memoh-server`. On first launch (and via the `Install Command Line Tool…` menu item) the main process offers to add `memoh` to PATH (`/usr/local/bin/memoh` symlink on macOS, `~/.local/bin/memoh` on Linux, HKCU PATH on Windows). The CLI talks to the local server at `127.0.0.1:18731`, self-logs in with the `[admin]` credentials in `userData/config.toml`, and shares the same pid file (`local-server.pid.json`) so either side can `start`/`stop` the server. See `apps/desktop/AGENTS.md` § Bundled CLI.
- `productName` is pinned to `Memoh` so userData lives at `~/Library/Application Support/Memoh/` (macOS), `%APPDATA%\Memoh\` (Windows), `~/.config/Memoh/` (Linux). The Go CLI hard-codes the same product name in `internal/tui/local/paths.go`; if you ever rename, both sides must change together.
- When desktop needs to diverge from the web experience, replace the re-export in `renderer/src/main.ts` with an inline copy of web's `main.ts` and customize from there — do **not** fork `apps/web` itself.

### Container / Workspace Management

- Each bot can have an isolated **workspace container** for file editing, command execution, and MCP tool hosting.
- Containers communicate with the host via a **gRPC bridge** over Unix Domain Sockets (UDS), not TCP.
- The bridge binary (`cmd/bridge/`) runs inside each container, mounting runtime binaries from `$WORKSPACE_RUNTIME_DIR` and UDS sockets from `/run/memoh/`. Bridge prompt templates live in `cmd/bridge/template/`.
- Container images are standard base images (debian, alpine, ubuntu, etc.) — no dedicated MCP Docker image needed.
- `internal/workspace/` manages container lifecycle (create, start, stop, reconcile) and maintains a gRPC connection pool.
- `internal/container/` provides the container runtime abstraction layer and adapter subpackages (`containerd`, `apple`, `docker`, `k8s`).
- SSE-based progress feedback is provided during container image pull and creation.

## Database Tables

The canonical source of truth for the full PostgreSQL schema is `db/postgres/migrations/0001_init.up.sql`. Key tables grouped by domain:

**Auth & Users**
- `users` — User accounts (username, email, role, display_name, avatar)
- `channel_identities` — Unified inbound identity subject (cross-platform)
- `user_channel_bindings` — Outbound delivery config per user/channel

**Bots & Sessions**
- `bots` — Bot definitions with model references and settings
- `bot_sessions` — Bot conversation sessions
- `bot_session_events` — Session event log
- `bot_channel_configs` — Per-bot channel configurations
- `bot_channel_routes` — Conversation route mapping (inbound thread → bot history)
- `bot_acl_rules` — Source-aware chat access control lists

**Messages & History**
- `bot_history_messages` — Unified message history under bot scope
- `bot_history_message_assets` — Message → content_hash asset links (with name and metadata)
- `bot_history_message_compacts` — Compacted message summaries

**Providers & Models**
- `providers` — LLM provider configurations (name, base_url, api_key)
- `provider_oauth_tokens` — Provider-level OAuth tokens
- `user_provider_oauth_tokens` — Per-user provider OAuth tokens
- `models` — Model definitions (chat/embedding/speech types, modalities, reasoning)
- `model_variants` — Model variant definitions (weight, metadata)
- `search_providers` — Search engine provider configurations
- `memory_providers` — Multi-provider memory adapter configurations

**MCP**
- `mcp_connections` — MCP connection configurations per bot
- `mcp_oauth_tokens` — MCP OAuth tokens

**Containers**
- `containers` — Bot container instances
- `snapshots` — Container snapshots
- `container_versions` — Container version tracking
- `lifecycle_events` — Container lifecycle events

**Email**
- `email_providers` — Pluggable email service backends (Mailgun, generic SMTP)
- `email_oauth_tokens` — OAuth2 tokens for email providers (Gmail)
- `bot_email_bindings` — Per-bot email provider binding with permissions
- `email_outbox` — Outbound email audit log

**Scheduling & Automation**
- `schedule` — Scheduled tasks (cron)
- `schedule_logs` — Schedule execution logs
- `bot_heartbeat_logs` — Heartbeat execution records
**Storage**
- `storage_providers` — Pluggable object storage backends
- `bot_storage_bindings` — Per-bot storage backend selection

## Configuration

The main configuration file is `config.toml` (copied from `conf/app.example.toml` or environment-specific templates for development), containing:

- `[log]` — Logging configuration (level, format)
- `[server]` — HTTP listen address
- `[admin]` — Admin account credentials
- `[auth]` — JWT authentication settings
- `[database]` — Database backend selection (`postgres` or `sqlite`)
- `[container]` — Workspace backend selection (`docker`, `kubernetes`, `containerd`, `apple`) and common workspace image/data/runtime/CNI settings
- `[containerd]` / `[docker]` / `[kubernetes]` / `[apple]` — Backend-specific runtime configuration
- `[postgres]` — PostgreSQL connection
- `[sqlite]` — SQLite database file and WAL/lock settings
- `[qdrant]` — Qdrant vector database connection
- `[sparse]` — Sparse (BM25) search service connection
- `[web]` — Web frontend address
- `[registry]` — Provider registry (`providers_dir` pointing to `conf/providers/`)
- `[supermarket]` — Supermarket integration (base_url)

Provider YAML templates in `conf/providers/` define preset configurations for various LLM providers (OpenAI, Anthropic, GitHub Copilot, etc.).

Configuration templates available in `conf/`:
- `app.example.toml` — Default template
- `app.docker.toml` — Docker deployment
- `app.apple.toml` — macOS (Apple Virtualization backend)
- `app.windows.toml` — Windows

Development configuration in `devenv/`:
- `app.dev.toml` — Development (connects to devenv docker-compose)

## Web Design

Please refer to `./apps/web/AGENTS.md`.
