import { ModelMessage } from 'ai'
import { ModelConfig } from './model'
import { AgentAttachment } from './attachment'
import { MCPConnection } from './mcp'

/** 与 Go gatewayIdentity 对齐 */
export interface IdentityContext {
  botId: string
  containerId: string
  channelIdentityId: string
  speakerAlias?: string
  displayName: string
  currentPlatform?: string
  conversationType?: string
  sessionToken?: string
}

export interface AgentAuthContext {
  bearer: string
  baseUrl: string
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

export interface AgentParams {
  model: ModelConfig
  language?: string
  activeContextTime?: number
  allowedActions?: AgentAction[]
  mcpConnections?: MCPConnection[]
  channels?: string[]
  currentChannel?: string
  identity?: IdentityContext
  auth: AgentAuthContext
  skills?: AgentSkill[]
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
  metadata?: Record<string, unknown>
}
