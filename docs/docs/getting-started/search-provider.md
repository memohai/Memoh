# Search Providers

Memoh allows you to connect your Bot to various search engines, enabling it to browse the web for real-time information and external knowledge.

## Concept: Real-Time Knowledge

A **Search Provider** is a connection to a specific search API (like Brave, Google, or Bing). Once configured, it becomes a tool that your Bot can use during conversation.

---

## Supported Search Engines

Configure your search engines from the **Search Provider** page in the sidebar.

| Supported Engines | Notes |
|-------------------|-------|
| **Brave** | Highly recommended for privacy and performance. |
| **Bing** | Powerful global search capability. |
| **Google** | Comprehensive web search. |
| **Tavily** | Designed specifically for AI-powered research. |
| **SearxNG** | Connect to your own self-hosted search aggregator. |
| **DuckDuckGo** | Privacy-focused search. |
| **Other Providers**| Sogou, Serper, Jina, Exa, Bocha, Yandex. |

---

## Configuration Flow

### 1. Adding a Search Provider

1. Navigate to the **Search Provider** page from the sidebar.
2. Click **Add Search Provider**.
3. Fill in the following:
    - **Name**: A friendly name (e.g., "My Brave Search").
    - **API Key**: Your credentials for the specific search engine API.
    - **Other Parameters**: Depending on the engine, you may need to specify additional settings (e.g., `base_url` for SearxNG).

### 2. Managing Providers

- **Edit**: Select a provider from the sidebar and update its configuration in the form on the right.
- **Delete**: Use the **Delete Provider** button in the provider settings form.

---

## Assigning a Search Provider to a Bot

1. Navigate to the **Bots** page and select your bot.
2. Go to the **General** tab.
3. Find the **Search Provider** dropdown and select the provider you created.
4. Click **Save**.

---

## Bot Interaction

- When a user's question requires real-time information, the bot will automatically call its assigned **Search Provider**.
- The bot will process the search results to provide a grounded and accurate response.
- Multiple search results can be synthesized to provide a comprehensive answer.
