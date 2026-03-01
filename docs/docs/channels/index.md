# Channels Overview

Channels are the gateways that connect your Memoh Bots to the outside world. By configuring channels, you can interact with your bots via your favorite messaging platforms.

Memoh currently supports the following channels:

- **[Telegram](./telegram)**: The most feature-rich integration with streaming and attachment support.
- **[Feishu (Lark)](./feishu)**: Enterprise-ready integration for business workflows.
- **[Discord](./discord)**: Community-focused integration for servers and direct messages.
- **Email**: Connect via standard SMTP and IMAP (configured through Email Providers).
- **Web**: Built-in chat interface for immediate access.

## General Setup Flow

1. **Create an external app/bot**: Register your bot on the target platform (e.g., via BotFather on Telegram).
2. **Obtain credentials**: Fetch API tokens, App IDs, or secrets.
3. **Configure in Memoh**: Add the channel to your Bot's settings and paste the credentials.
4. **Enable**: Activate the channel to start receiving and sending messages.

Choose a channel from the sidebar to see detailed configuration guides for each platform.
