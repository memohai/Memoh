# About Memoh

## What Is Memoh?

Memoh is a multi-member, structured long-memory, containerized AI agent platform. You can create multiple AI bots, give each bot its own isolated workspace and long-term memory, and interact with them through Telegram, Discord, Lark (Feishu), QQ, Matrix, Misskey, DingTalk, WeCom, WeChat, WeChat Official Account, Email, or the built-in Web UI.

Every bot has its own execution environment, tools, memory configuration, and channel integrations. In practice, that means each bot behaves more like its own computer-backed agent than a shared chat preset.

## What Makes Memoh Different

### Multi-Bot And Multi-User

Memoh is built for real sharing and real separation at the same time:

- create multiple bots for different roles or people
- let humans and bots interact in private chats, groups, or delegated workflows
- distinguish individual users in shared conversations
- bind identities across channels so the same person can be recognized consistently

### Containerized Workspaces

Each bot runs in its own isolated container workspace with a separate filesystem and network boundary. Bots can read and write files, run commands, and use tools inside that workspace without interfering with other bots.

### Long-Term Memory And Context Management

Memoh separates two different problems:

- **Long-term memory** stores durable facts and recalls them across conversations through memory providers
- **Session context compaction** reduces the prompt size of an active session when the current conversation gets too large

This distinction is important: context compaction changes the active session window, while memory compaction rewrites stored memory entries.

### Sessions And Discuss Mode

Each bot maintains independent **sessions** that preserve context. Memoh currently uses five session types:

- **Chat** — regular user-facing conversations
- **Discuss** — deliberative sessions where the bot can think through work and decide what to send outward
- **Heartbeat** — periodic autonomous sessions
- **Schedule** — cron-triggered task sessions
- **Subagent** — delegated task sessions

You can start or route sessions with slash commands such as `/new`, and the Web UI exposes a session status panel with metrics like context usage, cache hit rate, and used skills.

### Broad Channel Coverage

Memoh uses a unified channel adapter system so one bot can be reachable from many places at once.

Current user-facing integrations include:

- **Telegram**
- **Discord**
- **Lark (Feishu)**
- **QQ**
- **Matrix**
- **Misskey**
- **DingTalk**
- **WeCom**
- **WeChat**
- **WeChat Official Account**
- **Email**
- **Web**

Memoh also distinguishes between the personal **WeChat** QR-login integration and the webhook-based **WeChat Official Account** integration.

### Tools, Skills, MCP, And Supermarket

Bots can use a rich set of built-in capabilities, including:

- web search and web fetch
- browser automation
- file editing and command execution inside the bot workspace
- memory search and management
- messaging, email, and TTS
- subagents for delegated work
- **skills** for reusable behavior modules
- **MCP** connections for external tool servers
- **Supermarket** for curated skill and MCP template installation

### Providers And Models

Memoh supports multiple provider client types, including:

- OpenAI-compatible chat completions
- OpenAI Responses API
- Anthropic Messages
- Google Generative AI
- OpenAI Codex
- GitHub Copilot
- Edge Speech / TTS

Models are also separated by role:

- **chat** models for normal interaction
- **embedding** models for vector memory and search
- **speech** models for TTS

Image generation is configured through compatible chat/image models rather than a separate image-provider system.

### Operations And UI

The Web UI is designed so you can manage the whole system without editing config files by hand every day. It includes:

- bot configuration tabs for general settings, access, channels, heartbeat, compaction, and more
- provider and model management with OAuth flows where supported
- session-side controls such as immediate compaction and status inspection
- skill management with effective / shadowed / disabled visibility
- slash-command driven control from channels

## Where To Start

- **[Docker Installation](/installation/docker)** — get the stack running
- **[Providers And Models](/getting-started/provider-and-model)** — configure model access
- **[Bot Setup](/getting-started/bot)** — create and configure a bot
- **[Sessions](/getting-started/sessions)** — understand chat vs discuss behavior
- **[Channels](/getting-started/channels)** — choose where bots are reachable
- **[Skills](/getting-started/skills)** and **[Supermarket](/getting-started/supermarket)** — extend what bots can do
- **[Slash Commands](/getting-started/slash-commands)** — operate bots directly from chat
