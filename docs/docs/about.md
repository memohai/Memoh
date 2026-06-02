# About Memoh

## What Is Memoh?

Memoh is a multi-member, structured long-memory AI agent platform. You can create multiple AI bots, give each bot its own workspace and long-term memory, and interact with them through Telegram, Discord, Lark (Feishu), QQ, Matrix, Misskey, DingTalk, WeCom, WeChat, WeChat Official Account, Email, or the built-in Web UI.

Every bot has its own runtime, tools, memory configuration, channel integrations, and access policy. Depending on deployment mode, that workspace can be an isolated container or a trusted local workspace.

## Distribution Modes

### Desktop

The native desktop client is for personal and local use. It starts a local `memoh-server` on `127.0.0.1:18731`, manages local SQLite storage, starts embedded Qdrant for memory search, bundles the `memoh` CLI, and owns system tray lifecycle behavior.

Desktop is the fastest way to try Memoh on your own machine.

### Server Deploy

Server Deploy is for always-on shared usage. Use it when Memoh should serve multiple users, run continuously on a server, or connect to external channels while your personal computer is offline.

The server stack runs with Docker Compose and includes the backend, Web UI, database, memory services, and workspace runtime.

## What Makes Memoh Different

### Multi-Bot And Multi-User

Memoh is built for real sharing and real separation at the same time:

- create multiple bots for different roles or people
- let humans and bots interact in private chats, groups, or delegated workflows
- distinguish individual users in shared conversations
- bind identities across channels so the same person can be recognized consistently

### Independent Workspaces

Each bot can use an independent workspace for files, commands, MCP hosting, and long-running tasks. Server deployments normally use container workspaces through Docker, containerd, or Apple-backed runtimes. Desktop/local deployments can also use trusted local workspaces when host-level access is intentional.

Container workspaces can provide a full graphical desktop with VNC/RFB transport and a headed Chrome/Chromium browser. This enables workflows that require visible browser state rather than pure headless automation.

### Browser Use And Computer Use

Memoh separates browser and GUI operation into practical layers:

- **Headless browser commands** run as ordinary workspace commands.
- **Browser Use** operates the headed workspace browser over CDP.
- **Computer Use** drives the broader workspace desktop through screenshots, pointer input, and keyboard input.

Prefer Browser Use for web pages. Use Computer Use for native dialogs, non-browser apps, or GUI states that CDP cannot reach.

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

Current user-facing integrations include Telegram, Discord, Lark (Feishu), QQ, Matrix, Misskey, DingTalk, WeCom, WeChat, WeChat Official Account, Email, and Web.

### Tools, Skills, MCP, And Supermarket

Bots can use a rich set of built-in capabilities, including:

- web search and web fetch
- workspace file editing and command execution
- Browser Use and Computer Use
- memory search and management
- messaging, email, and TTS
- subagents for delegated work
- **skills** for reusable behavior modules
- **MCP** connections for external tool servers
- **Supermarket** for curated skill and MCP template installation

### Providers And Models

Memoh supports multiple provider client types, including OpenAI-compatible chat completions, OpenAI Responses API, Anthropic Messages, Google Generative AI, OpenAI Codex, GitHub Copilot, and Edge Speech/TTS.

Models are separated by role:

- **chat** models for normal interaction
- **embedding** models for vector memory and search
- **speech** models for TTS

Image generation is configured through compatible chat/image models rather than a separate image-provider system.

### Operations And UI

The Web UI is designed so you can manage the whole system without editing config files by hand every day. It includes bot configuration tabs, provider/model management, session controls, workspace files, terminal and display panes, skill management, and slash-command control from channels.

## Where To Start

- **[Installation Overview](/installation/)** — choose Desktop or Server Deploy
- **[Providers And Models](/getting-started/provider-and-model)** — configure model access
- **[Bot Setup](/getting-started/bot)** — create and configure a bot
- **[Browser / Computer Use](/getting-started/browser-computer-use)** — understand headed browser and desktop automation
- **[Channels](/getting-started/channels)** — choose where bots are reachable
- **[Skills](/getting-started/skills)** and **[Supermarket](/getting-started/supermarket)** — extend what bots can do
