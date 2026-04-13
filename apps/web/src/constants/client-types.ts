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
}

export const CLIENT_TYPE_LIST: ClientTypeMeta[] = Object.values(CLIENT_TYPE_META)
