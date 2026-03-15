# Bot Memory Management

Memoh's structured long-term memory system allows bots to remember information across multiple conversations, providing contextually relevant and personalized interactions.

## Prerequisites

Before using the **Memory** tab, make sure your bot already has a **Memory Provider** configured.

1. Create a provider from one of the [Memory Providers](/memory-providers/index.md) (Built-in, Mem0, or OpenViking).
2. Open your bot's **Settings** tab.
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

---

## Visualization: Vector Manifold

The Memory tab includes visual tools to help you understand how the memory system is performing:

### Top-K Bucket Chart
Shows the distribution of relevant memories retrieved for the most recent queries.

### CDF Curve (Cumulative Distribution Function)
Visualizes the scoring threshold of retrieved memories, helping you fine-tune how much relevant information the bot should consider.

---

## Bot Interaction

- The bot automatically searches and retrieves memories during chat.
- The assigned **Memory Provider** controls the memory backend used by the bot.
- Provider-specific settings (such as memory mode, embedding model, or API keys) are configured in the provider itself — see [Memory Providers](/memory-providers/index.md).
- Memories provide the long-term knowledge that makes each bot unique to its owner.
