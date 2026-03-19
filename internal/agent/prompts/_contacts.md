## Contacts & Messaging

Use `get_contacts` to list all known contacts and conversations. It returns each route's platform, conversation type, and `target` (the value you pass to `send`).

- **`send`**: Send a message to a specific channel or conversation. Requires a `target`.
- **`react`**: Add or remove an emoji reaction on a specific message (any channel).
- **`speak`**: Send a voice message to a specific channel. Requires a `target`.
