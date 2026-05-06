# Providers And Models

To use Memoh effectively, you usually configure:

- one or more **providers** that define how Memoh talks to upstream APIs
- one or more **models** under those providers
- optional **speech providers** if you want text-to-speech

The Web UI manages chat and embedding providers/models from the **Models** page. Speech models are managed separately from [TTS Providers](/tts-providers/index.md).

---

## Provider Basics

A **provider** stores connection information for one upstream service, such as:

- the API protocol (`client_type`)
- the base URL if the protocol needs one
- credentials such as an API key or OAuth token

Typical examples include OpenAI-compatible endpoints, Anthropic, Google Gemini, OpenAI Codex, and GitHub Copilot.

### Creating A Provider

1. Open the **Models** page from the settings sidebar.
2. Click **Add Provider**.
3. Fill in the provider form.
4. Save the provider.

Common fields:

| Field | Description |
|-------|-------------|
| **Name** | Friendly display name, such as `OpenAI` or `Copilot`. |
| **Client Type** | API protocol used by this provider. |
| **Base URL** | Root API endpoint, when required by the selected client type. |
| **API Key** | Token-based authentication, when the client type uses direct credentials. |

### Client Types

Memoh currently supports these client types:

| Client Type | Typical Use |
|-------------|-------------|
| `openai-responses` | OpenAI Responses API style providers |
| `openai-completions` | OpenAI Chat Completions compatible providers |
| `anthropic-messages` | Anthropic Messages API |
| `google-generative-ai` | Google Gemini API |
| `openai-codex` | OpenAI Codex / ChatGPT-backed coding workflow with OAuth |
| `github-copilot` | GitHub Copilot with device OAuth |
| `edge-speech` | Speech-only provider type for Microsoft Edge Read Aloud |

`edge-speech` is for speech synthesis, not for chat. Configure it through [TTS Providers](/tts-providers/index.md), not as your main chat provider.

---

## OAuth-Based Providers

Most provider types use a normal API key. Two notable exceptions are `openai-codex` and `github-copilot`.

### OpenAI Codex

- Uses the `openai-codex` client type
- Authenticates through the provider form's OAuth flow instead of a normal API key workflow
- The bundled preset points at `https://chatgpt.com/backend-api`

This is a good fit when you want Codex-style model access for coding-oriented workflows.

### GitHub Copilot

- Uses the `github-copilot` client type
- Uses **device authorization**
- The provider form shows a verification URL and a user code while authorization is pending
- After authorization completes, the provider stores the linked GitHub account token

GitHub Copilot is especially useful if you already have access to Copilot-backed chat and embedding models and want to reuse that access from Memoh.

---

## Importing Models

After creating a provider, you can import or add models under it.

Typical flow:

1. Select the provider.
2. Click **Import Models** if the provider can expose a model catalog.
3. Choose the models you want to save into Memoh.

You can also add models manually when you already know the upstream model ID.

---

## Model Types

Memoh distinguishes three model types:

| Type | Purpose |
|------|---------|
| `chat` | Main LLMs for conversation, tool use, reasoning, and image generation |
| `embedding` | Vector models for memory and retrieval |
| `speech` | Text-to-speech models used by TTS providers |

Important distinction:

- The **Models** page is primarily where you manage `chat` and `embedding` models.
- `speech` models are exposed through [TTS Providers](/tts-providers/index.md).

---

## Chat Model Configuration

When adding a chat model, the most important fields are:

| Field | Description |
|-------|-------------|
| **Model ID** | Exact upstream identifier, such as `gpt-4o` or `claude-sonnet-4.6`. |
| **Name** | Friendly display name shown in the UI. |
| **Compatibilities** | Feature flags such as `vision`, `tool-call`, `image-output`, and `reasoning`. |
| **Context Window** | Approximate maximum context budget for the model. |

### Compatibilities

Memoh uses compatibility flags to decide which features a model can safely power:

| Compatibility | Meaning |
|---------------|---------|
| `vision` | Model can accept images as input |
| `tool-call` | Model can call tools |
| `image-output` | Model can generate images |
| `reasoning` | Model exposes explicit reasoning modes / effort levels |

If a model supports reasoning, it may also declare `reasoning_efforts` such as `none`, `low`, `medium`, `high`, or `xhigh`.

### `context_window`

`context_window` is important because Memoh uses it to:

- calculate session context usage in the Web UI
- power `/status` output
- decide when a session is approaching its prompt limit
- guide [Context Compaction](/getting-started/compaction)

If you leave `context_window` empty, the model can still be used, but Memoh cannot show an exact usage percentage for that model.

### Image Generation Models

Memoh now lets you assign an **Image Generation Model** to a bot. This model must be a chat model whose compatibilities include `image-output`.

That keeps image generation separate from your default chat model when needed.

---

## Embedding Models

Embedding models are used for semantic indexing and retrieval.

The required field is:

| Field | Description |
|-------|-------------|
| **Dimensions** | Vector size for the embedding output, such as `1536`. |

Use embedding models with memory providers or any feature that relies on vector search.

---

## Speech Models

Speech models are managed from [TTS Providers](/tts-providers/index.md), not from the standard chat provider flow.

Current built-in example:

- **Edge TTS** via `edge-speech`

This separation matters because speech models have voice, format, speed, and pitch settings that do not apply to chat or embedding models.

---

## Recommended Mental Model

For most bots, think in terms of three parallel model roles:

- **Chat model** for normal conversations
- **Embedding model** for memory search
- **Speech / image models** for side capabilities such as TTS and image generation

You do not need to force one model to do everything.

---

## Next Steps

- To assign chat, image, memory, and TTS settings to a bot, see [Bot Management](/getting-started/bot).
- To configure speech providers and speech models, see [TTS Providers](/tts-providers/index.md).
