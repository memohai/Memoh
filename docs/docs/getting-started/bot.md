# Bot Management

A Bot is an independent AI agent that comes with its own isolated container, persistent memory, and configurable personality. Bots can chat via various messaging platforms (Channels) and perform complex tasks using specialized tools.

## Creating a Bot

1. Navigate to the **Bots** page from the sidebar.
2. Click the **Create Bot** button.
3. Fill in the basic info:
    - **Display Name**: The name users will see in chats.
    - **Avatar**: A URL for the bot's profile picture.
4. Click **Create**.

---

## Bot Detail Page

Once created, clicking on a bot card takes you to its **Detail Page**, where you can manage its entire lifecycle through specialized tabs.

### Tab Overview

| Tab | Description |
|-----|-------------|
| **Overview** | Health checks for container, database, channels, and memory. |
| **General** | Core settings: models, providers, reasoning, heartbeat, compaction, and danger zone. |
| **Container** | Container lifecycle (create/start/stop), snapshots, data export/import. |
| **Memory** | Browse, search, create, edit, and compact memories. |
| **Platforms** | Channel configurations (Telegram, Discord, Feishu, QQ, Matrix, WeCom, WeChat, Web). |
| **Access** | ACL rules — control who can chat with the bot. |
| **Email** | Email bindings and outbox. |
| **Terminal** | Interactive terminal access to the bot's container. |
| **Files** | File manager for the bot's container filesystem. |
| **MCP** | MCP connection management (Stdio, Remote, OAuth). |
| **Heartbeat** | Heartbeat configuration and execution logs. |
| **Compaction** | Memory compaction logs. |
| **Schedule** | Cron-based scheduled tasks and execution logs. |
| **Skills** | Markdown-based skill files that define bot personality and capabilities. |

---

## Configuring the Bot's Core Settings

After creating a bot, the most important step is configuring its runtime settings. These define how the bot talks, remembers, searches, and uses browser automation.

1. Navigate to your bot's **Detail Page**.
2. Go to the **General** tab.
3. Configure the core fields:
   - **Chat Model**: Used for standard conversations with users.
   - **Memory Provider**: Select the memory backend the bot should use.
   - **Search Provider**: Select the search engine provider for web search.
   - **Browser Context**: Select the browser profile the bot should use for browser automation.
4. Click **Save** at the bottom of the form.

If you have not created these resources yet, set them up first:

- [LLM Provider and Model](/getting-started/provider-and-model.md)
- [Built-in Memory Provider](/memory-providers/builtin.md)
- [Search Providers](/getting-started/search-provider.md)
- [Browser Contexts](/getting-started/browser.md)

---

## General Tab Reference

The **General** tab contains all the core parameters that define your bot's behavior and runtime configuration.

| Field | Description |
|-------|-------------|
| **Chat Model** | The main LLM used for generating chat responses. |
| **Memory Provider** | The memory backend assigned to the bot. The built-in provider can optionally define its own memory and embedding models. |
| **Search Provider** | The search engine used for web browsing capabilities. |
| **Browser Context** | The browser environment used for web automation, such as viewport, locale, and mobile behavior. |
| **Language** | The bot's primary communication language. |
| **Reasoning Enabled** | If the selected model supports reasoning (like OpenAI o1), enable this to use its deep thinking capabilities. |
| **Reasoning Effort** | Set the level of reasoning effort (`low`, `medium`, `high`). |
| **Heartbeat Enabled** | Toggle periodic autonomous activity. |
| **Heartbeat Interval** | How often (in minutes) the heartbeat triggers. |
| **Heartbeat Model** | The LLM used for heartbeat tasks (can differ from the chat model). |
| **Compaction Enabled** | Toggle automatic memory compaction. |
| **Compaction Model** | The LLM used for memory compaction. |
| **ACL Default Effect** | Default access control behavior (`allow` or `deny`) when no ACL rule matches. |

---

## Terminal Tab

The **Terminal** tab provides interactive shell access to the bot's container:

- Open multiple terminal tabs simultaneously.
- Execute commands directly inside the container.
- Requires the container to be running.

---

## Deleting a Bot

To permanently remove a bot and all its associated data (including container files and memory):
1. Navigate to the **General** tab in the Bot Detail page.
2. Scroll to the **Danger Zone** at the bottom.
3. Click **Delete Bot** and confirm the action.

> **Warning**: This action is irreversible. All persistent data for this bot will be lost.
