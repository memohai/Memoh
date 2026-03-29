# Slash Commands

Memoh bots support **slash commands** — text commands prefixed with `/` that can be sent in any channel (Telegram, Discord, Feishu, Web, etc.) to perform administrative actions without going through the AI agent.

Slash commands are intercepted before they reach the LLM, so they execute instantly and don't consume tokens.

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `/help` | Show all available commands |
| `/new` | Start a new conversation session |
| `/schedule` | Manage scheduled tasks |
| `/mcp` | Manage MCP connections |
| `/settings` | View and update bot settings |
| `/model` | Manage bot models |
| `/memory` | Manage memory provider |
| `/search` | Manage search provider |
| `/browser` | Manage browser context |
| `/usage` | View token usage statistics |
| `/email` | View email configuration |
| `/heartbeat` | View heartbeat logs |
| `/skill` | View bot skills |
| `/fs` | Browse container filesystem |

---

## Command Format

Commands follow the pattern:

```
/resource [action] [arguments...]
```

- **resource** — The command group (e.g., `schedule`, `model`).
- **action** — The sub-command (e.g., `list`, `set`, `create`). Some commands have a default action.
- **arguments** — Additional parameters. Use quotes for values with spaces.

Example: `/schedule create daily-report "0 9 * * *" "Send me a daily summary"`

---

## Permissions

Commands that modify bot settings are marked as **owner-only**. Only the bot owner can execute these commands. Read-only commands (listing, viewing) are available to all users who have chat access.

Owner-only commands are marked with `[owner]` in the `/help` output.

---

## Global Commands

### `/help`

Displays a list of all available commands and their usage.

### `/new`

Starts a new conversation session, resetting the current context. See [Sessions](/getting-started/sessions) for details.

---

## `/schedule` — Manage Scheduled Tasks

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/schedule list` | All |
| `get` | `/schedule get <name>` | All |
| `create` | `/schedule create <name> <pattern> <command>` | Owner |
| `update` | `/schedule update <name> [--pattern P] [--command C]` | Owner |
| `delete` | `/schedule delete <name>` | Owner |
| `enable` | `/schedule enable <name>` | Owner |
| `disable` | `/schedule disable <name>` | Owner |

**Examples:**

```
/schedule list
/schedule create morning-news "0 9 * * *" "Summarize today's top tech news"
/schedule disable morning-news
```

---

## `/mcp` — Manage MCP Connections

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/mcp list` | All |
| `get` | `/mcp get <name>` | All |
| `delete` | `/mcp delete <name>` | Owner |

---

## `/settings` — View and Update Bot Settings

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `get` | `/settings` or `/settings get` | All |
| `update` | `/settings update [options]` | Owner |

**Update options:**

| Option | Values |
|--------|--------|
| `--language` | Language code (e.g., `en`, `zh`) |
| `--acl_default_effect` | `allow` or `deny` |
| `--reasoning_enabled` | `true` or `false` |
| `--reasoning_effort` | `low`, `medium`, or `high` |
| `--heartbeat_enabled` | `true` or `false` |
| `--heartbeat_interval` | Minutes (integer) |
| `--chat_model_id` | Model UUID |
| `--heartbeat_model_id` | Model UUID |

**Example:**

```
/settings update --language en --heartbeat_enabled true --heartbeat_interval 30
```

---

## `/model` — Manage Bot Models

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/model list` | All |
| `set` | `/model set <provider_name> <model_name>` | Owner |
| `set-heartbeat` | `/model set-heartbeat <provider_name> <model_name>` | Owner |

**Example:**

```
/model list
/model set OpenAI gpt-4o
/model set-heartbeat OpenAI gpt-4o-mini
```

---

## `/memory` — Manage Memory Provider

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/memory list` | All |
| `set` | `/memory set <name>` | Owner |

---

## `/search` — Manage Search Provider

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/search list` | All |
| `set` | `/search set <name>` | Owner |

---

## `/browser` — Manage Browser Context

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/browser list` | All |
| `set` | `/browser set <name>` | Owner |

---

## `/usage` — View Token Usage

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `summary` | `/usage` or `/usage summary` | All |
| `by-model` | `/usage by-model` | All |

Shows token usage for the last 7 days, broken down by session type (chat, heartbeat, schedule) or by model.

---

## `/email` — View Email Configuration

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `providers` | `/email providers` | All |
| `bindings` | `/email bindings` | All |
| `outbox` | `/email outbox` | All |

---

## `/heartbeat` — View Heartbeat Logs

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `logs` | `/heartbeat` or `/heartbeat logs` | All |

Shows the 10 most recent heartbeat execution logs.

---

## `/skill` — View Bot Skills

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/skill` or `/skill list` | All |

---

## `/fs` — Browse Container Filesystem

| Sub-command | Usage | Permission |
|-------------|-------|------------|
| `list` | `/fs list [path]` | All |
| `read` | `/fs read <path>` | All |

**Examples:**

```
/fs list /
/fs list /home
/fs read /home/bot/IDENTITY.md
```

File content is truncated to 2000 characters when displayed in chat.

---

## Mention-Prefixed Commands

In group chats, you can prefix commands with a mention:

```
@BotName /help
@BotName /new
```

The bot will strip the mention and process the slash command normally.
