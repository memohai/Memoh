import { client } from '@memohai/sdk/client'
import {
  getBotsByBotIdMessagesLocate,
  getBotsByBotIdSessionsBySessionIdMessagesEvents,
  getBotsByBotIdSessionsEvents,
  postBotsByBotIdWebMessages,
} from '@memohai/sdk'
import type { ChannelAttachment, ChannelMessage } from '@memohai/sdk'
import type {
  BotSessionActivityEvent,
  ChatAttachment,
  FetchMessagesOptions,
  FetchMessagesUIResult,
  SessionMessageStreamEvent,
  UITurn,
} from './useChat.types'

export async function fetchMessagesUI(
  botId: string,
  sessionId: string,
  options?: FetchMessagesOptions,
): Promise<FetchMessagesUIResult> {
  const sid = sessionId.trim()
  if (!sid) throw new Error('session id is required')
  const response = await client.get({
    url: '/bots/{bot_id}/messages',
    path: { bot_id: botId },
    query: {
      session_id: sid,
      format: 'ui',
      limit: options?.limit ?? 30,
      ...(options?.includeGraph ? { include_graph: '1' } : {}),
      ...(options?.headTurnId?.trim() ? { head_turn_id: options.headTurnId.trim() } : {}),
      ...(options?.before?.trim() ? { before: options.before.trim() } : {}),
      ...(options?.beforeId?.trim() ? { before_id: options.beforeId.trim() } : {}),
    },
    throwOnError: true,
  })

  const data = response.data as FetchMessagesUIResult | undefined
  const result: FetchMessagesUIResult = {
    items: data?.items ?? [],
  }
  if (data && 'default_head_turn_id' in data) result.default_head_turn_id = data.default_head_turn_id
  if (data && 'head_turn_ids' in data) result.head_turn_ids = data.head_turn_ids ?? []
  if (data && 'nodes' in data) result.nodes = data.nodes ?? []
  return result
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
  options?: { headTurnId?: string },
): Promise<LocateMessageResult> {
  const response = await getBotsByBotIdMessagesLocate({
    path: { bot_id: botId },
    query: {
      session_id: sessionId,
      external_message_id: externalMessageId,
      before,
      after,
      ...(options?.headTurnId?.trim() ? { head_turn_id: options.headTurnId.trim() } : {}),
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
  baseHeadTurnId?: string
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
  const body: Record<string, unknown> = { message: msg }
  if (overrides?.modelId) body.model_id = overrides.modelId
  if (overrides?.reasoningEffort) body.reasoning_effort = overrides.reasoningEffort
  if (overrides?.baseHeadTurnId) body.base_head_turn_id = overrides.baseHeadTurnId
  await postBotsByBotIdWebMessages({
    path: { bot_id: botId },
    body: body as { message: ChannelMessage; model_id?: string; reasoning_effort?: string; base_head_turn_id?: string },
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
  headTurnId?: string,
): Promise<void> {
  const bid = botId.trim()
  const sid = sessionId.trim()
  if (!bid) throw new Error('bot id is required')
  if (!sid) throw new Error('session id is required')

  const { stream } = await getBotsByBotIdSessionsBySessionIdMessagesEvents({
    path: { bot_id: bid, session_id: sid },
    query: {
      ...(headTurnId?.trim() ? { head_turn_id: headTurnId.trim() } : {}),
    },
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
