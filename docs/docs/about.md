# About Memoh

## What is Memoh?

Memoh is a multi-member, structured long-memory, containerized AI agent system platform. You can create your own AI bots and chat with them via Telegram, Discord, Lark (Feishu), QQ, Matrix, WeCom, WeChat, Email, or Web. Every bot has an independent container and memory system, allowing it to edit files, execute commands, and access the network within its own container — like having its own computer and brain.

## Key Features

### Multi-Bot Management

Create multiple bots. Humans and bots, or bots with each other, can chat privately, in groups, or collaborate. Build bot teams, or set up accounts for family members to manage daily tasks with bots.

### Multi-User & Identity Recognition

Bots can distinguish individual users in group chats, remember each person's context separately, and send direct messages to specific users. Cross-platform identity binding unifies the same person across Telegram, Discord, Lark, and Web.

### Containerized Isolation

Each bot runs in its own isolated container (powered by Containerd) with a separate filesystem and network. Bots can freely read/write files and execute commands within their containers without interfering with each other. Supports container snapshots for save/restore, data export/import, and versioning.

### Memory Engineering

A deeply engineered memory layer:

- Automatically extracts key facts from each conversation turn and stores them as structured memories
- Hybrid retrieval: semantic search (via Qdrant vector database) + keyword retrieval (BM25)
- Loads the last 24 hours of conversation context by default
- Automatic memory compaction and rebuild capabilities
- Multi-language auto-detection

### Multi-Platform Support

Unified channel adapter architecture for connecting to multiple messaging platforms:

- **Telegram** — Full support with streaming, Markdown, attachments, and replies
- **Discord** — Full support
- **Lark (Feishu)** — Full support
- **QQ** — Full support
- **Matrix** — Decentralized messaging protocol support
- **WeCom (WeWork)** — Enterprise messaging integration
- **WeChat** — Personal messaging via the WeChat AI bot platform
- **Email** — Inbound webhook + outbound providers (Mailgun, generic SMTP, Gmail OAuth)
- **Web** — Built-in web chat interface with streaming

### Agent Capabilities

Bots come with a rich set of built-in tools:

- **Web Search** — Configurable search providers (Brave, Bing, Google, Tavily, SearXNG, DuckDuckGo, and more) for real-time information
- **Web Fetch** — Retrieve and parse web page content
- **Browser Automation** — Use Playwright-powered browser tools for navigation, clicking, form filling, screenshots, PDF export, and rendered page inspection
- **Subagents** — Create specialized subagents with independent context, assign skills, and delegate complex tasks in parallel
- **Skills** — Define bot personality via IDENTITY.md, SOUL.md, and modular skill files that bots can enable/disable at runtime
- **Container Operations** — Read/write files, edit code, and execute commands inside the container
- **Memory Management** — Search and manage memories
- **Messaging** — Send messages and reactions to specific users or channels
- **Email** — Compose and send emails through configured email providers
- **Text-to-Speech** — Synthesize spoken audio from text using configurable TTS providers
- **MCP Federation** — Access external tools and resources via federated MCP connections
- **Session History** — Access and manage conversation session history

### Sessions

Each bot maintains **sessions** — independent conversation threads that preserve context. Sessions come in four types:

- **Chat** — Standard user conversations
- **Heartbeat** — Periodic autonomous activity sessions
- **Schedule** — Cron-triggered task execution sessions
- **Subagent** — Delegated task sessions for sub-agents

Users can start a new session at any time using the `/new` slash command in any channel, which resets the conversation context.

### Slash Commands

Bots support a comprehensive set of slash commands that can be used directly in any channel:

- `/help` — Show available commands
- `/new` — Start a new conversation session
- `/schedule` — Manage scheduled tasks
- `/settings` — View and update bot settings
- `/model` — Switch chat or heartbeat models
- `/usage` — View token usage statistics
- And more — see [Slash Commands](/getting-started/slash-commands) for the full reference.

### Multi-LLM Provider Support

Flexibly switch between a wide range of LLM providers via four client types:

- OpenAI Responses API, OpenAI Chat Completions API (including compatible services)
- Anthropic Messages API, Google Generative AI API

Per-bot model assignment for chat, memory, and embedding. Providers support OAuth authentication and automatic model import.

### MCP Protocol Support

Full MCP (Model Context Protocol) support via HTTP, SSE, and Stdio to connect external tool services. Built-in tools for container operations, memory search, web search, scheduling, messaging, and more. Each bot can have its own independent MCP connections with OAuth authentication support.

### Scheduled Tasks

Configure scheduled tasks using cron expressions to automatically run commands at specified times. Supports max execution count limits and manual triggers.

### Memory Compaction

Automatic memory compaction reduces redundancy in the bot's memory pool over time. Configurable compaction ratios and decay windows keep the most relevant memories while optimizing storage and retrieval quality.

### Graphical Configuration

Modern web UI (Vue 3 + Tailwind CSS) with real-time streaming chat, tool call visualization, container filesystem browser, and visual configuration for bots, channels, providers, models, MCP, skills, and all other settings. Dark/light theme, i18n. No coding required to set up your own AI bot.

## Installation

To get Memoh running:

- **[Docker](/installation/docker)** — Recommended. One-click or manual setup with Docker Compose. Includes all services (PostgreSQL, Qdrant, Containerd, server, web) — no extra dependencies on the host.
