# Bot Channels

Channels connect your Bot to various messaging platforms, allowing you to interact with it using your favorite chat applications.

## Concept: Unified Communication

Memoh acts as a hub that bridges different messaging services. You can configure multiple channels for a single bot, enabling it to chat on Telegram, Discord, and more simultaneously.

---

## Supported Channels

Configure your bot's connections from the **Channels** tab in the Bot Detail page.

### Popular Platforms

For detailed step-by-step guides on how to create and configure bots for each platform, see:

- **[Telegram Configuration](/channels/telegram)**
- **[Feishu (Lark) Configuration](/channels/feishu)**
- **[Discord Configuration](/channels/discord)**

---

## Configuration Flow

### 1. Adding a Channel

1. Click **Add Channel**.
2. Select the platform from the list.
3. Fill in the required credentials and configuration. The fields are dynamic and change based on the selected channel.

### 2. Common Fields

| Field | Description |
|-------|-------------|
| **Credentials** | API tokens, secrets, or bot keys provided by the platform. |
| **Disabled** | Quickly enable or disable a channel without removing its configuration. |
| **Routing** | Configure how messages are mapped between the platform and Memoh. |

### 3. Special Case: Feishu Webhook

If using **Feishu** in `webhook` inbound mode:
1. Memoh will generate a **Webhook Callback URL**.
2. Copy this URL and paste it into your Feishu App's event configuration.
3. This allows Feishu to send messages directly to Memoh.

---

## Operations

- **Save**: Update the configuration.
- **Save and Enable**: Update and immediately activate the channel.
- **Enable/Disable Toggle**: Switch the channel's active status.
- **Delete**: Permanently remove a channel's configuration.
