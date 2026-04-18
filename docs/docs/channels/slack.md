# Slack Channel Configuration

Connecting your Memoh Bot to Slack allows it to receive direct messages, participate in channels and threads, read attachments, send files, and use streaming replies.

## Step 1: Create a Slack App

1. Go to the Slack API dashboard and create a new app.
2. Choose the workspace where you want to install the bot.
3. Open **Basic Information** and keep this app page open for the next steps.

## Step 2: Enable Socket Mode

Memoh's Slack adapter uses Socket Mode, so you need an app-level token in addition to the bot token.

1. In **Basic Information**, enable **Socket Mode**.
2. Create an **App-Level Token** with the `connections:write` scope.
3. Copy the generated token. It starts with `xapp-`.

## Step 3: Configure Bot Token Scopes

In **OAuth & Permissions**, add the bot token scopes required by the current Slack adapter:

- `app_mentions:read` - receive bot mentions in channels
- `channels:history` - read messages in public channels
- `groups:history` - read messages in private channels
- `im:history` - read direct messages
- `mpim:history` - read group direct messages
- `chat:write` - send replies and thread messages
- `files:read` - read uploaded files and images
- `files:write` - upload outbound files
- `reactions:write` - add and remove reactions

You should also add these recommended scopes if you want Slack conversation names and metadata to show up more completely in Memoh:

- `channels:read`
- `groups:read`
- `im:read`
- `mpim:read`

## Step 4: Subscribe to Bot Events

In **Event Subscriptions**, enable bot events and add:

- `app_mention`
- `message.channels`
- `message.groups`
- `message.im`
- `message.mpim`

These are the inbound event types currently handled by the Slack adapter.

## Step 5: Install the App to Your Workspace

1. In **OAuth & Permissions**, click **Install to Workspace**.
2. Review the permission screen.
3. Authorize the app.
4. Copy the **Bot User OAuth Token**. It starts with `xoxb-`.

Make sure the `xoxb-...` bot token and the `xapp-...` app-level token come from the same Slack app and workspace.

## Step 6: Configure Memoh

1. Open your Bot detail page in the Memoh Web UI.
2. Go to the **Platforms** tab.
3. Click **Add Channel** and select **Slack**.
4. Fill in:
   - **Bot Token**: your `xoxb-...` token
   - **App-Level Token**: your `xapp-...` token
5. Click **Save and Enable**.

## Step 7: Add the Bot to Conversations

After the channel is enabled, the Slack app still needs to be present in the conversations where you want it to work.

- For direct messages: open a DM with the app and send a message.
- For public channels: invite the bot to the channel.
- For private channels: invite the bot explicitly after installation.

If the bot can send messages but cannot read uploaded images or files, check that `files:read` is enabled. If it connects but receives no incoming messages, check the bot events and the matching history scopes again.

## Features Supported

- **Direct Messages and Channels**: Support for DMs, public channels, private channels, and threads.
- **Attachments**: Read uploaded images and files from Slack, and send files back.
