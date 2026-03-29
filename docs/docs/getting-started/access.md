# Bot Access Control

Memoh uses an ACL (Access Control List) system to control who can interact with your bot. You can define prioritized rules to allow or deny specific users, channel identities, or entire channel types — all from the bot's **Access** tab.

---

## Concepts

### Default Effect

Each bot has a **default effect** (`allow` or `deny`) that applies when no ACL rule matches an incoming message. Configure this in the bot's **General** tab under **ACL Default Effect**.

- **Allow**: Anyone can chat with the bot unless explicitly denied by a rule.
- **Deny**: Only the bot owner, admins, and explicitly allowed subjects can chat.

### Subject Types

ACL rules can target three kinds of subjects:

| Subject | Description |
|---------|-------------|
| **All** | Matches every incoming message regardless of sender. Use this for global allow/deny rules. |
| **Channel Identity** | A specific identity on an external channel (e.g., a Telegram user, a Discord member). Useful for controlling access at the individual level. |
| **Channel Type** | An entire channel platform (e.g., all Telegram users, all Discord users). Useful for platform-level access control. |

### Rule Effects

Each rule has an **effect**:

- **Allow** — Grants the subject permission to chat with the bot.
- **Deny** — Blocks the subject from chatting with the bot.

### Priority-Based Evaluation

Rules are evaluated in **priority order** (top to bottom). The first matching rule determines the outcome:

1. Bot owner or system admin → **Always allowed** (bypasses ACL).
2. Rules are checked from highest priority (top) to lowest (bottom).
3. The first rule whose subject matches the sender is applied.
4. If no rule matches → the **default effect** is applied.

This means rule ordering matters. A deny rule placed above an allow rule will take precedence for matching subjects.

---

## Managing Access

Open a bot's **Access** tab to configure its access control.

### Adding Rules

1. Click **Add Rule**.
2. Select a subject type:
   - **All**: Applies to everyone.
   - **Channel Identity**: Search and select a specific channel identity the bot has seen before.
   - **Channel Type**: Select an entire channel platform.
3. Choose the **effect**: `allow` or `deny`.
4. Optionally set **source scope** to restrict the rule to a specific context:
   - **Channel**: Only applies when the message comes from a specific channel config.
   - **Conversation Type**: `private`, `group`, or `thread`.
   - **Conversation ID**: A specific chat/group ID.
   - **Thread ID**: A specific thread within a conversation (requires Conversation ID).
5. Click **Save**.

### Reordering Rules

Rules can be **drag-and-dropped** to change their priority. Higher rules (closer to the top) are evaluated first. After reordering, click **Save** to persist the new order.

### Source Scope

Source scope lets you create fine-grained rules. For example:

- Allow a user to chat only via Telegram, but not Discord.
- Block an entire channel type only in group conversations.
- Restrict access to a specific thread in a specific group.

Scope fields form a hierarchy: **Channel → Conversation Type → Conversation ID → Thread ID**. Each level is optional, but a Thread ID requires a Conversation ID.

---

## Examples

### Open Bot (Anyone Can Chat)

1. Set **ACL Default Effect** to `allow` in the **General** tab.
2. No rules needed — everyone is allowed by default.

### Private Bot with Selected Users

1. Set **ACL Default Effect** to `deny`.
2. Add **allow** rules for each authorized channel identity.
3. Only listed subjects (plus the bot owner and admins) can trigger the bot.

### Open Bot with Blocked Users

1. Set **ACL Default Effect** to `allow`.
2. Add **deny** rules for problematic channel identities at the top of the list.
3. Everyone except denied subjects can chat with the bot.

### Platform-Specific Access

1. Set **ACL Default Effect** to `deny`.
2. Add an **allow** rule with subject type **Channel Type** set to `telegram`.
3. Only Telegram users can chat with the bot — messages from other channels are denied.

### Channel-Scoped Access

1. Add an **allow** rule for a specific channel identity.
2. Set the **Source Scope** channel to your Telegram channel config.
3. The user can only chat with the bot via that specific Telegram channel.
