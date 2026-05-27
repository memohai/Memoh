import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { UIStreamEvent, UIStreamEventHandler } from '@/composables/api/useChat'
import { useChatStore } from './chat-list'

const api = vi.hoisted(() => ({
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  fetchSessions: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  sendLocalChannelMessage: vi.fn(),
  streamMessageEvents: vi.fn(),
  connectWebSocket: vi.fn(),
  locateMessageUI: vi.fn(),
}))

vi.mock('@/composables/api/useChat', () => api)

function flushPromises() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

describe('chat-list store', () => {
  let streamHandler: UIStreamEventHandler | null
  let sendEvents: UIStreamEvent[]
  let lastStreamId = ''
  let lastSessionId = ''

  beforeEach(() => {
    setActivePinia(createPinia())
    streamHandler = null
    lastStreamId = ''
    lastSessionId = ''
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'error', message: 'model failed' } as UIStreamEvent,
    ]
    vi.clearAllMocks()

    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot' },
    ])
    api.fetchSessions.mockResolvedValue([])
    api.createSession.mockResolvedValue({
      id: 'session-1',
      bot_id: 'bot-1',
      title: 'New session',
      type: 'chat',
    })
    api.fetchMessagesUI.mockResolvedValue([])
    api.streamMessageEvents.mockImplementation((_botId: string, signal: AbortSignal) => new Promise<void>((resolve) => {
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { stream_id?: string; session_id?: string }) => {
          lastStreamId = message.stream_id ?? ''
          lastSessionId = message.session_id ?? ''
          for (const event of sendEvents) {
            onStreamEvent({
              ...event,
              stream_id: lastStreamId,
              session_id: lastSessionId,
            } as UIStreamEvent)
          }
        }),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
  })

  it('returns startup stream errors to the composer when no assistant output exists', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('hello')

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      error: 'model failed',
      restoreInput: 'hello',
    })
    expect(store.messages).toHaveLength(0)
    expect(store.startupSendFailure).toMatchObject({
      botId: 'bot-1',
      sessionId: 'session-1',
      error: 'model failed',
      restoreInput: 'hello',
    })
  })

  it('renders stream errors in the chat transcript after assistant output starts', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: { id: 0, type: 'text', content: 'partial response' },
      } as UIStreamEvent,
      { type: 'error', message: 'model failed' } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('hello')

    expect(result).toMatchObject({ ok: false, stage: 'stream', error: 'model failed' })
    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'hello' })
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [
        { type: 'text', content: 'partial response' },
        { type: 'error', content: 'model failed' },
      ],
      streaming: false,
    })
    expect(store.startupSendFailure).toBeNull()
  })

  it('keeps an ephemeral error visible when refresh returns only the persisted user turn', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: { id: 0, type: 'text', content: 'partial response' },
      } as UIStreamEvent,
      { type: 'error', message: 'model failed' } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.sendMessage('hello')

    api.fetchMessagesUI.mockResolvedValueOnce([{
      role: 'user',
      id: 'server-user-1',
      text: 'hello',
      timestamp: '2026-05-17T08:00:00.000Z',
    }])
    streamHandler?.({ type: 'end', stream_id: lastStreamId, session_id: lastSessionId } as UIStreamEvent)
    await flushPromises()

    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'hello' })
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [{ type: 'error', content: 'model failed' }],
      streaming: false,
    })
  })

  it('routes interleaved websocket events by stream id', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
    ])
    api.fetchMessagesUI.mockResolvedValue([])

    const sent: Array<{ stream_id?: string; session_id?: string }> = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { stream_id?: string; session_id?: string }) => {
          sent.push(message)
        }),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })

    const store = useChatStore()

    await store.selectBot('bot-1')
    const first = store.sendMessage('first')
    await flushPromises()

    await store.selectSession('session-b')
    const second = store.sendMessage('second')
    await flushPromises()

    const streamA = sent.find(item => item.session_id === 'session-a')?.stream_id
    const streamB = sent.find(item => item.session_id === 'session-b')?.stream_id
    expect(streamA).toBeTruthy()
    expect(streamB).toBeTruthy()
    expect(store.isSessionStreaming('session-a')).toBe(true)
    expect(store.isSessionStreaming('session-b')).toBe(true)

    streamHandler?.({
      type: 'message',
      stream_id: streamA,
      session_id: 'session-a',
      data: { id: 0, type: 'text', content: 'answer A' },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: streamB,
      session_id: 'session-b',
      data: { id: 0, type: 'text', content: 'answer B' },
    } as UIStreamEvent)
    expect(store.sessionId).toBe('session-b')
    expect(store.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({
        role: 'assistant',
        messages: [expect.objectContaining({ type: 'text', content: 'answer B' })],
      }),
    ]))

    await store.selectSession('session-a')
    expect(store.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({
        role: 'assistant',
        messages: [expect.objectContaining({ type: 'text', content: 'answer A' })],
      }),
    ]))

    streamHandler?.({ type: 'end', stream_id: streamA, session_id: 'session-a' } as UIStreamEvent)
    streamHandler?.({ type: 'end', stream_id: streamB, session_id: 'session-b' } as UIStreamEvent)
    await first
    await second
  })
})
