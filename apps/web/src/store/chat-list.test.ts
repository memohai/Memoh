import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

const sdkClient = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/composables/api/useChat', () => api)
vi.mock('@memohai/sdk/client', () => ({ client: sdkClient }))

function flushPromises() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

class TestCustomEvent<T = unknown> extends Event {
  detail: T

  constructor(type: string, init?: CustomEventInit<T>) {
    super(type, init)
    this.detail = init?.detail as T
  }
}

describe('chat-list store', () => {
  let streamHandler: UIStreamEventHandler | null
  let sendEvents: UIStreamEvent[]

  beforeEach(() => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', {
      addEventListener: windowTarget.addEventListener.bind(windowTarget),
      removeEventListener: windowTarget.removeEventListener.bind(windowTarget),
      dispatchEvent: windowTarget.dispatchEvent.bind(windowTarget),
    })
    vi.stubGlobal('CustomEvent', TestCustomEvent)

    setActivePinia(createPinia())
    streamHandler = null
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
        send: vi.fn(() => {
          for (const event of sendEvents) {
            onStreamEvent(event)
          }
        }),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    sdkClient.post.mockResolvedValue({ data: { id: 'run-1', status: 'queued' } })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
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
    streamHandler?.({ type: 'end' } as UIStreamEvent)
    await flushPromises()

    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'hello' })
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [{ type: 'error', content: 'model failed' }],
      streaming: false,
    })
  })

  it('starts an AI development run for /codex chat commands', async () => {
    const store = useChatStore()
    const navigationEvents: Array<{ runId: string; target: string }> = []
    const onNavigate = (event: Event) => {
      navigationEvents.push((event as CustomEvent<{ runId: string; target: string }>).detail)
    }
    window.addEventListener('memoh:ai-development-engine-open', onNavigate)

    await store.selectBot('bot-1')
    const ws = api.connectWebSocket.mock.results.at(-1)?.value
    const result = await store.sendMessage('/codex 修复登录问题')

    expect(result).toMatchObject({ ok: true })
    expect(sdkClient.post).toHaveBeenCalledWith(expect.objectContaining({
      url: '/ai-development-engine/runs',
      body: expect.objectContaining({
        prompt: '修复登录问题',
        workspacePath: 'F:\\Deep AI2026\\memoh',
        autoRepair: true,
      }),
    }))
    expect(ws?.send).not.toHaveBeenCalled()
    expect(navigationEvents).toEqual([{
      runId: 'run-1',
      target: '/settings/ai-development-engine?runId=run-1',
    }])
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [expect.objectContaining({
        type: 'text',
        content: expect.stringContaining('Codex CLI task started: run-1'),
      })],
    })
    window.removeEventListener('memoh:ai-development-engine-open', onNavigate)
  })
})
