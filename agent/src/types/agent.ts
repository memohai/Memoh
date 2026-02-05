import { ModelMessage } from 'ai'
import { ModelConfig } from './model'
import { AgentAttachment } from './attachment'

export interface IdentityContext {
  botId: string
  sessionId: string
  containerId: string

  contactId: string
  contactName: string
  contactAlias?: string
  userId?: string

  currentPlatform?: string
  replyTarget?: string
  sessionToken?: string
}

export enum AgentAction {
  Web = 'web',
  Message = 'message',
  Contact = 'contact',
  Subagent = 'subagent',
  Schedule = 'schedule',
  Skill = 'skill',
  Memory = 'memory',
}

export const allActions = Object.values(AgentAction)

export interface BraveConfig {
  apiKey: string
  baseUrl: string
}

export interface AgentParams {
  model: ModelConfig
  language?: string
  activeContextTime?: number
  allowedActions?: AgentAction[]
  brave?: BraveConfig
  identity: IdentityContext
  platforms?: string[]
  currentPlatform?: string
}

export interface AgentInput {
  messages: ModelMessage[]
  attachments: AgentAttachment[]
  skills: string[]
  query: string
}

export interface AgentSkill {
  name: string
  description: string
  content: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  metadata: Record<string, any>
}
