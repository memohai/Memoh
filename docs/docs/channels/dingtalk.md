# DingTalk Channel Configuration

Memoh supports DingTalk bots for private chats and group chats. The adapter uses DingTalk's stream connection for inbound events and the official APIs for outbound replies and media.

## Step 1: Create A DingTalk App

1. Open the DingTalk developer platform for your organization.
2. Create or choose the app that will act as your bot.
3. Enable the bot / messaging capability for that app.
4. Copy the app credentials:
   - **App Key**
   - **App Secret**

Depending on your DingTalk environment, you may also need to grant message permissions and publish the app before it is available to end users.

## Step 2: Configure Memoh

1. Open your bot in the Memoh Web UI.
2. Go to **Platforms**.
3. Click **Add Channel** and choose **DingTalk**.
4. Fill in **App Key** and **App Secret**.
5. Click **Save and Enable**.

Memoh maintains the DingTalk stream connection automatically. For the normal setup, you do not need to manually paste a webhook callback URL.

## Step 3: Verify Messaging

After the channel is enabled:

1. Send a private message to the DingTalk bot, or mention it in a supported group chat.
2. Confirm the bot receives the message and can reply.

## Features Supported

- **Private chats**
- **Group chats**
- **Text and Markdown-style output**
- **Replies**
- **Attachments and media**

Current behavior note:

- Outbound responses are non-streaming on DingTalk in Memoh.
