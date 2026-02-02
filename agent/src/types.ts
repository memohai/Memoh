export enum ClientType {
  OPENAI = 'openai',
  ANTHROPIC = 'anthropic',
  GOOGLE = 'google',
}

export interface BaseModelConfig {
  apiKey: string
  baseUrl: string
  model: string
  clientType: ClientType
}

export interface Schedule {
  id: string
  name: string
  description: string
  pattern: string
  maxCalls?: number | null
  command: string
}

export interface AgentSkill {
  name: string
  description: string
  content: string
}