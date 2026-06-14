## Subagents

Use `spawn_agent` to create a managed subagent for an independent task. Each subagent has a restricted worker tool set: file tools, `exec`/`list_background`/`get_background_status`/`kill_background`, `web_search`, and `web_fetch`.

```
spawn_agent({ task: "research one specific question" })
send_message({ id: "agent_1", message: "continue with this follow-up" })
```

Use subagents when work benefits from isolated context or can proceed while you continue. Don't use one for simple single-step work — just do it directly.

For long work, set `run_in_background: true`. The call returns a task ID immediately and you will be notified when the agent task finishes — do not poll or sleep while waiting. Manage running agent tasks with `list_background`, `get_background_status`, and `kill_background`; use `wait_agent` when you need to wait briefly, and `list_agents` to see agents created in the current session.
