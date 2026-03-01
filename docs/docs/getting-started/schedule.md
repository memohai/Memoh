# Scheduled Tasks

Scheduled Tasks allow you to automate recurring bot actions using cron expressions. This is useful for sending periodic reports, checking external data, performing maintenance, or triggering any routine task without manual intervention.

## Concept: Cron-Based Automation

A **Schedule** is a cron-triggered rule bound to a specific bot. When a schedule fires, it sends a command to the agent, which executes it using its available tools, skills, and context. The bot can then deliver results to any connected channel.

---

## Schedule Fields

| Field | Description |
|-------|-------------|
| **Name** | A display name for the task (e.g., "Morning News Summary"). |
| **Description** | A brief explanation of what the task does. |
| **Pattern** | A cron expression that defines when the task runs (e.g., `0 9 * * *` for daily at 9:00 AM). |
| **Command** | The natural-language instruction sent to the agent when the schedule triggers (e.g., "Summarize today's top tech news and send it to the Telegram channel"). |
| **Enabled** | Whether the schedule is currently active. |
| **Max Calls** | Optional limit on the total number of executions. Leave empty for unlimited. |
| **Current Calls** | The number of times this schedule has already fired. |

---

## Cron Pattern Reference

The `pattern` field uses standard cron syntax with five fields:

```
┌───────────── minute (0–59)
│ ┌───────────── hour (0–23)
│ │ ┌───────────── day of month (1–31)
│ │ │ ┌───────────── month (1–12)
│ │ │ │ ┌───────────── day of week (0–6, Sun=0)
│ │ │ │ │
* * * * *
```

**Common examples:**

| Pattern | Meaning |
|---------|---------|
| `0 9 * * *` | Every day at 9:00 AM |
| `*/30 * * * *` | Every 30 minutes |
| `0 0 * * 1` | Every Monday at midnight |
| `0 8,20 * * *` | Every day at 8:00 AM and 8:00 PM |
| `0 0 1 * *` | First day of every month at midnight |

---

## Viewing Schedules

1. Navigate to the Bot **Detail Page**.
2. Select the **Schedule** tab.
3. The list shows all schedules for this bot, including their name, cron pattern, enabled status, and execution counts.
4. Click **Refresh** to reload the list.

---

## Creating Schedules

Schedules are primarily created in two ways:

### Via the Bot (Conversational)

The bot itself has access to a `schedule` tool. You can ask the bot to create a schedule in natural language:

> "Create a schedule called 'Daily Digest' that runs every day at 8 AM and sends me a summary of my unread emails."

The bot will translate this into a cron expression and register the schedule automatically.

### Via the API

You can also create schedules programmatically using the REST API:

```
POST /api/bots/{bot_id}/schedule
```

```json
{
  "name": "Daily Digest",
  "description": "Summarize unread emails every morning",
  "pattern": "0 8 * * *",
  "command": "Summarize my unread emails and send the result to Telegram.",
  "enabled": true,
  "max_calls": null
}
```

---

## Execution Flow

When a schedule fires:

1. The cron service triggers the schedule.
2. The `current_calls` counter is incremented.
3. If `max_calls` is set and reached, the schedule is automatically disabled.
4. The agent receives the `command` along with the schedule context.
5. The agent executes the command using its tools (e.g., web search, file read, send message).
6. Results can be delivered to any connected channel.

---

## Schedule vs. Heartbeat

| Feature | Schedule | Heartbeat |
|---------|----------|-----------|
| **Trigger** | Cron expression (flexible timing) | Fixed interval (minutes) |
| **Command** | Custom natural-language instruction | Generic "routine check" prompt |
| **Max Calls** | Optional execution limit | Unlimited |
| **Use Case** | Specific recurring tasks | Periodic autonomous thinking |

Both features complement each other: use **Heartbeat** for general-purpose periodic activity and **Schedule** for precise, task-oriented automation.
