# About Memoh

## What is Memoh?

Memoh is a multi-member, structured long-memory, containerized AI agent system platform. You can create your own AI bots and chat with them via Telegram, Lark (Feishu), and more. Every bot has an independent container and memory system, allowing it to edit files, execute commands, and access the network within its own container — like having its own computer.

## Key Features

### Multi-Bot Management

Create multiple bots. Humans and bots, or bots with each other, can chat privately, in groups, or collaborate. Build bot teams, or set up accounts for family members to manage daily tasks with bots.

### Containerized Isolation

Each bot runs in its own isolated container (powered by Containerd) with a separate filesystem and network. Bots can freely read/write files and execute commands within their containers without interfering with each other.

### Memory Engineering

A deeply engineered memory layer inspired by Mem0:

- Automatically extracts key facts from each conversation turn and stores them as structured memories
- Supports semantic search (via Qdrant vector database) and keyword retrieval (BM25)
- Loads the last 24 hours of conversation context by default
- Automatic memory compaction to keep the memory store lean
- Multi-language auto-detection

### Multi-Platform Support

Unified channel adapter architecture for connecting to multiple messaging platforms:

- **Telegram** — Full support with streaming, Markdown, attachments, and replies
- **Lark (Feishu)** — Full support
- **Web** — Built-in web chat interface
- **CLI** — Command-line chat

### Agent Capabilities

Bots come with a rich set of built-in tools:

- **Web Search** — Brave Search integration for real-time information
- **Subagents** — Create specialized subagents, assign skills, and delegate complex tasks
- **Skills** — Configurable skill system to extend bot capabilities
- **Container Operations** — Read/write files, edit code, and execute commands inside the container
- **Memory Management** — Search and manage memories
- **Messaging** — Send messages and reactions

### Multi-LLM Provider Support

Flexibly switch between a wide range of LLM providers via four client types:

- OpenAI Responses API, OpenAI Chat Completions API (including compatible services)
- Anthropic Messages API, Google Generative AI API

### MCP Protocol Support

Supports Model Context Protocol (MCP) via HTTP and SSE to connect external tool services. Each bot can have its own independent MCP connections.

### Scheduled Tasks

Configure scheduled tasks using cron expressions to automatically run commands at specified times. Supports max execution count limits and manual triggers.

### Graphical Configuration

Configure bots, channels, providers, models, MCP, skills, and all other settings through a web management UI — no coding required to set up your own AI bot.

### CLI Tool

A command-line tool for bot management, channel configuration, model management, streaming chat, and more — designed for developers who prefer the terminal. See [CLI documentation](/cli/).

## Installation

To get Memoh running:

- **[Docker](/installation/docker)** — Recommended. One-click or manual setup with Docker Compose. Includes all services (PostgreSQL, Qdrant, Containerd, server, agent, web) — no extra dependencies on the host.
- **[config.toml](/installation/config-toml)** — Reference for all configuration fields.
