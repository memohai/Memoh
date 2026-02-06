import z from 'zod'
import { allActions } from './types'

export const AgentSkillModel = z.object({
  name: z.string().min(1, 'Skill name is required'),
  description: z.string().min(1, 'Skill description is required'),
  content: z.string().min(1, 'Skill content is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const ClientTypeModel = z.enum(['openai', 'openai-compatible', 'anthropic', 'google'])

export const ModelConfigModel = z.object({
  modelId: z.string().min(1, 'Model ID is required'),
  clientType: ClientTypeModel,
  input: z.array(z.enum(['text', 'image'])),
  apiKey: z.string().min(1, 'API key is required'),
  baseUrl: z.string(),
})

export const AllowedActionModel = z.enum(allActions)

export const IdentityContextModel = z.object({
  botId: z.string().min(1, 'Bot ID is required'),
  sessionId: z.string().min(1, 'Session ID is required'),
  containerId: z.string().min(1, 'Container ID is required'),
  contactId: z.string().min(1, 'Contact ID is required'),
  contactName: z.string().min(1, 'Contact name is required'),
  contactAlias: z.string().optional(),
  userId: z.string().optional(),
  currentPlatform: z.string().optional(),
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
  base64: z.string().min(1, 'Image base64 is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const FileAttachmentModel = z.object({
  type: z.literal('file'),
  path: z.string().min(1, 'File path is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const AttachmentModel = z.union([ImageAttachmentModel, FileAttachmentModel])