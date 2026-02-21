import { client } from '@memoh/sdk/client'
import { postBotsByBotIdWebMessages } from '@memoh/sdk'
import type { ChannelAttachment, ChannelMessage } from '@memoh/sdk'
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
  chatId: string,
  options?: FetchMessagesOptions,
): Promise<Message[]> {
  if (botId.trim() !== chatId.trim()) {
    throw new Error('chat id must match bot id')
  }

  const query: Record<string, string> = {}
  query.limit = String(options?.limit ?? 30)
  if (options?.before?.trim()) {
    query.before = options.before.trim()
  }

  const { data } = await client.get({
    url: '/bots/{bot_id}/messages',
    path: { bot_id: botId },
    query,
    throwOnError: true,
  }) as { data: { items?: Message[] } }

  return data?.items ?? []
}

export async function sendLocalChannelMessage(
  botId: string,
  text: string,
  attachments?: ChatAttachment[],
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
  await postBotsByBotIdWebMessages({
    path: { bot_id: botId },
    body: { message: msg },
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

  const { data: body } = await client.get({
    url: '/bots/{bot_id}/web/stream',
    path: { bot_id: id },
    parseAs: 'stream',
    signal,
    throwOnError: true,
  }) as { data: ReadableStream<Uint8Array> }

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

  const { data: body } = await client.get({
    url: '/bots/{bot_id}/messages/events',
    path: { bot_id: id },
    query,
    parseAs: 'stream',
    signal,
    throwOnError: true,
  }) as { data: ReadableStream<Uint8Array> }

  if (!body) throw new Error('No response body')

  await readSSEStream(body, (payload) => {
    const parsed = parseStreamPayload(payload)
    if (!parsed || typeof parsed !== 'object' || !('type' in parsed)) return
    if (typeof parsed.type !== 'string' || !parsed.type.trim()) return
    onEvent(parsed as unknown as MessageStreamEvent)
  })
}
