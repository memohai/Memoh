export interface ProviderPreset {
  id: string
  name: string
  registryName?: string
  clientType: string
  baseUrl: string
  icon: string
}

export const providerPresets: ProviderPreset[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    clientType: 'openai-responses',
    baseUrl: 'https://api.openai.com/v1',
    icon: 'openai',
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    clientType: 'anthropic-messages',
    baseUrl: 'https://api.anthropic.com',
    icon: 'anthropic',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    clientType: 'openai-completions',
    baseUrl: 'https://openrouter.ai/api/v1',
    icon: 'openrouter',
  },
  {
    id: 'google',
    name: 'Google Gemini',
    registryName: 'Google',
    clientType: 'google-generative-ai',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    icon: 'gemini-color',
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    clientType: 'openai-completions',
    baseUrl: 'https://api.deepseek.com/v1',
    icon: 'deepseek-color',
  },
  {
    id: 'moonshot',
    name: 'Moonshot',
    clientType: 'openai-completions',
    baseUrl: 'https://api.moonshot.cn/v1',
    icon: 'moonshot',
  },
]
