# Bot Management

A Bot is an independent AI agent that comes with its own isolated container, persistent memory, and configurable personality. Bots can chat via various messaging platforms (Channels) and perform complex tasks using specialized tools.

## Creating a Bot

1. Navigate to the **Bots** page from the sidebar.
2. Click the **Create Bot** button.
3. Fill in the basic info:
    - **Display Name**: The name users will see in group chats.
    - **Avatar**: A URL for the bot's profile picture.
    - **Type**: Choose `personal` (private to owner) or `public` (accessible to guests).
4. Click **Create**.

---

## Bot Detail Page

Once created, clicking on a bot card takes you to its **Detail Page**, where you can manage its entire lifecycle through several specialized tabs.

### Overview Tab

The **Overview** tab provides a quick health check of the bot's services. It monitors:
- Container status (running/stopped)
- Database connectivity
- Channel configurations
- Memory system health

If any check shows a warning or error, follow the provided details to troubleshoot.

### Settings Tab

The **Settings** tab is where you configure the bot's "brain" and runtime parameters.

| Field | Description |
|-------|-------------|
| **Chat Model** | The main LLM used for generating chat responses. |
| **Memory Model** | The LLM used for summarizing context and managing memories. |
| **Embedding Model** | The model used to generate vector embeddings for semantic memory search. |
| **Search Provider** | The search engine used for web browsing capabilities. |
| **Max Context Load Time** | Time limit (seconds) for loading context before generation. |
| **Max Context Tokens** | Token limit for the loaded conversation history. |
| **Language** | The bot's primary communication language. |
| **Reasoning Enabled** | If the selected model supports reasoning (like OpenAI o1), enable this to use its deep thinking capabilities. |
| **Reasoning Effort** | Set the level of reasoning effort (`low`, `medium`, `high`). |
| **Allow Guest** | (Public bots only) If enabled, non-registered users can interact with the bot. |

---

## Deleting a Bot

To permanently remove a bot and all its associated data (including container files and memory):
1. Navigate to the **Settings** tab in the Bot Detail page.
2. Scroll to the **Danger Zone** at the bottom.
3. Click **Delete Bot** and confirm the action.

> **Warning**: This action is irreversible. All persistent data for this bot will be lost.
