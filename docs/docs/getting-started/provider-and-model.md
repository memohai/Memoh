# Configure Provider and Model

Before creating bots, you need to configure at least one LLM provider and add chat and embedding models. This guide walks you through the Web UI.

## Prerequisites

- Memoh is running (see [Docker installation](/installation/docker))
- You have logged in to the Web UI at http://localhost:8082

## Step 1: Open the Models Page

Click **Models** in the left sidebar to open the Provider and Model configuration page.

![Models page - sidebar](/getting-started/provider-model-01-sidebar.png)

The page has two panels:
- **Left**: Provider list and search
- **Right**: Selected provider's details and models (or an empty state if none selected)

## Step 2: Add a Provider

Click the **Add Provider** button (with a plus icon) at the bottom of the left panel.

![Add Provider button](/getting-started/provider-model-02-add-provider.png)

In the dialog, fill in:

| Field | Description |
|-------|-------------|
| **Name** | A display name for this provider (e.g. `my-openai`, `ollama-local`) |
| **API Key** | Your API key. For local services like Ollama, you can use a placeholder like `ollama` |
| **Base URL** | The API base URL (e.g. `https://api.openai.com/v1`, `http://localhost:11434/v1` for Ollama) |

![Add Provider dialog](/getting-started/provider-model-03-provider-dialog.png)

**Example — OpenAI:**
- Name: `openai`
- API Key: `sk-...`
- Base URL: `https://api.openai.com/v1`

**Example — Ollama (local):**
- Name: `ollama`
- API Key: `ollama`
- Base URL: `http://localhost:11434/v1`

Click **Add** to save. The new provider appears in the left sidebar.

## Step 3: Add Models

Select a provider from the left panel. The right panel shows the provider form and the model list.

![Provider selected - model list](/getting-started/provider-model-04-provider-selected.png)

Click **Add Model** to open the model creation dialog.

Fill in:

| Field | Description |
|-------|-------------|
| **Client Type** | API protocol: `openai-responses`, `openai-completions`, `anthropic-messages`, or `google-generative-ai` |
| **Type** | `chat` or `embedding` |
| **Model** | Model ID (e.g. `gpt-4`, `llama3.2`, `text-embedding-3-small`) |
| **Display Name** | Optional display name |
| **Dimensions** | Required for embedding models (e.g. `1536` for OpenAI embeddings) |
| **Multimodal** | For chat models only — enable if the model supports images |

**You need at least:**
- One **chat** model (for conversation)
- One **embedding** model (for memory)

Add them under the same or different providers. For example:
- Chat: `gpt-4` with client type `openai-responses` (OpenAI) or `llama3.2` with client type `openai-completions` (Ollama)
- Embedding: `text-embedding-3-small` with client type `openai-completions` (OpenAI) or `nomic-embed-text` with client type `openai-completions` (Ollama)

## Step 4: Edit or Delete

- **Provider**: After selecting a provider, you can edit Name, API Key, and Base URL in the right panel, or delete the provider.
- **Model**: Use the edit or delete actions on each model card in the list.

## Next Steps

Once you have at least one chat model and one embedding model, you can create a bot (via **Bots** in the sidebar) and assign these models in the bot settings.
