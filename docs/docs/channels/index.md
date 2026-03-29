# Channels Overview

Channels are the gateways that connect your Memoh Bots to the outside world. By configuring channels, you can interact with your bots via your favorite messaging platforms.

Memoh currently supports the following channels:

- **[Telegram](./telegram)**: Feature-rich integration with streaming and attachment support.
- **[Feishu (Lark)](./feishu)**: Enterprise-ready integration for business workflows.
- **[Discord](./discord)**: Community-focused integration for servers and direct messages.
- **[QQ](./qq)**: Quick setup for personal DM bots via the dedicated AI bot registration portal.
- **[Matrix](./matrix)**: Decentralized messaging protocol support for any Matrix homeserver.
- **[WeCom (WeWork)](./wecom)**: Enterprise messaging integration for WeCom workspaces.
- **[WeChat](./weixin)**: Personal messaging via the WeChat AI bot platform.
- **Email**: Connect via SMTP providers, Mailgun, or Gmail OAuth (configured through Email Providers).
- **Web**: Built-in chat interface for immediate access.

## General Setup Flow

1. **Create an external app/bot**: Register your bot on the target platform (e.g., via BotFather on Telegram).
2. **Obtain credentials**: Fetch API tokens, App IDs, or secrets.
3. **Configure in Memoh**: Add the channel to your Bot's **Platforms** tab and paste the credentials.
4. **Enable**: Activate the channel to start receiving and sending messages.

Choose a channel from the sidebar to see detailed configuration guides for each platform.
