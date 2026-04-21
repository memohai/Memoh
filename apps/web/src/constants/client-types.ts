export interface ClientTypeMeta {
  value: string
  label: string
  hint: string
}

export const CLIENT_TYPE_META: Record<string, ClientTypeMeta> = {
  'openai-responses': {
    value: 'openai-responses',
    label: 'OpenAI Responses',
    hint: 'Responses API (streaming, built-in tools)',
  },
  'openai-completions': {
    value: 'openai-completions',
    label: 'OpenAI Completions',
    hint: 'Chat Completions API (widely compatible)',
  },
  'openai-codex': {
    value: 'openai-codex',
    label: 'OpenAI Codex',
    hint: 'Codex API (OAuth, coding-optimized)',
  },
  'github-copilot': {
    value: 'github-copilot',
    label: 'GitHub Copilot',
    hint: 'Device OAuth with GitHub account',
  },
  'anthropic-messages': {
    value: 'anthropic-messages',
    label: 'Anthropic Messages',
    hint: 'Messages API (Claude models)',
  },
  'google-generative-ai': {
    value: 'google-generative-ai',
    label: 'Google Generative AI',
    hint: 'Gemini API',
  },
  'edge-speech': {
    value: 'edge-speech',
    label: 'Edge Speech',
    hint: 'Microsoft Edge Read Aloud TTS',
  },
  'openai-speech': {
    value: 'openai-speech',
    label: 'OpenAI Speech',
    hint: 'OpenAI /audio/speech compatible TTS',
  },
  'openrouter-speech': {
    value: 'openrouter-speech',
    label: 'OpenRouter Speech',
    hint: 'OpenRouter audio modality TTS',
  },
  'elevenlabs-speech': {
    value: 'elevenlabs-speech',
    label: 'ElevenLabs Speech',
    hint: 'ElevenLabs text-to-speech',
  },
  'deepgram-speech': {
    value: 'deepgram-speech',
    label: 'Deepgram Speech',
    hint: 'Deepgram TTS',
  },
  'minimax-speech': {
    value: 'minimax-speech',
    label: 'MiniMax Speech',
    hint: 'MiniMax TTS',
  },
  'volcengine-speech': {
    value: 'volcengine-speech',
    label: 'Volcengine Speech',
    hint: 'Volcengine SAMI TTS',
  },
  'alibabacloud-speech': {
    value: 'alibabacloud-speech',
    label: 'Alibaba Cloud Speech',
    hint: 'DashScope CosyVoice TTS',
  },
  'microsoft-speech': {
    value: 'microsoft-speech',
    label: 'Microsoft Speech',
    hint: 'Azure Cognitive Services TTS',
  },
}

export const CLIENT_TYPE_LIST: ClientTypeMeta[] = Object.values(CLIENT_TYPE_META)

export const LLM_CLIENT_TYPE_LIST: ClientTypeMeta[] = CLIENT_TYPE_LIST
  .filter(ct => !ct.value.endsWith('-speech'))
