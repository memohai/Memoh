# LLM Provider and Model

To use Memoh, you first need to configure at least one LLM Provider and at least one Model. 

## LLM Provider

An LLM Provider represents a connection to an AI service (like OpenAI, Anthropic, or a self-hosted compatible API). It stores the base URL and authentication credentials.

### Creating a Provider

1. Navigate to the **Providers** page from the sidebar.
2. Click the **Add Provider** button at the bottom of the sidebar.
3. Fill in the following fields:
    - **Name**: A display name for this provider (e.g., "OpenAI").
    - **Base URL**: The root URL of the API (e.g., `https://api.openai.com/v1`).
    - **API Key**: Your authentication token for the service.
4. Click **Create**.

### OAuth Authentication

Some providers (like OpenAI) support OAuth-based authentication. If a provider supports OAuth:

1. Select the provider from the list.
2. Click **Connect with OAuth** in the provider settings form.
3. Follow the authorization flow in the popup window.
4. Once authorized, the provider will use the OAuth token instead of a manual API key.

### Import Models

Memoh can automatically discover and import available models from a provider:

1. Select a provider from the list.
2. Click **Import Models**.
3. Memoh will query the provider's API to fetch available models.
4. Select which models to import and click **Import**.

This saves time compared to manually adding each model one by one.

### Managing Providers

- **Edit**: Select a provider from the list and use the form on the right to update its name, URL, or API key.
- **Test**: Click **Test Connection** to verify the provider is reachable.
- **Delete**: Use the **Delete Provider** button in the provider settings form.

---

## Model

A Model is a specific AI instance (like `gpt-4o` or `text-embedding-3-small`) that belongs to a Provider. Memoh distinguishes between **Chat** models (for conversation) and **Embedding** models (for memory search).

### Adding a Model

1. Select a Provider from the list on the **Providers** page.
2. Click **Add Model** in the model list section.
3. Configure the following fields:

| Field | Required | Description |
|-------|----------|-------------|
| **Type** | Yes | `chat` for conversation, `embedding` for vector search. |
| **Model ID** | Yes | The exact identifier used by the provider (e.g., `gpt-4o`). |
| **Name** | No | A friendly display name (defaults to Model ID). |
| **Client Type** | Yes (Chat) | The API protocol: `openai-responses`, `openai-completions`, `anthropic-messages`, or `google-generative-ai`. |
| **Input Modalities**| Yes (Chat) | Capabilities supported: `text` (default), `image`, `audio`, `video`, `file`. |
| **Supports Reasoning**| No | Enable if the model supports internal reasoning steps (e.g., OpenAI o1). |
| **Dimensions** | Yes (Embed) | The vector size for embedding models (e.g., 1536). |

4. Click **Create**.

### Managing Models

- **Edit**: Click the edit icon next to a model in the list.
- **Delete**: Click the trash icon next to a model to remove it.

---

## Next Steps

Now that you have configured your models, you can proceed to [Create and Configure a Bot](/getting-started/bot).
