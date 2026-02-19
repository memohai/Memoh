# Configure Telegram Channel

This guide walks you through connecting your bot to Telegram, allowing users to chat with your bot via Telegram messages.

## Prerequisites

- Memoh is running (see [Docker installation](/installation/docker))
- You have logged in to the Web UI at http://localhost:8082
- You have created a bot (see [Create Bot](/getting-started/create-bot))
- A Telegram account

## Step 1: Create a Telegram Bot

Open Telegram and search for the official bot `@BotFather`.

Send the `/newbot` command to BotFather and follow the prompts:

1. Enter a **name** for your bot (display name, e.g., `My Memoh Bot`)
2. Enter a **username** for your bot (must end with `bot`, e.g., `my_memoh_bot`)

BotFather will create the bot and provide a **Bot Token** (e.g., `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`).

**Save this token securely** — you will need it in the next step.

## Step 2: Open the Bot Platforms Page

In the Memoh Web UI, click **Bots** in the left sidebar to open the Bots page.

Select the bot you want to connect to Telegram.

Click the **Platforms** tab to open the channel configuration page.

## Step 3: Add Telegram Channel

Click the **Add Channel** button.



In the dialog, select **Telegram** as the channel type.

Fill in the configuration:

| Field | Description |
|-------|-------------|
| **Bot Token** | The token from BotFather (e.g., `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`) |

Click **Save** to add the channel.

![Add Channel button](/getting-started/platform-telegram-01-platforms.png)



## Step 4: Bind Your Telegram Account

Open the Memoh web ui setting page, find `Bind Code` section, select telegram platform and necessary TTL(seconds), Generate bind code.

![Bind Code](/getting-started/platform-telegram-02-bindcode.png)


Open the bot dialog in telegram, send `Bind Code` to chat, you should get `Binding successful! Your identity has been linked.` message if successful 


Click **Save** to complete the binding.

## Step 6: Test the Connection

Send a message to your bot on Telegram:

- For `public` bots: Add the bot to a group, have others mention your bot when sending messages.
- For `person` bots: Send a direct message (requires binding in Step 5)

The bot should respond according to its configured model and system prompt.

## Next Steps

- Configure [Memory](/concepts/memory) to enable long-term memory for your bot
- Set up [Skills](/concepts/skills) to extend your bot's capabilities
- Add [Schedules](/concepts/schedule) for automated tasks
