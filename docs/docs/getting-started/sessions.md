# Sessions

A **Session** is an independent conversation thread between a user and a bot. Each session maintains its own context window and message history, allowing the bot to focus on a specific topic or task without interference from other conversations.

---

## Concept: Conversation Isolation

When you chat with a bot, your messages are grouped into a session. The bot uses the session's history to maintain context. Starting a new session resets this context, giving you a fresh conversation without losing the previous one.

Sessions are scoped per bot — each bot manages its own set of sessions independently.

---

## Session Types

Memoh uses five session types to separate different kinds of bot activity:

| Type | Description |
|------|-------------|
| **Chat** | Standard user-initiated conversations. This is the default session type when chatting with a bot. |
| **Discuss** | Observation-oriented conversation mode. The bot may stay silent by default and only speaks when it decides to send a real reply into the conversation. |
| **Heartbeat** | Automatically created when a bot's heartbeat triggers. Contains the bot's periodic autonomous activity. |
| **Schedule** | Created when a scheduled task fires. Contains the bot's execution of a cron-triggered command. |
| **Subagent** | Created when the bot delegates a task to a subagent. Contains the subagent's independent work context. |

Only **Chat** and **Discuss** sessions are directly created from user conversation routes. The other session types are system-managed and appear as read-only records in the session list.

### Chat vs Discuss

`chat` and `discuss` are the two session types you are most likely to see in normal conversation threads.

**Chat** means:

- the conversation behaves like a normal direct assistant exchange
- users expect a visible reply when they send a prompt
- this is the default in the Web UI and in direct-message style conversations

**Discuss** means:

- the bot is observing an ongoing conversation, often in a group
- the model's direct text output is treated as internal monologue
- the bot only speaks to the conversation when it explicitly issues a `send` action
- staying silent is valid and often desirable

In practice, `discuss` is what makes Memoh feel less like a synchronous chatbot and more like a participant that can decide whether to join in.

---

## Starting a New Session with `/new`

The `/new` slash command creates a fresh session on the current conversation route, resetting the active session context without deleting old history.

Supported forms:

- `/new` — create a new session using the default session type for the current context
- `/new chat` — force a normal chat session
- `/new discuss` — force a discuss session

Default routing behavior:

- **Web UI local chat** defaults to `chat`
- **private conversations** default to `chat`
- **group conversations on channel adapters** default to `discuss`

`/new discuss` is not supported from the built-in Web UI local channel. Use a real channel adapter such as Telegram, Discord, or Misskey if you want to explicitly create discuss sessions.

This works across supported channels:

### In External Channels (Telegram, Discord, Feishu, etc.)

Send `/new`, `/new chat`, or `/new discuss` as a message to the bot. The bot will:

1. Create a new session of the requested or inferred type.
2. Route all subsequent messages from you to this new session.
3. The previous session's history is preserved but no longer active.

This is especially useful when:

- You want to change topics without the bot referencing old context.
- The conversation has become too long and you want a clean start.
- You are switching between different tasks.

### In the Web UI

The Web UI provides a session sidebar where you can:

- Click the **New Session** button to create a fresh chat session.
- Switch between existing sessions by clicking on them.
- Search sessions by content.
- Filter sessions by type (`chat`, `discuss`, `heartbeat`, `schedule`, `subagent`).
- Rename or delete sessions.

---

## Managing Sessions

### Viewing Sessions

In the Web UI, the session sidebar lists all sessions for the currently selected bot. Each entry shows:

- **Title** — The session name (auto-generated or user-defined).
- **Type** — The session type icon.
- **Last Activity** — When the session was last active.

For everyday chat use, `chat` and `discuss` sessions are intentionally shown together because they both represent user-facing conversation threads.

### Renaming Sessions

Click on a session title to rename it. This helps organize conversations by topic.

### Deleting Sessions

Remove sessions you no longer need. Deleting a session removes its message history permanently.

---

## Session Status Panel

The session status panel provides a compact runtime summary for the active session. It is the same information surfaced by `/status`.

Key fields include:

- **Messages** — total message count in the session
- **Context Usage** — current used tokens relative to the selected model's `context_window`
- **Cache Hit Rate** — how much of the input came from cache reads
- **Cache Read / Cache Write** — token counts associated with caching
- **Skills** — effective skills used by the session

The panel also exposes **Compact Now**, which triggers immediate [Context Compaction](/getting-started/compaction) for the current session.

---

## How Sessions Relate to Other Features

- **Discuss** sessions are optimized for channels where the bot should observe and selectively speak, especially in group conversations.
- **Heartbeat** sessions are created on each heartbeat trigger. You can view what the bot did during its autonomous activity by opening the corresponding heartbeat session.
- **Schedule** sessions are created when a scheduled task runs. Check these to see the results of cron-triggered commands.
- **Subagent** sessions track delegated tasks. They show the independent work context of each subagent invocation.
- **Memory** is shared across all sessions for a bot — memories extracted from one session are available in all others.
