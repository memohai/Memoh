# Email Providers and Bindings

Memoh allows your Bot to send and receive emails, providing another powerful channel for communication and task management.

## Concept: Email as a Bot Channel

Connecting email to your Bot involves two steps:
1. **Email Provider**: Configure a connection to an email service (like Mailgun or a generic SMTP server).
2. **Email Binding**: Link a specific email address from a provider to a Bot.

---

## Email Provider

Manage your email service connections from the **Email Provider** page in the sidebar.

### Creating a Provider

1. Navigate to the **Email Provider** page from the sidebar.
2. Click **Add Email Provider**.
3. Choose the provider type:
    - **Mailgun**: For high-volume delivery.
    - **Generic SMTP**: For traditional email servers.
4. Fill in the required fields (e.g., `domain`, `api_key`, `host`, `port`, `username`, `password`).
5. Click **Create**.

---

## Bot Email Bindings

Configure your bot's email capabilities from the **Email** tab in the Bot Detail page.

### Adding a Binding

1. Click **Add Binding**.
2. Select the **Email Provider** you created.
3. Provide the **Email Address** to be associated with the bot.
4. Set the bot's permissions:
    - **Can Read**: If enabled, the bot can check and process incoming emails.
    - **Can Write**: If enabled, the bot can compose and send outgoing emails.
    - **Can Delete**: If enabled, the bot can manage its own email inbox.
5. Click **Create**.

---

## Outbox and History

The **Email** tab also provides an **Outbox** showing an audit log of all emails sent by the bot:

- **To**: The recipient's email address.
- **Subject**: The email's subject line.
- **Status**: Whether the email was sent successfully.
- **Sent At**: The timestamp of the email delivery.

---

## Bot Interaction

- The bot can use its email permissions to **send reports**, **respond to user inquiries**, or **trigger actions** based on incoming mail.
- Outgoing emails are tracked in the outbox for monitoring and troubleshooting.
- The bot handles email in a structured way, allowing it to "converse" via email just as it does via chat.
