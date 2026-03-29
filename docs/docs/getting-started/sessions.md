# Sessions

A **Session** is an independent conversation thread between a user and a bot. Each session maintains its own context window and message history, allowing the bot to focus on a specific topic or task without interference from other conversations.

---

## Concept: Conversation Isolation

When you chat with a bot, your messages are grouped into a session. The bot uses the session's history to maintain context. Starting a new session resets this context, giving you a fresh conversation without losing the previous one.

Sessions are scoped per bot — each bot manages its own set of sessions independently.

---

## Session Types

Memoh uses four session types to separate different kinds of bot activity:

| Type | Description |
|------|-------------|
| **Chat** | Standard user-initiated conversations. This is the default session type when chatting with a bot. |
| **Heartbeat** | Automatically created when a bot's heartbeat triggers. Contains the bot's periodic autonomous activity. |
| **Schedule** | Created when a scheduled task fires. Contains the bot's execution of a cron-triggered command. |
| **Subagent** | Created when the bot delegates a task to a subagent. Contains the subagent's independent work context. |

Only **Chat** sessions are directly created by users. The other types are system-managed and appear as read-only records in the session list.

---

## Starting a New Session with `/new`

The `/new` slash command creates a fresh chat session, resetting the conversation context. This works across all channels:

### In External Channels (Telegram, Discord, Feishu, etc.)

Send `/new` as a message to the bot. The bot will:

1. Create a new chat session.
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
- Filter sessions by type (chat, heartbeat, schedule, subagent).
- Rename or delete sessions.

---

## Managing Sessions

### Viewing Sessions

In the Web UI, the session sidebar lists all sessions for the currently selected bot. Each entry shows:

- **Title** — The session name (auto-generated or user-defined).
- **Type** — The session type icon.
- **Last Activity** — When the session was last active.

### Renaming Sessions

Click on a session title to rename it. This helps organize conversations by topic.

### Deleting Sessions

Remove sessions you no longer need. Deleting a session removes its message history permanently.

---

## How Sessions Relate to Other Features

- **Heartbeat** sessions are created on each heartbeat trigger. You can view what the bot did during its autonomous activity by opening the corresponding heartbeat session.
- **Schedule** sessions are created when a scheduled task runs. Check these to see the results of cron-triggered commands.
- **Subagent** sessions track delegated tasks. They show the independent work context of each subagent invocation.
- **Memory** is shared across all sessions for a bot — memories extracted from one session are available in all others.
