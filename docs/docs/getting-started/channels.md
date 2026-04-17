# Bot Channels

Channels connect your Bot to various messaging platforms, allowing you to interact with it using your favorite chat applications.

## Concept: Unified Communication

Memoh acts as a hub that bridges different messaging services. You can configure multiple channels for a single bot, enabling it to chat on Telegram, Discord, Matrix, and more simultaneously.

---

## Supported Channels

Configure your bot's connections from the **Platforms** tab in the Bot Detail page.

### Platform Guides

| Platform | Guide | Notes |
|----------|-------|-------|
| Telegram | [Telegram Configuration](/channels/telegram) | Strong attachment and streaming support |
| Feishu (Lark) | [Feishu Configuration](/channels/feishu) | Supports webhook-style inbound mode |
| Discord | [Discord Configuration](/channels/discord) | Good fit for communities and servers |
| QQ | [QQ Configuration](/channels/qq) | Personal DM oriented |
| Matrix | [Matrix Configuration](/channels/matrix) | Decentralized homeserver support |
| Misskey | [Misskey Configuration](/channels/misskey) | Replies and reactions, no streaming |
| DingTalk | [DingTalk Configuration](/channels/dingtalk) | Enterprise private/group chat |
| WeCom (WeWork) | [WeCom Configuration](/channels/wecom) | Enterprise workspace integration |
| WeChat | [WeChat Configuration](/channels/weixin) | Personal QR login flow |
| WeChat Official Account | [WeChat Official Account Configuration](/channels/wechatoa) | Official account webhook flow |

Two WeChat adapters exist on purpose:

- **WeChat** is the QR-login personal messaging integration.
- **WeChat Official Account** is the official account / webhook integration for private-message scenarios.

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

### 4. Special Case: WeChat QR Login

If using **WeChat**:
1. After enabling the channel, a QR code flow is provided for connecting.
2. Scan the QR code with WeChat to link the bot.

### 5. Special Case: WeChat Official Account Webhook

If using **WeChat Official Account**:
1. Create and save the channel first.
2. Memoh generates a **Webhook Callback URL** for that channel.
3. Copy the callback URL into the WeChat Official Account platform configuration.
4. Keep the configured `Token`, `Encryption Mode`, and optional AES settings aligned between Memoh and WeChat.

### 6. Special Case: DingTalk Stream Connection

If using **DingTalk**:
1. Configure `App Key` and `App Secret`.
2. Save and enable the channel.
3. Memoh maintains the stream connection for inbound events; you do not need to manage a separate webhook callback URL for the standard setup.

---

## Operations

- **Save**: Update the configuration.
- **Save and Enable**: Update and immediately activate the channel.
- **Enable/Disable Toggle**: Switch the channel's active status.
- **Delete**: Permanently remove a channel's configuration.
