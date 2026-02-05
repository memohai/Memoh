export enum ClientType {
  OpenAI = 'openai',
  OpenAICompatible = 'openai-compatible',
  Anthropic = 'anthropic',
  Google = 'google',
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