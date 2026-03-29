# OpenViking Memory Provider

OpenViking is a memory provider that can be self-hosted or used as a SaaS service. It provides an alternative memory backend for bots that need a dedicated memory API.

---

## Creating an OpenViking Provider

1. Navigate to the **Memory Providers** page.
2. Click **Add Memory Provider**.
3. Fill in the following fields:
   - **Name**: A display name for this provider.
   - **Provider Type**: Select `openviking`.
4. Click **Create**.

---

## Configuring an OpenViking Provider

After creating a provider, select it from the list and configure its settings.

| Field | Required | Description |
|-------|----------|-------------|
| **Base URL** | Yes | OpenViking API endpoint (e.g. `http://openviking:8088`). |
| **API Key** | No | API key for authentication (stored as a secret). |

---

## Assigning an OpenViking Provider to a Bot

1. Navigate to the **Bots** page and open your bot.
2. Go to the **General** tab.
3. Find the **Memory Provider** dropdown.
4. Select the OpenViking provider you created.
5. Click **Save**.

---

## Usage

Once assigned, the bot will use OpenViking as its memory backend. Memory extraction, search, and management operations are routed through the OpenViking API.

For day-to-day memory operations, see [Bot Memory Management](/getting-started/memory.md).
