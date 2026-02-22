export enum ClientType {
  OpenAIResponses = 'openai-responses',
  OpenAICompletions = 'openai-completions',
  AnthropicMessages = 'anthropic-messages',
  GoogleGenerativeAI = 'google-generative-ai',
}

export enum ModelInput {
  Text = 'text',
  Image = 'image',
  Audio = 'audio',
  Video = 'video',
  File = 'file',
}

export type ReasoningEffort = 'low' | 'medium' | 'high'

export interface ReasoningConfig {
  enabled: boolean
  effort: ReasoningEffort
}

export interface ModelConfig {
  apiKey: string
  baseUrl: string
  modelId: string
  clientType: ClientType
  input: ModelInput[]
  reasoning?: ReasoningConfig
}

export const hasInputModality = (config: ModelConfig, modality: ModelInput): boolean =>
  config.input.includes(modality)
