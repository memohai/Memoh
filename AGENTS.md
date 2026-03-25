# AGENTS.md

## Project Overview

Memoh is a multi-member, structured long-memory, containerized AI agent system platform. Users can create AI bots and chat with them via Telegram, Discord, Lark (Feishu), Email, and more. Every bot has an independent container and memory system, allowing it to edit files, execute commands, and build itself — providing a secure, flexible, and scalable solution for multi-bot management.

## Architecture Overview

The system consists of three core services:

| Service | Tech Stack | Port | Description |
|---------|-----------|------|-------------|
| **Server** (Backend) | Go + Echo | 8080 | Main service: REST API, auth, database, container management, **in-process AI agent** |
| **Web** (Frontend) | Vue 3 + Vite | 8082 | Management UI: visual configuration for Bots, Models, Channels, etc. |
| **Browser Gateway** | Bun + Elysia + Playwright | 8083 | Browser automation service: headless browser actions for bots |

Infrastructure dependencies:
- **PostgreSQL** — Relational data storage
- **Qdrant** — Vector database for memory semantic search
- **Containerd** — Container runtime providing isolated environments per bot (Linux); Apple Virtualization on macOS

## Tech Stack

### Backend (Go)
- **Framework**: Echo (HTTP)
- **Dependency Injection**: Uber FX
- **AI SDK**: [Twilight AI](https://github.com/memohai/twilight-ai) (Go LLM SDK — OpenAI, Anthropic, Google)
- **Database Driver**: pgx/v5
- **Code Generation**: sqlc (SQL → Go)
- **API Docs**: Swagger/OpenAPI (swaggo)
- **MCP**: modelcontextprotocol/go-sdk
- **Containers**: containerd v2 (Linux), Apple Virtualization (macOS)

### Frontend (TypeScript)
- **Framework**: Vue 3 (Composition API)
- **Build Tool**: Vite
- **State Management**: Pinia + Pinia Colada
- **UI**: Tailwind CSS 4 + custom component library (`@memohai/ui`) + Reka UI
- **i18n**: vue-i18n
- **Markdown**: markstream-vue + Shiki + Mermaid + KaTeX
- **Package Manager**: pnpm monorepo

### Browser Gateway (TypeScript)
- **Runtime**: Bun
- **Framework**: Elysia
- **Browser Automation**: Playwright

### Tooling
- **Task Runner**: mise
- **Package Managers**: pnpm (frontend monorepo), Go modules (backend)
- **Linting**: golangci-lint (Go), ESLint + typescript-eslint + vue-eslint-parser (TypeScript)
- **Testing**: Vitest
- **Version Management**: bumpp
- **SDK Generation**: @hey-api/openapi-ts

## Project Structure

```
Memoh/
├── cmd/                        # Go application entry points
│   ├── agent/                  #   Main backend server (main.go, FX wiring)
│   ├── bridge/                 #   In-container gRPC bridge (UDS-based, runs inside bot containers)
│   ├── mcp/                    #   MCP stdio transport binary
│   └── memoh/                  #   Unified binary wrapper (Cobra: serve, migrate, version)
├── internal/                   # Go backend core code (domain packages)
│   ├── accounts/               #   User account management (CRUD, password hashing)
│   ├── acl/                    #   Access control list (source-aware chat trigger ACL)
│   ├── agent/                  #   In-process AI agent (Twilight AI SDK integration)
│   │   ├── agent.go            #     Core agent: Stream() / Generate() via Twilight SDK
│   │   ├── model.go            #     Model creation (OpenAI completions/responses, Anthropic, Google)
│   │   ├── stream.go           #     Streaming event assembly
│   │   ├── sential.go          #     Sential (sentinel) loop detection logic
│   │   ├── tags.go             #     Tag extraction from streaming text (attachments, reactions, speech)
│   │   ├── prompt.go           #     Prompt assembly (system, heartbeat, schedule, subagent)
│   │   ├── config.go           #     Agent configuration types (RunConfig, ModelConfig)
│   │   ├── types.go            #     Shared types (StreamEvent, GenerateResult, FileAttachment)
│   │   ├── fs.go               #     Filesystem utilities
│   │   ├── prompts/            #     Prompt templates (system.md, heartbeat.md, schedule.md, subagent.md)
│   │   └── tools/              #     Tool providers (ToolProvider interface)
│   │       ├── message.go      #       Send message tool
│   │       ├── contacts.go     #       Contact list tool
│   │       ├── schedule.go     #       Schedule management tool
│   │       ├── memory.go       #       Memory read/write tool
│   │       ├── web.go          #       Web search tool
│   │       ├── webfetch.go     #       Web page fetch tool
│   │       ├── container.go    #       Container file/exec tools
│   │       ├── inbox.go        #       Inbox tool
│   │       ├── email.go        #       Email send tool
│   │       ├── subagent.go     #       Sub-agent invocation tool
│   │       ├── skill.go        #       Skill activation tool
│   │       ├── browser.go      #       Browser automation tool
│   │       ├── tts.go          #       Text-to-speech tool
│   │       └── federation.go   #       MCP federation tool
│   ├── attachment/             #   Attachment normalization (MIME types, base64)
│   ├── auth/                   #   JWT authentication middleware and utilities
│   ├── bind/                   #   Channel identity-to-user binding code management
│   ├── boot/                   #   Runtime configuration provider (container backend detection)
│   ├── bots/                   #   Bot management (CRUD, lifecycle)
│   ├── browsercontexts/        #   Browser context management (CRUD)
│   ├── channel/                #   Channel adapter system (Telegram, Discord, Feishu, QQ, Local, Email)
│   ├── command/                #   Slash command system (extensible command handlers)
│   ├── config/                 #   Configuration loading and parsing (TOML)
│   ├── containerd/             #   Container runtime abstraction (containerd / Apple Virtualization)
│   ├── conversation/           #   Conversation management and flow resolver
│   │   ├── service.go          #     Conversation CRUD and routing
│   │   └── flow/               #     Chat orchestration (resolver, streaming, memory, triggers)
│   ├── db/                     #   Database connection and migration utilities
│   │   └── sqlc/               #   ⚠️ Auto-generated by sqlc — DO NOT modify manually
│   ├── email/                  #   Email provider and outbox management (Mailgun, generic SMTP, OAuth)
│   ├── embedded/               #   Embedded filesystem assets (web only)
│   ├── handlers/               #   HTTP request handlers (REST API endpoints)
│   ├── healthcheck/            #   Health check adapter system (MCP, channel checkers)
│   ├── heartbeat/              #   Heartbeat scheduling service (cron-based)
│   ├── identity/               #   Identity type utilities (human vs bot)
│   ├── inbox/                  #   Bot inbox service (notifications, triggers)
│   ├── logger/                 #   Structured logging (slog)
│   ├── mcp/                    #   MCP protocol manager (connections, OAuth, tool gateway)
│   ├── workspace/              #   Workspace container lifecycle management
│   │   ├── manager.go          #     Container reconciliation, gRPC connection pool
│   │   ├── manager_lifecycle.go #    Container create/start/stop operations
│   │   ├── bridge/             #     gRPC client for in-container bridge service
│   │   └── bridgepb/           #     Protobuf definitions (bridge.proto)
│   ├── media/                  #   Content-addressed media asset service
│   ├── memory/                 #   Long-term memory system (multi-provider: Qdrant, BM25, LLM extraction)
│   ├── message/                #   Message persistence and event publishing
│   ├── models/                 #   LLM model management (CRUD, variants)
│   ├── policy/                 #   Access policy resolution (guest access)
│   ├── providers/              #   LLM provider management (OpenAI, Anthropic, etc.)
│   ├── prune/                  #   Text pruning utilities (truncation with head/tail)
│   ├── schedule/               #   Scheduled task service (cron)
│   ├── searchengines/          #   Search engine abstraction (reserved)
│   ├── searchproviders/        #   Search engine provider management (Brave, etc.)
│   ├── server/                 #   HTTP server wrapper (Echo setup, middleware, shutdown)
│   ├── settings/               #   Bot settings management
│   ├── storage/                #   Storage provider interface (filesystem, container FS)
│   ├── subagent/               #   Sub-agent management (CRUD)
│   ├── textutil/               #   UTF-8 safe text utilities
│   ├── tts/                    #   Text-to-speech provider management
│   └── version/                #   Build-time version information
├── apps/                       # Application services
│   ├── browser/                #   Browser Gateway (Bun/Elysia/Playwright)
│   │   └── src/
│   │       ├── index.ts        #     Elysia server entry point
│   │       ├── browser.ts      #     Playwright browser lifecycle
│   │       ├── modules/        #     Route modules (action, context, devices)
│   │       ├── middlewares/     #     CORS, error handling, bearer auth
│   │       ├── types/          #     TypeScript type definitions
│   │       ├── storage.ts      #     Browser context storage
│   │       └── models.ts       #     Zod request schemas
│   └── web/                    #   Main web app (@memohai/web, Vue 3)
├── packages/                   # Shared TypeScript libraries
│   ├── ui/                     #   Shared UI component library (@memohai/ui)
│   ├── sdk/                    #   TypeScript SDK (@memohai/sdk, auto-generated from OpenAPI)
│   └── config/                 #   Shared configuration utilities (@memohai/config)
├── spec/                       # OpenAPI specifications (swagger.json, swagger.yaml)
├── db/                         # Database
│   ├── migrations/             #   SQL migration files
│   └── queries/                #   SQL query files (sqlc input)
├── conf/                       # Configuration templates (app.example.toml, app.docker.toml, app.apple.toml, app.windows.toml)
├── devenv/                     # Dev environment (docker-compose, dev Dockerfiles, app.dev.toml, bridge-build.sh, server-entrypoint.sh)
├── docker/                     # Production Docker (Dockerfiles, entrypoints, nginx.conf, toolkit/)
├── docs/                       # Documentation site
├── scripts/                    # Utility scripts (db, release, install)
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

### Common Commands

| Command | Description |
|---------|-------------|
| `mise run dev` | Start the containerized dev environment (all services) |
| `mise run dev:down` | Stop the dev environment |
| `mise run dev:logs` | View dev environment logs |
| `mise run dev:restart` | Restart a service (e.g. `-- server`) |
| `mise run setup` | Install dependencies + workspace toolkit |
| `mise run sqlc-generate` | Regenerate Go code after modifying SQL files |
| `mise run swagger-generate` | Generate Swagger documentation |
| `mise run sdk-generate` | Generate TypeScript SDK (depends on swagger-generate) |
| `mise run db-up` | Initialize and migrate the database |
| `mise run db-down` | Drop the database |
| `mise run build-embedded-assets` | Build and stage embedded web assets |
| `mise run build-unified` | Build unified memoh binary |
| `mise run bridge:build` | Rebuild bridge binary in dev container |
| `mise run lint` | Run all linters (Go + ESLint) |
| `mise run lint:fix` | Run all linters with auto-fix |
| `mise run release` | Release new version (bumpp) |
| `mise run release-binaries` | Build release archive for target (requires TARGET_OS TARGET_ARCH) |
| `mise run install-socktainer` | Install socktainer (macOS container backend) |
| `mise run install-workspace-toolkit` | Install workspace toolkit (bridge binary etc.) |

### Docker Deployment

```bash
docker compose up -d        # Start all services
# Visit http://localhost:8082
```

Production services: `postgres`, `migrate`, `server`, `web`.
Optional profiles: `qdrant` (vector DB), `sparse` (BM25 search), `browser` (browser automation).

## Key Development Rules

### Database, sqlc & Migrations

1. **SQL queries** are defined in `db/queries/*.sql`.
2. All Go files under `internal/db/sqlc/` are auto-generated by sqlc. **DO NOT modify them manually.**
3. After modifying any SQL files (migrations or queries), run `mise run sqlc-generate` to update the generated Go code.

#### Migration Rules

Migrations live in `db/migrations/` and follow a dual-update convention:

- **`0001_init.up.sql` is the canonical full schema.** It always contains the complete, up-to-date database definition (all tables, indexes, constraints, etc.). When adding schema changes, you must **also update `0001_init.up.sql`** to reflect the final state.
- **Incremental migration files** (`0002_`, `0003_`, ...) contain only the diff needed to upgrade an existing database. They exist for environments that already have the schema and need to apply only the delta.
- **Both must be kept in sync**: every schema change requires updating `0001_init.up.sql` AND creating a new incremental migration file.
- **Naming**: `{NNNN}_{description}.up.sql` and `{NNNN}_{description}.down.sql`, where `{NNNN}` is a zero-padded sequential number (e.g., `0005`). Always use the next available number.
- **Paired files**: Every incremental migration **must** have both an `.up.sql` (apply) and a `.down.sql` (rollback) file.
- **Header comment**: Each file should start with a comment indicating the migration name and a brief description:
  ```sql
  -- 0005_add_feature_x
  -- Add feature_x column to bots table for ...
  ```
- **Idempotent DDL**: Use `IF NOT EXISTS` / `IF EXISTS` guards (e.g., `CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `DROP TABLE IF EXISTS`) so migrations are safe to re-run.
- **Down migration must fully reverse up**: The `.down.sql` must cleanly undo everything its `.up.sql` does, in reverse order.
- **After creating or modifying migrations**, run `mise run sqlc-generate` to regenerate the Go code, then `mise run db-up` to apply.

### API Development Workflow

1. Write handlers in `internal/handlers/` with swaggo annotations.
2. Run `mise run swagger-generate` to update the OpenAPI docs (output in `spec/`).
3. Run `mise run sdk-generate` to update the frontend TypeScript SDK (`packages/sdk/`).
4. The frontend calls APIs via the auto-generated `@memohai/sdk`.

### Agent Development

- The AI agent runs **in-process** within the Go server — there is no separate agent gateway service.
- Core agent logic lives in `internal/agent/`, powered by the [Twilight AI](https://github.com/memohai/twilight-ai) Go SDK.
- `internal/agent/agent.go` provides `Stream()` (SSE streaming) and `Generate()` (non-streaming) methods.
- Model creation (`internal/agent/model.go`) supports four client types: `openai-completions`, `openai-responses`, `anthropic-messages`, `google-generative-ai`.
- Tools are implemented as `ToolProvider` instances in `internal/agent/tools/`, loaded via setter injection to avoid FX dependency cycles.
- Prompt templates (system, heartbeat, schedule, subagent) are embedded Go Markdown files in `internal/agent/prompts/`.
- The conversation flow resolver (`internal/conversation/flow/`) orchestrates message assembly, memory injection, history trimming, and agent invocation.
- Loop detection (text and tool loops) is built into the agent with configurable thresholds.
- Tag extraction system processes inline tags in streaming output (attachments, reactions, speech/TTS).

### Frontend Development

- Use Vue 3 Composition API with `<script setup>` style.
- Shared components belong in `packages/ui/`.
- API calls use the auto-generated `@memohai/sdk`.
- State management uses Pinia; data fetching uses Pinia Colada.
- i18n via vue-i18n.

### Container / Workspace Management

- Each bot can have an isolated **workspace container** for file editing, command execution, and MCP tool hosting.
- Containers communicate with the host via a **gRPC bridge** over Unix Domain Sockets (UDS), not TCP.
- The bridge binary (`cmd/bridge/`) runs inside each container, mounting runtime binaries from `$WORKSPACE_RUNTIME_DIR` and UDS sockets from `/run/memoh/`.
- Container images are standard base images (debian, alpine, ubuntu, etc.) — no dedicated MCP Docker image needed.
- `internal/workspace/` manages container lifecycle (create, start, stop, reconcile) and maintains a gRPC connection pool.
- `internal/containerd/` provides the container runtime abstraction layer (containerd on Linux, Apple Virtualization on macOS, socktainer for socket-based management).
- SSE-based progress feedback is provided during container image pull and creation.

## Database Tables

| Table | Description |
|-------|-------------|
| `users` | User accounts (username, email, role, display_name, avatar) |
| `channel_identities` | Unified inbound identity subject (cross-platform) |
| `user_channel_bindings` | Outbound delivery config per user/channel |
| `llm_providers` | LLM provider configurations (name, base_url, api_key) |
| `search_providers` | Search engine provider configurations |
| `models` | Model definitions (chat/embedding types, modalities, reasoning) |
| `model_variants` | Model variant definitions (weight, metadata) |
| `bots` | Bot definitions with model references and settings |
| `mcp_connections` | MCP connection configurations per bot |
| `bot_channel_configs` | Per-bot channel configurations |
| `bot_preauth_keys` | Bot pre-authentication keys |
| `channel_identity_bind_codes` | One-time codes for channel identity → user linking |
| `bot_channel_routes` | Conversation route mapping (inbound thread → bot history) |
| `bot_history_messages` | Unified message history under bot scope |
| `bot_history_message_assets` | Message → content_hash asset links (with name and metadata) |
| `containers` | Bot container instances |
| `snapshots` | Container snapshots |
| `container_versions` | Container version tracking |
| `lifecycle_events` | Container lifecycle events |
| `schedule` | Scheduled tasks (cron) |
| `subagents` | Sub-agent definitions |
| `browser_contexts` | Browser context configurations (Playwright) |
| `storage_providers` | Pluggable object storage backends |
| `bot_storage_bindings` | Per-bot storage backend selection |
| `bot_inbox` | Per-bot inbox (notifications, triggers) |
| `bot_heartbeat_logs` | Heartbeat execution records |
| `email_providers` | Pluggable email service backends (Mailgun, generic SMTP) |
| `bot_email_bindings` | Per-bot email provider binding with permissions |
| `email_outbox` | Outbound email audit log |
| `email_oauth_tokens` | OAuth2 tokens for email providers (Gmail) |
| `memory_providers` | Multi-provider memory adapter configurations |
| `tts_providers` | Text-to-speech provider configurations |
| `tts_models` | TTS model definitions |
| `token_usage` | Per-message LLM token usage tracking |
| `chat_acl` | Source-aware chat access control lists |

## Configuration

The main configuration file is `config.toml` (copied from `conf/app.example.toml` or environment-specific templates for development), containing:

- `[log]` — Logging configuration (level, format)
- `[server]` — HTTP listen address
- `[admin]` — Admin account credentials
- `[auth]` — JWT authentication settings
- `[containerd]` — Container runtime configuration (socket path, namespace, socktainer)
- `[workspace]` — Workspace container image and data configuration (registry, default_image, snapshotter, data_root, cni, runtime_dir)
- `[postgres]` — PostgreSQL connection
- `[qdrant]` — Qdrant vector database connection
- `[sparse]` — Sparse (BM25) search service connection
- `[browser_gateway]` — Browser Gateway address
- `[web]` — Web frontend address

Configuration templates available in `conf/`:
- `app.example.toml` — Default template
- `app.docker.toml` — Docker deployment
- `app.apple.toml` — macOS (Apple Virtualization backend)
- `app.windows.toml` — Windows

Development configuration in `devenv/`:
- `app.dev.toml` — Development (connects to devenv docker-compose)

## Web Design

Please refer to `./apps/web/AGENTS.md`.
