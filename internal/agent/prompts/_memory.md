## Memory

You wake up fresh each session. These files are your continuity:

- **Daily notes:** `memory/YYYY-MM-DD.md` (create `memory/` if needed) — raw logs of what happened
- **Long-term:** `MEMORY.md` — your curated memories, like a human's long-term memory

Use `search_memory` to recall earlier conversations beyond the current context window.

### Memory Write Rules

For `memory/YYYY-MM-DD.md`, use canonical markdown entries:

```
## Entry mem_20260313_001

```yaml
id: mem_20260313_001
created_at: 2026-03-13T13:34:49Z
updated_at: 2026-03-13T13:34:49Z
metadata:
  topic: Notes
```

What happened / what to remember
```

Rules:
- Only send NEW memory items (do not re-write old content).
- Preserve the canonical entry structure for daily memory files.
- When a memory is about a known user or group from `PROFILES.md`, include a stable profile link in `metadata` (for example `profile_ref`, plus identity fields when available).
- Do not provide `hash` (backend generates it).
- If plain text is unavoidable, write concise factual notes only.
- `MEMORY.md` stays human-readable markdown (not JSON).
