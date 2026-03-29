import { client } from '@memohai/sdk/client'
import { getBotsByBotIdMessages, postBotsByBotIdWebMessages } from '@memohai/sdk'
import type { ChannelAttachment, ChannelMessage } from '@memohai/sdk'
import type {
  ChatAttachment,
  FetchMessagesOptions,
  Message,
  MessageStreamEvent,
  StreamEventHandler,
} from './useChat.types'
import { parseStreamPayload, readSSEStream } from './useChat.sse'

export async function fetchMessages(
  botId: string,
  sessionId?: string,
  options?: FetchMessagesOptions,
): Promise<Message[]> {
  const { data } = await getBotsByBotIdMessages({
    path: { bot_id: botId },
    query: {
      limit: options?.limit ?? 30,
      ...(options?.before?.trim() ? { before: options.before.trim() } : {}),
      ...(sessionId?.trim() ? { session_id: sessionId.trim() } : {}),
    },
    throwOnError: true,
  })

  return (data as unknown as { items?: Message[] })?.items ?? []
}

export interface SendMessageOverrides {
  modelId?: string
  reasoningEffort?: string
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
  await postBotsByBotIdWebMessages({
    path: { bot_id: botId },
    body: body as { message: ChannelMessage; model_id?: string; reasoning_effort?: string },
    throwOnError: true,
  })
}

export async function streamLocalChannel(
  botId: string,
  signal: AbortSignal,
  onEvent: StreamEventHandler,
): Promise<void> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')

  const response = await client.get({
    url: '/bots/{bot_id}/web/stream',
    path: { bot_id: id },
    parseAs: 'stream',
    signal,
    throwOnError: true,
  })
  const body = response.data as ReadableStream<Uint8Array> | null

  if (!body) throw new Error('No response body')

  await readSSEStream(body, (payload) => {
    const event = parseStreamPayload(payload)
    if (event) onEvent(event)
  })
}

export async function streamMessageEvents(
  botId: string,
  signal: AbortSignal,
  onEvent: (event: MessageStreamEvent) => void,
  since?: string,
): Promise<void> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')

  const query: Record<string, string> = {}
  if (since?.trim()) query.since = since.trim()

  const response = await client.get({
    url: '/bots/{bot_id}/messages/events',
    path: { bot_id: id },
    query,
    parseAs: 'stream',
    signal,
    throwOnError: true,
  })
  const body = response.data as ReadableStream<Uint8Array> | null

  if (!body) throw new Error('No response body')

  await readSSEStream(body, (payload) => {
    try {
      const parsed = JSON.parse(payload)
      if (!parsed || typeof parsed !== 'object' || !('type' in parsed)) return
      if (typeof parsed.type !== 'string' || !parsed.type.trim()) return
      onEvent(parsed as MessageStreamEvent)
    } catch {
      // Ignore unparsable payloads
    }
  })
}
