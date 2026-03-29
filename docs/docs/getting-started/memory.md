# Bot Memory Management

Memoh's structured long-term memory system allows bots to remember information across multiple conversations, providing contextually relevant and personalized interactions.

## Prerequisites

Before using the **Memory** tab, make sure your bot already has a **Memory Provider** configured.

1. Create a provider from one of the [Memory Providers](/memory-providers/index.md) (Built-in, Mem0, or OpenViking).
2. Open your bot's **General** tab.
3. Select the provider in the **Memory Provider** field.
4. Click **Save**.

Without a memory provider, the bot will not have an active memory backend configuration.

---

## Concept: Memory Retrieval

Memories are stored and retrieved through the assigned memory provider. Depending on the provider type and mode, retrieval may use file-based indexing, sparse vectors, dense embeddings, or an external API. When a user sends a message, Memoh finds the most relevant memories and includes them in the bot's runtime context.

---

## Operations

Manage your bot's memories from the **Memory** tab in the Bot Detail page.

### 1. Creating Memories

- **New Memory**: Manually enter a memory's content in the provided textarea.
- **From Conversation**: Select specific messages from the bot's conversation history to extract into memory.

### 2. Searching and Managing

- **Search**: Filter memories by ID or text content.
- **Edit**: Modify existing memory entries directly in the list.
- **Delete**: Remove memories that are no longer needed.

---

## Memory Compression (Compact)

Over time, memories can accumulate and become redundant. The **Compact** feature helps optimize the memory pool.

- **Ratio**: Set the compression ratio (for example `0.8`, `0.5`, or `0.3`) to determine how much information is retained.
- **Decay Days**: Optionally specify a time window to compact only memories older than a certain number of days.

For more details on compaction, see [Memory Compaction](/getting-started/compaction).

---

## Rebuild

The **Rebuild** feature re-indexes all memories from scratch. This is useful when:

- You have changed the memory provider's mode (e.g., switching from `off` to `sparse`).
- The vector index has become inconsistent.
- You want to re-process all memories with updated settings.

Click **Rebuild** in the Memory tab to start the process. You can monitor the rebuild status in real-time.

---

## Status

The Memory tab shows the current **status** of the memory provider for this bot:

- **Connected** — The memory backend is reachable and operational.
- **Error** — There is an issue with the memory provider configuration or connectivity.

Use the status indicator to quickly verify that the memory system is working before troubleshooting other issues.

---

## Usage Statistics

The Memory tab displays storage usage information:

- **Total Memories** — The number of memory entries stored for this bot.
- **Index Status** — Whether the vector index is up-to-date.

---

## Bot Interaction

- The bot automatically searches and retrieves memories during chat.
- The assigned **Memory Provider** controls the memory backend used by the bot.
- Provider-specific settings (such as memory mode, embedding model, or API keys) are configured in the provider itself — see [Memory Providers](/memory-providers/index.md).
- Memories provide the long-term knowledge that makes each bot unique to its owner.
