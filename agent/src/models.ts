import z from 'zod'
import { allActions } from '@memoh/agent'

export const AgentSkillModel = z.object({
  name: z.string().min(1, 'Skill name is required'),
  description: z.string().min(1, 'Skill description is required'),
  content: z.string().min(1, 'Skill content is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

export const ClientTypeModel = z.enum([
  'openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai',
])

export const ReasoningConfigModel = z.object({
  enabled: z.boolean(),
  effort: z.enum(['low', 'medium', 'high']),
}).optional()

export const ModelConfigModel = z.object({
  modelId: z.string().min(1, 'Model ID is required'),
  clientType: ClientTypeModel,
  input: z.array(z.enum(['text', 'image', 'audio', 'video', 'file'])),
  apiKey: z.string().min(1, 'API key is required'),
  baseUrl: z.string(),
  reasoning: ReasoningConfigModel,
})

export const AllowedActionModel = z.enum(allActions)

export const IdentityContextModel = z.object({
  botId: z.string().min(1, 'Bot ID is required'),
  containerId: z.string().min(1, 'Container ID is required'),
  channelIdentityId: z.string().min(1, 'Channel identity ID is required'),
  displayName: z.string().min(1, 'Display name is required'),
  currentPlatform: z.string().optional(),
  conversationType: z.string().optional(),
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

export const HeartbeatModel = z.object({
  interval: z.number().int().positive().default(30),
})

export const AttachmentModel = z.object({
  contentHash: z.string().optional(),
  type: z.string().min(1, 'Attachment type is required'),
  mime: z.string().optional(),
  size: z.number().int().nonnegative().optional(),
  name: z.string().optional(),
  transport: z.enum(['inline_data_url', 'public_url', 'tool_file_ref']),
  payload: z.string().min(1, 'Attachment payload is required'),
  metadata: z.record(z.string(), z.any()).optional(),
})

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

export const InboxItemModel = z.object({
  id: z.string(),
  source: z.string(),
  header: z.record(z.string(), z.unknown()).default({}),
  content: z.string().default(''),
  createdAt: z.string(),
})