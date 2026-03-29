# Mem0 Memory Provider

Mem0 is a SaaS memory provider that connects your bot to the [Mem0](https://mem0.ai) platform. Instead of managing memory infrastructure yourself, Mem0 handles storage, retrieval, and indexing through its cloud API.

---

## Creating a Mem0 Provider

1. Navigate to the **Memory Providers** page.
2. Click **Add Memory Provider**.
3. Fill in the following fields:
   - **Name**: A display name for this provider.
   - **Provider Type**: Select `mem0`.
4. Click **Create**.

---

## Configuring a Mem0 Provider

After creating a provider, select it from the list and configure its settings.

| Field | Required | Description |
|-------|----------|-------------|
| **Base URL** | No | Mem0 API base URL. Defaults to `https://api.mem0.ai` when empty. |
| **API Key** | Yes | API key for Mem0 authentication (stored as a secret). |
| **Organization ID** | No | Organization ID for workspace scoping. |
| **Project ID** | No | Project ID for workspace scoping. |

---

## Assigning a Mem0 Provider to a Bot

1. Navigate to the **Bots** page and open your bot.
2. Go to the **General** tab.
3. Find the **Memory Provider** dropdown.
4. Select the Mem0 provider you created.
5. Click **Save**.

---

## Usage

Once assigned, the bot will use Mem0 as its memory backend. Memory extraction, search, and management operations are routed through the Mem0 API.

For day-to-day memory operations, see [Bot Memory Management](/getting-started/memory.md).
