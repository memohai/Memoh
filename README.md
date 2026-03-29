<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" width="100" height="100">
  <h1>Memoh</h1>
  <p>Self hosted, always-on AI agent platform run in containers.</p>
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

Memoh is an always-on, containerized AI agent system. Create multiple AI bots, each running in its own isolated container with persistent memory, and interact with them across Telegram, Discord, Lark (Feishu), QQ, Matrix, WeCom, WeChat, Email, or the built-in Web UI. Bots can execute commands, edit files, browse the web, call external tools via MCP, and remember everything — like giving each bot its own computer and brain.

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
> curl -fsSL https://memoh.sh | sudo MEMOH_VERSION=v0.6.0 sh
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

Memoh is built for **always-on continuity** — an AI that stays online, and a memory that stays yours.

- **Lightweight & Fast**: Built with Go as home/studio infrastructure, runs efficiently on edge devices.
- **Containerized by default**: Each bot gets an isolated container with its own filesystem, network, and tools.
- **Hybrid split**: Cloud inference for frontier model capability, local-first memory and indexing for privacy.
- **Multi-user first**: Explicit sharing and privacy boundaries across users and bots.
- **Full graphical configuration**: Configure bots, channels, MCP, skills, and all settings through a modern web UI — no coding required.

## Features

### Core

- 🤖 **Multi-Bot & Multi-User**: Create multiple bots that chat privately, in groups, or with each other. Bots distinguish individual users in group chats, remember each person's context, and support cross-platform identity binding.
- 📦 **Containerized**: Each bot runs in its own isolated containerd container with a dedicated filesystem and network — like having its own computer. Supports snapshots, data export/import, and versioning.
- 🧠 **Memory Engineering**: LLM-driven fact extraction, hybrid retrieval (dense + sparse + BM25), 24-hour context loading, memory compaction & rebuild. Pluggable backends: Built-in (off / sparse / dense), [Mem0](https://mem0.ai), OpenViking.
- 💬 **9 Channels**: Telegram, Discord, Lark (Feishu), QQ, Matrix, WeCom, WeChat, Email (Mailgun / SMTP / Gmail OAuth), and built-in Web UI — with unified streaming, rich text, and attachments.

### Agent Capabilities

- 🔧 **MCP (Model Context Protocol)**: Full MCP support (HTTP / SSE / Stdio / OAuth). Connect external tool servers for extensibility; each bot manages its own independent MCP connections.
- 🌐 **Browser Automation**: Headless Chromium/Firefox via Playwright — navigate, click, fill forms, screenshot, read accessibility trees, manage tabs.
- 🎭 **Skills & Subagents**: Define bot personality via modular skill files; delegate complex tasks to sub-agents with independent context.
- ⏰ **Automation**: Cron-based scheduled tasks and periodic heartbeat for autonomous bot activity.

### Management

- 🖥️ **Web UI**: Modern dashboard (Vue 3 + Tailwind CSS) — streaming chat, tool call visualization, file manager, visual configuration for all settings. Dark/light theme, i18n.
- 🔐 **Access Control**: Priority-based ACL rules with allow/deny effects, scoped by channel identity, channel type, or conversation.
- 🧪 **Multi-Model**: Any OpenAI-compatible, Anthropic, or Google provider. Per-bot model assignment, provider OAuth, and automatic model import.
- 🚀 **One-Click Deploy**: Docker Compose with automatic migration, containerd setup, and CNI networking.

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
        CH["Channels<br/>Telegram · Discord · Feishu · QQ<br/>Matrix · WeCom · WeChat · Email"]
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
