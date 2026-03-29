# Memory Compaction

As a bot accumulates memories over time, the memory pool can grow large and contain redundant or outdated entries. **Memory Compaction** is an automated process that consolidates and optimizes the bot's memory store, keeping the most relevant information while reducing noise.

---

## Concept: Why Compact?

Each conversation turn can generate new memory entries. Over weeks or months of use, thousands of memories accumulate. Many of these may overlap, become stale, or lose relevance. Compaction addresses this by:

- **Merging redundant memories** — Combining entries that express the same information.
- **Removing outdated entries** — Discarding memories that are no longer accurate.
- **Reducing retrieval noise** — Fewer, higher-quality memories lead to better search results during chat.

---

## Configuration

Configure compaction from the **General** tab in the Bot Detail page.

| Field | Description |
|-------|-------------|
| **Compaction Enabled** | Toggle automatic memory compaction on or off. |
| **Compaction Model** | The LLM used to evaluate and merge memories during compaction. This can be different from the chat model. |

When enabled, compaction runs periodically as part of the bot's memory maintenance cycle.

---

## Manual Compaction

You can also trigger compaction manually from the bot's **Memory** tab:

1. Navigate to the **Memory** tab in the Bot Detail page.
2. Click **Compact**.
3. Configure the compaction parameters:
   - **Ratio** — The compression ratio (e.g., `0.8`, `0.5`, `0.3`). Lower values mean more aggressive compaction.
   - **Decay Days** — Optionally restrict compaction to memories older than a specified number of days.
4. Click **Start Compaction**.

---

## Compaction Logs

The **Compaction** tab in the Bot Detail page provides an audit trail of all compaction runs:

- **Status** — Whether the compaction completed successfully, encountered an issue, or failed.
- **Time** — When the compaction was triggered.
- **Duration** — How long the compaction took.
- **Result** — A summary of what was compacted (memories merged, removed, etc.).

### Managing Logs

- **Refresh** — Reload the log list.
- **Clear Logs** — Remove old compaction records.

---

## Relationship to Memory

Compaction works with whatever **Memory Provider** is assigned to the bot. The compaction process:

1. Reads all existing memories from the provider.
2. Uses the configured **Compaction Model** to evaluate which memories are redundant or stale.
3. Merges, updates, or removes entries as needed.
4. Writes the optimized memory set back to the provider.

This process preserves the semantic content of important memories while reducing the total count. After compaction, the bot's memory retrieval becomes faster and more focused.

---

## Next Steps

- To manage individual memories, see [Memory Management](/getting-started/memory).
- To configure the memory backend, see [Memory Providers](/memory-providers/index).
