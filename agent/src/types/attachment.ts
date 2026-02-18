export type GatewayAttachmentTransport =
  | 'inline_data_url'
  | 'public_url'
  | 'tool_file_ref'

export interface GatewayInputAttachment {
  contentHash?: string
  type: string
  mime?: string
  size?: number
  name?: string
  transport: GatewayAttachmentTransport
  payload: string
  metadata?: Record<string, unknown>
}

export interface BaseAgentAttachment {
  type: string
  url?: string
  name?: string
  mime?: string
  content_hash?: string
  metadata?: Record<string, unknown>
}

export interface ImageAttachment extends BaseAgentAttachment {
  type: 'image'
  base64?: string
  url?: string
  path?: string
}

export interface ContainerFileAttachment extends BaseAgentAttachment {
  type: 'file'
  path: string
}

export type AgentAttachment = ImageAttachment | ContainerFileAttachment