import type { ModelsClientType } from '@memoh/sdk'

export interface ClientTypeMeta {
  value: ModelsClientType
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
}

export const CLIENT_TYPE_LIST: ClientTypeMeta[] = Object.values(CLIENT_TYPE_META)
