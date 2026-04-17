# Channels Overview

Channels are the gateways that connect your Memoh Bots to the outside world. By configuring channels, you can interact with your bots via your favorite messaging platforms.

Memoh currently supports the following channels:

- **[Telegram](./telegram)**: Feature-rich integration with streaming and attachment support.
- **[Feishu (Lark)](./feishu)**: Enterprise-ready integration for business workflows.
- **[Discord](./discord)**: Community-focused integration for servers and direct messages.
- **[QQ](./qq)**: Quick setup for personal DM bots via the dedicated AI bot registration portal.
- **[Matrix](./matrix)**: Decentralized messaging protocol support for any Matrix homeserver.
- **[Misskey](./misskey)**: Federated social/chat style integration with replies and reactions.
- **[DingTalk](./dingtalk)**: Enterprise chat integration for private and group conversations.
- **[WeCom (WeWork)](./wecom)**: Enterprise messaging integration for WeCom workspaces.
- **[WeChat](./weixin)**: Personal messaging via QR login.
- **[WeChat Official Account](./wechatoa)**: Official account webhook integration for private message scenarios.
- **Email**: Connect via SMTP providers, Mailgun, or Gmail OAuth (configured through Email Providers).
- **Web**: Built-in chat interface for immediate access.

### WeChat vs WeChat Official Account

Memoh supports two different WeChat-related adapters:

- **WeChat (`weixin`)** is the personal-account style integration that relies on QR login.
- **WeChat Official Account (`wechatoa`)** is the official-account / webhook style integration that uses `App ID`, `App Secret`, `Token`, and optional AES settings.

Choose the one that matches your actual WeChat deployment model.

## General Setup Flow

1. **Create an external app/bot**: Register your bot on the target platform.
2. **Obtain credentials**: Fetch API tokens, App IDs, app secrets, or access tokens.
3. **Configure in Memoh**: Add the channel from your bot's **Platforms** tab.
4. **Save and enable**: Activate the channel to start receiving and sending messages.

Depending on the platform, the final step may involve:

- copying a webhook callback URL into the platform console
- approving a QR login on mobile
- leaving a long-lived stream/WebSocket connection running through Memoh

Choose a channel from the sidebar to see detailed configuration guides for each platform.
