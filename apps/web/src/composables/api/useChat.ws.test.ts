import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { client } from '@memohai/sdk/client'
import {
  reduceSessionRuntimeDelta,
  reduceSessionRuntimeSnapshot,
  type SessionRuntimeDeltaEvent,
  type SessionRuntimeReducerState,
} from '@memohai/sdk/session-runtime'
import { richActiveRunContractFixture, runtimeRecoveryContractFixture } from '../../store/runtime-contract-fixtures.test-support'
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
  readonly url: string

  constructor(url: string) {
    this.url = url
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

  emit(payload: unknown) {
    this.onmessage?.({ data: JSON.stringify(payload) })
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

  afterEach(() => {
    vi.useRealTimers()
  })

  it('rejects outbound messages until the socket opens', () => {
    const onStreamEvent = vi.fn()
    const ws = connectWebSocket('bot-1', onStreamEvent)
    const socket = MockWebSocket.instances[0]!

    expect(socket).toBeDefined()
    expect(() => ws.send({ type: 'message', stream_id: 'stream-1', text: 'hello', session_id: 'session-1' }))
      .toThrow('WebSocket is not connected')
    expect(socket.sent).toEqual([])

    socket.open()
    ws.send({ type: 'message', stream_id: 'stream-1', text: 'hello', session_id: 'session-1' })
    expect(JSON.parse(socket.sent[0]!)).toEqual({
      type: 'message',
      stream_id: 'stream-1',
      text: 'hello',
      session_id: 'session-1',
    })
  })

  it('sends targeted abort messages', () => {
    const ws = connectWebSocket('bot-1', vi.fn())
    const socket = MockWebSocket.instances[0]!
    socket.open()

    ws.abort('stream-1', 'session-1', 'generation-1')

    expect(JSON.parse(socket.sent[0]!)).toEqual({
      type: 'abort',
      stream_id: 'stream-1',
      session_id: 'session-1',
      generation: 'generation-1',
    })
  })

  it('sends an abort before the runtime generation is observed', () => {
    const ws = connectWebSocket('bot-1', vi.fn())
    const socket = MockWebSocket.instances[0]!
    socket.open()

    ws.abort('stream-early', 'session-1')

    expect(JSON.parse(socket.sent[0]!)).toEqual({
      type: 'abort',
      stream_id: 'stream-early',
      session_id: 'session-1',
    })
  })

  it('sends runtime subscription commands through the existing bot websocket', () => {
    const ws = connectWebSocket('bot-1', vi.fn())
    const socket = MockWebSocket.instances[0]!
    socket.open()

    ws.send({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    })
    ws.send({
      type: 'runtime_unsubscribe',
      session_id: 'session-1',
    })

    expect(JSON.parse(socket.sent[0]!)).toEqual({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    })
    expect(JSON.parse(socket.sent[1]!)).toEqual({
      type: 'runtime_unsubscribe',
      session_id: 'session-1',
    })
  })

  it('lets scripted server events drive each websocket client independently', () => {
    const firstHandler = vi.fn()
    const secondHandler = vi.fn()
    connectWebSocket('bot-1', firstHandler)
    connectWebSocket('bot-1', secondHandler)
    const first = MockWebSocket.instances[0]!
    const second = MockWebSocket.instances[1]!
    first.open()
    second.open()

    const firstSnapshot = structuredClone(richActiveRunContractFixture.runtime_snapshot)
    const secondSnapshot = structuredClone(runtimeRecoveryContractFixture.runtime_snapshot)
    first.emit(firstSnapshot)
    second.emit(secondSnapshot)
    const runtimeDelta = {
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-1',
      stream_id: 'stream-a',
      seq: 3,
      delta: {
        message_appends: [{ id: 0, type: 'text', content: ' next' }],
      },
    } satisfies SessionRuntimeDeltaEvent
    first.emit(runtimeDelta)

    expect(firstHandler).toHaveBeenCalledTimes(2)
    expect(firstHandler).toHaveBeenCalledWith(firstSnapshot)
    expect(firstHandler).toHaveBeenCalledWith(runtimeDelta)
    expect(secondHandler).toHaveBeenCalledTimes(1)
    expect(secondHandler).toHaveBeenCalledWith(secondSnapshot)
  })

  it('parses the Go-generated runtime contract through the websocket transport', () => {
    const handler = vi.fn()
    connectWebSocket('bot-1', handler)
    const socket = MockWebSocket.instances[0]!
    socket.open()

    const events = structuredClone([
      ...richActiveRunContractFixture.runtime_stream,
      ...(richActiveRunContractFixture.runtime_terminal_stream ?? []),
    ])
    for (const event of events) socket.emit(event)

    expect(handler.mock.calls.map(call => call[0])).toEqual(events)
    const snapshot = events.find(event => event.type === 'runtime_snapshot')
    expect(snapshot).toBeDefined()
    if (snapshot?.type !== 'runtime_snapshot' || !snapshot.snapshot) throw new Error('missing runtime snapshot')
    expect(snapshot.snapshot.current_run_view).not.toHaveProperty('owner_id')
    expect(snapshot.snapshot.current_run_view).not.toHaveProperty('owner_lease_expires_at')
  })

  it('closes the websocket when a malformed runtime envelope cannot be scoped safely', () => {
    const handler = vi.fn()
    const ws = connectWebSocket('bot-1', handler)
    const socket = MockWebSocket.instances[0]!
    socket.open()

    socket.emit({ type: 'runtime_delta', session_id: 'session-1', epoch: 'epoch-1', stream_id: 'stream-1', seq: 2, delta: {} })

    expect(handler).not.toHaveBeenCalled()
    expect(socket.readyState).toBe(MockWebSocket.CLOSED)
    ws.close()
  })

  it('feeds the Go-generated wire stream through transport parsing and the runtime reducer', () => {
    let state: SessionRuntimeReducerState = {}
    connectWebSocket('bot-1', (event) => {
      if (event.type === 'runtime_snapshot' && event.snapshot) {
        const reduction = reduceSessionRuntimeSnapshot(state, event.snapshot, event.seq, event.epoch)
        if (reduction.kind === 'applied') state = reduction.state
      } else if (event.type === 'runtime_delta') {
        const reduction = reduceSessionRuntimeDelta(state, event, event.bot_id, event.session_id)
        if (reduction.kind === 'applied') state = reduction.state
      }
    })
    const socket = MockWebSocket.instances[0]!
    socket.open()

    for (const event of structuredClone([
      ...richActiveRunContractFixture.runtime_stream,
      ...(richActiveRunContractFixture.runtime_terminal_stream ?? []),
    ])) socket.emit(event)

    expect(state.snapshot?.current_run_view).toMatchObject({
      stream_id: 'stream-rich',
      status: 'completed',
    })
    expect(state.snapshot?.current_run_view?.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'text', content: 'I will check the current state.' }),
      expect.objectContaining({ type: 'reasoning', content: 'I need to inspect the workspace.' }),
    ]))
  })

  it('parses every Go-generated recovery frame through the websocket transport', () => {
    const handler = vi.fn()
    const ws = connectWebSocket('bot-1', handler)
    const socket = MockWebSocket.instances[0]!
    socket.open()
    const fixture = structuredClone(runtimeRecoveryContractFixture)
    const events = [
      fixture.runtime_snapshot,
      fixture.gap_delta,
      fixture.delayed_delta,
      fixture.runtime_checkpoint,
      fixture.post_recovery_delta,
    ]

    for (const event of events) socket.emit(event)

    expect(handler.mock.calls.map(call => call[0])).toEqual(events)
    expect(socket.readyState).toBe(MockWebSocket.OPEN)
    ws.close()
  })

  it('does not swallow stream handler errors after parsing a valid event', () => {
    const failure = new Error('store reducer failed')
    connectWebSocket('bot-1', () => {
      throw failure
    })
    const socket = MockWebSocket.instances[0]!
    socket.open()

    expect(() => socket.emit(structuredClone(richActiveRunContractFixture.runtime_snapshot))).toThrow(failure)
  })

  it('reconnects without replaying commands sent while disconnected', () => {
    vi.useFakeTimers()
    const ws = connectWebSocket('bot-1', vi.fn())
    const first = MockWebSocket.instances[0]!
    first.open()

    first.close()
    expect(ws.connected).toBe(false)

    expect(() => ws.send({
      type: 'message',
      stream_id: 'stream-after-reconnect',
      session_id: 'session-1',
      text: 'resume',
    })).toThrow('WebSocket is not connected')

    vi.advanceTimersByTime(1000)
    const second = MockWebSocket.instances[1]!
    expect(second).toBeDefined()
    expect(second.sent).toEqual([])

    second.open()
    expect(second.sent).toEqual([])
  })

  it('rejects abort commands while reconnecting', () => {
    vi.useFakeTimers()
    const ws = connectWebSocket('bot-1', vi.fn())
    const first = MockWebSocket.instances[0]!
    first.open()

    first.close()
    expect(ws.connected).toBe(false)

    expect(() => ws.abort('stream-after-disconnect', 'session-1', 'generation-1')).toThrow('WebSocket is not connected')
    expect(first.sent).toEqual([])

    vi.advanceTimersByTime(1000)
    const second = MockWebSocket.instances[1]!
    second.open()
    expect(second.sent).toEqual([])
  })

  it('uses the configured absolute API base URL', () => {
    client.setConfig({ baseUrl: 'http://127.0.0.1:18080' })
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => 'token with spaces'),
    })

    connectWebSocket('bot 1', vi.fn())

    expect(MockWebSocket.instances[0]?.url).toBe('ws://127.0.0.1:18080/bots/bot%201/web/ws?token=token%20with%20spaces')
  })
})
