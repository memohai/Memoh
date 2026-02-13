export enum ClientType {
  OpenAI = 'openai',
  OpenAICompat = 'openai-compat',
  Anthropic = 'anthropic',
  Google = 'google',
  Azure = 'azure',
  Bedrock = 'bedrock',
  Mistral = 'mistral',
  XAI = 'xai',
  Ollama = 'ollama',
  Dashscope = 'dashscope',
}

export enum ModelInput {
  Text = 'text',
  Image = 'image',
}

export interface ModelConfig {
  apiKey: string
  baseUrl: string
  modelId: string
  clientType: ClientType
  input: ModelInput[]
}