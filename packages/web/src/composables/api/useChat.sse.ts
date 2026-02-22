import type { StreamEvent } from './useChat.types'

const LEGACY_STREAM_EVENT_TYPES = new Set<string>([
  'text_start',
  'text_delta',
  'text_end',
  'reasoning_start',
  'reasoning_delta',
  'reasoning_end',
  'attachment_delta',
  'agent_start',
  'agent_end',
  'processing_started',
  'processing_completed',
  'processing_failed',
  'error',
])

export async function readSSEStream(
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

export function parseStreamPayload(payload: string): StreamEvent | null {
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
    return normalizeStreamEvent(current as Record<string, unknown>)
  }
  return null
}

export function normalizeStreamEvent(raw: Record<string, unknown>): StreamEvent | null {
  const eventType = String(raw.type ?? '').trim().toLowerCase()
  if (!eventType) return null

  const metadata = (raw.metadata && typeof raw.metadata === 'object')
    ? raw.metadata as Record<string, unknown>
    : undefined

  function withMeta(event: StreamEvent): StreamEvent {
    if (metadata) (event as Record<string, unknown>).metadata = metadata
    return event
  }

  if (LEGACY_STREAM_EVENT_TYPES.has(eventType)) {
    return raw as StreamEvent
  }

  switch (eventType) {
    case 'status': {
      const status = String(raw.status ?? '').trim().toLowerCase()
      if (status === 'started') return withMeta({ type: 'processing_started' })
      if (status === 'completed') return withMeta({ type: 'processing_completed' })
      if (status === 'failed') {
        const err = String(raw.error ?? '').trim()
        return withMeta({ type: 'processing_failed', error: err, message: err })
      }
      return null
    }
    case 'delta': {
      const delta = String(raw.delta ?? '')
      const phase = String(raw.phase ?? '').trim().toLowerCase()
      if (phase === 'reasoning') {
        return withMeta({ type: 'reasoning_delta', delta })
      }
      return withMeta({ type: 'text_delta', delta })
    }
    case 'phase_start': {
      const phase = String(raw.phase ?? '').trim().toLowerCase()
      if (phase === 'reasoning') return withMeta({ type: 'reasoning_start' })
      if (phase === 'text') return withMeta({ type: 'text_start' })
      return null
    }
    case 'phase_end': {
      const phase = String(raw.phase ?? '').trim().toLowerCase()
      if (phase === 'reasoning') return withMeta({ type: 'reasoning_end' })
      if (phase === 'text') return withMeta({ type: 'text_end' })
      return null
    }
    case 'tool_call_start':
    case 'tool_call_end': {
      const toolCall = (raw.tool_call && typeof raw.tool_call === 'object')
        ? raw.tool_call as Record<string, unknown>
        : {}
      return withMeta({
        type: eventType as StreamEvent['type'],
        toolCallId: String(toolCall.call_id ?? ''),
        toolName: String(toolCall.name ?? ''),
        input: toolCall.input,
        result: toolCall.result,
      })
    }
    case 'attachment': {
      const attachments = Array.isArray(raw.attachments)
        ? raw.attachments as Array<Record<string, unknown>>
        : []
      if (!attachments.length) return null
      return withMeta({ type: 'attachment_delta', attachments })
    }
    case 'final':
      return raw as StreamEvent
    case 'processing_started':
    case 'processing_completed':
    case 'agent_start':
    case 'agent_end':
      return withMeta({ type: eventType as StreamEvent['type'] })
    case 'processing_failed': {
      const err = String(raw.error ?? raw.message ?? '').trim()
      return withMeta({ type: 'processing_failed', error: err, message: err })
    }
    case 'error': {
      const err = String(raw.error ?? raw.message ?? 'Stream error').trim()
      return withMeta({ type: 'error', error: err, message: err })
    }
    default:
      return null
  }
}
