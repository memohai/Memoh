import { describe, expect, it } from 'vitest'
import { normalizeStreamEvent, parseStreamPayload, readSSEStream } from './useChat.sse'

function streamFromString(content: string): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(encoder.encode(content))
      controller.close()
    },
  })
}

describe('useChat.sse', () => {
  it('parses plain text payload as text delta', () => {
    const event = parseStreamPayload('hello world')
    expect(event).toEqual({ type: 'text_delta', delta: 'hello world' })
  })

  it('parses double-encoded json payload', () => {
    const payload = JSON.stringify(JSON.stringify({ type: 'delta', phase: 'reasoning', delta: 'trace' }))
    const event = parseStreamPayload(payload)
    expect(event).toEqual({ type: 'reasoning_delta', delta: 'trace' })
  })

  it('returns null for done payload', () => {
    expect(parseStreamPayload('[DONE]')).toBeNull()
    expect(parseStreamPayload('   ')).toBeNull()
  })

  it('normalizes status event and preserves metadata', () => {
    const event = normalizeStreamEvent({
      type: 'status',
      status: 'failed',
      error: 'boom',
      metadata: { source_channel: 'telegram' },
    })
    expect(event).toEqual({
      type: 'processing_failed',
      error: 'boom',
      message: 'boom',
      metadata: { source_channel: 'telegram' },
    })
  })

  it('normalizes tool call start payload', () => {
    const event = normalizeStreamEvent({
      type: 'tool_call_start',
      tool_call: { call_id: 'call-1', name: 'search', input: { q: 'memoh' } },
    })
    expect(event).toEqual({
      type: 'tool_call_start',
      toolCallId: 'call-1',
      toolName: 'search',
      input: { q: 'memoh' },
      result: undefined,
    })
  })

  it('passes through final event', () => {
    const raw = { type: 'final', message: { id: 'm1' }, metadata: { role: 'assistant' } }
    expect(normalizeStreamEvent(raw)).toEqual(raw)
  })

  it('reads SSE stream data lines and skips done marker', async () => {
    const body = streamFromString('data: {"type":"delta","delta":"A"}\n\n\ndata: [DONE]\n\n\ndata: plain text\n\n')
    const payloads: string[] = []
    await readSSEStream(body, (payload) => {
      payloads.push(payload)
    })
    expect(payloads).toEqual(['{"type":"delta","delta":"A"}', 'plain text'])
  })
})
