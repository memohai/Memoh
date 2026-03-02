# Bot Memory Management

Memoh's structured long-term memory system allows Bots to remember information across multiple conversations, providing contextually relevant and personalized interactions.

## Concept: Semantic Search

Memories are stored in a **Qdrant** vector database. When a user asks a question, Memoh performs a semantic search to find the most relevant "memories" and includes them in the bot's system prompt.

---

## Operations

Manage your bot's memories from the **Memory** tab in the Bot Detail page.

### 1. Creating Memories

- **New Memory**: Manually enter a memory's content in the provided textarea.
- **From Conversation**: Select specific messages from the bot's conversation history to "extract" into memory.

### 2. Searching and Managing

- **Search**: Filter memories by ID or text content.
- **Edit**: Modify existing memory entries directly in the list.
- **Delete**: Remove memories that are no longer needed.

---

## Memory Compression (Compact)

Over time, memories can accumulate and become redundant. The **Compact** feature helps optimize the memory pool.

- **Ratio**: Set the compression ratio (e.g., `0.8`, `0.5`, `0.3`) to determine how much information is retained.
- **Decay Days**: Optionally specify a time window to only compact memories older than a certain number of days.

---

## Visualization: Vector Manifold

The Memory tab includes advanced visual tools to help you understand how the memory system is performing:

### Top-K Bucket Chart
Shows the distribution of relevant memories retrieved for the most recent queries.

### CDF Curve (Cumulative Distribution Function)
Visualizes the scoring threshold of retrieved memories, helping you fine-tune how much "relevant" information the bot should consider.

---

## Bot Interaction

- The bot will automatically search for and retrieve memories during every interaction.
- The **Memory Model** configured in the **Settings** tab is used for extracting and summarizing these memories.
- Memories provide the "long-term knowledge" that makes each bot unique to its owner.
