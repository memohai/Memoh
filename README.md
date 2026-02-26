<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" width="100" height="100">
  <h1>Memoh</h1>
  <p>Multi-Member, Structured Long-Memory, Containerized AI Agent System.</p>
  <p>📌 <a href="https://docs.memoh.ai/blogs/2026-02-16.html">Introduction to Memoh - The Case for an Always-On, Containerized Home Agent</a></p>
  <div align="center">
    <img src="https://img.shields.io/github/package-json/v/memohai/Memoh" alt="Version" />
    <img src="https://img.shields.io/github/license/memohai/Memoh" alt="License" />
    <img src="https://img.shields.io/github/stars/memohai/Memoh?style=social" alt="Stars" />
    <img src="https://img.shields.io/github/forks/memohai/Memoh?style=social" alt="Forks" />
    <img src="https://img.shields.io/github/last-commit/memohai/Memoh" alt="Last Commit" />
    <img src="https://img.shields.io/github/issues/memohai/Memoh" alt="Issues" />
    <img src="https://github.com/memohai/Memoh/actions/workflows/docker.yml/badge.svg" alt="Docker" />
  </div>
  <div align="center">
    [<a href="https://t.me/memohai">Telegram Group</a>]
    [<a href="https://docs.memoh.ai">Documentation</a>]
    [<a href="mailto:business@memoh.net">Cooperation</a>]
  </div>
  <hr>
</div>

Memoh is an always-on, containerized AI agent system. Create multiple AI bots, each running in its own isolated container with persistent memory, and interact with them across Telegram, Discord, Lark (Feishu), or the built-in Web/CLI. Bots can execute commands, edit files, browse the web, call external tools via MCP, and remember everything — like giving each bot its own computer and brain.

## Quick Start

One-click install (**requires [Docker](https://www.docker.com/get-started/)**):

```bash
curl -fsSL https://memoh.sh | sudo sh
```

*Silent install with all defaults: `curl -fsSL ... | sudo sh -s -- -y`*

Or manually:

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
sudo docker compose up -d
```

> If you experience slow image pulls, use the CN override:
```bash
sudo docker compose -f docker-compose.yml -f docker/docker-compose.cn.yml up -d
```

> On macOS or if your user is in the `docker` group, `sudo` is not required.

Visit <http://localhost:8082> after startup. Default login: `admin` / `admin123`

See [DEPLOYMENT.md](DEPLOYMENT.md) for custom configuration and production setup.

## Why Memoh?

OpenClaw is impressive, but it has notable drawbacks: stability issues, security concerns, cumbersome configuration, and high token costs. If you're looking for a stable, secure solution, consider Memoh.

Memoh is a multi-bot agent service built with Golang. It offers full graphical configuration for bots, Channels, MCP, and Skills. We use Containerd to provide container-level isolation for each bot and draw heavily from OpenClaw's Agent design.

Memoh Bot can distinguish and remember requests from multiple humans and bots, working seamlessly in any group chat. You can use Memoh to build bot teams, or set up accounts for family members to manage daily household tasks with bots.

## Features

- 🤖 **Multi-Bot Management**: Create multiple bots; humans and bots, or bots with each other, can chat privately, in groups, or collaborate. Supports role-based access control (owner / admin / member) with ownership transfer.
- 👥 **Multi-User & Identity Recognition**: Bots can distinguish individual users in group chats, remember each person's context separately, and send direct messages to specific users. Cross-platform identity binding unifies the same person across Telegram, Discord, Lark, and Web.
- 📦 **Containerized**: Each bot runs in its own isolated containerd container. Bots can freely execute commands, edit files, and access the network within their containers — like having their own computer. Supports container snapshots for save/restore.
- 🧠 **Memory Engineering**: Hybrid retrieval (dense vector search + BM25 keyword search) with LLM-driven fact extraction. Last 24 hours of context loaded by default, with memory compaction and rebuild capabilities.
- 💬 **Multi-Platform**: Supports Telegram, Discord, Lark (Feishu), and built-in Web/CLI. Unified message format with rich text, media attachments, reactions, and streaming across all platforms. Cross-platform identity binding.
- 🔧 **MCP (Model Context Protocol)**: Full MCP support (HTTP / SSE / Stdio). Built-in tools for container operations, memory search, web search, scheduling, messaging, and more. Connect external MCP servers for extensibility.
- 🧩 **Subagents**: Create specialized sub-agents per bot with independent context and skills, enabling multi-agent collaboration.
- 🎭 **Skills & Identity**: Define bot personality via IDENTITY.md, SOUL.md, and modular skill files that bots can enable/disable at runtime.
- 🔍 **Web Search**: Configurable search providers (Brave Search, etc.) for web search and URL content fetching.
- ⏰ **Scheduled Tasks**: Cron-based scheduling with max-call limits. Bots can autonomously run commands or tools at specified intervals.
- 📥 **Inbox**: Cross-channel inbox — messages from other channels are queued and surfaced in the system prompt so the bot never misses context.
- 🧪 **Multi-Model**: Works with any OpenAI-compatible, Anthropic, or Google Generative AI provider. Per-bot model assignment for chat, memory, and embedding.
- 🖥️ **Web UI**: Modern dashboard (Vue 3 + Tailwind CSS) with real-time streaming chat, tool call visualization, container filesystem browser, and visual configuration for all settings. Dark/light theme, i18n.
- 🚀 **One-Click Deploy**: Docker Compose with automatic migration, containerd setup, and CNI networking. Interactive install script included.

## Tech Stack

| Layer | Stack |
|-------|-------|
| Backend | Go, Echo, sqlc, Uber FX, pgx/v5, containerd v2 |
| Agent Gateway | Bun, Elysia |
| Frontend | Vue 3, Vite, Pinia, Tailwind CSS, Reka UI |
| Storage | PostgreSQL, Qdrant |
| Infra | Docker, containerd, CNI |
| Tooling | mise, pnpm, swaggo, sqlc |

## Architecture

```
┌─────────────┐    ┌─────────────────┐    ┌──────────────┐
│   Channels   │    │    Web UI        │    │   CLI        │
│  (TG/DC/FS)  │    │  (Vue 3 :8082)  │    │              │
└──────┬───────┘    └────────┬────────┘    └──────┬───────┘
       │                     │                     │
       ▼                     ▼                     ▼
┌──────────────────────────────────────────────────────────┐
│                   Server (Go :8080)                       │
│  Auth · Bots · Channels · Memory · Containers · MCP      │
└──────────────────────┬───────────────────────────────────┘
                       │
           ┌───────────┼───────────┐
           ▼           ▼           ▼
     ┌──────────┐ ┌─────────┐ ┌──────────────────┐
     │ PostgreSQL│ │ Qdrant  │ │ Agent Gateway     │
     │          │ │ (Vector)│ │ (Bun/Elysia :8081)│
     └──────────┘ └─────────┘ └────────┬──────────┘
                                       │
                               ┌───────┼───────┐
                               ▼       ▼       ▼
                          ┌─────┐ ┌─────┐ ┌─────┐
                          │Bot A│ │Bot B│ │Bot C│  ← containerd
                          └─────┘ └─────┘ └─────┘
```

## Roadmap

Please refer to the [Roadmap](https://github.com/memohai/Memoh/issues/86) for more details.

## Development

Refer to [CONTRIBUTING.md](CONTRIBUTING.md) for development setup.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=memohai/Memoh&type=date&legend=top-left)](https://www.star-history.com/#memohai/Memoh&type=date&legend=top-left)

## Contributors

<a href="https://github.com/memohai/Memoh/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=memohai/Memoh" />
</a>

**LICENSE**: AGPLv3

Copyright (C) 2026 Memoh. All rights reserved.
