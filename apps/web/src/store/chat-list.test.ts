import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { MessageStreamEvent, UIStreamEvent, UIStreamEventHandler, UIUserInput } from '@/composables/api/useChat'
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
  createACPRuntime: vi.fn(),
  setACPRuntimeModel: vi.fn(),
  setACPRuntimeModelByID: vi.fn(),
  closeACPRuntime: vi.fn(),
  streamMessageEvents: vi.fn(),
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
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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

  it('reconciles refreshed turns in place, preserving identity of unchanged turns', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{ id: 0, type: 'text', content: 'hello' }],
      timestamp: '2026-01-01T00:00:01Z',
    }])
    streamHandler?.({ type: 'start', stream_id: 'stream-a', session_id: 'session-1' } as UIStreamEvent)
    streamHandler?.({ type: 'end', stream_id: 'stream-a', session_id: 'session-1' } as UIStreamEvent)
    await flushPromises()

    const turn = store.messages.find(message => message.id === 'assistant-1')
    const block = turn?.role === 'assistant' ? turn.messages[0] : null
    expect(turn?.role).toBe('assistant')
    expect(block?.type).toBe('text')

    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{ id: 0, type: 'text', content: 'hello world' }],
      timestamp: '2026-01-01T00:00:01Z',
    }])
    streamHandler?.({ type: 'start', stream_id: 'stream-b', session_id: 'session-1' } as UIStreamEvent)
    streamHandler?.({ type: 'end', stream_id: 'stream-b', session_id: 'session-1' } as UIStreamEvent)
    await flushPromises()

    const turnAfter = store.messages.find(message => message.id === 'assistant-1')
    const blockAfter = turnAfter?.role === 'assistant' ? turnAfter.messages[0] : null
    expect(turnAfter).toBe(turn)
    expect(blockAfter).toBe(block)
    expect(blockAfter?.type === 'text' ? blockAfter.content : '').toBe('hello world')
  })

  it('adopts the server id onto the just-sent optimistic turn in place, keeping its key', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
    sendEvents = [
      { type: 'message', data: { id: 0, type: 'text', content: 'hello' } } as UIStreamEvent,
      { type: 'end' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'srv-user-1', role: 'user', text: 'hi', timestamp: '2026-01-01T00:00:00Z' },
      { id: 'srv-asst-1', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'hello' }], timestamp: '2026-01-01T00:00:01Z' },
    ])
    await store.sendMessage('hi')
    await flushPromises()

    const asstTurn = store.messages.find(message => message.role === 'assistant')
    expect(asstTurn).toBeTruthy()
    expect(asstTurn!.id).not.toBe('srv-asst-1')
    expect((asstTurn as { serverId?: string }).serverId).toBe('srv-asst-1')

    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'srv-user-1', role: 'user', text: 'hi', timestamp: '2026-01-01T00:00:00Z' },
      { id: 'srv-asst-1', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'hello' }], timestamp: '2026-01-01T00:00:01Z' },
      { id: 'srv-user-2', role: 'user', text: 'again', timestamp: '2026-01-01T00:00:02Z' },
      { id: 'srv-asst-2', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'reply 2' }], timestamp: '2026-01-01T00:00:03Z' },
    ])
    await store.sendMessage('again')
    await flushPromises()

    const asstTurnAfter = store.messages.find(message => (message as { serverId?: string }).serverId === 'srv-asst-1')
    expect(asstTurnAfter).toBe(asstTurn)
  })

  it('stamps session updated_at from the server message time, not the client clock or a reorder', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat', updated_at: '2026-01-01T00:00:00Z' },
      { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat', updated_at: '2026-01-02T00:00:00Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    messageEventsHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm1',
        bot_id: 'bot-1',
        session_id: 'session-2',
        role: 'assistant',
        content: 'hi',
        created_at: '2026-01-03T00:00:00Z',
      },
    })
    await flushPromises()

    const updated = store.sessions.find(session => session.id === 'session-2')
    expect(updated?.updated_at).toBe('2026-01-03T00:00:00Z')
    expect(store.sessions.map(session => session.id)).toEqual(['session-1', 'session-2'])
  })

  it('refreshes pending user input after response stream failure', async () => {
    api.fetchSessions.mockResolvedValueOnce([
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ])
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

  it('debounces fsChangedAt bumps when a fs-mutating tool completes', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'write',
          tool_call_id: 'call-write-1',
          running: false,
        },
      } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 2,
          type: 'tool',
          name: 'edit',
          tool_call_id: 'call-edit-1',
          running: false,
        },
      } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    const before = store.fsChangedAt
    await store.sendMessage('write file')

    expect(store.fsChangedAt).toBe(before)
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.fsChangedAt).toBeGreaterThan(before)
  })

  it('does not bump fsChangedAt for non-fs-mutating tools', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'web_search',
          tool_call_id: 'call-search',
          running: false,
        },
      } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    const before = store.fsChangedAt
    await store.sendMessage('search')
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.fsChangedAt).toBe(before)
  })

  it('does not bump fsChangedAt while a fs-mutating tool is still running', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'write',
          tool_call_id: 'call-write-pending',
          running: true,
        },
      } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    const before = store.fsChangedAt
    await store.sendMessage('start write')
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.fsChangedAt).toBe(before)
  })
})

describe('fsChangedAt markFsChanged()', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('bumps fsChangedAt after the debounce window', () => {
    const store = useChatStore()
    const before = store.fsChangedAt
    store.markFsChanged()
    expect(store.fsChangedAt).toBe(before)
    vi.advanceTimersByTime(149)
    expect(store.fsChangedAt).toBe(before)
    vi.advanceTimersByTime(1)
    expect(store.fsChangedAt).toBeGreaterThan(before)
  })

  it('collapses bursts of markFsChanged() within the window into a single bump', () => {
    const store = useChatStore()
    const before = store.fsChangedAt
    for (let i = 0; i < 5; i++) store.markFsChanged()
    expect(store.fsChangedAt).toBe(before)
    vi.advanceTimersByTime(150)
    const afterFirst = store.fsChangedAt
    expect(afterFirst).toBeGreaterThan(before)

    store.markFsChanged()
    expect(store.fsChangedAt).toBe(afterFirst)
    vi.advanceTimersByTime(150)
    expect(store.fsChangedAt).toBeGreaterThan(afterFirst)
  })
})

describe('path-tagged fs change batches', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('records the path in the batch and exposes it via affectsPath()', () => {
    const store = useChatStore()
    store.markFsChanged('/data/foo.ts')
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/data/foo.ts')).toBe(true)
    expect(store.affectsPath('/data/bar.ts')).toBe(false)
  })

  it('merges multiple paths within a single debounce window', () => {
    const store = useChatStore()
    store.markFsChanged('/data/a.ts')
    store.markFsChanged('/data/b.ts')
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/data/a.ts')).toBe(true)
    expect(store.affectsPath('/data/b.ts')).toBe(true)
    expect(store.affectsPath('/data/c.ts')).toBe(false)
  })

  it('null in any bump within the window poisons the batch to wildcard', () => {
    const store = useChatStore()
    store.markFsChanged('/data/a.ts')
    store.markFsChanged(null)
    store.markFsChanged('/data/b.ts')
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/anything.ts')).toBe(true)
  })

  it('treats markFsChanged() with no argument as wildcard', () => {
    const store = useChatStore()
    store.markFsChanged()
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/anything.ts')).toBe(true)
  })

  it('returns false from affectsPath() before any bump fires', () => {
    const store = useChatStore()
    store.markFsChanged('/data/a.ts')
    expect(store.affectsPath('/data/a.ts')).toBe(false)
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/data/a.ts')).toBe(true)
  })

  it('resets the path set after the batch fires', () => {
    const store = useChatStore()
    store.markFsChanged('/data/a.ts')
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/data/a.ts')).toBe(true)

    store.markFsChanged('/data/b.ts')
    vi.advanceTimersByTime(150)
    expect(store.affectsPath('/data/a.ts')).toBe(false)
    expect(store.affectsPath('/data/b.ts')).toBe(true)
  })
})

describe('fs-mutating tool path extraction', () => {
  let lastStreamId = ''
  let lastSessionId = ''
  let sendEvents: UIStreamEvent[]

  beforeEach(() => {
    setActivePinia(createPinia())
    lastStreamId = ''
    lastSessionId = ''
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    vi.clearAllMocks()

    api.fetchBots.mockResolvedValue([{ id: 'bot-1', status: 'active', name: 'Bot' }])
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
      return {
        get connected() { return true },
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

  it('tags the batch with input.path for write and edit tools', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'write',
          tool_call_id: 'call-write',
          input: { path: '/data/touched-by-write.ts', content: 'x' },
          running: false,
        },
      } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 2,
          type: 'tool',
          name: 'edit',
          tool_call_id: 'call-edit',
          input: { path: '/data/touched-by-edit.ts', old_text: 'a', new_text: 'b' },
          running: false,
        },
      } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.sendMessage('do work')

    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.affectsPath('/data/touched-by-write.ts')).toBe(true)
    expect(store.affectsPath('/data/touched-by-edit.ts')).toBe(true)
    expect(store.affectsPath('/data/untouched.ts')).toBe(false)
  })

  it('falls back to wildcard when exec completes (path unknown)', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          tool_call_id: 'call-exec',
          input: { command: 'rm -rf /data/maybe' },
          running: false,
        },
      } as UIStreamEvent,
      { type: 'error', message: 'done' } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.sendMessage('exec')

    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.affectsPath('/data/anything.ts')).toBe(true)
  })
})

describe('fs bump bot-switch isolation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot 1' },
      { id: 'bot-2', status: 'active', name: 'Bot 2' },
    ])
    api.fetchSessions.mockResolvedValue([])
    api.fetchMessagesUI.mockResolvedValue([])
    api.streamMessageEvents.mockImplementation((_botId: string, signal: AbortSignal) => new Promise<void>((resolve) => {
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
    api.connectWebSocket.mockImplementation(() => ({
      get connected() { return true },
      send: vi.fn(),
      abort: vi.fn(),
      close: vi.fn(),
      onOpen: null,
      onClose: null,
    }))
  })

  it('discards a pending fs batch when currentBotId changes before the timer fires', () => {
    vi.useFakeTimers()
    try {
      const store = useChatStore()
      store.currentBotId = 'bot-A'
      store.markFsChanged('/data/foo.ts')
      store.currentBotId = 'bot-B'
      vi.advanceTimersByTime(150)
      expect(store.lastFsChange).toBe(null)
      expect(store.affectsPath('/data/foo.ts')).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not leak a batch from one bot into the next after a switch', () => {
    vi.useFakeTimers()
    try {
      const store = useChatStore()
      store.currentBotId = 'bot-A'
      store.markFsChanged('/data/foo.ts')
      store.currentBotId = 'bot-B'
      vi.advanceTimersByTime(150)
      expect(store.affectsPath('/data/foo.ts')).toBe(false)

      store.markFsChanged('/data/bar.ts')
      vi.advanceTimersByTime(150)
      expect(store.affectsPath('/data/bar.ts')).toBe(true)
      expect(store.affectsPath('/data/foo.ts')).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('selectBot cancels any pending fs bump and clears lastFsChange', async () => {
    const store = useChatStore()
    await store.selectBot('bot-1')
    store.markFsChanged('/data/foo.ts')
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.lastFsChange).not.toBe(null)

    await store.selectBot('bot-2')
    expect(store.lastFsChange).toBe(null)

    store.markFsChanged('/data/foo.ts')
    await new Promise(resolve => setTimeout(resolve, 200))
    expect(store.lastFsChange).not.toBe(null)
  })
})
