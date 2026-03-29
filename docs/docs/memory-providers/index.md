# Memory Providers

Memoh uses a **Memory Provider** to define how a bot stores, retrieves, and manages long-term memory. A bot can bind one memory provider in its **General** tab, and that provider becomes the backend for memory extraction and memory search.

## Available Providers

Memoh supports the following memory providers:

- [Built-in](/memory-providers/builtin.md): The default memory system included with Memoh. Supports three modes — off (file-based), sparse (neural sparse vectors), and dense (embedding-based semantic search). Fully self-hosted.
- [Mem0](/memory-providers/mem0.md): SaaS memory provider via the Mem0 API. Requires an API key.
- [OpenViking](/memory-providers/openviking.md): Self-hosted or SaaS memory provider with its own API.

---

## Basic Flow

1. Open the **Memory Providers** page from the sidebar.
2. Create a provider instance using one of the supported provider types.
3. Configure the provider settings.
4. Open a bot's **General** tab and assign that provider in **Memory Provider**.
5. Manage actual memories from the bot's **Memory** tab.

---

## Next Steps

- [Built-in Memory Provider](/memory-providers/builtin.md) — Default, self-hosted with three memory modes.
- [Mem0 Memory Provider](/memory-providers/mem0.md) — SaaS via Mem0 API.
- [OpenViking Memory Provider](/memory-providers/openviking.md) — Self-hosted or SaaS.
- [Bot Memory Management](/getting-started/memory.md) — Manage memory entries after the provider is assigned.
