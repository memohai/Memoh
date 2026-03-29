# Matrix Channel Configuration

Connecting your Memoh Bot to Matrix allows it to communicate via the decentralized Matrix messaging protocol. Your bot can join rooms, respond to direct messages, and participate in group conversations on any Matrix homeserver.

## Step 1: Create a Matrix Bot Account

1. Register a new account on your Matrix homeserver (e.g., via Element or any Matrix client).
2. Note the **User ID** (e.g., `@mybot:matrix.org`).
3. Obtain an **Access Token** for the account. You can do this by:

- Option A: Using the Matrix Client Login API

```bash
curl -X POST "https://<homeserver>/_matrix/client/v3/login" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "m.login.password",
    "identifier": {
      "type": "m.id.user",
      "user": "<username>"
    },
    "password": "<password>"
  }'
```

- Option B: Extract from a client

## Step 2: Configure Memoh

1. Go to your Bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and select **Matrix**.
3. Fill in the required fields:

| Field | Required | Description |
|-------|----------|-------------|
| **Homeserver URL** | Yes | The base URL of your Matrix homeserver (e.g., `https://matrix.org`). |
| **Access Token** | Yes | The bot account's access token. |
| **User ID** | Yes | The bot's Matrix user ID (e.g., `@mybot:matrix.org`). |
| **Sync Timeout** | No | Long-polling timeout in seconds (default: 30). |
| **Auto Join Invites** | No | Automatically join rooms when invited (default: enabled). |

4. Click **Save and Enable**.

## Step 3: Invite the Bot

1. Open your Matrix client (Element, etc.).
2. Invite the bot's user ID to a room, or start a direct message.
3. If **Auto Join Invites** is enabled, the bot will automatically accept and join.

## Features Supported

- **Message Content**: Full access to text messages.
- **Rooms**: Join and participate in group rooms.
- **Direct Messages**: Private conversations with individual users.
- **Streaming**: Responses are streamed as they are generated.
- **Markdown**: Support for formatted text.
- **Media/Attachment**: Bots can receive and send files or photo.
- Check [the roadmap](https://github.com/memohai/Memoh/issues/249) for more features of Matrix.

## Official Resources

- [Matrix Specification](https://spec.matrix.org/)
- [Element Web Client](https://app.element.io/)
