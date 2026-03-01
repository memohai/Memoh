import { ModelMessage } from 'ai'
import { ModelConfig } from './model'
import { GatewayInputAttachment } from './attachment'
import { MCPConnection } from './mcp'

export interface IdentityContext {
  botId: string
  containerId: string
  channelIdentityId: string
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

export interface InboxItem {
  id: string
  source: string
  header: Record<string, unknown>
  content: string
  createdAt: string
}

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
  inbox?: InboxItem[]
}

export interface AgentInput {
  messages: ModelMessage[]
  attachments: GatewayInputAttachment[]
  skills: string[]
  query: string
}

export interface AgentSkill {
  name: string
  description: string
  content: string
  metadata?: Record<string, unknown>
}
