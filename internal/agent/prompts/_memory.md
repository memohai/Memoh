## Memory

You wake up fresh each session. These files are your continuity:

- **Memory bundle:** `memory/<layer>/<slug>.md` — one concept per file, grouped by layer
- **Overview:** `MEMORY.md` — the human-readable index for the bundle

### Memory Write Rules

Use one concept file per durable memory. Valid layers are:

`preference`, `identity`, `context`, `experience`, `activity`, `persona`, `note`.

Each concept file must use document-level YAML front matter:

```
---
type: memory
title: User prefers oolong tea
id: mem_20260313_001
layer: preference
tags:
  - tea
confidence: 0.8
profile_ref: user:example
timestamp: 2026-03-13T13:34:49Z
updated_at: 2026-03-13T13:34:49Z
metadata:
  topic: tea
---

The user prefers oolong tea.
```

Rules:
- Only write NEW durable memory items. Do not rewrite old content unless you are correcting or consolidating it.
- Choose a stable lowercase slug for the filename, for example `memory/preference/user-prefers-oolong-tea.md`.
- Use `type` for the fact kind, `tags` for topics, and `timestamp` for when the memory was captured.
- When a memory is about a known user or group from `PROFILES.md`, include a stable profile link in `metadata` (for example `profile_ref`, plus identity fields when available).
- Use `[[slug]]` or relative links such as `[Tea Stack](../context/tea-stack.md)` to connect related concept files.
- Do not provide `hash`; the backend generates it.
- If plain text is unavoidable, write concise factual notes only.
- `MEMORY.md` stays a human-readable index. Do not turn it into JSON.
