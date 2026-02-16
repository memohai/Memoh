import { client } from '@memoh/sdk/client'
import { getBots } from '@memoh/sdk'
import type { BotsBot } from '@memoh/sdk'

// ---- Types ----

export type Bot = BotsBot

export interface ChatSummary {
  id: string
  bot_id: string
  kind: string
  title?: string
  created_at?: string
  updated_at?: string
  access_mode?: 'participant' | 'channel_identity_observed'
  participant_role?: string
  last_observed_at?: string
}

export interface MessageAsset {
  asset_id: string
  role: string
  ordinal: number
  media_type: string
  mime: string
  size_bytes: number
  storage_key: string
  original_name?: string
  width?: number
  height?: number
  duration_ms?: number
}

export interface Message {
  id: string
  bot_id: string
  route_id?: string
  sender_channel_identity_id?: string
  sender_user_id?: string
  sender_display_name?: string
  sender_avatar_url?: string
  platform?: string
  external_message_id?: string
  source_reply_to_message_id?: string
  role: string
  content?: unknown
  metadata?: Record<string, unknown>
  assets?: MessageAsset[]
  created_at?: string
}

export interface StreamEvent {
  type?:
    | 'text_start' | 'text_delta' | 'text_end'
    | 'reasoning_start' | 'reasoning_delta' | 'reasoning_end'
    | 'tool_call_start' | 'tool_call_end'
    | 'attachment_delta'
    | 'agent_start' | 'agent_end'
    | 'processing_started' | 'processing_completed' | 'processing_failed'
    | 'error'
  delta?: string
  toolCallId?: string
  toolName?: string
  input?: unknown
  result?: unknown
  attachments?: Array<Record<string, unknown>>
  error?: string
  message?: string
  [key: string]: unknown
}

export type StreamEventHandler = (event: StreamEvent) => void

export interface MessageStreamEvent {
  type: string
  bot_id?: string
  message?: Message
}

export interface FetchMessagesOptions {
  limit?: number
  before?: string
}

// ---- SSE Parsing Utility ----

/**
 * Read an SSE stream line-by-line, calling onData for each `data:` payload.
 * Handles standard SSE format (events separated by double newlines).
 */
async function readSSEStream(
  body: ReadableStream<Uint8Array>,
  onData: (payload: string) => void,
): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      const chunks = buffer.split('\n\n')
      buffer = chunks.pop() ?? ''

      for (const chunk of chunks) {
        for (const line of chunk.split('\n')) {
          if (!line.startsWith('data:')) continue
          const payload = line.replace(/^data:\s*/, '').trim()
          if (payload && payload !== '[DONE]') onData(payload)
        }
      }
    }

    // Flush remaining buffer
    if (buffer.trim()) {
      for (const line of buffer.split('\n')) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const payload = trimmed.replace(/^data:\s*/, '').trim()
        if (payload && payload !== '[DONE]') onData(payload)
      }
    }
  } finally {
    reader.releaseLock()
  }
}

/**
 * Parse a raw SSE payload string into a StreamEvent.
 * Handles double-encoded JSON and plain text deltas.
 */
function parseStreamPayload(payload: string): StreamEvent | null {
  let current: unknown = payload
  for (let i = 0; i < 2; i += 1) {
    if (typeof current !== 'string') break
    const raw = current.trim()
    if (!raw || raw === '[DONE]') return null
    try {
      current = JSON.parse(raw)
    } catch {
      return { type: 'text_delta', delta: raw } as StreamEvent
    }
  }

  if (typeof current === 'string') {
    return { type: 'text_delta', delta: current.trim() } as StreamEvent
  }
  if (current && typeof current === 'object') {
    return current as StreamEvent
  }
  return null
}

// ---- Bot API ----

export async function fetchBots(): Promise<Bot[]> {
  const { data } = await getBots({ throwOnError: true })
  return data?.items ?? []
}

// ---- Chat helpers (chatId === botId, no backend call needed) ----

export async function fetchChats(botId: string): Promise<ChatSummary[]> {
  const id = botId.trim()
  if (!id) return []
  return [{ id, bot_id: id, kind: 'bot' }]
}

export async function createChat(botId: string): Promise<ChatSummary> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')
  return { id, bot_id: id, kind: 'bot' }
}

export async function deleteChat(botId: string, chatId: string): Promise<void> {
  if (botId.trim() !== chatId.trim()) {
    throw new Error('chat id must match bot id')
  }
  await client.delete({
    url: '/bots/{bot_id}/messages',
    path: { bot_id: botId },
    throwOnError: true,
  })
}

// ---- Message API ----

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

// ---- Stream API ----

/**
 * Stream a chat message via SSE. Sends parsed StreamEvents to onEvent callback.
 * Returns an abort function.
 */
export interface ChatAttachment {
  type: string
  base64: string
  mime?: string
  name?: string
}

export function streamMessage(
  botId: string,
  _chatId: string,
  text: string,
  onEvent: StreamEventHandler,
  onDone: () => void,
  onError: (err: Error) => void,
  attachments?: ChatAttachment[],
): () => void {
  const controller = new AbortController()

  ;(async () => {
    try {
      const reqBody: Record<string, unknown> = { query: text, current_channel: 'web', channels: ['web'] }
      if (attachments?.length) {
        reqBody.attachments = attachments
      }
      const { data: body } = await client.post({
        url: '/bots/{bot_id}/messages/stream',
        path: { bot_id: botId },
        body: reqBody,
        parseAs: 'stream',
        signal: controller.signal,
        throwOnError: true,
      }) as { data: ReadableStream<Uint8Array> }

      if (!body) {
        onError(new Error('No response body'))
        return
      }

      await readSSEStream(body, (payload) => {
        const event = parseStreamPayload(payload)
        if (event) onEvent(event)
      })

      onDone()
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        onError(err instanceof Error ? err : new Error(String(err)))
      }
    }
  })()

  return () => controller.abort()
}

/**
 * Listen for real-time message events via SSE (long-polling).
 */
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

// ---- Content extraction utilities ----

/**
 * Extract tool-call content parts from a stored assistant message.
 * The DB stores the full ModelMessage JSON in the content field.
 * Tool calls are stored as content parts with type "tool-call"
 * (Vercel AI SDK format).
 */
export function extractToolCalls(
  message: Message,
): Array<{ id: string; name: string; input: unknown }> {
  const parts = getContentParts(message)
  if (!parts) return []
  return parts
    .filter((p) => String((p as Record<string, unknown>).type ?? '').toLowerCase() === 'tool-call')
    .map((p) => {
      const part = p as Record<string, unknown>
      return {
        id: String(part.toolCallId ?? ''),
        name: String(part.toolName ?? ''),
        input: part.input ?? null,
      }
    })
}

/**
 * Extract tool_call_id from a stored tool-role message (first match).
 */
export function extractToolCallId(message: Message): string {
  const raw = message.content
  if (!raw || typeof raw !== 'object') return ''
  const obj = raw as Record<string, unknown>
  if (typeof obj.tool_call_id === 'string') return obj.tool_call_id.trim()
  const parts = getContentParts(message)
  if (!parts) return ''
  for (const p of parts) {
    const part = p as Record<string, unknown>
    if (String(part.type ?? '').toLowerCase() === 'tool-result' && typeof part.toolCallId === 'string') {
      return part.toolCallId.trim()
    }
  }
  return ''
}

/**
 * Extract ALL tool results from a tool-role message.
 * A single tool message can contain multiple tool-result parts.
 */
export function extractAllToolResults(
  message: Message,
): Array<{ toolCallId: string; output: unknown }> {
  const parts = getContentParts(message)
  if (!parts) return []
  return parts
    .filter((p) => String((p as Record<string, unknown>).type ?? '').toLowerCase() === 'tool-result')
    .map((p) => {
      const part = p as Record<string, unknown>
      return {
        toolCallId: String(part.toolCallId ?? ''),
        output: part.output ?? null,
      }
    })
}

/**
 * Get the inner content parts array from a message.
 * The DB content is the full ModelMessage JSON: { role, content: [...parts] }
 */
function getContentParts(message: Message): unknown[] | null {
  const raw = message.content
  if (!raw || typeof raw !== 'object') return null
  const obj = raw as Record<string, unknown>
  if (Array.isArray(obj.content)) return obj.content
  if (typeof obj.content === 'string') {
    try {
      const parsed = JSON.parse(obj.content)
      if (Array.isArray(parsed)) return parsed
    } catch { /* ignore */ }
  }
  return null
}

export function extractMessageText(message: Message): string {
  const raw = message.content
  if (!raw) return ''

  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return extractTextFromContent(parsed?.content ?? parsed).trim()
    } catch {
      return raw.trim()
    }
  }

  if (typeof raw === 'object') {
    const obj = raw as Record<string, unknown>
    if ('content' in obj && obj.content !== undefined && obj.content !== null) {
      return extractTextFromContent(obj.content).trim()
    }
    return extractTextFromContent(raw).trim()
  }

  return extractTextFromContent(raw).trim()
}

export function extractTextFromContent(content: unknown): string {
  if (typeof content === 'string') return content.trim()

  if (Array.isArray(content)) {
    return content
      .map((part) => {
        if (!part || typeof part !== 'object') return ''
        const value = part as Record<string, unknown>
        const partType = String(value.type ?? '').toLowerCase()
        if (partType === 'text' && typeof value.text === 'string') return value.text.trim()
        if (partType === 'link' && typeof value.url === 'string') return value.url.trim()
        if (partType === 'emoji' && typeof value.emoji === 'string') return value.emoji.trim()
        if (typeof value.text === 'string') return value.text.trim()
        return ''
      })
      .filter(Boolean)
      .join('\n')
      .trim()
  }

  if (content && typeof content === 'object') {
    const value = content as Record<string, unknown>
    if (typeof value.text === 'string') return value.text.trim()
  }

  return ''
}
