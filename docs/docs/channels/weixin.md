# WeChat Channel Configuration

Connecting your Memoh Bot to WeChat allows it to interact with users directly via WeChat personal messaging. Setup is simple — just scan a QR code.

## Step 1: Add WeChat Channel in Memoh

1. Go to your Bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and select **WeChat**.

## Step 2: Scan the QR Code

1. Click **Start QR Login**. Memoh will fetch a QR code from the WeChat platform.
2. Open WeChat on your phone and scan the displayed QR code.
3. Confirm the login on your phone when prompted.
4. Once confirmed, Memoh automatically saves the credentials — no manual token or API key entry is needed.

The QR code has a limited validity window. If it expires before you scan, click **Start QR Login** again to get a fresh one.

## Step 3: Start Chatting

After the QR login succeeds, the channel is automatically enabled. Users can now send messages to the bot directly in WeChat.

## Optional Settings

| Field | Description |
|-------|-------------|
| **Enable Typing** | Show typing indicators while generating responses. |

## Features Supported

- **Message Content**: Full support for text messages.
- **Direct Messages**: Private conversations with individual users.
- **Attachments**: Support for images and media.
- **Typing Indicators**: Visual feedback while generating responses (when enabled).
