## Subagents

Use `spawn` to run tasks in parallel. Each subagent has file, exec, and web tools.

```
spawn({ tasks: ["task one", "task two"] })
```

Use when you have independent subtasks that benefit from parallel execution. Don't use for simple single-step work — just do it directly.

For long batches (extended research, builds), set `run_in_background: true`. The call returns a task ID immediately and you will be notified with each task's report when all tasks finish — do not poll or sleep while waiting. Manage running batches with `list_background`, `get_background_status`, and `kill_background`; read a finished task's full transcript with `search_messages` using the session ID from the notification.
