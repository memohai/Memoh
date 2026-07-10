import { describe, expect, it, vi } from 'vitest'
import type {
  BotSessionActivityEvent,
  ChatWebSocket,
  SessionMessageStreamEvent,
  UIStreamEvent,
  WSClientMessage,
} from '@/composables/api/useChat'
import {
  createChatRealtimeController,
  type ChatRealtimeCallbacks,
  type ChatRealtimeTransport,
} from './realtime'

interface FakeRetryingStream {
  attempt: ((signal: AbortSignal) => Promise<void>) | null
  start: ReturnType<typeof vi.fn>
  stop: ReturnType<typeof vi.fn>
}

function createFakeRetryingStream(): FakeRetryingStream {
  const stream: FakeRetryingStream = {
    attempt: null,
    start: vi.fn((attempt: (signal: AbortSignal) => Promise<void>) => {
      stream.attempt = attempt
    }),
    stop: vi.fn(),
  }
  return stream
}

function createSocket(connected = true): ChatWebSocket & { send: ReturnType<typeof vi.fn>, abort: ReturnType<typeof vi.fn>, close: ReturnType<typeof vi.fn> } {
  return {
    connected,
    send: vi.fn(),
    abort: vi.fn(),
    close: vi.fn(),
    onOpen: null,
    onClose: null,
  }
}

function makeController() {
  const sockets: Array<{ botId: string, handler: (event: UIStreamEvent) => void, socket: ReturnType<typeof createSocket> }> = []
  const sessionRetry = createFakeRetryingStream()
  const activityRetry = createFakeRetryingStream()
  const retryingStreams = [sessionRetry, activityRetry]
  const sessionHandlers: Array<(event: SessionMessageStreamEvent) => void> = []
  const activityHandlers: Array<(event: BotSessionActivityEvent) => void> = []
  const callbacks: ChatRealtimeCallbacks = {
    onWebSocketEvent: vi.fn(),
    prepareSessionMessages: vi.fn().mockResolvedValue(undefined),
    onSessionMessageEvent: vi.fn(),
    onBotSessionsActivityEvent: vi.fn(),
  }
  const transport: ChatRealtimeTransport = {
    connectWebSocket: vi.fn((botId, handler) => {
      const socket = createSocket()
      sockets.push({ botId, handler, socket })
      return socket
    }),
    streamSessionMessageEvents: vi.fn(async (_botId, _sessionId, _signal, handler) => {
      sessionHandlers.push(handler)
    }),
    streamBotSessionsActivityEvents: vi.fn(async (_botId, _signal, handler) => {
      activityHandlers.push(handler)
    }),
    createRetryingStream: vi.fn(() => retryingStreams.shift()!),
  }
  return {
    callbacks,
    transport,
    sockets,
    sessionHandlers,
    activityHandlers,
    sessionRetry,
    activityRetry,
    controller: createChatRealtimeController(callbacks, transport),
  }
}

describe('chat realtime controller', () => {
  it('closes the previous websocket and rejects events from its generation', () => {
    const { controller, callbacks, sockets } = makeController()
    controller.startWebSocket('bot-1')
    const first = sockets[0]!
    controller.startWebSocket('bot-2')

    expect(first.socket.close).toHaveBeenCalledOnce()
    first.handler({ type: 'start', stream_id: 'stale' } as UIStreamEvent)
    sockets[1]!.handler({ type: 'start', stream_id: 'current' } as UIStreamEvent)

    expect(callbacks.onWebSocketEvent).toHaveBeenCalledOnce()
    expect(callbacks.onWebSocketEvent).toHaveBeenCalledWith('bot-2', expect.objectContaining({ stream_id: 'current' }))
  })

  it('does not expose a socket and sends only through the matching connected bot', () => {
    const { controller, sockets } = makeController()
    const message: WSClientMessage = { type: 'message', text: 'hello' }

    expect(controller.sendWebSocketMessage('bot-1', message)).toBe(true)
    expect(sockets[0]!.socket.send).toHaveBeenCalledWith(message)
    expect(controller.ensureWebSocketConnected('bot-1')).toBe(true)
    expect(sockets).toHaveLength(1)

    expect(controller.sendWebSocketMessage('bot-2', message)).toBe(true)
    expect(sockets[0]!.socket.close).toHaveBeenCalledOnce()
    expect(sockets[1]!.socket.send).toHaveBeenCalledWith(message)
  })

  it('aborts only a connected websocket for the matching bot', () => {
    const { controller, sockets } = makeController()
    controller.startWebSocket('bot-1')

    expect(controller.abortWebSocketStream('stream-1', 'bot-2')).toBe(false)
    expect(controller.abortWebSocketStream('stream-1', 'bot-1')).toBe(true)
    expect(sockets[0]!.socket.abort).toHaveBeenCalledOnce()
    expect(sockets[0]!.socket.abort).toHaveBeenCalledWith('stream-1')
  })

  it('prepares every session attempt and suppresses events after replacement', async () => {
    const { controller, callbacks, sessionRetry, sessionHandlers } = makeController()
    controller.startSessionMessagesStream('bot-1', 'session-1')
    await sessionRetry.attempt!(new AbortController().signal)
    expect(callbacks.prepareSessionMessages).toHaveBeenCalledWith('bot-1', 'session-1')
    const staleHandler = sessionHandlers[0]!

    controller.startSessionMessagesStream('bot-1', 'session-2')
    await sessionRetry.attempt!(new AbortController().signal)
    staleHandler({ type: 'ping' } as SessionMessageStreamEvent)
    sessionHandlers[1]!({ type: 'ping' } as SessionMessageStreamEvent)

    expect(callbacks.onSessionMessageEvent).toHaveBeenCalledOnce()
    expect(callbacks.onSessionMessageEvent).toHaveBeenCalledWith('bot-1', 'session-2', { type: 'ping' })
  })

  it('suppresses bot activity from a stopped generation', async () => {
    const { controller, callbacks, activityRetry, activityHandlers } = makeController()
    controller.startBotSessionsActivityStream('bot-1')
    await activityRetry.attempt!(new AbortController().signal)
    const staleHandler = activityHandlers[0]!

    controller.stopStreams()
    staleHandler({ type: 'ping' } as BotSessionActivityEvent)

    expect(callbacks.onBotSessionsActivityEvent).not.toHaveBeenCalled()
  })
})
