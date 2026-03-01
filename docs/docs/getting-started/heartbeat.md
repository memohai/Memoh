# Bot Heartbeat

The **Heartbeat** feature allows you to schedule periodic tasks for your Bot, enabling it to perform autonomous actions even when you aren't chatting with it.

## Concept: Scheduled Autonomy

A **Heartbeat** is a recurring trigger that prompts the bot to "think" and execute its skills or tools at a set interval. This is useful for:
- Periodic status checks
- Automatic data collection
- Cleaning up the filesystem
- Sending scheduled notifications

---

## Configuration

Configure the heartbeat from the **Heartbeat** tab in the Bot Detail page.

| Field | Description |
|-------|-------------|
| **Enabled** | Toggle the heartbeat on or off. |
| **Interval** | How often (in minutes) the heartbeat should trigger. The default is **30 minutes**. |
| **Model** | The LLM used to execute the heartbeat task. This can be different from the main chat model. |

---

## Logs and Monitoring

The Heartbeat tab provides a detailed audit log of every execution:

- **Status**: Whether the heartbeat completed successfully (`ok`), encountered an issue (`alert`), or failed (`error`).
- **Time**: When the heartbeat was triggered.
- **Duration**: How long the bot took to process the task.
- **Result**: A summary of the bot's action or response during that heartbeat.

### Managing Logs

- **Filter by Status**: Quickly find errors or alerts.
- **Refresh**: Load the latest log entries.
- **Clear Logs**: Remove old heartbeat records to keep the interface clean.
- **Load More**: View older history.

---

## Bot Interaction

- During a heartbeat, the bot receives a special system prompt that it should perform its "routine" tasks.
- The bot can use any of its assigned **Skills** or **MCP tools** during a heartbeat.
- Heartbeat logs provide the "memory" of the bot's autonomous activities.
