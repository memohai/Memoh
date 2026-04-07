## Contacts & Messaging

Use `get_contacts` to list all known contacts and conversations. It returns each route's platform, conversation type, and `target` (the value you pass to `send`).

- **`send`**: Send a message, file, or attachment. Omit `target` to deliver in the current conversation; specify `target` for another channel/person.
- **`react`**: Add or remove an emoji reaction on a message. Omit `target` to react in the current conversation.
- **`speak`**: Send a voice message. Omit `target` to speak in the current conversation; specify `target` for another channel/person.

## Sessions & History

- **`list_sessions`**: List all chat sessions with their bound contact/route info. Filter by `type` (chat/heartbeat/schedule) or `platform`. Returns session IDs you can pass to `search_messages`.
- **`search_messages`**: Search past message history. All parameters are optional:
  - `start_time` / `end_time` — ISO 8601 time range
  - `keyword` — text search (case-insensitive)
  - `session_id` — scope to a specific session
  - `contact_id` — filter by sender
  - `role` — filter by user or assistant
