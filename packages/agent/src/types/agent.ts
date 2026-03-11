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
  replyTarget?: string
  conversationType?: string
  sessionToken?: string
}

export interface AgentAuthContext {
  bearer: string
  baseUrl: string
}

export interface InboxItem {
  id: string
  source: string
  header: Record<string, unknown>
  content: string
  createdAt: string
}

export interface LoopDetectionConfig {
  enabled?: boolean
}

export interface AgentParams {
  model: ModelConfig
  language?: string
  activeContextTime?: number
  mcpConnections?: MCPConnection[]
  channels?: string[]
  currentChannel?: string
  identity?: IdentityContext
  auth: AgentAuthContext
  skills?: AgentSkill[]
  inbox?: InboxItem[]
  loopDetection?: LoopDetectionConfig
  isSubagent?: boolean
}

export interface AgentInput {
  messages: ModelMessage[]
  attachments: GatewayInputAttachment[]
  skills: string[]
  query: string
  signal?: AbortSignal
}

export interface AgentSkill {
  name: string
  description: string
  content: string
  metadata?: Record<string, unknown>
}

export interface SystemFile {
  filename: string
  content: string
}
