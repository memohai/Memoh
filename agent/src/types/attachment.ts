export interface BaseAgentAttachment {
  type: string
  url?: string
  name?: string
  mime?: string
  asset_id?: string
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