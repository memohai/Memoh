import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { BotSessionActivityEvent, SessionMessageStreamEvent, UIStreamEvent, UIStreamEventHandler, UIUserInput } from '@/composables/api/useChat'
import { REASONING_EFFORT_DISABLE } from '@/pages/bots/components/reasoning-effort'
import { useChatStore } from './chat-list'

const api = vi.hoisted(() => ({
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  fetchSession: vi.fn(),
  fetchSessions: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  sendLocalChannelMessage: vi.fn(),
  updateSessionAgent: vi.fn(),
  ensureACPRuntime: vi.fn(),
  createACPRuntime: vi.fn(),
  setACPRuntimeModel: vi.fn(),
  setACPRuntimeModelByID: vi.fn(),
  closeACPRuntime: vi.fn(),
  streamSessionMessageEvents: vi.fn(),
  streamBotSessionsActivityEvents: vi.fn(),
  connectWebSocket: vi.fn(),
  locateMessageUI: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  error: vi.fn(),
}))

vi.mock('@/composables/api/useChat', () => api)
vi.mock('vue-sonner', () => ({ toast }))
vi.mock('@memohai/ui', async (importOriginal) => {
  const original = await importOriginal<typeof import('@memohai/ui')>()
  return { ...original, toast }
})

function flushPromises() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

function singleSelectUserInput(id = 'input-1'): UIUserInput {
  return {
    user_input_id: id,
    short_id: id === 'input-1' ? 4 : 5,
    status: 'pending',
    questions: [{
      id: 'q1',
      text: id === 'input-1' ? 'Which plan?' : 'Second question?',
      kind: 'single_select',
      options: [
        { id: 'q1.o1', label: id === 'input-1' ? 'Plan A' : 'Plan B' },
        { id: 'q1.o2', label: id === 'input-1' ? 'Plan B' : 'Plan C' },
      ],
    }],
    can_respond: true,
  }
}

function askUserTurn(userInput: UIUserInput, toolCallId = 'call-ask') {
  return {
    id: 'assistant-1',
    role: 'assistant' as const,
    messages: [{
      id: 1,
      type: 'tool' as const,
      name: 'ask_user',
      input: { questions: [{ text: userInput.questions?.[0]?.text ?? 'Question?', kind: 'single_select' }] },
      tool_call_id: toolCallId,
      toolCallId,
      toolName: 'ask_user',
      running: false,
      done: true,
      result: null,
      userInput,
    }],
    timestamp: new Date().toISOString(),
    streaming: false,
  }
}

describe('chat-list store', () => {
  let streamHandler: UIStreamEventHandler | null
  // Captured but not driven by any test body yet; keep the capture so future
  // tests can simulate per-session SSE events without rewiring the mock.
  let _sessionMessageHandler: ((event: SessionMessageStreamEvent) => void) | null
  let sessionsActivityHandler: ((event: BotSessionActivityEvent) => void) | null
  let sendEvents: UIStreamEvent[]
  let sentWSMessages: Array<Record<string, unknown>>
  let lastStreamId = ''
  let lastSessionId = ''

  beforeEach(() => {
    setActivePinia(createPinia())
    streamHandler = null
    _sessionMessageHandler = null
    sessionsActivityHandler = null
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
    api.fetchSessions.mockResolvedValue({ items: [], nextCursor: null })
    api.fetchSession.mockResolvedValue({
      id: 'session-unknown',
      bot_id: 'bot-1',
      title: 'Unknown session',
      type: 'chat',
    })
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
    api.createACPRuntime.mockResolvedValue({
      runtime_id: 'rt_warm',
      agent_id: 'codex',
      state: 'idle',
      default_model_id: 'gpt-5.1-codex',
      models: {
        current_model_id: 'gpt-5.1-codex',
        available_models: [
          { id: 'gpt-5.1-codex', name: 'GPT-5.1 Codex' },
          { id: 'gpt-5.1-codex-high', name: 'GPT-5.1 Codex High' },
        ],
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
    api.setACPRuntimeModelByID.mockResolvedValue({
      runtime_id: 'rt_warm',
      agent_id: 'codex',
      state: 'idle',
      default_model_id: 'gpt-5.1-codex',
      models: {
        current_model_id: 'gpt-5.1-codex-high',
        available_models: [{ id: 'gpt-5.1-codex-high', name: 'GPT-5.1 Codex High' }],
      },
    })
    api.closeACPRuntime.mockResolvedValue(undefined)
    api.fetchMessagesUI.mockResolvedValue([])
    api.streamSessionMessageEvents.mockImplementation((_botId: string, _sessionId: string, signal: AbortSignal, onEvent: (event: SessionMessageStreamEvent) => void) => new Promise<void>((resolve) => {
      _sessionMessageHandler = onEvent
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
    api.streamBotSessionsActivityEvents.mockImplementation((_botId: string, signal: AbortSignal, onEvent: (event: BotSessionActivityEvent) => void) => new Promise<void>((resolve) => {
      sessionsActivityHandler = onEvent
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

  it('selects the first ready bot during initialization when none is selected', async () => {
    api.fetchBots.mockResolvedValueOnce([
      { id: 'bot-creating', status: 'creating', name: 'Creating' },
      { id: 'bot-ready', status: 'active', name: 'Ready' },
    ])

    const store = useChatStore()

    await store.initialize()

    expect(store.currentBotId).toBe('bot-ready')
    expect(api.fetchSessions).toHaveBeenCalledWith('bot-ready')
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

  it('merges ACP approval tool messages into the existing tool block by call id', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          input: { command: 'make test' },
          tool_call_id: 'mcp-http-call-1',
          running: true,
        },
      } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1000007,
          type: 'tool',
          name: 'exec',
          input: { command: 'make test' },
          tool_call_id: 'mcp-http-call-1',
          running: false,
          approval: {
            approval_id: 'approval-1',
            short_id: 7,
            status: 'pending',
            can_approve: true,
          },
        },
      } as UIStreamEvent,
      { type: 'error', message: 'stop after visible output' } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('run command')

    expect(result).toMatchObject({ ok: false, stage: 'stream' })
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (!assistant || assistant.role !== 'assistant') {
      throw new Error('assistant turn was not created')
    }
    expect(assistant.messages.filter(block => block.type === 'tool')).toHaveLength(1)
    const tool = assistant.messages.find(block => block.type === 'tool')
    expect(tool).toMatchObject({
      id: 1,
      type: 'tool',
      toolCallId: 'mcp-http-call-1',
      running: false,
      approval: {
        approval_id: 'approval-1',
        status: 'pending',
      },
    })
  })

  it('allows responding to ACP approval while the original stream is still active', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          input: { command: 'pwd' },
          tool_call_id: 'call-pwd',
          running: false,
          approval: {
            approval_id: 'approval-pwd',
            short_id: 9,
            status: 'pending',
            can_approve: true,
          },
        },
      } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('run pwd')
    await flushPromises()
    await flushPromises()

    expect(store.streaming).toBe(true)
    const initialMessageCount = store.messages.length
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    if (!assistant || assistant.role !== 'assistant') {
      throw new Error('assistant turn was not created')
    }
    const tool = assistant.messages.find(block => block.type === 'tool')
    expect(tool).toMatchObject({
      toolCallId: 'call-pwd',
      approval: {
        approval_id: 'approval-pwd',
        status: 'pending',
      },
    })

    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'end' } as UIStreamEvent,
    ]
    await store.respondToolApproval(tool!.approval!, 'approve')
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'tool_approval_response',
      session_id: 'session-1',
      approval_id: 'approval-pwd',
      decision: 'approve',
    })
    expect(store.messages).toHaveLength(initialMessageCount)
    const updatedAssistant = store.messages.find(turn => turn.role === 'assistant')
    if (!updatedAssistant || updatedAssistant.role !== 'assistant') {
      throw new Error('assistant turn was not found after approval')
    }
    const updatedTool = updatedAssistant.messages.find(block => block.type === 'tool')
    expect(updatedTool?.approval).toMatchObject({
      approval_id: 'approval-pwd',
      status: 'approved',
      can_approve: false,
    })

    const originalStreamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({
      type: 'message',
      stream_id: originalStreamId,
      session_id: 'session-1',
      data: {
        id: 1,
        type: 'tool',
        name: 'exec',
        input: { command: 'pwd' },
        tool_call_id: 'call-pwd',
        running: false,
        approval: {
          approval_id: 'approval-pwd',
          short_id: 9,
          status: 'pending',
          can_approve: true,
        },
      },
    } as UIStreamEvent)
    const staleAssistant = store.messages.find(turn => turn.role === 'assistant')
    if (!staleAssistant || staleAssistant.role !== 'assistant') {
      throw new Error('assistant turn was not found after stale pending')
    }
    const staleTool = staleAssistant.messages.find(block => block.type === 'tool')
    expect(staleTool?.approval).toMatchObject({
      approval_id: 'approval-pwd',
      status: 'approved',
      can_approve: false,
    })

    streamHandler?.({ type: 'end', stream_id: originalStreamId, session_id: 'session-1' } as UIStreamEvent)
    await expect(sendPromise).resolves.toMatchObject({ ok: true })
  })

  it('rolls the optimistic approval back to pending when the response stream errors', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          input: { command: 'pwd' },
          tool_call_id: 'call-pwd',
          running: false,
          approval: {
            approval_id: 'approval-pwd',
            short_id: 9,
            status: 'pending',
            can_approve: true,
          },
        },
      } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('run pwd')
    await flushPromises()
    await flushPromises()

    const assistant = store.messages.find(turn => turn.role === 'assistant')
    if (!assistant || assistant.role !== 'assistant') {
      throw new Error('assistant turn was not created')
    }
    const tool = assistant.messages.find(block => block.type === 'tool')

    // The approval response stream fails before the server applies the decision.
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'error', message: 'approval failed' } as UIStreamEvent,
    ]
    await store.respondToolApproval(tool!.approval!, 'approve')
    await flushPromises()

    expect(toast.error).toHaveBeenCalledWith('approval failed')
    const rolledBackTool = assistant.messages.find(block => block.type === 'tool')
    expect(rolledBackTool?.approval).toMatchObject({
      approval_id: 'approval-pwd',
      status: 'pending',
      can_approve: true,
    })

    // The user can retry, and the retry goes through.
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'end' } as UIStreamEvent,
    ]
    const retried = await store.respondToolApproval(rolledBackTool!.approval!, 'approve')
    await flushPromises()

    expect(retried).toBe(true)
    const approvalResponses = sentWSMessages.filter(message => message.type === 'tool_approval_response')
    expect(approvalResponses).toHaveLength(2)
    const retriedTool = assistant.messages.find(block => block.type === 'tool')
    expect(retriedTool?.approval).toMatchObject({
      status: 'approved',
      can_approve: false,
    })

    const originalStreamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({ type: 'end', stream_id: originalStreamId, session_id: 'session-1' } as UIStreamEvent)
    await expect(sendPromise).resolves.toMatchObject({ ok: true })
  })

  it('sends each ACP approval response only once while the response is in flight', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          input: { command: 'pwd' },
          tool_call_id: 'call-pwd',
          running: false,
          approval: {
            approval_id: 'approval-pwd',
            short_id: 9,
            status: 'pending',
            can_approve: true,
          },
        },
      } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('run pwd')
    await flushPromises()
    await flushPromises()

    const assistant = store.messages.find(turn => turn.role === 'assistant')
    if (!assistant || assistant.role !== 'assistant') {
      throw new Error('assistant turn was not created')
    }
    const tool = assistant.messages.find(block => block.type === 'tool')
    if (!tool?.approval) {
      throw new Error('tool approval was not created')
    }
    const approval = tool.approval

    sendEvents = [{ type: 'start' } as UIStreamEvent]
    await store.respondToolApproval(approval, 'approve')
    await store.respondToolApproval(approval, 'approve')
    await flushPromises()

    const approvalResponses = sentWSMessages.filter(message => message.type === 'tool_approval_response')
    expect(approvalResponses).toHaveLength(1)

    const approvalStreamId = approvalResponses[0]?.stream_id as string
    streamHandler?.({ type: 'end', stream_id: approvalStreamId, session_id: 'session-1' } as UIStreamEvent)
    const originalStreamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({ type: 'end', stream_id: originalStreamId, session_id: 'session-1' } as UIStreamEvent)
    await expect(sendPromise).resolves.toMatchObject({ ok: true })
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

  it('defaults new ACP sessions to the workspace root project', async () => {
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.createACPSession({
      agentId: 'codex',
    })

    expect(api.createSession).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    }))
  })

  it('defers ACP session creation until the first message is sent', async () => {
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })

    expect(api.createSession).not.toHaveBeenCalled()
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'codex',
      project_path: '/data',
      acp_project_mode: 'project',
    })

    const result = await store.sendMessage('hello codex')

    expect(result.ok).toBe(true)
    expect(api.createSession).toHaveBeenCalledTimes(1)
    expect(api.createSession).toHaveBeenCalledWith('bot-1', expect.objectContaining({
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    }))
    expect(store.sessionId).toBe('acp-session-1')
    expect(store.pendingACPSessionMetadata).toBeNull()
    expect(sentWSMessages[0]).toMatchObject({
      session_id: 'acp-session-1',
      text: 'hello codex',
    })
  })

  it('creates a warm runtime for the staged agent and binds it on first send', async () => {
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()

    // The runtime ID is server generated; the client never invents one.
    expect(api.createACPRuntime).toHaveBeenCalledWith('bot-1', expect.objectContaining({
      agentId: 'codex',
      projectPath: '/data',
    }))
    expect(store.pendingACPRuntimeId).toBe('rt_warm')
    expect(store.pendingACPRuntimeStatus?.models?.available_models).toHaveLength(2)

    await store.setPendingACPModel('gpt-5.1-codex-high')
    expect(store.pendingACPModelId).toBe('gpt-5.1-codex-high')
    expect(api.setACPRuntimeModelByID).toHaveBeenCalledWith('bot-1', 'rt_warm', 'gpt-5.1-codex-high')

    // Binding rides on session creation; ensure sees the warm runtime with
    // the chosen model, so no model fix-up and no runtime close happen.
    api.ensureACPRuntime.mockResolvedValueOnce({
      runtime_id: 'rt_warm',
      session_id: 'acp-session-1',
      agent_id: 'codex',
      state: 'idle',
      models: { current_model_id: 'gpt-5.1-codex-high', available_models: [] },
    })
    const result = await store.sendMessage('hello codex')

    expect(result.ok).toBe(true)
    expect(api.createSession).toHaveBeenCalledTimes(1)
    expect(api.createSession).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      type: 'acp_agent',
      acpRuntimeId: 'rt_warm',
    }))
    expect(api.setACPRuntimeModel).not.toHaveBeenCalled()
    expect(api.closeACPRuntime).not.toHaveBeenCalled()
    expect(sentWSMessages[0]).toMatchObject({
      session_id: 'acp-session-1',
      text: 'hello codex',
    })
  })

  it('re-applies the staged model when the bind fell back to a cold start', async () => {
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    await store.setPendingACPModel('gpt-5.1-codex-high')

    // The warm runtime was reaped before the send: the session-scoped ensure
    // cold starts with the default model, so the staged model is re-applied.
    api.ensureACPRuntime.mockResolvedValueOnce({
      runtime_id: 'rt_cold',
      session_id: 'acp-session-1',
      agent_id: 'codex',
      state: 'idle',
      models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
    })
    const result = await store.sendMessage('hello codex')

    expect(result.ok).toBe(true)
    expect(api.setACPRuntimeModel).toHaveBeenCalledWith('bot-1', 'acp-session-1', 'gpt-5.1-codex-high')
    expect(sentWSMessages[0]).toMatchObject({
      session_id: 'acp-session-1',
      text: 'hello codex',
    })
  })

  it('resets the warm runtime model when default is re-selected before first send', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()

    await store.setPendingACPModel('gpt-5.1-codex-high')
    expect(api.setACPRuntimeModelByID).toHaveBeenLastCalledWith('bot-1', 'rt_warm', 'gpt-5.1-codex-high')

    // Back to default: the server resets the runtime to the agent default
    // (empty model id), so the warm runtime always matches the picker.
    await store.setPendingACPModel('')
    expect(store.pendingACPModelId).toBe('')
    expect(api.setACPRuntimeModelByID).toHaveBeenLastCalledWith('bot-1', 'rt_warm', '')
  })

  it('does not touch the warm runtime when default is selected without a prior pick', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()

    await store.setPendingACPModel('')

    expect(store.pendingACPModelId).toBe('')
    expect(api.setACPRuntimeModelByID).not.toHaveBeenCalled()
  })

  it('starts a new runtime when the agent changes while a create is in flight', async () => {
    let resolveFirst!: (value: unknown) => void
    api.createACPRuntime
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirst = resolve
      }))
      .mockResolvedValueOnce({
        runtime_id: 'rt_claude',
        agent_id: 'claude-code',
        state: 'idle',
        models: { current_model_id: 'claude-default', available_models: [] },
      })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const first = store.ensurePendingACPRuntime()

    // Switching agents mid-create must NOT reuse the codex create promise:
    // the new staging starts its own runtime immediately.
    store.stageACPSession({ agentId: 'claude-code' })
    const second = await store.ensurePendingACPRuntime()

    expect(api.createACPRuntime).toHaveBeenCalledTimes(2)
    expect(api.createACPRuntime).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      agentId: 'claude-code',
    }))
    expect(store.pendingACPRuntimeId).toBe('rt_claude')
    expect(second?.runtime_id).toBe('rt_claude')

    // The late codex runtime is discarded, never adopted into claude staging.
    resolveFirst({
      runtime_id: 'rt_codex',
      agent_id: 'codex',
      state: 'idle',
      models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
    })
    await first
    expect(api.closeACPRuntime).toHaveBeenCalledWith('bot-1', 'rt_codex')
    expect(store.pendingACPRuntimeId).toBe('rt_claude')
  })

  it('starts a new runtime when the project changes while a create is in flight', async () => {
    let resolveFirst!: (value: unknown) => void
    api.createACPRuntime
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirst = resolve
      }))
      .mockResolvedValueOnce({
        runtime_id: 'rt_other-project',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const first = store.ensurePendingACPRuntime()

    store.stageACPSession({ agentId: 'codex', projectPath: '/data/other' })
    await store.ensurePendingACPRuntime()

    expect(api.createACPRuntime).toHaveBeenCalledTimes(2)
    expect(api.createACPRuntime).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      projectPath: '/data/other',
    }))
    expect(store.pendingACPRuntimeId).toBe('rt_other-project')

    // The old project's runtime must not be accepted into the new staging.
    resolveFirst({
      runtime_id: 'rt_old-project',
      agent_id: 'codex',
      state: 'idle',
      models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
    })
    await first
    expect(api.closeACPRuntime).toHaveBeenCalledWith('bot-1', 'rt_old-project')
    expect(store.pendingACPRuntimeId).toBe('rt_other-project')
  })

  it('ignores a stale create failure after staging changes', async () => {
    let rejectFirst!: (error: unknown) => void
    api.createACPRuntime
      .mockImplementationOnce(() => new Promise((_, reject) => {
        rejectFirst = reject
      }))
      .mockResolvedValueOnce({
        runtime_id: 'rt_claude',
        agent_id: 'claude-code',
        state: 'idle',
        models: { current_model_id: 'claude-default', available_models: [] },
      })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const first = store.ensurePendingACPRuntime()

    store.stageACPSession({ agentId: 'claude-code' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_claude')

    rejectFirst({ message: 'codex create failed' })
    await expect(first).resolves.toBeUndefined()
    expect(store.pendingACPRuntimeId).toBe('rt_claude')
  })

  it('abandons a stale model heal when staging changes mid-flight', async () => {
    api.createACPRuntime
      .mockResolvedValueOnce({
        runtime_id: 'rt_warm',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
      .mockResolvedValueOnce({
        runtime_id: 'rt_claude',
        agent_id: 'claude-code',
        state: 'idle',
        models: { current_model_id: 'claude-default', available_models: [] },
      })
    let rejectPatch!: (error: unknown) => void
    api.setACPRuntimeModelByID.mockImplementationOnce(() => new Promise((_, reject) => {
      rejectPatch = reject
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_warm')

    // The model PATCH hangs; the user switches agents meanwhile.
    const pick = store.setPendingACPModel('gpt-5.1-codex-high')
    store.stageACPSession({ agentId: 'claude-code' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_claude')

    // The old PATCH now fails with runtime-not-found: the heal must detect
    // the staging switch and exit silently — no recreate for the old
    // staging, no model PATCH against the claude runtime, no revert.
    rejectPatch({ message: 'runtime not found' })
    await pick

    expect(api.createACPRuntime).toHaveBeenCalledTimes(2)
    expect(api.setACPRuntimeModelByID).toHaveBeenCalledTimes(1)
    expect(store.pendingACPRuntimeId).toBe('rt_claude')
    expect(store.pendingACPModelId).toBe('')
  })

  it('abandons a stale model heal when the same agent is re-staged mid-flight', async () => {
    api.createACPRuntime
      .mockResolvedValueOnce({
        runtime_id: 'rt_warm',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
      .mockResolvedValueOnce({
        runtime_id: 'rt_new',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
    let rejectPatch!: (error: unknown) => void
    api.setACPRuntimeModelByID.mockImplementationOnce(() => new Promise((_, reject) => {
      rejectPatch = reject
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()

    // ABA: pick hangs → user leaves ACP → re-stages the SAME agent. The
    // staging key matches again, but the model intent was reset, so the
    // late heal must not push the abandoned model onto the new runtime.
    const pick = store.setPendingACPModel('gpt-5.1-codex-high')
    store.clearPendingACPSession()
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_new')

    rejectPatch({ message: 'runtime not found' })
    await pick

    expect(api.setACPRuntimeModelByID).toHaveBeenCalledTimes(1)
    expect(store.pendingACPModelId).toBe('')
    expect(store.pendingACPRuntimeId).toBe('rt_new')
  })

  it('reverts the pending model if runtime creation fails for the current staging', async () => {
    api.createACPRuntime.mockRejectedValueOnce({ message: 'runtime create failed' })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })

    await expect(store.setPendingACPModel('gpt-5.1-codex-high')).rejects.toMatchObject({
      message: 'runtime create failed',
    })
    expect(store.pendingACPModelId).toBe('')
    expect(store.pendingACPRuntimeId).toBe('')
  })

  it('recreates a reaped staged runtime when a model is picked after idling', async () => {
    api.createACPRuntime
      .mockResolvedValueOnce({
        runtime_id: 'rt_warm',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
      .mockResolvedValueOnce({
        runtime_id: 'rt_fresh',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
      })
    api.setACPRuntimeModelByID
      .mockRejectedValueOnce({ message: 'runtime not found' })
      .mockResolvedValueOnce({
        runtime_id: 'rt_fresh',
        agent_id: 'codex',
        state: 'idle',
        models: { current_model_id: 'gpt-5.1-codex-high', available_models: [] },
      })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_warm')

    // rt_warm was idle-reaped server-side; the pick must heal transparently.
    await store.setPendingACPModel('gpt-5.1-codex-high')

    expect(api.createACPRuntime).toHaveBeenCalledTimes(2)
    expect(api.setACPRuntimeModelByID).toHaveBeenLastCalledWith('bot-1', 'rt_fresh', 'gpt-5.1-codex-high')
    expect(store.pendingACPRuntimeId).toBe('rt_fresh')
    expect(store.pendingACPModelId).toBe('gpt-5.1-codex-high')
  })

  it('discards a staged runtime that finishes starting after the agent changed', async () => {
    let resolveCreate!: (value: unknown) => void
    api.createACPRuntime.mockImplementationOnce(() => new Promise((resolve) => {
      resolveCreate = resolve
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const ensurePromise = store.ensurePendingACPRuntime()

    // The user clears the staged agent while the runtime is still starting.
    store.clearPendingACPSession()
    resolveCreate({
      runtime_id: 'rt_late',
      agent_id: 'codex',
      state: 'idle',
      models: { current_model_id: 'gpt-5.1-codex', available_models: [] },
    })
    await ensurePromise

    // The late runtime is closed instead of being adopted into empty staging.
    expect(store.pendingACPRuntimeId).toBe('')
    expect(api.closeACPRuntime).toHaveBeenCalledWith('bot-1', 'rt_late')
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
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'agent_end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'user_input_response',
      session_id: 'session-1',
      user_input_id: 'input-1',
      short_id: 4,
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
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

  it('cancels user input over websocket and marks the block canceled', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'agent_end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))

    await store.respondUserInput(userInput, { canceled: true, reason: 'user_canceled' })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'user_input_response',
      session_id: 'session-1',
      user_input_id: 'input-1',
      short_id: 4,
      canceled: true,
      reason: 'user_canceled',
    })
    const block = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput?.status).toBe('canceled')
      expect(block.userInput?.can_respond).toBe(false)
    }
  })

  it('does not optimistically submit user input while websocket is disconnected', async () => {
    api.connectWebSocket.mockImplementationOnce((_botId: string, _onStreamEvent: UIStreamEventHandler) => ({
      get connected() {
        return false
      },
      send: vi.fn((message: Record<string, unknown>) => {
        sentWSMessages.push(message)
      }),
      abort: vi.fn(),
      close: vi.fn(),
      onOpen: null,
      onClose: null,
    }))
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    await flushPromises()

    expect(sentWSMessages).toHaveLength(0)
    expect(toast.error).toHaveBeenCalledWith('Connection lost. Reconnect and try again.')
    expect(store.messages).toHaveLength(1)
    const block = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput?.status).toBe('pending')
      expect(block.userInput?.can_respond).toBe(true)
    }
  })

  it('responds to multi-select and text questions over websocket', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = {
      user_input_id: 'input-1',
      short_id: 4,
      status: 'pending',
      questions: [
        {
          id: 'q1',
          text: 'Which plans?',
          kind: 'multi_select' as const,
          options: [
            { id: 'q1.o1', label: 'Plan A' },
            { id: 'q1.o2', label: 'Plan B' },
          ],
        },
        {
          id: 'q2',
          text: 'Anything else?',
          kind: 'text' as const,
        },
      ],
      can_respond: true,
    }
    store.messages.push({
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'ask_user',
        input: { questions: [{ text: 'Which plans?', kind: 'multi_select' }] },
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

    await store.respondUserInput(userInput, {
      answers: [
        { question_id: 'q1', option_ids: ['q1.o1', 'q1.o2'] },
        { question_id: 'q2', text: 'nothing else' },
      ],
    })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'user_input_response',
      session_id: 'session-1',
      user_input_id: 'input-1',
      answers: [
        { question_id: 'q1', option_ids: ['q1.o1', 'q1.o2'] },
        { question_id: 'q2', text: 'nothing else' },
      ],
      canceled: false,
    })
  })

  it('does not refresh a user input response stream while the original session stream is still active', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    api.fetchMessagesUI.mockClear()

    streamHandler?.({ type: 'start', stream_id: 'main-stream', session_id: 'session-1' } as UIStreamEvent)
    expect(store.isSessionStreaming('session-1')).toBe(true)

    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    await flushPromises()

    expect(api.fetchMessagesUI).not.toHaveBeenCalled()

    streamHandler?.({
      type: 'message',
      stream_id: 'main-stream',
      session_id: 'session-1',
      data: {
        id: 2,
        type: 'tool',
        name: 'ask_user',
        input: { questions: [{ text: 'Second question?', kind: 'single_select' }] },
        tool_call_id: 'call-ask-2',
        running: false,
        user_input: singleSelectUserInput('input-2'),
      },
    } as UIStreamEvent)

    const hasSecondPendingInput = store.messages.some(message => message.role === 'assistant' && message.messages.some((block) => {
      return block.type === 'tool' && block.userInput?.user_input_id === 'input-2' && block.userInput.status === 'pending'
    }))
    expect(hasSecondPendingInput).toBe(true)
    expect(api.fetchMessagesUI).not.toHaveBeenCalled()

    streamHandler?.({ type: 'end', stream_id: 'main-stream', session_id: 'session-1' } as UIStreamEvent)
    await flushPromises()

    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(1)
  })

  it('stamps session updated_at from the server message time, not the client clock or a reorder', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat', updated_at: '2026-01-01T00:00:00Z' },
      { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat', updated_at: '2026-01-02T00:00:00Z' },
    ], nextCursor: null })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    sessionsActivityHandler?.({
      type: 'session_touched',
      session_id: 'session-2',
      updated_at: '2026-01-03T00:00:00Z',
    })
    await flushPromises()

    const updated = store.sessions.find(session => session.id === 'session-2')
    expect(updated?.updated_at).toBe('2026-01-03T00:00:00Z')
    expect(store.sessions.map(session => session.id)).toEqual(['session-1', 'session-2'])
  })

  it('refreshes pending user input after response stream failure', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'ask_user',
        input: { questions: [{ text: 'Which plan?', kind: 'single_select' }] },
        tool_call_id: 'call-ask',
        running: false,
        user_input: userInput,
      }],
      timestamp: new Date().toISOString(),
    }])

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
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
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'acp-session-1', bot_id: 'bot-1', title: '', type: 'acp_agent' },
    ], nextCursor: null })
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
      .mockResolvedValueOnce({ items: [
        { id: 'session-old', bot_id: 'bot-1', title: 'Old', type: 'chat' },
      ], nextCursor: null })
      .mockResolvedValueOnce({ items: [
        { id: 'session-new', bot_id: 'bot-1', title: 'New from channel', type: 'chat' },
        { id: 'session-old', bot_id: 'bot-1', title: 'Old', type: 'chat' },
      ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    expect(store.sessionId).toBe('session-old')

    sessionsActivityHandler?.({
      type: 'session_touched',
      session_id: 'session-new',
      updated_at: '2026-06-02T10:00:00.000Z',
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
    // Two parallel assistant streams in two sessions: each turn must be
    // updated by its own stream id, never crossed. Cross-session view
    // visibility (showing session A's content while on session B) is no
    // longer a thing — switching sessions issues a fresh REST refresh, and
    // any in-flight stream on a non-active session keeps writing to its
    // optimistic turn but the user does not see it until the server has
    // persisted the response and a switch-back triggers a fetch.
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
    ], nextCursor: null })
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
    // Active session is B, so its optimistic turn shows the message.
    expect(store.sessionId).toBe('session-b')
    expect(store.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({
        role: 'assistant',
        messages: [expect.objectContaining({ type: 'text', content: 'answer B' })],
      }),
    ]))

    streamHandler?.({ type: 'end', stream_id: streamA, session_id: 'session-a' } as UIStreamEvent)
    streamHandler?.({ type: 'end', stream_id: streamB, session_id: 'session-b' } as UIStreamEvent)
    await first
    await second
  })

  it('hydrates hidden subagent session summaries before selecting them', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-parent', bot_id: 'bot-1', title: 'Parent', type: 'chat' },
    ], nextCursor: null })
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-subagent',
      bot_id: 'bot-1',
      title: 'Subagent',
      type: 'subagent',
      parent_session_id: 'session-parent',
    })
    api.fetchMessagesUI.mockResolvedValue([])

    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.selectSession('session-subagent')

    expect(api.fetchSession).toHaveBeenCalledWith('bot-1', 'session-subagent')
    expect(store.activeSession).toMatchObject({
      id: 'session-subagent',
      type: 'subagent',
    })
    expect(store.activeChatReadOnly).toBe(true)

    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-parent', bot_id: 'bot-1', title: 'Parent', type: 'chat' },
    ], nextCursor: null })
    await store.initialize()

    expect(store.sessionId).toBe('session-subagent')
    expect(store.activeSession).toMatchObject({
      id: 'session-subagent',
      type: 'subagent',
    })
  })

  it('paginates the sessions list and clears hasMoreSessions when the cursor is exhausted', async () => {
    api.fetchSessions
      .mockResolvedValueOnce({
        items: [
          { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
          { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
        ],
        nextCursor: 'cursor-2',
      })
      .mockResolvedValueOnce({
        items: [
          // Duplicate must be deduped; new entry appends.
          { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
          { id: 'session-3', bot_id: 'bot-1', title: 'C', type: 'chat' },
        ],
        nextCursor: null,
      })
    const store = useChatStore()

    await store.selectBot('bot-1')
    expect(store.sessions.map(s => s.id)).toEqual(['session-1', 'session-2'])
    expect(store.hasMoreSessions).toBe(true)
    expect(store.sessionsCursor).toBe('cursor-2')

    await store.loadMoreSessions()

    expect(api.fetchSessions).toHaveBeenLastCalledWith('bot-1', { cursor: 'cursor-2' })
    expect(store.sessions.map(s => s.id)).toEqual(['session-1', 'session-2', 'session-3'])
    expect(store.hasMoreSessions).toBe(false)
    expect(store.sessionsCursor).toBeNull()

    // Further load attempts are a no-op once the cursor is exhausted.
    await store.loadMoreSessions()
    expect(api.fetchSessions).toHaveBeenCalledTimes(2)
  })

  it('resets hasLoadedOlder on initialize so a fresh bot does not inherit the previous scroll-back flag', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    // Simulate the user having scrolled back in the previous session.
    store._hasLoadedOlder = true
    expect(store._hasLoadedOlder).toBe(true)

    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-2', bot_id: 'bot-2', title: 'Chat 2', type: 'chat' }],
      nextCursor: null,
    })
    await store.selectBot('bot-2')
    await flushPromises()

    expect(store._hasLoadedOlder).toBe(false)
  })

  it('does not duplicate the optimistic user turn when stream-end refresh returns the persisted version', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: { id: 0, type: 'text', content: 'hello' },
      } as UIStreamEvent,
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    // Drive the actual send: this appends an optimistic user turn plus a
    // streaming assistant turn. The WS mock auto-replays sendEvents.
    const sendPromise = store.sendMessage('hi')
    await flushPromises()
    expect(store.messages.map(m => m.role)).toEqual(['user', 'assistant'])

    // Stream-end triggers refreshCurrentSession; the persisted user turn
    // carries server ids different from the optimistic ids and a server
    // timestamp slightly OLDER than the optimistic client timestamp (clock
    // skew). The previous timestamp-based merge heuristic misclassified
    // this as "user has scrolled back" and merged the two copies; the fix
    // keys off the explicit hasLoadedOlder flag instead.
    const past = new Date(Date.now() - 1_000).toISOString()
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'server-user',
        role: 'user',
        text: 'hi',
        attachments: [],
        timestamp: past,
      },
      {
        id: 'server-assistant',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'hello', running: false }],
        timestamp: past,
      },
    ])
    streamHandler?.({ type: 'end', stream_id: lastStreamId, session_id: lastSessionId } as UIStreamEvent)
    await sendPromise
    await flushPromises()

    expect(store.messages.map(m => m.role)).toEqual(['user', 'assistant'])
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'hi' })
    expect(store._hasLoadedOlder).toBe(false)
  })

  it('keeps loadingMessages owned by the latest session start when an earlier refresh resolves late', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    // Inject sessions directly so we can drive selectSession ourselves
    // (the initialize path would auto-pick the first session and consume
    // the first fetchMessagesUI mock).
    store.sessions.push(
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' } as never,
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' } as never,
    )

    let resolveA: (v: unknown[]) => void = () => {}
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise((resolve) => {
      resolveA = resolve as (v: unknown[]) => void
    }))
    store.selectSession('session-a')
    await flushPromises()
    expect(store.loadingMessages).toBe(true)

    let resolveB: (v: unknown[]) => void = () => {}
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise((resolve) => {
      resolveB = resolve as (v: unknown[]) => void
    }))
    store.selectSession('session-b')
    await flushPromises()
    expect(store.loadingMessages).toBe(true)

    // A's late refresh resolves: its `finally` MUST NOT clear B's flag.
    resolveA([])
    await flushPromises()
    expect(store.loadingMessages).toBe(true)

    resolveB([])
    await flushPromises()
    expect(store.loadingMessages).toBe(false)
  })

  it('preserves scrolled-back history when an SSE refresh fires', async () => {
    // Initialize via fetchMessagesUI returning the most recent page; the
    // initialize path auto-selects the only session and triggers the
    // initial transcript refresh.
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    // Initial page must be >= PAGE_SIZE (30) so hasMoreOlder stays true and
    // loadOlderMessages is willing to fetch the older page.
    const initialPage = Array.from({ length: 30 }, (_, idx) => ({
      id: `recent-${idx}`,
      role: 'user' as const,
      text: `recent ${idx}`,
      attachments: [],
      timestamp: `2026-06-19T00:01:${String(idx).padStart(2, '0')}Z`,
    }))
    api.fetchMessagesUI.mockResolvedValueOnce(initialPage)
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    await flushPromises()
    expect(store.hasMoreOlder).toBe(true)
    expect(store.messages.length).toBe(30)

    // User scrolls back and pulls in older content.
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'msg-1', role: 'user', text: 'oldest', attachments: [], timestamp: '2026-06-19T00:00:01Z' },
      { id: 'msg-2', role: 'user', text: 'older', attachments: [], timestamp: '2026-06-19T00:00:02Z' },
    ])
    await store.loadOlderMessages()
    expect(store._hasLoadedOlder).toBe(true)
    expect(store.messages[0]?.id).toBe('msg-1')
    expect(store.messages[1]?.id).toBe('msg-2')
    expect(store.messages.length).toBe(32)

    // SSE-triggered refresh fetches only the most recent page; merge MUST
    // preserve the older content the user pulled in.
    api.fetchMessagesUI.mockResolvedValueOnce(initialPage)
    _sessionMessageHandler?.({
      type: 'message_created',
      message: { id: 'recent-29', session_id: 'session-1', created_at: '2026-06-19T00:01:29Z' },
    } as never)
    await new Promise(r => setTimeout(r, 150))
    await flushPromises()
    expect(store.messages[0]?.id).toBe('msg-1')
    expect(store.messages[1]?.id).toBe('msg-2')
  })

  it('replaces a scrolled-back optimistic user turn with its server twin instead of duplicating', async () => {
    // The id-keyed dedup in the previous mergeMessages happily kept the
    // optimistic and server copies side by side while the user was scrolled
    // back. Logical-turn matching (role + content + ~timestamp), gated on
    // the explicit `__optimistic` flag, collapses them in place. The flag
    // (not id shape) is what isOptimisticTurn keys off, so this test also
    // pins that a server turn whose id contains dashes (UUID-like) is NOT
    // treated as optimistic — the prior heuristic would have misclassified
    // it and eaten real history.
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const initialPage = Array.from({ length: 30 }, (_, idx) => ({
      id: `recent-${idx}`,
      role: 'user' as const,
      text: `recent ${idx}`,
      attachments: [],
      timestamp: `2026-06-19T00:01:${String(idx).padStart(2, '0')}Z`,
    }))
    api.fetchMessagesUI.mockResolvedValueOnce(initialPage)
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    await flushPromises()

    // Pull older history so hasLoadedOlder flips on. Use a UUID-shaped id to
    // exercise the path where a server id contains dashes; the flag-based
    // isOptimisticTurn must still leave it alone.
    const dashedServerId = '550e8400-e29b-41d4-a716-446655440000'
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: dashedServerId, role: 'user', text: 'oldest', attachments: [], timestamp: '2026-06-19T00:00:01Z' },
    ])
    await store.loadOlderMessages()
    expect(store._hasLoadedOlder).toBe(true)
    const baseLength = store.messages.length
    // The dashed-id server turn survived the merge — it must not be classified
    // as optimistic regardless of id shape.
    expect(store.messages.some(m => m.id === dashedServerId)).toBe(true)

    // Push an optimistic user turn directly: simulates send while scrolled
    // back, without engaging the streaming machinery the surrounding suite
    // already covers. The id intentionally has no dash so the test cannot
    // accidentally pass via id-shape heuristics — the __optimistic flag is
    // what drives the merge.
    const optimisticTimestamp = '2026-06-19T00:02:00Z'
    store.messages.push({
      id: '1700000000000',
      role: 'user',
      text: 'just sent',
      attachments: [],
      timestamp: optimisticTimestamp,
      streaming: false,
      isSelf: true,
      __optimistic: true,
    } as never)
    expect(store.messages.length).toBe(baseLength + 1)

    // SSE-triggered refresh returns the server twin (different id, same
    // role+content, timestamp within 5s).
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'server-user-1',
        role: 'user',
        text: 'just sent',
        attachments: [],
        timestamp: '2026-06-19T00:02:01Z',
      },
    ])
    _sessionMessageHandler?.({
      type: 'message_created',
      message: { id: 'server-user-1', session_id: 'session-1', created_at: '2026-06-19T00:02:01Z' },
    } as never)
    await new Promise(r => setTimeout(r, 150))
    await flushPromises()

    // Both copies would survive id-keyed dedup; only the server turn must
    // remain, and the optimistic id must be gone.
    expect(store.messages.length).toBe(baseLength + 1)
    const justSent = store.messages.filter(m => m.role === 'user' && (m as { text?: string }).text === 'just sent')
    expect(justSent.length).toBe(1)
    expect(justSent[0]?.id).toBe('server-user-1')
    // And the dashed-id server turn from the older page is still here.
    expect(store.messages.some(m => m.id === dashedServerId)).toBe(true)
  })

  it('keeps hasMoreOlder true after a short initial page (turn count is not a terminal signal)', async () => {
    // The server pages by raw `bot_history_messages` rows but returns merged
    // UI turns, so a 30-row page collapses to ~28 turns even when thousands
    // of rows remain — the old `turns.length >= PAGE_SIZE` check truncated
    // long sessions to one page on first paint (Project Memoh: 1144 raw rows
    // collapsed to 28 turns => hasMoreOlder=false => scroll-up blocked).
    // Initial loads now stay optimistic; `loadOlderMessages` flips the flag
    // off the first time the server returns an authoritatively empty page.
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const shortPage = Array.from({ length: 5 }, (_, idx) => ({
      id: `msg-${idx}`,
      role: 'user' as const,
      text: 'hi',
      attachments: [],
      timestamp: `2026-06-19T00:00:${String(idx).padStart(2, '0')}Z`,
    }))
    api.fetchMessagesUI.mockResolvedValueOnce(shortPage)
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    await flushPromises()
    expect(store.messages.length).toBe(5)
    expect(store.hasMoreOlder).toBe(true)
  })

  it('flips hasMoreOlder to false when the older page is empty and stops re-firing', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    // Initial page == PAGE_SIZE so hasMoreOlder is true after refresh; the
    // older fetch then returns empty to simulate end-of-history.
    const initialPage = Array.from({ length: 30 }, (_, idx) => ({
      id: `msg-${idx}`,
      role: 'user' as const,
      text: 'hi',
      attachments: [],
      timestamp: `2026-06-19T00:00:${String(idx).padStart(2, '0')}Z`,
    }))
    api.fetchMessagesUI.mockResolvedValueOnce(initialPage)
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    await flushPromises()
    expect(store.hasMoreOlder).toBe(true)

    api.fetchMessagesUI.mockResolvedValueOnce([])
    await store.loadOlderMessages()
    expect(store.hasMoreOlder).toBe(false)

    const callsBefore = api.fetchMessagesUI.mock.calls.length
    await store.loadOlderMessages()
    expect(api.fetchMessagesUI.mock.calls.length).toBe(callsBefore)
  })

  it('appends sessions emitted by the bot-wide activity stream', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')

    // session_created on the activity stream triggers a sessions-list reload
    // (the server payload omits session type/metadata so a client-built stub
    // would be incomplete).
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-2', bot_id: 'bot-1', title: 'New', type: 'discuss' },
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
      ],
      nextCursor: null,
    })
    sessionsActivityHandler?.({
      type: 'session_created',
      session_id: 'session-2',
      session_type: 'discuss',
      title: 'New',
    })
    await flushPromises()

    expect(store.sessions.map(s => s.id)).toEqual(['session-2', 'session-1'])
  })
})
