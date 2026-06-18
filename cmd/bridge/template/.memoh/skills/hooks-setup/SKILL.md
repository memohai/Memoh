---
name: hooks-setup
description: Configure Memoh hooks in /data/.memoh/hooks.json, including events, matchers, command/tool actions, decisions, on_error behavior, append_context, and validation. Use when the user asks to inspect, create, update, debug, or explain workspace hooks.
---

# Hooks Setup

Use this skill when the user wants to inspect, create, update, debug, or explain Memoh hooks.

## Files

- User hooks live at `/data/.memoh/hooks.json`.
- Detailed schema, events, payloads, and examples live in `references/reference.md`.

Read `references/reference.md` before adding anything beyond an empty config or a simple logging hook.

## Empty Config

If `/data/.memoh/hooks.json` is missing, create:

```json
{
  "version": 1,
  "enabled": true,
  "hooks": []
}
```

## Minimal Workflow

1. Read `/data/.memoh/hooks.json`.
2. Preserve existing hooks unless the user asks to replace them.
3. Choose the narrowest event and matcher.
4. Prefer logging hooks with `on_error: "ignore"`.
5. Use `on_error: "block"` only for guardrail hooks that should deny on failure.
6. Validate JSON syntax before finishing.

## Common Pattern

```json
{
  "version": 1,
  "enabled": true,
  "defaults": {
    "timeout": "10s",
    "on_error": "fail",
    "max_output_bytes": 65536,
    "trigger_nested_hooks": false
  },
  "env": {},
  "hooks": [
    {
      "name": "log workspace commands",
      "event": "AfterWorkspaceCommand",
      "enabled": true,
      "priority": 10,
      "matcher": ".*",
      "actions": [
        {
          "type": "command",
          "command": "mkdir -p .memoh && cat >> .memoh/hooks.log",
          "on_error": "ignore"
        }
      ]
    }
  ]
}
```

## Action Output

Command and tool actions may return:

```json
{
  "decision": "allow",
  "reason": "optional explanation",
  "append_context": "optional context"
}
```

Allowed decisions are `allow`, `deny`, `ask_approval`, and `append_context`.

For full examples and the complete event catalog, read `references/reference.md`.
