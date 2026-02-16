import z from 'zod'
import { allActions } from './types'

export const AgentSkillModel = z.object({
  name: z.string().min(1, 'Skill name is required'),
  description: z.string().min(1, 'Skill description is required'),
  content: z.string().min(1, 'Skill content is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const ClientTypeModel = z.enum([
  'openai', 'openai-compat', 'anthropic', 'google',
  'azure', 'bedrock', 'mistral', 'xai', 'ollama', 'dashscope',
])

export const ModelConfigModel = z.object({
  modelId: z.string().min(1, 'Model ID is required'),
  clientType: ClientTypeModel,
  input: z.array(z.enum(['text', 'image', 'audio', 'video', 'file'])),
  apiKey: z.string().min(1, 'API key is required'),
  baseUrl: z.string(),
})

export const AllowedActionModel = z.enum(allActions)

export const IdentityContextModel = z.object({
  botId: z.string().min(1, 'Bot ID is required'),
  containerId: z.string().min(1, 'Container ID is required'),
  channelIdentityId: z.string().min(1, 'Channel identity ID is required'),
  displayName: z.string().min(1, 'Display name is required'),
  contactId: z.string().optional(),
  contactName: z.string().optional(),
  contactAlias: z.string().optional(),
  userId: z.string().optional(),
  currentPlatform: z.string().optional(),
  conversationType: z.string().optional(),
  replyTarget: z.string().optional(),
  sessionToken: z.string().optional(),
})

export const ScheduleModel = z.object({
  id: z.string().min(1, 'Schedule ID is required'),
  name: z.string().min(1, 'Schedule name is required'),
  description: z.string().min(1, 'Schedule description is required'),
  pattern: z.string().min(1, 'Schedule pattern is required'),
  maxCalls: z.number().nullable().optional(),
  command: z.string().min(1, 'Schedule command is required'),
})

export const ImageAttachmentModel = z.object({
  type: z.literal('image'),
  base64: z.string().optional(),
  path: z.string().optional(),
  mime: z.string().optional(),
  name: z.string().optional(),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const FileAttachmentModel = z.object({
  type: z.literal('file'),
  path: z.string().min(1, 'File path is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const AttachmentModel = z.union([ImageAttachmentModel, FileAttachmentModel])

export const HTTPMCPConnectionModel = z.object({
  name: z.string().min(1, 'Name is required'),
  type: z.literal('http'),
  url: z.string().min(1, 'URL is required'),
  headers: z.record(z.string(), z.string()).optional(),
})

export const SSEMCPConnectionModel = z.object({
  name: z.string().min(1, 'Name is required'),
  type: z.literal('sse'),
  url: z.string().min(1, 'URL is required'),
  headers: z.record(z.string(), z.string()).optional(),
})

export const StdioMCPConnectionModel = z.object({
  name: z.string().min(1, 'Name is required'),
  type: z.literal('stdio'),
  command: z.string().min(1, 'Command is required'),
  args: z.array(z.string()),
  env: z.record(z.string(), z.string()).optional(),
  cwd: z.string().optional(),
})

export const MCPConnectionModel = z.union([HTTPMCPConnectionModel, SSEMCPConnectionModel, StdioMCPConnectionModel])