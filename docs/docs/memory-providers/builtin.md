# Built-in Memory Provider

The built-in memory provider is the standard memory backend shipped with Memoh. It works with Memoh's memory pipeline and supports:

- Automatic memory extraction from conversations
- Semantic memory retrieval during chat
- Manual memory creation and editing
- Memory compaction and rebuild workflows

The built-in provider operates in one of three **memory modes**, each with different infrastructure requirements and retrieval capabilities.

---

## Memory Modes

| Mode | Index | Requirements | Use Case |
|------|-------|-------------|----------|
| **Off** | File-based only | None | Lightweight setup, no vector search |
| **Sparse** | Neural sparse vectors | Sparse service + Qdrant (`--profile sparse`) | Good retrieval quality without embedding API costs |
| **Dense** | Dense embeddings | Embedding model + Qdrant (`--profile qdrant`) | Highest-quality semantic search |

### How Sparse Mode Works

Sparse mode uses the [`opensearch-neural-sparse-encoding-multilingual-v1`](https://huggingface.co/opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1) model (from the OpenSearch project) to convert text into sparse vectors — compact lists of token indices with importance weights. Unlike dense mode, which requires an external embedding API, the sparse model runs locally in the `sparse` container with no API key or cost. It supports multiple languages and provides significantly better retrieval quality than keyword-only search.

---

## Creating a Built-in Provider

1. Navigate to the **Memory Providers** page.
2. Click **Add Memory Provider**.
3. Fill in the following fields:
   - **Name**: A display name for this provider.
   - **Provider Type**: Select `builtin`.
4. Click **Create**.

---

## Configuring a Built-in Provider

After creating a provider, select it from the list and configure its settings.

| Field | Description |
|-------|-------------|
| **Memory Mode** | `off` (default), `sparse`, or `dense`. Controls how memories are indexed and retrieved. |
| **Embedding Model** | Embedding model for dense vector search. Only used in `dense` mode. |
| **Qdrant Collection** | Qdrant collection name. Defaults to `memory_sparse`. |

### Managing Providers

- **Edit**: Select a provider and update its settings.
- **Delete**: Remove a provider you no longer use.

---

## Infrastructure Requirements

### Off Mode

No additional infrastructure required. Memories are stored and retrieved using file-based indexing only.

### Sparse Mode

Requires the **sparse service** (runs the [`opensearch-neural-sparse-encoding-multilingual-v1`](https://huggingface.co/opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1) model locally) and **Qdrant** vector database. Enable both with Docker Compose profiles:

```bash
docker compose --profile qdrant --profile sparse up -d
```

The following sections must be present in `config.toml`:

```toml
[qdrant]
base_url = "http://qdrant:6334"

[sparse]
base_url = "http://sparse:8085"
```

### Dense Mode

Requires an **embedding model** (configured in the provider settings) and **Qdrant**:

```bash
docker compose --profile qdrant up -d
```

The Qdrant section must be present in `config.toml`:

```toml
[qdrant]
base_url = "http://qdrant:6334"
```

---

## Assigning a Memory Provider to a Bot

1. Navigate to the **Bots** page and open your bot.
2. Go to the **General** tab.
3. Find the **Memory Provider** dropdown.
4. Select the provider you created.
5. Click **Save**.

If no memory provider is selected, the bot will not use that provider configuration in its runtime settings.

---

## Using Memory After Setup

Once a memory provider is assigned to the bot, you can manage actual memories from the bot's **Memory** tab:

- Create memories manually
- Extract memories from conversations
- Search, edit, and delete memories
- Compact or rebuild the memory store

For day-to-day memory operations, continue with [Bot Memory Management](/getting-started/memory.md).
