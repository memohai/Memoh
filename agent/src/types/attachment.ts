export interface BaseAgentAttachment {
  type: string
  metadata?: Record<string, unknown>
}

export interface ImageAttachment extends BaseAgentAttachment {
  type: 'image'
  base64: string
}

export interface ContainerFileAttachment extends BaseAgentAttachment {
  type: 'file'
  path: string
}

export type AgentAttachment = ImageAttachment | ContainerFileAttachment