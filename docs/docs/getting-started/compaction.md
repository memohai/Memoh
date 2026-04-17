# Context Compaction

**Context Compaction** reduces the prompt footprint of a single conversation session by summarizing older turns and keeping the active context smaller.

This page is about **session context**, not long-term memory storage. If you want to merge or rewrite stored memories in a memory provider, see [Bot Memory Management](/getting-started/memory).

---

## Why It Exists

As a conversation grows, the bot needs to send more prior messages back to the model. That increases:

- token usage
- latency
- pressure on the model's context window
- the chance that older but still important turns will crowd out newer ones

Context compaction helps by replacing older conversational detail with a shorter summary that still preserves enough continuity for the next turns.

---

## What It Changes

Context compaction affects the **active session context** only.

It does **not**:

- delete the bot itself
- change the configured memory provider
- merge long-term memory records
- replace the need for memory search

In practice, it changes how much historical session text is carried into future model calls.

---

## Automatic Compaction

Configure automatic context compaction from the bot's **General** tab.

Relevant fields:

| Field | Description |
|-------|-------------|
| **Compaction Enabled** | Enable or disable automatic context compaction for this bot. |
| **Compaction Threshold** | Estimated token threshold that triggers background compaction. |
| **Compaction Ratio** | How aggressively the session should be reduced during compaction. |
| **Compaction Model** | The model used to summarize old session context. |

When enabled, Memoh can compact context in the background after a turn when the estimated input size passes the configured threshold.

Memoh also uses the selected model's `context_window` to understand how close the session is to the available budget.

---

## Immediate Compaction

You can trigger compaction immediately for the current session in two ways:

### From The Session Status Panel

1. Open the active conversation.
2. Open the session status panel.
3. Click **Compact Now**.

The status panel also shows the current context usage, cache hit rate, and used skills, which helps you decide whether compaction is useful right now.

### From Slash Commands

Run:

```text
/compact
```

or:

```text
/compact run
```

This runs synchronous context compaction for the current session and returns a status result to chat.

---

## Status And Logs

The **Compaction** tab in the bot detail page provides an audit trail for context compaction runs.

Typical fields include:

- **Status** — whether the compaction finished successfully or failed
- **Summary** — the compacted summary text or a summary preview
- **Message Count** — how many messages were involved
- **Started / Completed Time** — when the run happened
- **Model / Usage** — metadata about the model and token usage when available

The log list is useful when you want to verify that automatic compaction is actually running or diagnose a failure.

---

## Relationship To `context_window`

Memoh tracks the current session against the selected chat model's `context_window`.

You can see this in:

- the Web UI session status panel
- the `/status` slash command

Compaction becomes more valuable as the active session gets closer to the model's context limit. A dedicated compaction model can also be used to summarize more cheaply than the main chat model.

---

## Context Compaction vs Memory Compaction

These two features sound similar but solve different problems:

| Feature | Scope | Trigger | Result |
|---------|-------|---------|--------|
| **Context Compaction** | One active session | Session panel or `/compact` | Summarizes older chat history for future turns |
| **Memory Compaction** | Long-term memory provider | Memory tab | Rewrites stored memory entries |

Use **Context Compaction** when one conversation has become too large.

Use **Memory Compaction** when the bot's stored memories themselves have become noisy or redundant.

---

## Next Steps

- To inspect session runtime information, see [Sessions](/getting-started/sessions).
- To understand slash-triggered compaction, see [Slash Commands](/getting-started/slash-commands).
- To manage long-term memory instead of session context, see [Bot Memory Management](/getting-started/memory).
