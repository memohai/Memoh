# QQ Channel Configuration

Connecting your Memoh Bot to QQ allows it to interact with users through QQ's official Bot platform.

## Step 1: Create a QQ Bot

1. Go to the [QQ Bot Open Platform](https://q.qq.com/qqbot/openclaw/) and log in with your QQ account.
2. Click **Create Bot** (创建机器人) - no approval required. Each account can create up to 5 bots.
3. Copy the **AppID** and **AppSecret** displayed on the page.

> **Important**: AppSecret is only shown once. Save it securely - viewing it again will force a reset.

## Step 2: Configure Memoh

1. Go to your Bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and select **QQ**.
3. Paste your **AppID** and **AppSecret**.
4. Configure optional settings:
   - **Markdown Support**: Enable Markdown formatting (default: enabled).
   - **Enable Input Hint**: Show typing indicators (default: enabled).
5. Click **Save and Enable**.

## Features Supported

- **Message Content**: Full support for text messages.
- **Markdown**: Rich formatting with bold, italic, code blocks, and links.
- **Attachments**: Support for images and files.
- **Typing Indicators**: Visual feedback while generating responses.
- **Target Types**: C2C (direct message), group, and channel.

## Official Resources

- [QQ Bot Open Platform](https://q.qq.com/)
- [QQ Bot Documentation](https://bot.q.qq.com/wiki/)
