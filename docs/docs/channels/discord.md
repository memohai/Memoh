# Discord Channel Configuration

Connecting your Memoh Bot to Discord allows it to join servers and interact with community members.

## Step 1: Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications).
2. Click **New Application** and give it a name.
3. In the left sidebar, go to **Bot**.
4. Click **Reset Token** to generate a **Bot Token**. Copy this token and store it securely.

## Step 2: Configure Bot Settings

1. Scroll down to the **Privileged Gateway Intents** section.
2. Enable `Message Content Intent`, `Server Members Intent` and `Presence Intent`.
3. Save changes.

## Step 3: Invite the Bot to Your Server

1. Go to **OAuth2** > **URL Generator**.
2. Select scopes: `bot`, `applications.commands`.
3. Select permissions: `Send Messages`, `Read Message History`, `Embed Links`, `Attach Files`.
4. Copy the generated URL and open it in your browser.
5. Select the server you want to add the bot to and authorize it.

> Official Guide: [Discord Developer Portal - Bots](https://discord.com/developers/docs/intro)

## Step 4: Configure Memoh

1. Go to your Bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and select **Discord**.
3. Paste your **Bot Token**.
4. Click **Save and Enable**.

## Features Supported

- **Message Content**: Full access to chat text.
- **Attachments**: Support for images and files.
- **Commands**: Integration with Discord's slash commands (if configured via MCP).
