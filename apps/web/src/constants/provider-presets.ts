export interface ProviderPreset {
  id: string
  name: string
  registryName?: string
  clientType: string
  baseUrl: string
  icon: string
  source: string
  requiresApiKey?: boolean
}

export const providerPresets: ProviderPreset[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    clientType: 'openai-responses',
    baseUrl: 'https://api.openai.com/v1',
    icon: 'openai',
    source: 'openai.yaml',
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    clientType: 'anthropic-messages',
    baseUrl: 'https://api.anthropic.com',
    icon: 'anthropic',
    source: 'anthropic.yaml',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    clientType: 'openai-completions',
    baseUrl: 'https://openrouter.ai/api/v1',
    icon: 'openrouter',
    source: 'openrouter.yaml',
  },
  {
    id: 'google',
    name: 'Google Gemini',
    registryName: 'Google',
    clientType: 'google-generative-ai',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    icon: 'gemini-color',
    source: 'google.yaml',
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    clientType: 'openai-completions',
    baseUrl: 'https://api.deepseek.com/v1',
    icon: 'deepseek-color',
    source: 'deepseek.yaml',
  },
  {
    id: 'moonshot',
    name: 'Moonshot',
    clientType: 'openai-completions',
    baseUrl: 'https://api.moonshot.cn/v1',
    icon: 'moonshot',
    source: 'moonshot.yaml',
  },
  {
    id: 'minimax',
    name: 'MiniMax',
    registryName: 'Minimax',
    clientType: 'openai-completions',
    baseUrl: 'https://api.minimaxi.com/v1',
    icon: 'minimax-color',
    source: 'minimax.yaml',
  },
  {
    id: 'xai',
    name: 'xAI Grok',
    registryName: 'xAI (Grok)',
    clientType: 'openai-responses',
    baseUrl: 'https://api.x.ai/v1',
    icon: 'xai',
    source: 'xai.yaml',
  },
  {
    id: 'groq',
    name: 'Groq',
    clientType: 'openai-completions',
    baseUrl: 'https://api.groq.com/openai/v1',
    icon: 'groq',
    source: 'groq.yaml',
  },
  {
    id: 'mistral',
    name: 'Mistral',
    clientType: 'openai-completions',
    baseUrl: 'https://api.mistral.ai/v1',
    icon: 'mistral-color',
    source: 'mistral.yaml',
  },
  {
    id: 'qwen',
    name: 'Aliyun Bailian',
    clientType: 'openai-completions',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    icon: 'qwen-color',
    source: 'qwen.yaml',
  },
  {
    id: 'huggingface',
    name: 'HuggingFace',
    clientType: 'openai-completions',
    baseUrl: 'https://router.huggingface.co/hf-inference/v1',
    icon: 'huggingface-color',
    source: 'huggingface.yaml',
  },
  {
    id: 'ollama',
    name: 'Ollama',
    clientType: 'openai-completions',
    baseUrl: 'http://127.0.0.1:11434/v1',
    icon: 'ollama',
    source: 'ollama.yaml',
    requiresApiKey: false,
  },
  {
    id: 'lmstudio',
    name: 'LM Studio',
    clientType: 'openai-completions',
    baseUrl: 'http://127.0.0.1:1234/v1',
    icon: 'lmstudio',
    source: 'lmstudio.yaml',
    requiresApiKey: false,
  },
  {
    id: 'newapi',
    name: 'New API',
    clientType: 'openai-completions',
    baseUrl: 'https://your.new-api-provider.com',
    icon: 'newapi-color',
    source: 'newapi.yaml',
  },
  {
    id: 'openai-codex',
    name: 'OpenAI Codex',
    clientType: 'openai-codex',
    baseUrl: 'https://chatgpt.com/backend-api',
    icon: 'openai',
    source: 'codex.yaml',
    requiresApiKey: false,
  },
  {
    id: 'github-copilot',
    name: 'GitHub Copilot',
    clientType: 'github-copilot',
    baseUrl: '',
    icon: 'github-copilot',
    source: 'github-copilot.yaml',
    requiresApiKey: false,
  },
]

const onboardingPresetIds = new Set([
  'openai',
  'anthropic',
  'openrouter',
  'google',
  'deepseek',
  'moonshot',
])

export const onboardingProviderPresets = providerPresets.filter(preset =>
  onboardingPresetIds.has(preset.id),
)
