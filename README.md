<div align="right">
  <span>[<a href="./README.md">English</a>]<span>
  </span>[<a href="./README_CN.md">简体中文</a>]</span>
</div>  

<div align="center">
  <img src="./assets/logo.png" alt="Memoh" height="80">
  <h1>Memoh</h1>
  <p>Self hosted, always-on AI agent orchestrator in containers.</p>
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
    <a href="https://t.me/memohai">
      <img src="https://img.shields.io/badge/Telegram-Group-26A5E4?logo=telegram&logoColor=white" alt="Telegram" />
    </a>
    <a href="https://docs.memoh.ai">
      <img src="https://img.shields.io/badge/Docs-memoh.ai-3eaf7c?logo=readthedocs&logoColor=white" alt="Documentation" />
    </a>
  </div>
</div>

**Memoh(/ˈmemoʊ/)** is an always-on, containerized AI agent orchestrator. Create multiple AI bots, each running in its own isolated container with persistent memory, and interact with them across Telegram, Discord and so on. Bots can execute commands, edit files, browse the web, call external tools via MCP, and remember everything — like giving each bot its own computer and brain.

## Quick Start

One-click install (**requires [Docker](https://www.docker.com/get-started/)**):

```bash
curl -fsSL https://memoh.sh | sh
```

*Silent install with all defaults: `curl -fsSL ... | sh -s -- -y`*

Or manually:

```bash
git clone --depth 1 https://github.com/memohai/Memoh.git
cd Memoh
cp conf/app.docker.toml config.toml
# Edit config.toml
docker compose up -d
```

> **Install a specific version:**
> ```bash
> curl -fsSL https://memoh.sh | MEMOH_VERSION=v0.6.0 sh
> ```
>
> **Use CN mirror for slow image pulls:**
> ```bash
> curl -fsSL https://memoh.sh | USE_CN_MIRROR=true sh
> ```
>
> Do not run the whole installer with `sudo`. The installer will use `sudo docker`
> internally if Docker requires it. On macOS or if your user is in the `docker`
> group, `sudo` is not required for Docker either.

Visit <http://localhost:8082> after startup. Default login: `admin` / `admin123`

See [DEPLOYMENT.md](DEPLOYMENT.md) for custom configuration and production setup.

Documentation entry points:

- [About Memoh](https://docs.memoh.ai/about)
- [Providers & Models](https://docs.memoh.ai/getting-started/provider-and-model)
- [Bot Setup](https://docs.memoh.ai/getting-started/bot)
- [Sessions & Discuss Mode](https://docs.memoh.ai/getting-started/sessions)
- [Channels](https://docs.memoh.ai/getting-started/channels)
- [Skills](https://docs.memoh.ai/getting-started/skills)
- [Supermarket](https://docs.memoh.ai/getting-started/supermarket)
- [Slash Commands](https://docs.memoh.ai/getting-started/slash-commands)

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
- 🗂️ **Persistent File System**: Every bot has a writable home directory that survives restarts, upgrades, and migrations. Bots can read, write, and organize files freely; you can browse, upload, download, and edit them visually through the web UI's file manager.
- 🧠 **Memory Engineering**: LLM-driven fact extraction, hybrid retrieval (dense + sparse + BM25), provider-based long-term memory, memory compaction, and separate session-level context compaction. Pluggable backends: Built-in (off / sparse / dense), [Mem0](https://mem0.ai), OpenViking.
- 💬 **Broad Channel Coverage**: Telegram, Discord, Lark (Feishu), QQ, Matrix, Misskey, DingTalk, WeCom, WeChat, WeChat Official Account, Email (Mailgun / SMTP / Gmail OAuth), and built-in Web UI.

### Agent Capabilities

- 🔧 **MCP (Model Context Protocol)**: Full MCP support (HTTP / SSE / Stdio / OAuth). Connect external tool servers for extensibility; each bot manages its own independent MCP connections.
- 🌐 **Browser Automation**: Headless Chromium/Firefox via Playwright — navigate, click, fill forms, screenshot, read accessibility trees, manage tabs.
- 🎭 **Skills, Supermarket & Subagents**: Define bot behavior through modular skills, install curated skills and MCP templates from Supermarket, and delegate complex tasks to sub-agents with independent context.
- 💭 **Sessions & Discuss Mode**: Use chat, discuss, schedule, heartbeat, and subagent sessions with slash-command control and session status inspection.
- ⏰ **Automation**: Cron-based scheduled tasks and periodic heartbeat for autonomous bot activity.

### Management

- 🖥️ **Web UI**: Modern dashboard (Vue 3 + Tailwind CSS) — streaming chat, tool call visualization, file manager, visual configuration for all settings. Dark/light theme, i18n.
- 🔐 **Access Control**: Priority-based ACL rules with presets, allow/deny effects, and scope by channel identity, channel type, or conversation.
- 🧪 **Multi-Model**: OpenAI-compatible, Anthropic, Google, OpenAI Codex, GitHub Copilot, and Edge TTS providers. Per-bot model assignment, provider OAuth, and automatic model import.
- 🎙️ **Speech & Transcription**: Bots can speak through 10+ TTS providers (Edge, OpenAI, ElevenLabs, Deepgram, Azure, Google, MiniMax, Volcengine, Alibaba, OpenRouter) and listen — voice messages received from Telegram, Discord, etc. are auto-transcribed via STT models (OpenAI / OpenRouter), and bots can transcribe any audio file on demand through a built-in tool.
- 🚀 **One-Click Deploy**: Docker Compose with automatic migration, containerd setup, and CNI networking.

## Memory System

Memoh ships with a **fully self-hosted memory engine** out of the box — no external API, no SaaS dependency. Every bot remembers what you've told it across sessions, days, and platforms; in group chats, each user's memories are kept separately so the bot doesn't mix you up with the rest.

### Built-in Memory (default)

Three modes, switchable per bot from the web UI:

| Mode | Backend | When to use |
|------|---------|-------------|
| **Off** | Plain file storage, no vector search | Small bots, debugging, or when you want minimal moving parts |
| **Sparse** | Neural sparse vectors via a local model + BM25 | Zero API cost, runs entirely on your machine, strong recall for short factual memories |
| **Dense** | Embedding model + Qdrant vector DB | Best semantic recall — finds memories by meaning, not just keywords |

Under the hood:

- **LLM-driven fact extraction** — every conversation turn is parsed, deduplicated, and stored as structured memories rather than raw transcripts.
- **Hybrid retrieval** — dense vectors, sparse vectors and BM25 are combined and re-ranked, so both "what was that API key" (lexical) and "the project I told you about last week" (semantic) hit reliably.
- **Memory compaction** — redundant or stale entries are periodically merged by an LLM, keeping the index small and recall sharp.
- **Inspect & edit anything** — browse, search, manually create/edit memories, rebuild the whole index, and visualize the vector manifold (Top-K distribution & CDF curves) from the web UI.

### Other providers

If you'd rather plug into an existing memory service, Memoh also supports [**Mem0**](https://mem0.ai) (SaaS) and **OpenViking** (self-hosted or SaaS) as drop-in alternatives — same bot binding, same chat experience, just a different backend.

See the [documentation](https://docs.memoh.ai/memory-providers/) for full setup details.

## FAQ

### General

**Q: What is Memoh?**
Memoh is a self-hosted, always-on AI agent orchestrator that runs in Docker containers. Each bot gets its own isolated environment with persistent memory, and can interact across Telegram, Discord, and other channels.

**Q: How is Memoh different from other AI chatbots?**
Unlike simple chatbots, Memoh provides persistent memory (bots remember past conversations), multi-channel support, MCP tool integration, and a skills marketplace — all running locally on your hardware.

**Q: What LLM providers are supported?**
Memoh supports any OpenAI-compatible API, including OpenAI, Anthropic, Google Gemini, Ollama, vLLM, and local models. Configure providers in `config.toml`.

### Installation & Setup

**Q: What are the system requirements?**
Docker and Docker Compose are required. For memory providers, Qdrant needs ~512MB RAM. Minimum 2GB RAM recommended for basic usage.

**Q: How do I update Memoh?**
Run the installer again (`curl -fsSL https://memoh.sh | sh`) or use `docker compose pull && docker compose up -d` to update to the latest version.

**Q: Can I run Memoh on a remote server?**
Yes. The installer works on any Linux server with Docker. Access the web UI at `http://your-server-ip:8082`.

### Bot & Channel Configuration

**Q: How do I connect Telegram/Discord?**
Create a bot token in Telegram Bot Father or Discord Developer Portal, then configure the channel in the web UI under Channel settings.

**Q: Can I create multiple bots?**
Yes. Each bot runs in its own isolated container with independent memory, skills, and channel connections.

**Q: What are Skills?**
Skills are reusable modules that extend bot capabilities — from web browsing to file operations. Browse and install skills from the Supermarket.

### Memory & Tools

**Q: How does Memoh memory work?**
Memoh extracts facts from conversations and stores them as structured memories. Three modes are available: Off (no memory), Sparse (local BM25), and Dense (vector DB with Qdrant). Hybrid retrieval combines all methods for best recall.

**Q: Can I use MCP tools?**
Yes. Configure MCP servers in the bot settings to give your agents access to external tools and APIs.

### Troubleshooting

**Q: Docker container fails to start**
Check Docker logs with `docker compose logs`. Common issues: port 8082 already in use, insufficient memory, or Docker daemon not running.

**Q: Bot not responding to messages**
Verify the LLM provider API key is correct in `config.toml`, check the bot is enabled in the web UI, and review container logs for errors.

**Q: Memory not working**
Ensure the memory mode is set to Sparse or Dense in bot settings. For Dense mode, verify Qdrant is running (`docker compose ps`).

---

## Gallery

<table>
  <tr>
    <td><img src="./assets/gallery/01.png" alt="Gallery 1" width="100%"></td>
    <td><img src="./assets/gallery/02.png" alt="Gallery 2" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">Chat</strong></td>
    <td><strong text-align="center">Container</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/03.png" alt="Gallery 3" width="100%"></td>
    <td><img src="./assets/gallery/04.png" alt="Gallery 4" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">Providers</strong></td>
    <td><strong text-align="center">File Manager</strong></td>
  </tr>
  <tr>
    <td><img src="./assets/gallery/05.png" alt="Gallery 5" width="100%"></td>
    <td><img src="./assets/gallery/06.png" alt="Gallery 6" width="100%"></td>
  </tr>
  <tr>
    <td><strong text-align="center">Scheduled Tasks</strong></td>
    <td><strong text-align="center">Token Usage</strong></td>
  </tr>
</table>


## Sub-projects Born for This Project

- [**Twilight AI**](https://github.com/memohai/twilight-ai) — A lightweight, idiomatic AI SDK for Go — inspired by [Vercel AI SDK](https://sdk.vercel.ai/). Provider-agnostic (OpenAI, Anthropic, Google), with first-class streaming, tool calling, MCP support, and embeddings.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=memohai/Memoh&type=date&legend=top-left)](https://www.star-history.com/#memohai/Memoh&type=date&legend=top-left)

## Contributors

<a href="https://github.com/memohai/Memoh/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=memohai/Memoh" />
</a>

## Community

- 🌐 [**Website**](https://memoh.ai)
- 📚 [**Documentation**](https://docs.memoh.ai) — setup, concepts, and guides
- 🤝 [**Cooperation**](mailto:business@memoh.net) — business@memoh.net
- 💬 [**Telegram Group**](https://t.me/memohai) — community chat & support
- 🛒 [**Supermarket**](https://github.com/memohai/supermarket) — curated skills & MCP templates

---

**LICENSE**: AGPLv3

Made with ❤️ by MemohAI Team,

Copyright (C) 2026 MemohAI (memoh.ai). All rights reserved.
