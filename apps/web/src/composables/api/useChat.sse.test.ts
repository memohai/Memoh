import { describe, expect, it } from 'vitest'
import { readSSEStream } from './useChat.sse'

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
  it('reads SSE stream data lines and skips done marker', async () => {
    const body = streamFromString('data: {"type":"delta","delta":"A"}\n\n\ndata: [DONE]\n\n\ndata: plain text\n\n')
    const payloads: string[] = []
    await readSSEStream(body, (payload) => {
      payloads.push(payload)
    })
    expect(payloads).toEqual(['{"type":"delta","delta":"A"}', 'plain text'])
  })
})
