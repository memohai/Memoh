import {
  getBots,
  deleteBotsByBotIdMessages,
  deleteBotsByBotIdAcpRuntimesByRuntimeId,
  getBotsByBotIdSessions,
  getBotsByBotIdSessionsBySessionIdAcpRuntime,
  postBotsByBotIdAcpRuntimes,
  postBotsByBotIdSessions,
  postBotsByBotIdSessionsBySessionIdAcpRuntime,
  deleteBotsByBotIdSessionsBySessionId,
  patchBotsByBotIdAcpRuntimesByRuntimeIdModel,
  patchBotsByBotIdSessionsBySessionId,
  patchBotsByBotIdSessionsBySessionIdAcpRuntimeModel,
} from '@memohai/sdk'
import type { AcpagentRuntimeStatus } from '@memohai/sdk'
import type { Bot, SessionSummary } from './useChat.types'

export interface CreateSessionOptions {
  title?: string
  type?: string
  metadata?: Record<string, unknown>
  /** Warm pre-session ACP runtime to bind at creation time. */
  acpRuntimeId?: string
}

export interface CreateACPRuntimeOptions {
  agentId: string
  projectPath?: string
}

export async function fetchBots(): Promise<Bot[]> {
  const { data } = await getBots({ throwOnError: true })
  return data?.items ?? []
}

export interface FetchSessionsOptions {
  types?: string[]
  parentSessionId?: string
  limit?: number
  cursor?: string
}

export interface FetchSessionsResult {
  items: SessionSummary[]
  nextCursor: string | null
}

const DEFAULT_SESSION_TYPES = ['chat', 'discuss', 'acp_agent']
const DEFAULT_SESSION_PAGE_SIZE = 50

export async function fetchSessions(botId: string, options?: FetchSessionsOptions): Promise<FetchSessionsResult> {
  const id = botId.trim()
  if (!id) return { items: [], nextCursor: null }
  const types = (options?.types ?? DEFAULT_SESSION_TYPES).map(t => t.trim()).filter(Boolean)
  const cursor = options?.cursor?.trim() ?? ''
  const { data } = await getBotsByBotIdSessions({
    path: { bot_id: id },
    query: {
      types: types.join(','),
      limit: options?.limit ?? DEFAULT_SESSION_PAGE_SIZE,
      ...(cursor ? { cursor } : {}),
      ...(options?.parentSessionId?.trim() ? { parent_session_id: options.parentSessionId.trim() } : {}),
    },
    throwOnError: true,
  })
  const payload = data as { items?: SessionSummary[]; next_cursor?: string } | undefined
  return {
    items: payload?.items ?? [],
    nextCursor: payload?.next_cursor?.trim() || null,
  }
}

export async function fetchAllSessions(botId: string, options: FetchSessionsOptions = {}): Promise<SessionSummary[]> {
  const items: SessionSummary[] = []
  let cursor: string | undefined = options.cursor?.trim() || undefined
  do {
    const page = await fetchSessions(botId, { ...options, cursor })
    items.push(...page.items)
    cursor = page.nextCursor?.trim() || undefined
  } while (cursor)
  return items
}

export async function createSession(botId: string, options?: string | CreateSessionOptions): Promise<SessionSummary> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')
  const body = typeof options === 'string'
    ? { title: options, channel_type: 'local' }
    : {
        title: options?.title ?? '',
        channel_type: 'local',
        type: options?.type,
        metadata: options?.metadata,
        acp_runtime_id: options?.acpRuntimeId?.trim() || undefined,
      }
  const { data } = await postBotsByBotIdSessions({
    path: { bot_id: id },
    body,
    throwOnError: true,
  })
  return data as SessionSummary
}

export async function updateSessionTitle(botId: string, sessionId: string, title: string): Promise<SessionSummary> {
  const { data } = await patchBotsByBotIdSessionsBySessionId({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    body: { title },
    throwOnError: true,
  })
  return data as SessionSummary
}

export async function updateSessionAgent(botId: string, sessionId: string, type: string, metadata: Record<string, unknown>): Promise<SessionSummary> {
  const { data } = await patchBotsByBotIdSessionsBySessionId({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    body: { type, metadata },
    throwOnError: true,
  })
  return data as SessionSummary
}

export async function ensureACPRuntime(botId: string, sessionId: string): Promise<AcpagentRuntimeStatus> {
  const { data } = await postBotsByBotIdSessionsBySessionIdAcpRuntime({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    throwOnError: true,
  })
  return data as AcpagentRuntimeStatus
}

export async function getACPRuntime(botId: string, sessionId: string): Promise<AcpagentRuntimeStatus> {
  const { data } = await getBotsByBotIdSessionsBySessionIdAcpRuntime({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    throwOnError: true,
  })
  return data as AcpagentRuntimeStatus
}

export async function setACPRuntimeModel(botId: string, sessionId: string, modelId: string): Promise<AcpagentRuntimeStatus> {
  const { data } = await patchBotsByBotIdSessionsBySessionIdAcpRuntimeModel({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    body: { model_id: modelId },
    throwOnError: true,
  })
  return data as AcpagentRuntimeStatus
}

export async function createACPRuntime(botId: string, options: CreateACPRuntimeOptions): Promise<AcpagentRuntimeStatus> {
  const { data } = await postBotsByBotIdAcpRuntimes({
    path: { bot_id: botId.trim() },
    body: {
      acp_agent_id: options.agentId.trim(),
      project_path: options.projectPath?.trim(),
    },
    throwOnError: true,
  })
  return data as AcpagentRuntimeStatus
}

export async function setACPRuntimeModelByID(botId: string, runtimeId: string, modelId: string): Promise<AcpagentRuntimeStatus> {
  const { data } = await patchBotsByBotIdAcpRuntimesByRuntimeIdModel({
    path: { bot_id: botId.trim(), runtime_id: runtimeId.trim() },
    // An empty model_id resets the runtime to the agent default model.
    body: { model_id: modelId.trim() },
    throwOnError: true,
  })
  return data as AcpagentRuntimeStatus
}

export async function closeACPRuntime(botId: string, runtimeId: string): Promise<void> {
  await deleteBotsByBotIdAcpRuntimesByRuntimeId({
    path: { bot_id: botId.trim(), runtime_id: runtimeId.trim() },
    throwOnError: true,
  })
}

export async function deleteSession(botId: string, sessionId: string): Promise<void> {
  await deleteBotsByBotIdSessionsBySessionId({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    throwOnError: true,
  })
}

export async function deleteAllMessages(botId: string): Promise<void> {
  await deleteBotsByBotIdMessages({
    path: { bot_id: botId },
    throwOnError: true,
  })
}
