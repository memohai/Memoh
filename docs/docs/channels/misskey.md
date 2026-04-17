# Misskey Channel Configuration

Memoh can connect a bot to a Misskey server so it can read inbound mentions and reply as a Misskey account. This adapter is best for text-first social interactions on self-hosted or public Misskey instances.

## Step 1: Prepare A Misskey Account And Token

1. Sign in to the Misskey instance you want to use.
2. Create or choose the account that should represent your bot.
3. Generate an **Access Token** for that account.

Memoh needs:

- **Instance URL**: for example `https://misskey.io`
- **Access Token**: a token for the bot account

The exact token-creation UI varies by Misskey instance. Make sure the token is allowed to read inbound events for the bot account and publish replies.

## Step 2: Configure Memoh

1. Open your bot in the Memoh Web UI.
2. Go to **Platforms**.
3. Click **Add Channel** and choose **Misskey**.
4. Enter the **Instance URL**.
5. Paste the **Access Token**.
6. Click **Save and Enable**.

Memoh uses these credentials to discover the bot identity and start the Misskey connection.

## Step 3: Start Chatting

After the channel is enabled, users can interact with the bot on that Misskey instance.

Misskey in Memoh is generally best suited for:

- replies to users
- text and Markdown-style output
- reaction-aware social conversations

## Features Supported

- **Text**
- **Markdown**
- **Replies**
- **Reactions**

Current limitations:

- **No attachments / media upload**
- **No streaming output**
