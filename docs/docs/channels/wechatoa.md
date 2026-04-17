# WeChat Official Account Channel Configuration

This guide covers the **WeChat Official Account** adapter in Memoh. It is different from the personal **WeChat** QR-login adapter: this one is for official-account webhook integration and is intended for inbound private-message scenarios.

## Step 1: Prepare Official Account Credentials

From the WeChat Official Account platform, prepare:

- **App ID**
- **App Secret**
- **Token**

You may also need:

- **Encoding AES Key** if you use encrypted webhook delivery
- an outbound **HTTP Proxy URL** if your deployment must reach WeChat APIs through a proxy

## Step 2: Add The Channel In Memoh

1. Open your bot in the Memoh Web UI.
2. Go to **Platforms**.
3. Click **Add Channel** and choose **WeChat Official Account**.
4. Fill in the required fields:
   - **App ID**
   - **App Secret**
   - **Token**
5. Choose the **Encryption Mode**.
6. If you use `safe` or `compat` mode, also provide the **Encoding AES Key**.
7. Save the channel.

Memoh generates a **Webhook Callback URL** after the channel has been saved.

## Step 3: Configure The WeChat Platform

1. Copy the **Webhook Callback URL** from Memoh.
2. Paste it into the WeChat Official Account platform callback configuration.
3. Make sure the WeChat platform and Memoh use the same:
   - **Token**
   - **Encryption Mode**
   - **Encoding AES Key** when encryption is enabled

WeChat will verify the callback before delivering real messages.

## Step 4: Enable And Test

1. Enable the channel in Memoh.
2. Send a test private message from the official account side.
3. Confirm that Memoh receives the message and the bot can reply.

## Features Supported

- **Private chats**
- **Replies**
- **Attachments and media**

Current behavior notes:

- This adapter is intended for **private-message** conversations, not group-style chats.
- Outbound responses are non-streaming on this channel.
