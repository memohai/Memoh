# Provider and Model

In Memoh, **provider** and **model** are separate but connected concepts:

- A **provider** is the LLM service configuration (API endpoint and key)
- A **model** is the concrete chat or embedding model under that provider, including its **client type** which determines which API protocol to use

## Client Types

Each model has a `client_type` that determines how Memoh communicates with the LLM service:

| Client Type | Description |
|-------------|-------------|
| `openai-responses` | OpenAI Responses API |
| `openai-completions` | OpenAI Chat Completions API (also works with compatible services like Ollama, Mistral, etc.) |
| `anthropic-messages` | Anthropic Messages API |
| `google-generative-ai` | Google Generative AI API |

## Typical Setup

At minimum, a production-ready bot usually needs:

- One **chat** model for dialog generation
- One **embedding** model for memory indexing and retrieval

## Model Assignment to Bot

Bots reference model IDs in settings:

- `chat_model_id`
- `memory_model_id`
- `embedding_model_id`

This enables per-bot customization (for quality, latency, or cost).

## Web UI Path

- `Models > Add Provider > Select Provider > Add Model`
- `Bots > Select a bot > Settings > Choose chat/memory/embedding models`
