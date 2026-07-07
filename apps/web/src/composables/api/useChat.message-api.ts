import { client } from '@memohai/sdk/client'
import {
  getBotsByBotIdMessages,
  getBotsByBotIdMessagesLocate,
  getBotsByBotIdSessionsBySessionIdMessagesEvents,
  getBotsByBotIdSessionsEvents,
  getBotsByBotIdSkillsCatalog,
  postBotsByBotIdQuickActionsExecute,
  postBotsByBotIdWebMessages,
} from '@memohai/sdk'
import type { ChannelAttachment, ChannelMessage, HandlersLocalChannelMessageRequest } from '@memohai/sdk'
import type {
  BotSessionActivityEvent,
  ChatAttachment,
  CommandEventResponse,
  FetchMessagesOptions,
  Message,
  RequestedSkillSelection,
  SessionMessageStreamEvent,
  UITurn,
} from './useChat.types'

export async function fetchMessages(
  botId: string,
  sessionId: string,
  options?: FetchMessagesOptions,
): Promise<Message[]> {
  const sid = sessionId.trim()
  if (!sid) throw new Error('session id is required')
  const { data } = await getBotsByBotIdMessages({
    path: { bot_id: botId },
    query: {
      session_id: sid,
      limit: options?.limit ?? 30,
      ...(options?.before?.trim() ? { before: options.before.trim() } : {}),
    },
    throwOnError: true,
  })

  return (data as unknown as { items?: Message[] })?.items ?? []
}

export async function fetchMessagesUI(
  botId: string,
  sessionId: string,
  options?: FetchMessagesOptions,
): Promise<UITurn[]> {
  const sid = sessionId.trim()
  if (!sid) throw new Error('session id is required')
  const response = await client.get({
    url: '/bots/{bot_id}/messages',
    path: { bot_id: botId },
    query: {
      session_id: sid,
      limit: options?.limit ?? 30,
      format: 'ui',
      ...(options?.beforeMessageId?.trim() ? { before_message_id: options.beforeMessageId.trim() } : {}),
      ...(options?.before?.trim() ? { before: options.before.trim() } : {}),
    },
    throwOnError: true,
  })

  return (response.data as { items?: UITurn[] } | undefined)?.items ?? []
}

export interface LocateMessageResult {
  items: UITurn[]
  target_id?: string
  target_external_message_id?: string
}

export async function locateMessageUI(
  botId: string,
  sessionId: string,
  externalMessageId: string,
  before = 30,
  after = 30,
): Promise<LocateMessageResult> {
  const response = await getBotsByBotIdMessagesLocate({
    path: { bot_id: botId },
    query: {
      session_id: sessionId,
      external_message_id: externalMessageId,
      before,
      after,
    },
    throwOnError: true,
  })

  const data = response.data as unknown as LocateMessageResult | undefined
  return {
    items: data?.items ?? [],
    target_id: data?.target_id,
    target_external_message_id: data?.target_external_message_id,
  }
}

export interface SendMessageOverrides {
  modelId?: string
  reasoningEffort?: string
}

function isCommandEvent(value: unknown): value is CommandEventResponse {
  if (!value || typeof value !== 'object') return false
  const type = String((value as { type?: unknown }).type ?? '').trim()
  return type === 'command_result' || type === 'command_error'
}

export async function fetchSafeSkillCatalog(botId: string): Promise<RequestedSkillSelection[]> {
  const bid = botId.trim()
  if (!bid) return []
  const { data } = await getBotsByBotIdSkillsCatalog({
    path: { bot_id: bid },
    throwOnError: true,
  })
  return (data?.skills ?? []).flatMap((item): RequestedSkillSelection[] => {
    const name = item.name?.trim()
    if (!name) return []
    return [{
      name,
      display_name: item.display_name?.trim() || undefined,
      description: item.description?.trim() || undefined,
      source_kind: item.source_kind?.trim() || undefined,
      state: item.state?.trim() || undefined,
    }]
  })
}

export async function executeQuickAction(
  botId: string,
  actionId: string,
  options: { invocationId?: string; composerScope?: string; sessionId?: string; skillActivationAllowed?: boolean } = {},
): Promise<CommandEventResponse> {
  const bid = botId.trim()
  const aid = actionId.trim()
  if (!bid) throw new Error('bot id is required')
  if (!aid) throw new Error('action id is required')
  const { data } = await postBotsByBotIdQuickActionsExecute({
    path: { bot_id: bid },
    body: {
      action_id: aid,
      invocation_id: options.invocationId?.trim() || undefined,
      composer_scope: options.composerScope?.trim() || undefined,
      session_id: options.sessionId?.trim() || undefined,
      params: options.skillActivationAllowed === false
        ? { skill_activation_allowed: false }
        : undefined,
    },
    throwOnError: true,
  })
  if (isCommandEvent(data)) return data
  throw new Error('invalid quick action response')
}

export async function sendLocalChannelMessage(
  botId: string,
  text: string,
  attachments?: ChatAttachment[],
  overrides?: SendMessageOverrides,
): Promise<void> {
  const msg: ChannelMessage = {}
  const trimmedText = text.trim()
  if (trimmedText) {
    msg.text = trimmedText
  }
  if (attachments?.length) {
    msg.attachments = attachments.map((item): ChannelAttachment => ({
      type: item.type as ChannelAttachment['type'],
      base64: item.base64,
      mime: item.mime ?? '',
      name: item.name ?? '',
    }))
  }
  const body: HandlersLocalChannelMessageRequest = { message: msg }
  if (overrides?.modelId) body.model_id = overrides.modelId
  if (overrides?.reasoningEffort) body.reasoning_effort = overrides.reasoningEffort
  await postBotsByBotIdWebMessages({
    path: { bot_id: botId.trim() },
    body,
    throwOnError: true,
  })
}

// The SDK's `sse.get` yields parsed `data` payloads from the async generator.
// Wrap each subscription so callers receive typed events and a promise that
// resolves when the stream ends (signal abort or server close).
async function consumeSSE<T extends { type: string }>(
  stream: AsyncGenerator<unknown>,
  isEvent: (value: unknown) => value is T,
  onEvent: (event: T) => void,
): Promise<void> {
  for await (const payload of stream) {
    if (isEvent(payload)) onEvent(payload)
  }
}

function isTypedEvent(value: unknown): value is { type: string } {
  return !!value && typeof value === 'object' && 'type' in value
    && typeof (value as { type: unknown }).type === 'string'
    && (value as { type: string }).type.trim().length > 0
}

export async function streamSessionMessageEvents(
  botId: string,
  sessionId: string,
  signal: AbortSignal,
  onEvent: (event: SessionMessageStreamEvent) => void,
): Promise<void> {
  const bid = botId.trim()
  const sid = sessionId.trim()
  if (!bid) throw new Error('bot id is required')
  if (!sid) throw new Error('session id is required')

  const { stream } = await getBotsByBotIdSessionsBySessionIdMessagesEvents({
    path: { bot_id: bid, session_id: sid },
    signal,
    // The SDK's built-in reconnect would race the store's per-session
    // lifecycle; we drive retries from the caller via useRetryingStream.
    sseMaxRetryAttempts: 1,
  })

  await consumeSSE(stream, (value): value is SessionMessageStreamEvent => isTypedEvent(value), onEvent)
}

export async function streamBotSessionsActivityEvents(
  botId: string,
  signal: AbortSignal,
  onEvent: (event: BotSessionActivityEvent) => void,
): Promise<void> {
  const bid = botId.trim()
  if (!bid) throw new Error('bot id is required')

  const { stream } = await getBotsByBotIdSessionsEvents({
    path: { bot_id: bid },
    signal,
    sseMaxRetryAttempts: 1,
  })

  await consumeSSE(stream, (value): value is BotSessionActivityEvent => isTypedEvent(value), onEvent)
}
