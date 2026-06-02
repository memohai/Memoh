import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { MessageStreamEvent, UIStreamEvent, UIStreamEventHandler } from '@/composables/api/useChat'
import { REASONING_EFFORT_DISABLE } from '@/pages/bots/components/reasoning-effort'
import { useChatStore } from './chat-list'

const api = vi.hoisted(() => ({
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  fetchSessions: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  sendLocalChannelMessage: vi.fn(),
  updateSessionAgent: vi.fn(),
  ensureACPRuntime: vi.fn(),
  setACPRuntimeModel: vi.fn(),
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
  let messageEventsHandler: ((event: MessageStreamEvent) => void) | null
  let sendEvents: UIStreamEvent[]
  let sentWSMessages: Array<Record<string, unknown>>
  let lastStreamId = ''
  let lastSessionId = ''

  beforeEach(() => {
    setActivePinia(createPinia())
    streamHandler = null
    messageEventsHandler = null
    lastStreamId = ''
    lastSessionId = ''
    sentWSMessages = []
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
    api.updateSessionAgent.mockResolvedValue({
      id: 'session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data/app',
      },
    })
    api.ensureACPRuntime.mockResolvedValue({
      session_id: 'session-1',
      agent_id: 'codex',
      models: {
        current_model_id: 'gpt-5.1-codex',
        available_models: [{ id: 'gpt-5.1-codex', name: 'GPT-5.1 Codex' }],
      },
    })
    api.setACPRuntimeModel.mockResolvedValue({
      session_id: 'session-1',
      agent_id: 'codex',
      models: {
        current_model_id: 'gpt-5.1-codex-high',
        available_models: [{ id: 'gpt-5.1-codex-high', name: 'GPT-5.1 Codex High' }],
      },
    })
    api.fetchMessagesUI.mockResolvedValue([])
    api.streamMessageEvents.mockImplementation((_botId: string, signal: AbortSignal, onEvent: (event: MessageStreamEvent) => void) => new Promise<void>((resolve) => {
      messageEventsHandler = onEvent
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { stream_id?: string; session_id?: string }) => {
          sentWSMessages.push(message as Record<string, unknown>)
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

  it('creates ACP sessions without a placeholder title', async () => {
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data/app',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.createACPSession({
      agentId: 'codex',
      projectPath: '/data/app',
      projectMode: 'project',
    })

    expect(api.createSession).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      title: '',
      type: 'acp_agent',
    }))
  })

  it('stores ACP runtime models when starting an ACP session', async () => {
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data/app',
      },
    })
    api.ensureACPRuntime.mockResolvedValueOnce({
      session_id: 'acp-session-1',
      agent_id: 'codex',
      models: {
        current_model_id: 'gpt-5.1-codex',
        available_models: [{ id: 'gpt-5.1-codex', name: 'GPT-5.1 Codex' }],
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.createACPSession({
      agentId: 'codex',
      projectPath: '/data/app',
      projectMode: 'project',
      startRuntime: true,
    })

    const key = store.acpRuntimeKey('bot-1', 'acp-session-1')
    expect(api.ensureACPRuntime).toHaveBeenCalledTimes(1)
    expect(store.acpRuntimeStatuses[key]?.models?.current_model_id).toBe('gpt-5.1-codex')
    expect(store.acpRuntimePending[key]).toBeUndefined()
  })

  it('responds to user input over websocket and marks the block answered', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
    sendEvents = [{ type: 'agent_end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = {
      user_input_id: 'input-1',
      short_id: 4,
      status: 'pending',
      question: 'Which plan?',
      options: [{ id: 'a', label: 'Plan A', value: 'A' }],
      can_respond: true,
    }
    store.messages.push({
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'ask_user',
        input: { question: 'Which plan?' },
        tool_call_id: 'call-ask',
        toolCallId: 'call-ask',
        toolName: 'ask_user',
        running: false,
        done: true,
        result: null,
        userInput,
      }],
      timestamp: new Date().toISOString(),
      streaming: false,
    })

    await store.respondUserInput(userInput, { optionId: 'a', answer: 'A' })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'user_input_response',
      session_id: 'session-1',
      user_input_id: 'input-1',
      short_id: 4,
      option_id: 'a',
      answer: 'A',
      canceled: false,
    })
    const block = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput?.status).toBe('submitted')
      expect(block.userInput?.can_respond).toBe(false)
    }
  })

  it('refreshes pending user input after response stream failure', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = {
      user_input_id: 'input-1',
      short_id: 4,
      status: 'pending',
      question: 'Which plan?',
      options: [{ id: 'a', label: 'Plan A', value: 'A' }],
      can_respond: true,
    }
    store.messages.push({
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'ask_user',
        input: { question: 'Which plan?' },
        tool_call_id: 'call-ask',
        toolCallId: 'call-ask',
        toolName: 'ask_user',
        running: false,
        done: true,
        result: null,
        userInput,
      }],
      timestamp: new Date().toISOString(),
      streaming: false,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'ask_user',
        input: { question: 'Which plan?' },
        tool_call_id: 'call-ask',
        running: false,
        user_input: userInput,
      }],
      timestamp: new Date().toISOString(),
    }])

    await store.respondUserInput(userInput, { optionId: 'a', answer: 'A' })
    await flushPromises()
    await flushPromises()

    const block = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput?.status).toBe('pending')
      expect(block.userInput?.can_respond).toBe(true)
    }
  })

  it('deduplicates concurrent ACP runtime ensure calls', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'acp-session-1', bot_id: 'bot-1', title: '', type: 'acp_agent' },
    ])
    let resolveRuntime!: (value: unknown) => void
    api.ensureACPRuntime.mockReturnValueOnce(new Promise(resolve => {
      resolveRuntime = resolve
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const first = store.ensureACPRuntime('acp-session-1')
    const second = store.ensureACPRuntime('acp-session-1')
    expect(api.ensureACPRuntime).toHaveBeenCalledTimes(1)

    resolveRuntime({
      session_id: 'acp-session-1',
      agent_id: 'codex',
      models: {
        current_model_id: 'gpt-5.1-codex',
        available_models: [{ id: 'gpt-5.1-codex', name: 'GPT-5.1 Codex' }],
      },
    })
    await Promise.all([first, second])

    expect(api.ensureACPRuntime).toHaveBeenCalledTimes(1)
    expect(store.acpRuntimeStatuses[store.acpRuntimeKey('bot-1', 'acp-session-1')]?.models?.available_models).toHaveLength(1)
  })

  it('refreshes the session list when message events arrive for an unknown session', async () => {
    api.fetchSessions
      .mockResolvedValueOnce([
        { id: 'session-old', bot_id: 'bot-1', title: 'Old', type: 'chat' },
      ])
      .mockResolvedValueOnce([
        { id: 'session-new', bot_id: 'bot-1', title: 'New from channel', type: 'chat' },
        { id: 'session-old', bot_id: 'bot-1', title: 'Old', type: 'chat' },
      ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    expect(store.sessionId).toBe('session-old')

    messageEventsHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'message-1',
        bot_id: 'bot-1',
        session_id: 'session-new',
        role: 'user',
        created_at: '2026-06-02T10:00:00.000Z',
      },
    })
    await flushPromises()

    expect(api.fetchSessions).toHaveBeenCalledTimes(2)
    expect(store.sessions.map(session => session.id)).toEqual(['session-new', 'session-old'])
    expect(store.sessionId).toBe('session-old')
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

  it('sends disable as an explicit reasoning effort override', async () => {
    sendEvents = []
    const sent: Array<{ reasoning_effort?: string; stream_id?: string; session_id?: string }> = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { reasoning_effort?: string; stream_id?: string; session_id?: string }) => {
          sent.push(message)
          onStreamEvent({ type: 'start', stream_id: message.stream_id, session_id: message.session_id } as UIStreamEvent)
          onStreamEvent({ type: 'end', stream_id: message.stream_id, session_id: message.session_id } as UIStreamEvent)
        }),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.overrideReasoningEffort = REASONING_EFFORT_DISABLE
    const result = await store.sendMessage('hello')

    expect(result).toMatchObject({ ok: true })
    expect(sent).toHaveLength(1)
    expect(sent[0].reasoning_effort).toBe(REASONING_EFFORT_DISABLE)
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
