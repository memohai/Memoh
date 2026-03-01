# Bot Subagents

Subagents are specialized AI entities with their own independent conversation context. They are managed by the main Bot to delegate complex tasks or focus on specific domains.

## Concept: Task Specialization

A **Subagent** is like a specialized teammate for your Bot. While the main Bot handles general conversation, it can spin up and communicate with a Subagent to perform deep analysis, research, or execution.

---

## Fields

Configure Subagents from the **Subagents** tab in the Bot Detail page.

| Field | Description |
|-------|-------------|
| **Name** | The identifier for the subagent (e.g., "Research Assistant"). |
| **Description** | A brief explanation of the subagent's purpose and role. |
| **Skills** | A list of specific **Skills** assigned from the bot's container. |
| **Messages** | The conversation history and context specific to this subagent. |
| **Usage** | Statistics on token consumption and activity. |

---

## Operations

- **Add Subagent**: Create a new entity by providing a name and description.
- **Edit**: Update the name or description of an existing subagent.
- **Delete**: Permanently remove a subagent and its independent context.
- **View Context**: Open a dialog to inspect the subagent's conversation history and usage metrics.

---

## Bot Interaction

- The main Bot uses the **Subagent Tool** to create, communicate with, and receive results from subagents.
- Subagents inherit the main bot's container permissions but operate with their own "mental workspace."
- This modular approach allows for building multi-agent systems within a single Bot's scope.
