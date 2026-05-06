# Bot Management

A Bot is an independent AI agent that comes with its own isolated container, persistent memory, and configurable personality. Bots can chat via various messaging platforms (Channels) and perform complex tasks using specialized tools.

## Creating a Bot

1. Navigate to the **Bots** page from the sidebar.
2. Click the **Create Bot** button.
3. Fill in the basic info:
   - **Display Name**: The name users will see in chats.
   - **Avatar**: A URL for the bot's profile picture.
   - **Timezone**: Optional per-bot timezone. If left empty, the bot inherits the user or system timezone.
   - **ACL Preset**: Quick-start access policy such as `allow_all` or `private_only`.
4. Click **Create**.

---

## Bot Detail Page

Once created, clicking on a bot card takes you to its **Detail Page**, where you can manage its entire lifecycle through specialized tabs.

### Tab Overview

| Tab | Description |
|-----|-------------|
| **Overview** | Health checks for container, database, channels, and memory. |
| **General** | Core runtime settings: chat/title/image models, memory/search/TTS bindings, timezone, language, reasoning, and danger zone. |
| **Container** | Container lifecycle (create/start/stop), snapshots, data export/import. |
| **Memory** | Browse, search, create, edit, and compact memories. |
| **Platforms** | Channel configurations such as Telegram, Discord, Feishu, QQ, Matrix, WeCom, WeChat, Misskey, DingTalk, and Web. |
| **Access** | ACL rules and default access behavior. |
| **Email** | Email bindings and outbox. |
| **Terminal** | Interactive terminal access to the bot's container. |
| **Files** | File manager for the bot's container filesystem. |
| **MCP** | MCP connection management (Stdio, Remote, OAuth). |
| **Heartbeat** | Heartbeat configuration, model selection, and execution logs. |
| **Compaction** | Session context compaction settings and logs. |
| **Schedule** | Cron-based scheduled tasks and execution logs. |
| **Skills** | Markdown-based skill files that define bot personality and capabilities. |

---

## Configuring the Bot's Core Settings

After creating a bot, the most important step is configuring its runtime settings. These settings are split across a few tabs instead of living in one giant form.

1. Navigate to your bot's **Detail Page**.
2. Start with the **General** tab for chat/runtime bindings.
3. Use the **Heartbeat** tab for scheduled autonomous activity.
4. Use the **Compaction** tab for session context compaction behavior.
5. Use the **Access** tab to refine ACL rules after the initial ACL preset.

If you have not created these resources yet, set them up first:

- [Providers And Models](/getting-started/provider-and-model.md)
- [Built-in Memory Provider](/memory-providers/builtin.md)
- [Search Providers](/getting-started/search-provider.md)
- [TTS Providers](/tts-providers/index.md)

---

## General Tab Reference

The **General** tab contains the settings that shape everyday conversation behavior.

| Field | Description |
|-------|-------------|
| **Chat Model** | The main LLM used for generating chat responses. |
| **Title Model** | Optional model used to generate session titles. |
| **Image Generation Model** | Optional model used by image-generation features. Pick a chat model that supports `image-output`. |
| **Memory Provider** | The memory backend assigned to the bot. The built-in provider can optionally define its own memory and embedding models. |
| **Search Provider** | The search engine used for web browsing capabilities. |
| **TTS Model** | Optional speech model used for text-to-speech output. Speech models come from the TTS Providers flow, not the normal chat provider flow. |
| **Timezone** | Per-bot timezone. If empty, Memoh inherits the user timezone and then falls back to the system timezone. |
| **Language** | The bot's primary communication language. |
| **Reasoning Enabled** | Available when the selected chat model exposes `reasoning` compatibility. |
| **Reasoning Effort** | Set the level of reasoning effort (`low`, `medium`, `high`). |

Notes:

- The **Image Generation Model** is intentionally separate from the normal chat model so you can dedicate an image-capable model only to visual generation tasks.
- The **TTS Model** comes from the [TTS Providers](/tts-providers/index.md) system and uses `speech` models such as Edge TTS voices.
- The selected chat model's `context_window` influences session status reporting and [Context Compaction](/getting-started/compaction).

---

## Heartbeat Tab Reference

Heartbeat is configured from its own tab.

| Field | Description |
|-------|-------------|
| **Heartbeat Enabled** | Enable or disable periodic autonomous activity. |
| **Heartbeat Interval** | How often the heartbeat runs, in minutes. |
| **Heartbeat Model** | Optional dedicated model for heartbeat tasks. This can differ from the main chat model. |

The Heartbeat tab also includes heartbeat execution logs, so you can review what the bot did during autonomous runs.

---

## Compaction Tab Reference

Compaction is now about **session context compaction**, not memory maintenance.

| Field | Description |
|-------|-------------|
| **Compaction Enabled** | Enable or disable automatic context compaction. |
| **Compaction Threshold** | Estimated token threshold that triggers compaction. |
| **Compaction Ratio** | How aggressively the session context should be reduced. |
| **Compaction Model** | Optional dedicated model used to summarize old session context. |

The Compaction tab also exposes compaction logs so you can see recent successful, pending, or failed runs.

For the runtime behavior, see [Context Compaction](/getting-started/compaction).

---

## Access And ACL

At creation time, the bot starts from an **ACL preset**. After that, use the **Access** tab for fine-grained control.

Two layers matter:

- **ACL Preset** gives you a sensible starting policy for a new bot.
- **ACL Default Effect** controls the default result when no rule matches.

Use the **Access** tab to refine conversation, group, and thread rules after the initial setup.

---

## Discuss-Related Advanced Settings

Most users only need the `chat` and `discuss` behavior described in [Sessions](/getting-started/sessions).

If you manage bot settings through the API or custom automation, the settings schema also includes `discuss_probe_model_id` for discuss-mode specific setups. Treat it as an advanced setting rather than a required field for normal bot creation.

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
