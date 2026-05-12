import { beforeEach, describe, expect, it, vi } from 'vitest'
import { client } from '@memohai/sdk/client'
import { connectWebSocket } from './useChat.ws'

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  static instances: MockWebSocket[] = []

  readyState = MockWebSocket.CONNECTING
  sent: string[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null

  constructor(public readonly url: string) {
    MockWebSocket.instances.push(this)
  }

  send(payload: string) {
    this.sent.push(payload)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }

  open() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.()
  }
}

describe('useChat.ws', () => {
  beforeEach(() => {
    MockWebSocket.instances = []
    vi.unstubAllGlobals()
    client.setConfig({ baseUrl: '/api' })
    vi.stubGlobal('window', {
      location: {
        protocol: 'http:',
        host: 'localhost:8082',
      },
    })
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => ''),
    })
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  it('queues outbound messages until socket opens', () => {
    const onStreamEvent = vi.fn()
    const ws = connectWebSocket('bot-1', onStreamEvent)
    const socket = MockWebSocket.instances[0]

    expect(socket).toBeDefined()
    ws.send({ type: 'message', text: 'hello', session_id: 'session-1' })
    expect(socket.sent).toEqual([])

    socket.open()

    expect(socket.sent).toHaveLength(1)
    expect(JSON.parse(socket.sent[0]!)).toEqual({
      type: 'message',
      text: 'hello',
      session_id: 'session-1',
    })
  })

  it('uses the configured absolute API base URL', () => {
    client.setConfig({ baseUrl: 'http://127.0.0.1:18731' })
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => 'token with spaces'),
    })

    connectWebSocket('bot 1', vi.fn())

    expect(MockWebSocket.instances[0]?.url).toBe('ws://127.0.0.1:18731/bots/bot%201/web/ws?token=token%20with%20spaces')
  })
})
