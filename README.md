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
    <a href="https://deepwiki.com/memohai/Memoh">
      <img src="https://deepwiki.com/badge.svg" alt="DeepWiki" />
    </a>
    <img src="https://github.com/memohai/Memoh/actions/workflows/docker.yml/badge.svg" alt="Docker" />
  </div>
  <div align="center">
    [<a href="https://t.me/memohai">Telegram Group</a>]
    [<a href="https://docs.memoh.ai">Documentation</a>]
    [<a href="mailto:business@memoh.net">Cooperation</a>]
  </div>
  <hr>
</div>

Memoh is an always-on, containerized AI agent system. Create multiple AI bots, each running in its own isolated container with persistent memory, and interact with them across Telegram, Discord, Lark (Feishu), Email, or the built-in Web UI. Bots can execute commands, edit files, browse the web, call external tools via MCP, and remember everything — like giving each bot its own computer and brain.

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
cp conf/app.docker.toml config.toml
# Edit config.toml
sudo docker compose up -d
```

> **Install a specific version:**
> ```bash
> curl -fsSL https://memoh.sh | sudo MEMOH_VERSION=v1.0.0 sh
> ```
>
> **Use CN mirror for slow image pulls:**
> ```bash
> curl -fsSL https://memoh.sh | sudo USE_CN_MIRROR=true sh
> ```
>
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
- 🧠 **Memory Engineering**: Multi-provider memory architecture — Built-in (off / sparse / dense modes), [Mem0](https://mem0.ai), and OpenViking. LLM-driven fact extraction, hybrid retrieval (dense semantic search + BM25 keyword + neural sparse vectors), 24-hour context loading, memory compaction & rebuild, and multi-language auto-detection.
- 💬 **Multi-Platform**: Supports Telegram, Discord, Lark (Feishu), Email, and the built-in Web UI. Unified message format with rich text, media attachments, reactions, and streaming across all platforms. Cross-platform identity binding.
- 📧 **Email**: Multi-adapter email service (Mailgun, generic SMTP) with per-bot binding and outbound audit log. Bots can send and receive emails as a channel.
- 🔧 **MCP (Model Context Protocol)**: Full MCP support (HTTP / SSE / Stdio). Built-in tools for container operations, memory search, web search, scheduling, messaging, and more. Connect external MCP servers for extensibility.
- 🧩 **Subagents**: Create specialized sub-agents per bot with independent context and skills, enabling multi-agent collaboration.
- 🎭 **Skills & Identity**: Define bot personality via IDENTITY.md, SOUL.md, and modular skill files that bots can enable/disable at runtime.
- 🌐 **Browser**: Each bot can have its own headless Chromium browser (via Playwright). Navigate pages, click elements, fill forms, take screenshots (with annotated element labels), read accessibility trees, manage tabs, and more — enabling real web automation and AI-driven browsing.
- 🔍 **Web Search**: 12 built-in search providers — Brave, Bing, Google, Tavily, DuckDuckGo, SearXNG, Serper, Sogou, Jina, Exa, Bocha, and Yandex — for web search and URL content fetching.
- ⏰ **Scheduled Tasks**: Cron-based scheduling with max-call limits. Bots can autonomously run commands or tools at specified intervals.
- 💓 **Heartbeat**: Periodic autonomous tasks — bots can perform routine operations (e.g., check-ins, summaries, monitoring) at configurable intervals with execution logging.
- 📥 **Inbox**: Cross-channel inbox — messages from other channels are queued and surfaced in the system prompt so the bot never misses context.
- 📊 **Token Usage Tracking**: Monitor token consumption per bot with usage statistics and visualization.
- 🧪 **Multi-Model**: Works with any OpenAI-compatible, Anthropic, or Google Generative AI provider. Per-bot model assignment for chat, memory, and embedding.
- 🖥️ **Web UI**: Modern dashboard (Vue 3 + Tailwind CSS) with real-time streaming chat, tool call visualization, in-chat file manager, container filesystem browser, and visual configuration for all settings. Dark/light theme, i18n.
- 🚀 **One-Click Deploy**: Docker Compose with automatic migration, containerd setup, and CNI networking. Interactive install script included.

## Memory System

Memoh's memory system is built around **Memory Providers** — pluggable backends that control how a bot stores, retrieves, and manages long-term memory.

| Provider | Description |
|----------|-------------|
| **Built-in** | Self-hosted, ships with Memoh. Three modes: **Off** (file-based, no vector search), **Sparse** (neural sparse vectors via local model, no API cost), **Dense** (embedding-based semantic search via Qdrant). |
| **Mem0** | SaaS memory via the [Mem0](https://mem0.ai) API. |
| **OpenViking** | Self-hosted or SaaS memory with its own API. |

Each bot binds one provider. During chat, the bot automatically extracts key facts from every conversation turn and stores them as structured memories. On each new message, the most relevant memories are retrieved via hybrid search and injected into the bot's context — giving it personalized, long-term recall across conversations.

Additional capabilities include memory compaction (merge redundant entries), rebuild, manual creation/editing, and vector manifold visualization (Top-K distribution & CDF curves). See the [documentation](https://docs.memoh.ai/memory-providers/) for setup details.

## Gallery

<table>
  <tr>
    <td><img src="./assets/gallery/01.png" alt="Gallery 1" width="100%"></td>
    <td><img src="./assets/gallery/02.png" alt="Gallery 2" width="100%"></td>
    <td><img src="./assets/gallery/03.png" alt="Gallery 3" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">Chat with Bots</strong></td>
    <td><strong text-align="center">Container & Bot Management</strong></td>
    <td><strong text-align="center">Provider & Model Configuration</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/04.png" alt="Gallery 4" width="100%"></td>
    <td><img src="./assets/gallery/05.png" alt="Gallery 5" width="100%"></td>
    <td><img src="./assets/gallery/06.png" alt="Gallery 6" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">Container File Manager</strong></td>
    <td><strong text-align="center">Scheduled Tasks</strong></td>
    <td><strong text-align="center">Token Usage Tracking</strong></td>
  </tr>
</table>

## Architecture

```mermaid
flowchart TB
    subgraph Clients [" Clients "]
        direction LR
        CH["Channels<br/>Telegram · Discord · Feishu · QQ · Email"]
        WEB["Web UI (Vue 3 :8082)"]
    end

    CH & WEB --> API

    subgraph Server [" Server · Go :8080 "]
        API["REST API & Channel Adapters"]

        subgraph Agent [" In-process AI Agent "]
            TWILIGHT["Twilight AI SDK<br/>OpenAI · Anthropic · Google"]
            CONV["Conversation Flow<br/>Streaming · Sential · Loop Detection"]
        end

        subgraph ToolProviders [" Tool Providers "]
            direction LR
            T_CORE["Memory · Web Search<br/>Schedule · Contacts · Inbox"]
            T_EXT["Container · Email · Browser<br/>Subagent · Skill · TTS<br/>MCP Federation"]
        end

        API --> Agent --> ToolProviders
    end

    PG[("PostgreSQL")]
    QD[("Qdrant")]
    BROWSER["Browser Gateway<br/>(Playwright :8083)"]

    subgraph Workspace [" Workspace Containers · containerd "]
        direction LR
        BA["Bot A"] ~~~ BB["Bot B"] ~~~ BC["Bot C"]
    end

    Server --- PG
    Server --- QD
    ToolProviders -.-> BROWSER
    ToolProviders -- "gRPC Bridge over UDS" --> Workspace
```

## Sub-projects Born for This Project

- [**Twilight AI**](https://github.com/memohai/twilight-ai) — A lightweight, idiomatic AI SDK for Go — inspired by [Vercel AI SDK](https://sdk.vercel.ai/). Provider-agnostic (OpenAI, Anthropic, Google), with first-class streaming, tool calling, MCP support, and embeddings.

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
