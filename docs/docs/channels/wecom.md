# WeCom (WeWork) Channel Configuration

Connecting your Memoh Bot to WeCom (企业微信) allows it to interact with users within your organization's WeCom workspace.

## Step 1: Create a WeCom Bot

1. Log in to the [WeCom Admin Console](https://work.weixin.qq.com/).
2. Navigate to **App Management** > **Custom Apps** or **Bot Management**.
3. Create a new bot application.
4. Note the **Bot ID** and **Secret** credentials.

## Step 2: Configure Memoh

1. Go to your Bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and select **WeCom**.
3. Fill in the required fields:

| Field | Required | Description |
|-------|----------|-------------|
| **Bot ID** | Yes | The WeCom bot identifier. |
| **Secret** | Yes | The bot's authentication credential. |
| **WebSocket URL** | No | Custom WebSocket endpoint (uses default if empty). |

4. Click **Save and Enable**.

## Step 3: Start Chatting

Once the channel is enabled, the bot will connect to WeCom via WebSocket and begin receiving messages. Users in your WeCom workspace can send messages to the bot directly or in group chats.

## Features Supported

- **Message Content**: Full support for text messages.
- **Direct Messages**: Private conversations with individual users.
- **Group Chats**: Participate in group conversations.
- **Streaming**: Responses are streamed in real-time.

## Official Resources

- [WeCom Open Platform](https://developer.work.weixin.qq.com/)
- [WeCom Bot Documentation](https://developer.work.weixin.qq.com/document/path/91770)
