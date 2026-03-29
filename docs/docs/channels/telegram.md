# Telegram Channel Configuration

This guide walks you through connecting your Memoh Bot to Telegram. Telegram is one of the most supported platforms in Memoh, featuring streaming responses, Markdown formatting, and attachment support.

## Step 1: Create a Telegram Bot

You need to create a bot on Telegram to get an API token.

1. Open Telegram and search for the official **@BotFather** bot.
2. Send the `/newbot` command.
3. Follow the prompts:
   - **Name**: Choose a display name for your bot (e.g., `My Memoh Bot`).
   - **Username**: Choose a unique username ending in `bot` (e.g., `my_memoh_bot`).
4. BotFather will provide you with an **API Token** (e.g., `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`). **Keep this token secret.**

> Official Guide: [Telegram Bot Tutorial](https://core.telegram.org/bots/tutorial)

## Step 2: Configure Memoh

1. Go to your Bot's **Detail Page** in the Memoh Web UI.
2. Select the **Platforms** tab.
3. Click **Add Channel** and select **Telegram**.
4. Paste your **API Token** into the credentials field.
5. Click **Save and Enable**.

## Step 3: Bind Your Identity (Optional)

Binding your Telegram identity to your Memoh account enables cross-platform identity recognition — the bot will know who you are regardless of which channel you use.

1. In Memoh Web UI, go to **Profile** > **Bind Code**.
2. Select **Telegram** and click **Generate**.
3. Copy the code.
4. Open a chat with your new bot on Telegram and send the bind code.
5. The bot should respond that your identity is successfully linked.

## Features Supported

- **Streaming**: Responses appear as the bot "thinks."
- **Markdown**: Support for bold, italic, code blocks, and links.
- **Attachments**: Send images or files to the bot; the bot can also send files back.
- **Replies**: The bot understands context from message replies.
