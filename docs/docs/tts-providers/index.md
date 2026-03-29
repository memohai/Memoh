# TTS Providers

Memoh supports **Text-to-Speech (TTS)** so bots can synthesize spoken audio from text. The TTS system is organized into three layers:

- **TTS Provider**: A service type (e.g. Edge TTS). You create named provider instances from the TTS Providers page.
- **TTS Model**: A specific voice/model under a provider (e.g. `edge-read-aloud`). Models have configurable voice, format, speed, and pitch settings.
- **Bot Assignment**: In Bot Settings, select a TTS Model. The bot can then synthesize speech in conversations.

---

## Basic Flow

1. Navigate to the **TTS Providers** page from the sidebar.
2. Click **Add** and select a provider type (e.g. `edge`).
3. Click **Create** — the provider's default model is auto-imported.
4. Click the model to configure voice, format, speed, and pitch.
5. Test synthesis with the built-in test button.
6. Open a bot's **General** tab and select the TTS Model.
7. Save — the bot can now synthesize speech.

---

## Available Providers

| Provider | Description |
|----------|-------------|
| [Edge TTS](/tts-providers/edge.md) | Free, uses Microsoft Edge's public read-aloud API. 256+ voices across 50+ languages. No API key required. |

---

## Next Steps

- To set up the currently available provider, continue with [Edge TTS](/tts-providers/edge.md).
