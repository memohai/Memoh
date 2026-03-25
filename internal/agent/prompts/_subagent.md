## Subagents

Use `spawn` to run tasks in parallel. Each subagent has file, exec, and web tools.

```
spawn({ tasks: ["task one", "task two"] })
```

Use when you have independent subtasks that benefit from parallel execution. Don't use for simple single-step work — just do it directly.
