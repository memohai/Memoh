# Bot Access Control

Memoh uses an ACL (Access Control List) system to control who can interact with your bot. You can configure guest access, whitelist specific users or channel identities, and blacklist others — all from the bot's **Access** tab.

---

## Concepts

### Authorization Layers

Bot access is enforced at two levels:

1. **Management Access**: Only the bot **owner** and system **admins** can edit bot settings, manage ACL rules, and configure the bot. This is not configurable — it is based on ownership.
2. **Chat Trigger Access**: Controls who can send messages to the bot and trigger a response. This is what the ACL system manages.

### Subject Types

ACL rules can target three kinds of subjects:

| Subject | Description |
|---------|-------------|
| **Guest (all)** | A global toggle — when enabled, anyone can chat with the bot without being explicitly listed. |
| **User** | A specific Memoh user account. |
| **Channel Identity** | A specific identity on an external channel (e.g. a Telegram user, a Discord member). Useful when the person doesn't have a Memoh account. |

### Evaluation Order

When an incoming message arrives, the bot evaluates access in this order:

1. Bot owner or system admin → **Allow**
2. User or channel identity has a **deny** rule → **Deny**
3. User or channel identity has an **allow** rule → **Allow**
4. Guest access is enabled → **Allow**
5. None of the above → **Deny**

Blacklist (deny) rules are always checked before whitelist (allow) rules. This means a blacklisted user cannot bypass the block even if guest access is enabled.

---

## Managing Access

Open a bot's **Access** tab to configure its access control.

### Guest Access

Toggle **Allow Guest Access** to let anyone chat with the bot without an explicit whitelist entry. This is useful for public-facing bots.

When guest access is disabled, only the bot owner, admins, and explicitly whitelisted users/identities can trigger the bot.

### Whitelist

The whitelist grants specific users or channel identities permission to chat with the bot.

1. Click **Add** in the Whitelist section.
2. Select a subject type:
   - **User**: Search and select a Memoh user.
   - **Channel Identity**: Search and select a channel identity (e.g. a Telegram user the bot has seen before).
3. Optionally set **source scope** to restrict the rule to a specific context:
   - **Channel**: Only applies when the message comes from a specific channel (e.g. your Telegram bot channel).
   - **Conversation Type**: `private`, `group`, or `thread`.
   - **Conversation ID**: A specific chat/group ID.
   - **Thread ID**: A specific thread within a conversation (requires Conversation ID).
4. Click **Save**.

Without source scope, the rule applies globally — the subject can chat with the bot from any channel.

### Blacklist

The blacklist denies specific users or channel identities from chatting with the bot. The setup process is the same as the whitelist.

Blacklist rules take priority over whitelist rules and guest access. Use this to block specific users while keeping the bot open to others.

### Source Scope

Source scope lets you create fine-grained rules. For example:

- Allow a user to chat only via Telegram, but not Discord
- Block a channel identity only in group conversations
- Restrict access to a specific thread in a specific group

Scope fields form a hierarchy: **Channel → Conversation Type → Conversation ID → Thread ID**. Each level is optional, but a Thread ID requires a Conversation ID, and a Conversation ID requires a Channel.

---

## Examples

### Public Bot (Anyone Can Chat)

1. Open the bot's **Access** tab.
2. Enable **Allow Guest Access**.
3. Done — anyone on any connected channel can now message the bot.

### Private Bot with Selected Users

1. Disable **Allow Guest Access**.
2. Add each authorized user or channel identity to the **Whitelist**.
3. Only listed subjects (plus the bot owner and admins) can trigger the bot.

### Public Bot with Blocked Users

1. Enable **Allow Guest Access**.
2. Add problematic users/identities to the **Blacklist**.
3. Everyone except blacklisted subjects can chat with the bot.

### Channel-Scoped Access

1. Add a whitelist rule for a user.
2. Set the **Channel** source scope to your Telegram channel.
3. The user can only chat with the bot via Telegram — messages from other channels are denied.
