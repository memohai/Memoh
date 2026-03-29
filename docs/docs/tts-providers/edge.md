# Edge TTS

Edge TTS uses Microsoft Edge's public read-aloud API for speech synthesis. It is free, requires no API key, and supports 256+ voices across 50+ languages.

---

## Creating an Edge TTS Provider

1. Navigate to the **TTS Providers** page.
2. Click **Add**.
3. Select `edge` as the provider type.
4. Click **Create**.

The default model `edge-read-aloud` is automatically imported when the provider is created.

---

## Configuring the Model

Click the `edge-read-aloud` model to configure its settings.

| Field | Description |
|-------|-------------|
| **Voice** | Language + voice ID. Default: `en-US-EmmaMultilingualNeural`. Over 256 voices available across 50+ languages. |
| **Format** | Audio output format. Options: `audio-24khz-48kbitrate-mono-mp3` (default), `audio-24khz-96kbitrate-mono-mp3`, `webm-24khz-16bit-mono-opus`. |
| **Speed** | Playback speed. Options: `0.5`, `1.0` (default), `2.0`, `3.0`. |
| **Pitch** | Voice pitch adjustment. Range: `-100` to `+100`, default `0`. |

---

## Assigning to a Bot

1. Open a bot's **General** tab.
2. Find the **TTS Model** dropdown.
3. Select the configured Edge TTS model.
4. Click **Save**.

The bot can now synthesize speech using Edge TTS.

---

## Testing

Use the built-in synthesis test button on the model configuration page to preview how the selected voice, format, speed, and pitch sound before assigning to a bot.
