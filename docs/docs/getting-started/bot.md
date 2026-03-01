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

## Configuring the Bot's Brain (Models)

After creating a bot, the most important step is assigning its LLM models. These define how the bot thinks, remembers, and processes information.

1.  Navigate to your bot's **Detail Page**.
2.  Go to the **Settings** tab.
3.  In the **Model Selection** section, you will find three dropdowns:
    -   **Chat Model**: Used for standard conversations with users. Select a high-quality chat model (e.g., GPT-4o).
    -   **Memory Model**: Used for summarizing context and extracting key facts into the bot's long-term memory.
    -   **Embedding Model**: Used to convert text into vector embeddings for semantic search within the memory system. This must be an `embedding` type model.
4.  Select the models you previously configured in the [Models](/getting-started/provider-and-model) page.
5.  Click **Save** at the bottom of the form.

---

## Settings Tab Reference

The **Settings** tab contains all the core parameters that define your bot's behavior and runtime configuration.

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
