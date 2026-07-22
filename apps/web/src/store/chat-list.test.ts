import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia, type Pinia } from 'pinia'
import type { BotSessionActivityEvent, SessionMessageStreamEvent, UIStreamEvent, UIStreamEventHandler, UIToolApproval, UITurn, UIUserInput } from '@/composables/api/useChat'
import { REASONING_EFFORT_DISABLE } from '@/pages/bots/components/reasoning-effort'
import { AUTH_SESSION_CLEARED_EVENT } from '@/lib/auth-session'
import { useChatSelectionStore } from './chat-selection'
import { useChatStore } from './chat-list'
import type { ConversationUiMessage, ConversationUiTurn, SessionruntimeRunOperationView, SessionruntimeSnapshot } from '@memohai/sdk'
import {
  generationReuseContractFixture,
  interruptedRunContractFixture,
  replacementOperationsContractFixture,
  richActiveRunContractFixture,
  runtimeRecoveryContractFixture,
} from './runtime-contract-fixtures.test-support'

const api = vi.hoisted(() => ({
  createSession: vi.fn(),
  deleteSession: vi.fn(),
  forkSessionFromMessage: vi.fn(),
  fetchSession: vi.fn(),
  fetchSessions: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  fetchSessionRuntime: vi.fn(),
  sendLocalChannelMessage: vi.fn(),
  executeQuickAction: vi.fn(),
  fetchSafeSkillCatalog: vi.fn(),
  updateSessionAgent: vi.fn(),
  ensureACPRuntime: vi.fn(),
  createACPRuntime: vi.fn(),
  fetchACPRuntimeByID: vi.fn(),
  setACPRuntimeModel: vi.fn(),
  setACPRuntimeModelByID: vi.fn(),
  setACPRuntimeReasoning: vi.fn(),
  setACPRuntimeReasoningByID: vi.fn(),
  closeACPRuntime: vi.fn(),
  streamSessionMessageEvents: vi.fn(),
  streamBotSessionsActivityEvents: vi.fn(),
  connectWebSocket: vi.fn(),
  locateMessageUI: vi.fn(),
}))

const toast = vi.hoisted(() => ({
  error: vi.fn(),
}))

const sdk = vi.hoisted(() => ({
  getBotsByBotIdSettings: vi.fn(),
}))

vi.hoisted(() => {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
      clear: () => {},
    },
  })
})

vi.mock('@/composables/api/useChat', () => api)
vi.mock('@memohai/sdk', () => ({ getBotsByBotIdSettings: sdk.getBotsByBotIdSettings }))
vi.mock('vue-sonner', () => ({ toast }))
vi.mock('@felinic/ui', async (importOriginal) => {
  const original = await importOriginal<typeof import('@felinic/ui')>()
  return { ...original, toast }
})

function flushPromises() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

let testPiniaInstances: Pinia[] = []

function createTestPinia() {
  const pinia = createPinia()
  testPiniaInstances.push(pinia)
  return pinia
}

function applyLatestDraftRequest(store: ReturnType<typeof useChatStore>) {
  const request = store.draftViewRequested
  if (!request) throw new Error('Expected a Draft view request')
  store.applyDraftViewRequest(request, true)
}

async function applyLatestForkRequest(store: ReturnType<typeof useChatStore>) {
  const request = store.forkedSessionRequested
  if (!request) throw new Error('Expected a Forked Session request')
  await store.selectSession(request.sessionId)
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

function askUserTurn(userInput: UIUserInput, toolCallId = 'call-ask', blockId = 1) {
  return {
    id: 'assistant-1',
    role: 'assistant' as const,
    messages: [{
      id: blockId,
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

function approvalTurn(approval: UIToolApproval, blockId = 1) {
  return {
    id: 'assistant-approval',
    role: 'assistant' as const,
    messages: [{
      id: blockId,
      type: 'tool' as const,
      name: 'exec',
      input: { command: 'pwd' },
      tool_call_id: 'call-pwd',
      toolCallId: 'call-pwd',
      toolName: 'exec',
      running: false,
      done: true,
      result: null,
      approval,
    }],
    timestamp: new Date().toISOString(),
    streaming: false,
  }
}

function richActiveRunStoreScript(sessionId = 'session-1', streamId = 'stream-rich'): UIStreamEvent[] {
  return structuredClone(richActiveRunContractFixture.runtime_snapshot.snapshot.current_run_view?.messages ?? []).map(data => ({
    type: 'message',
    data,
    stream_id: streamId,
    session_id: sessionId,
  })) as UIStreamEvent[]
}

function runtimeSnapshotFromScript(
  script: UIStreamEvent[],
  sessionId = 'session-1',
  streamId = 'stream-rich',
  status = 'running',
  seq = 10,
  error = '',
  requestUserTurn?: ConversationUiTurn,
  historyCommitted = false,
  canonicalReady = false,
): SessionruntimeSnapshot {
  const messages = script.flatMap((event) => {
    if (event.type !== 'message') return []
    return [event.data as ConversationUiMessage]
  })
  return {
    bot_id: 'bot-1',
    session_id: sessionId,
    epoch: `epoch-${sessionId}`,
    seq,
    current_run_view: {
      stream_id: streamId,
      generation: `generation-${streamId}`,
      status,
      messages,
      ...(historyCommitted ? { history_committed: true } : {}),
      ...(canonicalReady ? { canonical_ready: true } : {}),
      ...(requestUserTurn ? { request_user_turn: requestUserTurn } : {}),
      ...(error ? { error } : {}),
    },
    queue: [],
  }
}

function runtimeReplacementSnapshot(
  streamId: string,
  operation: SessionruntimeRunOperationView,
  messages: ConversationUiMessage[] = [],
  status = 'running',
  seq = 10,
  sessionId = 'session-1',
  canonicalReady = false,
  historyCommitted = true,
): UIStreamEvent {
  const epoch = `epoch-${sessionId}`
  return {
    type: 'runtime_snapshot',
    bot_id: 'bot-1',
    session_id: sessionId,
    epoch,
    seq,
    snapshot: {
      bot_id: 'bot-1',
      session_id: sessionId,
      epoch,
      seq,
      current_run_view: {
        stream_id: streamId,
        generation: `generation-${streamId}`,
        status,
        messages,
        operation,
        ...(historyCommitted ? { history_committed: true } : {}),
        ...(canonicalReady ? { canonical_ready: true } : {}),
      },
      queue: [],
    },
  } as UIStreamEvent
}

function richActiveRunRuntimeSnapshot(): SessionruntimeSnapshot {
  return structuredClone(richActiveRunContractFixture.runtime_snapshot.snapshot)
}

describe('chat-list store', () => {
  let streamHandler: UIStreamEventHandler | null
  // Captured but not driven by any test body yet; keep the capture so future
  // tests can simulate per-session SSE events without rewiring the mock.
  let _sessionMessageHandler: ((event: SessionMessageStreamEvent) => void) | null
  let sessionMessageHandlers: Map<string, (event: SessionMessageStreamEvent) => void>
  let sessionsActivityHandler: ((event: BotSessionActivityEvent) => void) | null
  let sendEvents: UIStreamEvent[]
  let sentWSMessages: Array<Record<string, unknown>>
  let wsOutboundTimeline: Array<Record<string, unknown>>
  let abortedWSStreams: string[]
  let runtimeSubscribeMessages: Array<Record<string, unknown>>
  let runtimeUnsubscribeMessages: Array<Record<string, unknown>>
  let lastStreamId = ''
  let lastSessionId = ''

  beforeEach(() => {
    testPiniaInstances = []
    setActivePinia(createTestPinia())
    streamHandler = null
    _sessionMessageHandler = null
    sessionMessageHandlers = new Map()
    sessionsActivityHandler = null
    lastStreamId = ''
    lastSessionId = ''
    sentWSMessages = []
    wsOutboundTimeline = []
    abortedWSStreams = []
    runtimeSubscribeMessages = []
    runtimeUnsubscribeMessages = []
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
      reasoning: {
        current_effort: 'medium',
        available_efforts: [
          { id: 'medium', name: 'Medium' },
          { id: 'high', name: 'High' },
        ],
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
      reasoning: {
        current_effort: 'medium',
        available_efforts: [
          { id: 'medium', name: 'Medium' },
          { id: 'high', name: 'High' },
        ],
      },
    })
    api.fetchACPRuntimeByID.mockResolvedValue({
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
      reasoning: {
        current_effort: 'medium',
        available_efforts: [
          { id: 'medium', name: 'Medium' },
          { id: 'high', name: 'High' },
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
    api.setACPRuntimeReasoning.mockResolvedValue({
      session_id: 'session-1',
      agent_id: 'codex',
      reasoning: {
        current_effort: 'low',
        available_efforts: [{ id: 'low', name: 'Low' }],
      },
    })
    api.setACPRuntimeReasoningByID.mockResolvedValue({
      runtime_id: 'rt_warm',
      agent_id: 'codex',
      state: 'idle',
      reasoning: {
        current_effort: 'high',
        available_efforts: [
          { id: 'medium', name: 'Medium' },
          { id: 'high', name: 'High' },
        ],
      },
    })
    api.closeACPRuntime.mockResolvedValue(undefined)
    api.fetchMessagesUI.mockResolvedValue([])
    api.fetchSessionRuntime.mockImplementation((botId: string, sessionId: string) => Promise.resolve({
      bot_id: botId,
      session_id: sessionId,
      seq: 0,
      queue: [],
    }))
    api.executeQuickAction.mockResolvedValue(null)
    api.fetchSafeSkillCatalog.mockResolvedValue([])
    sdk.getBotsByBotIdSettings.mockResolvedValue({ data: { chat_runtime: 'model' } })
    api.streamSessionMessageEvents.mockImplementation((botId: string, targetSessionId: string, signal: AbortSignal, onEvent: (event: SessionMessageStreamEvent) => void) => new Promise<void>((resolve) => {
      _sessionMessageHandler = onEvent
      const key = `${botId}:${targetSessionId}`
      sessionMessageHandlers.set(key, onEvent)
      signal.addEventListener('abort', () => {
        if (sessionMessageHandlers.get(key) === onEvent) sessionMessageHandlers.delete(key)
        resolve()
      }, { once: true })
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
          wsOutboundTimeline.push(message as Record<string, unknown>)
          if ((message as Record<string, unknown>).type === 'runtime_subscribe') {
            runtimeSubscribeMessages.push(message as Record<string, unknown>)
            return
          }
          if ((message as Record<string, unknown>).type === 'runtime_unsubscribe') {
            runtimeUnsubscribeMessages.push(message as Record<string, unknown>)
            return
          }
          sentWSMessages.push(message as Record<string, unknown>)
          lastStreamId = message.stream_id ?? ''
          lastSessionId = message.session_id ?? ''
          for (const event of sendEvents) {
            const commandInvocation = event.type === 'command_result' || event.type === 'command_error'
              ? { invocation_id: event.invocation_id ?? lastStreamId }
              : {}
            onStreamEvent({
              ...event,
              ...commandInvocation,
              stream_id: lastStreamId,
              session_id: lastSessionId,
            } as UIStreamEvent)
          }
        }),
        abort: vi.fn((streamId: string, _sessionId: string, _generation?: string) => {
          abortedWSStreams.push(streamId)
        }),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
  })

  afterEach(async () => {
    for (const pinia of testPiniaInstances) disposePinia(pinia)
    await flushPromises()
    vi.unstubAllGlobals()
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

  it('requests the Desktop once when each Browser Use or Computer Use call starts', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'browser_action',
          input: { action: 'click' },
          tool_call_id: 'call-browser',
          running: true,
        },
      } as UIStreamEvent,
    ]
    const store = useChatStore()
    await store.selectBot('bot-1')

    const sending = store.sendMessage('use the browser')
    await flushPromises()
    expect(store.guiToolUseRequested).toMatchObject({
      botId: 'bot-1',
      sessionId: 'session-1',
      toolCallId: 'call-browser',
      toolName: 'browser_action',
      seq: 1,
    })

    streamHandler?.({
      type: 'message',
      stream_id: lastStreamId,
      session_id: 'session-1',
      data: {
        id: 1,
        type: 'tool',
        name: 'browser_action',
        input: { action: 'click', coordinate: [10, 20] },
        tool_call_id: 'call-browser',
        running: true,
      },
    } as UIStreamEvent)
    expect(store.guiToolUseRequested?.seq).toBe(1)

    streamHandler?.({
      type: 'message',
      stream_id: lastStreamId,
      session_id: 'session-1',
      data: {
        id: 2,
        type: 'tool',
        name: 'computer_observe',
        input: { observe: 'snapshot' },
        tool_call_id: 'call-computer',
        running: true,
      },
    } as UIStreamEvent)
    expect(store.guiToolUseRequested).toMatchObject({
      toolCallId: 'call-computer',
      toolName: 'computer_observe',
      seq: 2,
    })

    streamHandler?.({
      type: 'end',
      stream_id: lastStreamId,
      session_id: 'session-1',
    } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: true })
  })

  it('returns startup stream errors to the composer when no assistant output exists', async () => {
    const store = useChatStore()
    const onBeforeTurnAppend = vi.fn()
    const onTurnAppendAborted = vi.fn()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('hello', undefined, {
      onBeforeTurnAppend,
      onTurnAppendAborted,
      workspaceTargetId: 'computer-b',
    })

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
    expect(onBeforeTurnAppend).toHaveBeenCalledOnce()
    expect(onTurnAppendAborted).toHaveBeenCalledOnce()
    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'message',
      workspace_target_id: 'computer-b',
    })
  })

  it('localizes structured WebSocket errors by stable code', async () => {
    const acpSession = {
      id: 'session-1',
      bot_id: 'bot-1',
      title: 'Codex',
      type: 'chat',
      session_mode: 'chat',
      runtime_type: 'acp_agent',
      runtime_metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
      },
    }
    api.fetchSessions.mockResolvedValue({ items: [acpSession], nextCursor: null })
    api.fetchSession.mockResolvedValue(acpSession)
    api.fetchMessagesUI.mockResolvedValue([])
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'error',
        message: 'backend fallback',
        feedback: {
          code: 'acp.config_update_failed',
          args: {},
          detail: 'backend fallback',
        },
      } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.selectSession('session-1')
    const result = await store.sendMessage('hello')

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      error: 'The external agent could not apply the selected settings. Please retry.',
      errorCode: 'acp.config_update_failed',
    })
    expect(store.messages).toHaveLength(0)
    expect(store.startupSendFailure).toMatchObject({
      error: 'The external agent could not apply the selected settings. Please retry.',
      restoreInput: 'hello',
    })
  })

  it('uses structured API feedback for startup send failures', async () => {
    api.createSession.mockRejectedValueOnce({
      body: {
        i18n_key: 'chat.acp.agentNotConfigured',
        message: 'raw backend message',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('hello')

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      error: 'External agent setup is incomplete for this bot.',
      restoreInput: 'hello',
    })
    expect(store.startupSendFailure).toMatchObject({
      error: 'External agent setup is incomplete for this bot.',
      restoreInput: 'hello',
    })
  })

  it('handles /new codex in WebUI without sending it to the model', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/new codex')
    applyLatestDraftRequest(store)

    expect(result.ok).toBe(true)
    expect(api.createSession).not.toHaveBeenCalled()
    expect(sentWSMessages).toHaveLength(0)
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'codex',
      project_path: '/data',
      acp_project_mode: 'project',
    })
  })

  it('handles /new codex from an existing session as a fresh ACP composer', async () => {
    sendEvents = [{ type: 'end' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Existing', type: 'chat' }],
      nextCursor: null,
    })
    api.createSession.mockResolvedValueOnce({
      id: 'acp-session-1',
      bot_id: 'bot-1',
      title: '',
      type: 'acp_agent',
      runtime_type: 'acp_agent',
      runtime_metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.messages.push({
      id: 'existing-user',
      role: 'user',
      text: 'old message',
      attachments: [],
      timestamp: new Date().toISOString(),
      streaming: false,
      isSelf: true,
    })

    const commandResult = await store.sendMessage('/new codex')
    applyLatestDraftRequest(store)

    expect(commandResult.ok).toBe(true)
    expect(store.sessionId).toBeNull()
    expect(store.messages).toHaveLength(0)
    expect(store.pendingACPSessionMetadata?.acp_agent_id).toBe('codex')

    const sendResult = await store.sendMessage('hello codex')

    expect(sendResult.ok).toBe(true)
    expect(api.createSession).toHaveBeenCalledWith('bot-1', expect.objectContaining({
      type: 'chat',
      sessionMode: 'chat',
      runtimeType: 'acp_agent',
      runtimeMetadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    }))
    expect(sentWSMessages.at(-1)).toMatchObject({
      session_id: 'acp-session-1',
      text: 'hello codex',
    })
  })

  it('handles /new chat codex in WebUI as a fresh ACP chat composer', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/new chat codex')
    applyLatestDraftRequest(store)

    expect(result.ok).toBe(true)
    expect(sentWSMessages).toHaveLength(0)
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata?.acp_agent_id).toBe('codex')
    expect(store.activeChatTarget).toMatchObject({
      kind: 'draft-acp',
      runtimeType: 'acp_agent',
      isACP: true,
      isPendingACP: true,
      metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
  })

  it('keeps draft activation eligible for default ACP without clearing staged ACP', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })
    store.selectDraft({ explicitSelection: false })

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata?.acp_agent_id).toBe('codex')
    expect(store.hasExplicitSessionSelection).toBe(false)
    expect(store.activeChatTarget).toMatchObject({
      kind: 'draft-acp',
      explicitSelection: false,
      runtimeType: 'acp_agent',
    })
  })

  it('restages the bot default ACP when opening a non-explicit draft after an ACP session', async () => {
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
        chat_acp_project_path: '/data',
        chat_acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.createACPSession({ agentId: 'codex' })

    expect(store.sessionId).toBe('session-1')
    expect(store.pendingACPSessionMetadata).toBeNull()
    expect(store.hasExplicitSessionSelection).toBe(true)
    sdk.getBotsByBotIdSettings.mockClear()

    store.selectDraft({ explicitSelection: false })

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'codex',
      project_path: '/data',
      acp_project_mode: 'project',
    })
    expect(store.hasExplicitSessionSelection).toBe(false)
    expect(sdk.getBotsByBotIdSettings).not.toHaveBeenCalled()
    expect(store.activeChatTarget).toMatchObject({
      kind: 'draft-acp',
      explicitSelection: false,
      metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
  })

  it('keeps an explicit draft as Memoh even when the bot default runtime is ACP', async () => {
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.createACPSession({ agentId: 'codex' })
    sdk.getBotsByBotIdSettings.mockClear()

    store.selectDraft({ explicitSelection: true })
    await flushPromises()

    expect(sdk.getBotsByBotIdSettings).not.toHaveBeenCalled()
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toBeNull()
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(store.activeChatTarget).toMatchObject({
      kind: 'draft-native',
      explicitSelection: true,
      runtimeType: 'model',
      isACP: false,
    })
  })

  it('treats bare /new as an explicit empty composer override', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })
    const result = await store.sendMessage('/new')
    applyLatestDraftRequest(store)

    expect(result.ok).toBe(true)
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toBeNull()
    expect(store.hasExplicitSessionSelection).toBe(true)
  })

  it('uses matching default ACP project settings for /new codex', async () => {
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
        chat_acp_project_path: '/data/custom',
        chat_acp_project_mode: 'project',
      },
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/new codex')
    applyLatestDraftRequest(store)

    expect(result.ok).toBe(true)
    expect(store.pendingACPSessionMetadata).toMatchObject({
      acp_agent_id: 'codex',
      project_path: '/data/custom',
      acp_project_mode: 'project',
    })
  })

  it('handles /new discuss codex in WebUI as a fresh ACP discuss composer', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/new discuss codex')
    applyLatestDraftRequest(store)

    expect(result.ok).toBe(true)
    expect(sentWSMessages).toHaveLength(0)
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata?.acp_agent_id).toBe('codex')

    sendEvents = [{ type: 'end' } as UIStreamEvent]
    const sendResult = await store.sendMessage('start discuss')

    expect(sendResult.ok).toBe(true)
    expect(api.createSession).toHaveBeenCalledWith('bot-1', expect.objectContaining({
      type: 'discuss',
      sessionMode: 'discuss',
      runtimeType: 'acp_agent',
    }))
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

    sendEvents = [{ type: 'command_result', action_id: 'tool_approval_response', terminal: true } as UIStreamEvent]
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

    // The approval side-band command fails before the server applies the decision.
    sendEvents = [{
      type: 'command_error',
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'approval failed' },
    } as UIStreamEvent]
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
    sendEvents = [{ type: 'command_result', action_id: 'tool_approval_response', terminal: true } as UIStreamEvent]
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

  it('rolls back a detached approval turn when the response fails after switching sessions', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'Approval', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'Other', type: 'chat' },
      ],
      nextCursor: null,
    })
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      {
        type: 'message',
        data: {
          id: 1,
          type: 'tool',
          name: 'exec',
          input: { command: 'pwd' },
          tool_call_id: 'call-detached',
          running: false,
          approval: {
            approval_id: 'approval-detached',
            short_id: 12,
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
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    if (!assistant || assistant.role !== 'assistant') throw new Error('approval turn was not streamed')
    const tool = assistant.messages.find(block => block.type === 'tool')
    if (!tool?.approval) throw new Error('approval block was not streamed')

    sendEvents = [{ type: 'start' } as UIStreamEvent]
    await store.respondToolApproval(tool.approval, 'approve')
    const responseStreamId = sentWSMessages.at(-1)?.stream_id as string
    await store.selectSession('session-2')
    streamHandler?.({
      type: 'error',
      stream_id: responseStreamId,
      session_id: 'session-1',
      message: 'approval failed',
    } as UIStreamEvent)
    await flushPromises()
    await flushPromises()

    await store.selectSession('session-1')
    await flushPromises()
    await flushPromises()

    const assistantTurns = store.messages.filter(turn => turn.role === 'assistant')
    expect(assistantTurns).toHaveLength(1)
    expect(assistantTurns[0]).toBe(assistant)
    expect(assistant.streaming).toBe(true)
    expect(tool.approval).toMatchObject({
      approval_id: 'approval-detached',
      status: 'pending',
      can_approve: true,
    })

    store.abort()
    const originalStreamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: originalStreamId,
      seq: 1,
      snapshot: runtimeSnapshotFromScript([], 'session-1', originalStreamId, 'aborted', 1),
    } as UIStreamEvent)
    await expect(sendPromise).resolves.toMatchObject({ ok: false, stage: 'stream' })
    expect(assistant.streaming).toBe(false)
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

  it('does not create a second active run for a visible approval response', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const approval: UIToolApproval = {
      approval_id: 'approval-pwd',
      short_id: 9,
      status: 'pending',
      can_approve: true,
    }
    store.messages.push(approvalTurn(approval))
    await expect(store.respondToolApproval(approval, 'approve')).resolves.toBe(true)
    const responseMessage = sentWSMessages.find(message => message.type === 'tool_approval_response')
    const responseStreamId = responseMessage?.stream_id as string
    const ws = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }

    store.abort()
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'response failed' },
    } as UIStreamEvent)
    await flushPromises()

    expect(ws.abort).not.toHaveBeenCalled()
    expect(store.streaming).toBe(false)
    const block = store.messages[0]?.role === 'assistant' ? store.messages[0].messages[0] : null
    expect(block?.type).toBe('tool')
    if (block?.type !== 'tool' || !block.approval) throw new Error('approval block missing')
    expect(block.approval).toMatchObject({ status: 'pending', can_approve: true })

    sendEvents = [{ type: 'command_result', action_id: 'tool_approval_response', terminal: true } as UIStreamEvent]
    await expect(store.respondToolApproval(block.approval, 'approve')).resolves.toBe(true)
  })

  it('aborts only the active run while an approval command is in flight', async () => {
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
    const runtimeEvents = structuredClone(sendEvents)
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('run pwd')
    await flushPromises()
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    if (!assistant || assistant.role !== 'assistant') throw new Error('assistant turn missing')
    const tool = assistant.messages.find(block => block.type === 'tool')
    if (!tool?.approval) throw new Error('approval block missing')
    const originalStreamId = sentWSMessages[0]?.stream_id as string

    sendEvents = [{ type: 'start' } as UIStreamEvent]
    await expect(store.respondToolApproval(tool.approval, 'approve')).resolves.toBe(true)
    const responseMessage = sentWSMessages.find(message => message.type === 'tool_approval_response')
    const responseStreamId = responseMessage?.stream_id as string
    const ws = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }
    const messageCount = store.messages.length

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: originalStreamId,
      seq: 1,
      snapshot: runtimeSnapshotFromScript(runtimeEvents, 'session-1', originalStreamId, 'running', 1),
    } as UIStreamEvent)
    store.abort()
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'run stopped' },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: originalStreamId,
      seq: 2,
      snapshot: runtimeSnapshotFromScript(runtimeEvents, 'session-1', originalStreamId, 'aborted', 2),
    } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })
    await flushPromises()

    expect(ws.abort).toHaveBeenCalledTimes(1)
    expect(ws.abort).toHaveBeenCalledWith(originalStreamId, 'session-1', `generation-${originalStreamId}`)
    expect(tool.approval).toMatchObject({ status: 'pending', can_approve: true })
    expect(store.messages).toHaveLength(messageCount)

    streamHandler?.({
      type: 'message',
      stream_id: responseStreamId,
      session_id: 'session-1',
      data: { id: 99, type: 'text', content: 'late approval output' },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'error',
      stream_id: responseStreamId,
      session_id: 'session-1',
      message: 'late approval error',
    } as UIStreamEvent)
    expect(store.messages).toHaveLength(messageCount)
    expect(toast.error).not.toHaveBeenCalledWith('late approval error')
  })

  it('does not optimistically submit tool approval while websocket is disconnected', async () => {
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
    const approval: UIToolApproval = {
      approval_id: 'approval-pwd',
      short_id: 9,
      status: 'pending',
      can_approve: true,
    }
    store.messages.push(approvalTurn(approval))

    const result = await store.respondToolApproval(approval, 'approve')
    await flushPromises()

    expect(result).toBe(false)
    expect(sentWSMessages.filter(message => message.type !== 'runtime_subscribe')).toHaveLength(0)
    expect(toast.error).toHaveBeenCalledWith('Connection lost. Reconnect and try again.')
    expect(store.messages).toHaveLength(1)
    const block = store.messages[0]?.role === 'assistant' ? store.messages[0].messages[0] : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.approval).toMatchObject({ status: 'pending', can_approve: true })
    }
  })

  it('rolls back tool approval optimistic state when websocket send throws', async () => {
    api.connectWebSocket.mockImplementationOnce((_botId: string, _onStreamEvent: UIStreamEventHandler) => ({
      get connected() {
        return true
      },
      send: vi.fn(() => {
        throw new Error('send failed')
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
    const approval: UIToolApproval = {
      approval_id: 'approval-pwd',
      short_id: 9,
      status: 'pending',
      can_approve: true,
    }
    store.messages.push(approvalTurn(approval))

    const result = await store.respondToolApproval(approval, 'approve')
    await flushPromises()

    expect(result).toBe(false)
    expect(toast.error).toHaveBeenCalledWith('send failed')
    expect(store.messages).toHaveLength(1)
    const block = store.messages[0]?.role === 'assistant' ? store.messages[0].messages[0] : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.approval).toMatchObject({ status: 'pending', can_approve: true })
    }
  })

  it('replays an uncertain approval without exposing a conflicting decision', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const snapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'exec',
        tool_call_id: 'call-approval-reconnect',
        running: false,
        approval: {
          approval_id: 'approval-reconnect',
          short_id: 9,
          status: 'pending',
          can_approve: true,
        },
      },
    } as UIStreamEvent], 'session-1', 'stream-approval-reconnect', 'running', 10)
    const snapshotEvent = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-approval-reconnect',
      seq: 10,
      snapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(snapshotEvent))

    const pending = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.approval?.approval_id === 'approval-reconnect')
    if (pending?.type !== 'tool' || !pending.approval) throw new Error('pending approval was not projected')
    expect(await store.respondToolApproval(pending.approval, 'approve')).toBe(true)
    expect(pending.approval.status).toBe('approved')
    const initialResponse = sentWSMessages.find(message => message.type === 'tool_approval_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    websocket.onClose?.()
    websocket.onOpen?.()
    streamHandler?.(structuredClone(snapshotEvent))

    const replayedResponses = sentWSMessages.filter(message => message.type === 'tool_approval_response')
    expect(replayedResponses).toHaveLength(2)
    expect(replayedResponses[1]).toMatchObject({
      stream_id: responseStreamId,
      session_id: 'session-1',
      approval_id: 'approval-reconnect',
      short_id: 9,
      decision: 'approve',
    })
    const unresolved = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.approval?.approval_id === 'approval-reconnect')
    if (unresolved?.type !== 'tool' || !unresolved.approval) throw new Error('uncertain approval was not retained')
    expect(unresolved.approval.status).toBe('approved')
    expect(await store.respondToolApproval(unresolved.approval, 'reject')).toBe(false)
    expect(sentWSMessages.filter(message => message.type === 'tool_approval_response')).toHaveLength(2)

    streamHandler?.({
      type: 'command_result',
      invocation_id: responseStreamId,
      action_id: 'tool_approval_response',
      terminal: true,
    } as UIStreamEvent)
    expect(unresolved.approval.status).toBe('approved')
  })

  it('reconciles a lost approval result after a newer run replaces the old run', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const oldSnapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'exec',
        tool_call_id: 'call-approval-rollover',
        running: false,
        approval: {
          approval_id: 'approval-rollover',
          short_id: 11,
          status: 'pending',
          can_approve: true,
        },
      },
    } as UIStreamEvent], 'session-1', 'stream-old-run', 'running', 10)
    const oldSnapshotEvent = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-old-run',
      seq: 10,
      snapshot: oldSnapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(oldSnapshotEvent))
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'older-user-turn',
      role: 'user',
      text: 'older context',
      timestamp: '2026-01-01T00:00:00.000Z',
    } as UITurn])
    expect(await store.loadOlderMessages()).toBe(1)

    const approvalBlock = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.approval?.approval_id === 'approval-rollover')
    if (approvalBlock?.type !== 'tool' || !approvalBlock.approval) throw new Error('pending approval was not projected')
    expect(await store.respondToolApproval(approvalBlock.approval, 'approve')).toBe(true)
    const responseStreamId = String(sentWSMessages.find(message => message.type === 'tool_approval_response')?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    websocket.onClose?.()
    websocket.onOpen?.()
    streamHandler?.(structuredClone(oldSnapshotEvent))
    const replayedResponses = sentWSMessages.filter(message => message.type === 'tool_approval_response')
    expect(replayedResponses).toHaveLength(2)
    expect(replayedResponses[1]?.stream_id).toBe(responseStreamId)

    const approvedHistory = [approvalTurn({
      approval_id: 'approval-rollover',
      short_id: 11,
      status: 'approved',
      can_approve: false,
    })] as UITurn[]
    const history = deferred<UITurn[]>()
    api.fetchMessagesUI.mockImplementation(() => history.promise)
    const newSnapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: { id: 0, type: 'text', content: 'new run output' },
    } as UIStreamEvent], 'session-1', 'stream-new-run', 'running', 20)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-new-run',
      seq: 20,
      snapshot: newSnapshot,
    } as UIStreamEvent)
    const subscriptionsBeforeError = runtimeSubscribeMessages.length
    const historyCallsBeforeError = api.fetchMessagesUI.mock.calls.length
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'approval request not found' },
    } as UIStreamEvent)

    const lockedApproval = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.approval?.approval_id === 'approval-rollover')
    if (lockedApproval?.type !== 'tool' || !lockedApproval.approval) throw new Error('uncertain approval left the active transcript')
    expect(lockedApproval.approval).toMatchObject({ status: 'approved', can_approve: false })
    expect(await store.respondToolApproval(lockedApproval.approval, 'reject')).toBe(false)
    expect(runtimeSubscribeMessages.length).toBeGreaterThan(subscriptionsBeforeError)
    expect(toast.error).not.toHaveBeenCalled()
    history.resolve(approvedHistory)
    await flushPromises()
    await flushPromises()
    expect(api.fetchMessagesUI.mock.calls.length).toBeGreaterThan(historyCallsBeforeError)

    const settledApproval = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.approval?.approval_id === 'approval-rollover')
    expect(settledApproval?.type === 'tool' ? settledApproval.approval : undefined).toMatchObject({
      status: 'approved',
      can_approve: false,
    })
    expect(store.messages.some(turn =>
      turn.role === 'assistant'
      && turn.messages.some(block => block.type === 'text' && block.content === 'new run output'),
    )).toBe(true)
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'late replay failure' },
    } as UIStreamEvent)
    expect(toast.error).not.toHaveBeenCalled()

    websocket.onClose?.()
    websocket.onOpen?.()
    const historyCallsBeforeSettledReconnect = api.fetchMessagesUI.mock.calls.length
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-new-run',
      seq: 20,
      snapshot: newSnapshot,
    } as UIStreamEvent)
    await flushPromises()
    expect(sentWSMessages.filter(message => message.type === 'tool_approval_response')).toHaveLength(2)
    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(historyCallsBeforeSettledReconnect)
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
      type: 'chat',
      sessionMode: 'chat',
      runtimeType: 'acp_agent',
      metadata: {},
      runtimeMetadata: {
        acp_agent_id: 'codex',
        project_path: '/data/app',
        acp_project_mode: 'project',
      },
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
      type: 'chat',
      sessionMode: 'chat',
      runtimeType: 'acp_agent',
      metadata: {},
      runtimeMetadata: {
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
      type: 'chat',
      sessionMode: 'chat',
      runtimeType: 'acp_agent',
      metadata: {},
      runtimeMetadata: {
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

  it('keeps a pending default ACP stage across session list initialization refreshes', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })

    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })

    await store.initialize()

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'codex',
      project_path: '/data',
      acp_project_mode: 'project',
    })
    expect(store.hasExplicitSessionSelection).toBe(false)
    expect(api.createACPRuntime).not.toHaveBeenCalled()
  })

  it('allows default ACP staging to override a restored historical session selection', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('history-session-1')
    const store = useChatStore()

    await store.initialize()

    expect(store.sessionId).toBe('history-session-1')
    expect(store.hasExplicitSessionSelection).toBe(false)

    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'codex',
      project_path: '/data',
      acp_project_mode: 'project',
    })
    expect(store.hasExplicitSessionSelection).toBe(false)
  })

  it('does not restore an auto-picked historical session when default chat runtime is ACP', async () => {
    api.fetchSessions.mockResolvedValue({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
      },
    })
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('history-session-1')
    const store = useChatStore()

    await store.initialize()
    await flushPromises()

    expect(sdk.getBotsByBotIdSettings).toHaveBeenCalled()
    expect(store.sessionId).toBeNull()
    expect(store.hasExplicitSessionSelection).toBe(false)
    expect(store.messages).toEqual([])
    expect(api.fetchMessagesUI).not.toHaveBeenCalled()
  })

  it('restores an explicitly selected historical session when default chat runtime is ACP', async () => {
    api.fetchSessions.mockResolvedValue({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
      },
    })
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('history-session-1', { explicitSelection: true })
    const store = useChatStore()

    await store.initialize()

    expect(sdk.getBotsByBotIdSettings).toHaveBeenCalled()
    expect(store.sessionId).toBe('history-session-1')
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(store.pendingACPSessionMetadata).toBeNull()
  })

  it('hydrates an explicitly restored ACP session that is outside the first session page', async () => {
    api.fetchSessions.mockImplementation(async () => ({
      items: [{
        id: 'visible-session-1',
        bot_id: 'bot-1',
        title: 'Visible',
        type: 'chat',
        session_mode: 'chat',
        runtime_type: 'model',
      }],
      nextCursor: 'next-page',
    }))
    api.fetchSession.mockResolvedValueOnce({
      id: 'acp-session-hidden',
      bot_id: 'bot-1',
      title: 'Codex',
      type: 'chat',
      session_mode: 'chat',
      runtime_type: 'acp_agent',
      runtime_metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    sdk.getBotsByBotIdSettings.mockResolvedValue({
      data: {
        chat_runtime: 'acp_agent',
        chat_acp_agent_id: 'codex',
      },
    })
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('acp-session-hidden', { explicitSelection: true })
    const store = useChatStore()

    await store.initialize()
    await flushPromises()

    expect(store.sessionId).toBe('acp-session-hidden')
    expect(api.fetchSession).toHaveBeenCalledWith('bot-1', 'acp-session-hidden')
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(store.activeSession).toMatchObject({
      id: 'acp-session-hidden',
      runtime_type: 'acp_agent',
      runtime_metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
    expect(store.activeChatTarget).toMatchObject({
      kind: 'session',
      sessionId: 'acp-session-hidden',
      runtimeType: 'acp_agent',
      isACP: true,
      metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
    expect(store.pendingACPSessionMetadata).toBeNull()
  })

  it('updates an early-read active target when a restored ACP session arrives in the first page', async () => {
    const sessionsResponse = {
      items: [{
        id: 'acp-session-visible',
        bot_id: 'bot-1',
        title: 'Codex visible',
        type: 'chat',
        session_mode: 'chat',
        runtime_type: 'acp_agent',
        runtime_metadata: {
          acp_agent_id: 'codex',
          project_path: '/data',
          acp_project_mode: 'project',
        },
      }],
      nextCursor: null,
    }
    let resolveSessions!: (value: typeof sessionsResponse) => void
    api.fetchSessions.mockImplementation(() => new Promise(resolve => {
      resolveSessions = resolve
    }))
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('acp-session-visible', { explicitSelection: true })
    const store = useChatStore()

    expect(store.activeChatTarget).toMatchObject({
      kind: 'session',
      sessionId: 'acp-session-visible',
      runtimeType: 'unknown',
      isACP: false,
    })

    await flushPromises()
    resolveSessions(sessionsResponse)
    await flushPromises()
    await flushPromises()

    expect(store.activeSession).toMatchObject({
      id: 'acp-session-visible',
      runtime_type: 'acp_agent',
    })
    expect(store.activeChatTarget).toMatchObject({
      kind: 'session',
      sessionId: 'acp-session-visible',
      runtimeType: 'acp_agent',
      isACP: true,
      metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
  })

  it('keeps an explicit empty Memoh composer across session list initialization refreshes', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })
    store.resetToEmptyComposer({ explicitSelection: true })

    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })

    await store.initialize()

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toBeNull()
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(api.createACPRuntime).not.toHaveBeenCalled()
  })

  it('keeps a manually staged ACP agent explicit so the default stage cannot reclaim it', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageDefaultACPSession({ agentId: 'codex', projectPath: '/data', projectMode: 'project' })
    store.stageACPSession({ agentId: 'claude-code', projectPath: '/data/other', projectMode: 'project' })

    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'history-session-1',
        bot_id: 'bot-1',
        title: 'History',
        type: 'chat',
      }],
      nextCursor: null,
    })

    await store.initialize()

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPSessionMetadata).toEqual({
      acp_agent_id: 'claude-code',
      project_path: '/data/other',
      acp_project_mode: 'project',
    })
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(api.createACPRuntime).not.toHaveBeenCalled()
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
    expect(store.pendingACPRuntimeStatus?.models?.current_model_id).toBe('gpt-5.1-codex-high')
    expect(api.setACPRuntimeModelByID).toHaveBeenCalledWith('bot-1', 'rt_warm', 'gpt-5.1-codex-high')

    await store.setPendingACPReasoning('high')
    expect(store.pendingACPRuntimeStatus?.reasoning?.current_effort).toBe('high')
    expect(api.setACPRuntimeReasoningByID).toHaveBeenCalledWith('bot-1', 'rt_warm', 'high')

    // Binding rides on session creation. The turn carries the selected model,
    // so send does not need another runtime setup request.
    const result = await store.sendMessage('hello codex', undefined, {
      modelId: 'gpt-5.1-codex-high',
      reasoningEffort: 'high',
    })

    expect(result.ok).toBe(true)
    expect(api.createSession).toHaveBeenCalledTimes(1)
    expect(api.createSession).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      type: 'chat',
      sessionMode: 'chat',
      runtimeType: 'acp_agent',
      acpRuntimeId: 'rt_warm',
    }))
    expect(api.setACPRuntimeModel).not.toHaveBeenCalled()
    expect(api.closeACPRuntime).not.toHaveBeenCalled()
    expect(sentWSMessages[0]).toMatchObject({
      session_id: 'acp-session-1',
      reasoning_effort: 'high',
      text: 'hello codex',
      model_id: 'gpt-5.1-codex-high',
    })
  })

  it('refreshes a staged runtime instead of reusing a stale capability snapshot', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()

    api.fetchACPRuntimeByID.mockResolvedValueOnce({
      runtime_id: 'rt_warm',
      agent_id: 'codex',
      state: 'idle',
      models: {
        current_model_id: 'gpt-5.1-codex-high',
        available_models: [{ id: 'gpt-5.1-codex-high', name: 'GPT-5.1 Codex High' }],
      },
      reasoning: {
        current_effort: 'xhigh',
        available_efforts: [{ id: 'xhigh', name: 'Extra high' }],
      },
    })

    const refreshed = await store.ensurePendingACPRuntime()

    expect(api.fetchACPRuntimeByID).toHaveBeenCalledWith('bot-1', 'rt_warm')
    expect(api.createACPRuntime).toHaveBeenCalledTimes(1)
    expect(refreshed?.models?.current_model_id).toBe('gpt-5.1-codex-high')
    expect(store.pendingACPRuntimeStatus?.reasoning?.current_effort).toBe('xhigh')
  })

  it('recreates a staged runtime when capability refresh reports it was reaped', async () => {
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
        models: { current_model_id: 'gpt-5.1-codex-high', available_models: [] },
      })
    api.fetchACPRuntimeByID.mockRejectedValueOnce({ body: { code: 'acp.runtime_not_found' } })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    const recreated = await store.ensurePendingACPRuntime()

    expect(api.fetchACPRuntimeByID).toHaveBeenCalledWith('bot-1', 'rt_warm')
    expect(api.createACPRuntime).toHaveBeenCalledTimes(2)
    expect(recreated?.runtime_id).toBe('rt_fresh')
    expect(store.pendingACPRuntimeId).toBe('rt_fresh')
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
    expect(store.pendingACPRuntimeId).toBe('rt_new')
  })

  it('leaves staged runtime creation retryable when a model pick cannot start it', async () => {
    api.createACPRuntime.mockRejectedValueOnce({ message: 'runtime create failed' })
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })

    await expect(store.setPendingACPModel('gpt-5.1-codex-high')).rejects.toMatchObject({
      message: 'runtime create failed',
    })
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
      .mockRejectedValueOnce({ body: { code: 'acp.runtime_not_found' } })
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
    expect(store.pendingACPRuntimeStatus?.models?.current_model_id).toBe('gpt-5.1-codex-high')
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

  it('responds to user input over websocket and marks the block answered', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{
      type: 'command_result',
      action_id: 'user_input_response',
      terminal: true,
    } as UIStreamEvent]
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
    expect(await store.respondUserInput(singleSelectUserInput(), {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)
  })

  it('sends only one response when user input is submitted twice', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))

    const [first, second] = await Promise.all([
      store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] }),
      store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] }),
    ])

    expect(first).toBe(true)
    expect(second).toBe(false)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(1)
  })

  it('replays an uncertain ask-user response when reconnect snapshot remains pending', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const epoch = 'epoch-ask-reconnect'
    const runtimeSnapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'ask_user',
        tool_call_id: 'call-ask',
        running: false,
        user_input: singleSelectUserInput(),
      },
    } as UIStreamEvent], 'session-1', 'stream-ask-reconnect', 'running', 10)
    runtimeSnapshot.epoch = epoch
    const reconnectSnapshotEvent = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-ask-reconnect',
      epoch,
      seq: 10,
      snapshot: runtimeSnapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(reconnectSnapshotEvent))

    const pending = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool'
        && block.userInput?.user_input_id === 'input-1'
        && block.userInput.status === 'pending')
    expect(pending?.type).toBe('tool')
    if (pending?.type !== 'tool' || !pending.userInput) throw new Error('pending user input was not projected')
    expect(await store.respondUserInput(pending.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    expect(pending.userInput.status).toBe('submitted')
    const initialResponse = sentWSMessages.find(message => message.type === 'user_input_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    websocket.onClose?.()
    websocket.onOpen?.()
    streamHandler?.(structuredClone(reconnectSnapshotEvent))

    const replayedResponses = sentWSMessages.filter(message => message.type === 'user_input_response')
    expect(replayedResponses).toHaveLength(2)
    expect(replayedResponses[1]).toMatchObject({
      type: 'user_input_response',
      stream_id: responseStreamId,
      session_id: 'session-1',
      user_input_id: 'input-1',
      short_id: 4,
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
      canceled: false,
    })
    const refreshed = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool'
        && block.userInput?.user_input_id === 'input-1'
        && block.userInput.status === 'submitted')
    expect(refreshed?.type).toBe('tool')
    if (refreshed?.type !== 'tool' || !refreshed.userInput) throw new Error('uncertain user input was not replayed')
    expect(await store.respondUserInput(refreshed.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(false)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)
  })

  it('retries an ask-user replay when the first hydration arrives before the websocket is connected', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const epoch = 'epoch-ask-replay-connect-race'
    const snapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'ask_user',
        tool_call_id: 'call-ask-connect-race',
        running: false,
        user_input: singleSelectUserInput(),
      },
    } as UIStreamEvent], 'session-1', 'stream-ask-connect-race', 'running', 10)
    snapshot.epoch = epoch
    const event = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-ask-connect-race',
      epoch,
      seq: 10,
      snapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(event))

    const pending = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.userInput?.user_input_id === 'input-1')
    if (pending?.type !== 'tool' || !pending.userInput) throw new Error('pending user input was not projected')
    expect(await store.respondUserInput(pending.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    const initialResponse = sentWSMessages.find(message => message.type === 'user_input_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    let connected = true
    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      connected: boolean
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    Object.defineProperty(websocket, 'connected', { configurable: true, get: () => connected })
    websocket.onClose?.()
    websocket.onOpen?.()

    connected = false
    streamHandler?.(structuredClone(event))
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(1)

    connected = true
    streamHandler?.(structuredClone(event))
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toEqual([
      expect.objectContaining({ stream_id: responseStreamId, user_input_id: 'input-1' }),
      expect.objectContaining({ stream_id: responseStreamId, user_input_id: 'input-1' }),
    ])
  })

  it('waits for the websocket checkpoint before replaying an uncertain ask-user response', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const epoch = 'epoch-ask-rest-reconnect'
    const snapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'ask_user',
        tool_call_id: 'call-ask-rest',
        running: false,
        user_input: singleSelectUserInput(),
      },
    } as UIStreamEvent], 'session-1', 'stream-ask-rest', 'running', 10)
    snapshot.epoch = epoch
    const event = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-ask-rest',
      epoch,
      seq: 10,
      snapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(event))
    const pending = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.userInput?.user_input_id === 'input-1')
    if (pending?.type !== 'tool' || !pending.userInput) throw new Error('pending user input was not projected')
    expect(await store.respondUserInput(pending.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    const initialResponse = sentWSMessages.find(message => message.type === 'user_input_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    websocket.onClose?.()
    websocket.onOpen?.()
    await flushPromises()

    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(1)
    streamHandler?.(structuredClone(event))

    const restored = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool'
        && block.userInput?.user_input_id === 'input-1'
        && block.userInput.status === 'submitted')
    if (restored?.type !== 'tool' || !restored.userInput) throw new Error('checkpoint did not replay pending user input')
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toEqual([
      expect.objectContaining({
        type: 'user_input_response',
        stream_id: responseStreamId,
        session_id: 'session-1',
        user_input_id: 'input-1',
        answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
      }),
      expect.objectContaining({
        type: 'user_input_response',
        stream_id: responseStreamId,
        session_id: 'session-1',
        user_input_id: 'input-1',
        answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
      }),
    ])
    expect(await store.respondUserInput(restored.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(false)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)
  })

  it('replays an uncertain ask-user response after a stale checkpoint is followed by the current checkpoint', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const epoch = 'epoch-ask-stale-rest'
    const current = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'ask_user',
        tool_call_id: 'call-ask-stale-rest',
        running: false,
        user_input: singleSelectUserInput(),
      },
    } as UIStreamEvent], 'session-1', 'stream-ask-stale-rest', 'running', 10)
    current.epoch = epoch
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', stream_id: 'stream-ask-stale-rest', epoch, seq: 10, snapshot: current,
    } as UIStreamEvent)
    const pending = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool' && block.userInput?.user_input_id === 'input-1')
    if (pending?.type !== 'tool' || !pending.userInput) throw new Error('pending user input was not projected')
    expect(await store.respondUserInput(pending.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    const initialResponse = sentWSMessages.find(message => message.type === 'user_input_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const stale = structuredClone(current)
    stale.seq = 9
    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }

    websocket.onClose?.()
    websocket.onOpen?.()
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', stream_id: 'stream-ask-stale-rest', epoch, seq: 9, snapshot: stale,
    } as UIStreamEvent)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(1)

    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', stream_id: 'stream-ask-stale-rest', epoch, seq: 10, snapshot: structuredClone(current),
    } as UIStreamEvent)
    const restored = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool'
        && block.userInput?.user_input_id === 'input-1'
        && block.userInput.status === 'submitted')
    if (restored?.type !== 'tool' || !restored.userInput) throw new Error('pending user input was not replayed after current checkpoint')
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)
    expect(await store.respondUserInput(restored.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(false)
  })

  it('replays an in-flight ask-user response after switching bots', async () => {
    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot 1' },
      { id: 'bot-2', status: 'active', name: 'Bot 2' },
    ])
    api.fetchSessions.mockImplementation((botId: string) => Promise.resolve(botId === 'bot-1'
      ? { items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }], nextCursor: null }
      : { items: [], nextCursor: null }))
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))
    expect(await store.respondUserInput(userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(true)
    const initialResponse = sentWSMessages.find(message => message.type === 'user_input_response')
    const responseStreamId = String(initialResponse?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    const epoch = 'epoch-ask-bot-switch'
    const snapshot = runtimeSnapshotFromScript([{
      type: 'message',
      data: {
        id: 1,
        type: 'tool',
        name: 'ask_user',
        tool_call_id: 'call-ask-switch',
        running: false,
        user_input: singleSelectUserInput(),
      },
    } as UIStreamEvent], 'session-1', 'stream-ask-switch', 'running', 10)
    snapshot.epoch = epoch
    const event = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-ask-switch',
      epoch,
      seq: 10,
      snapshot,
    } as UIStreamEvent

    await store.selectBot('bot-2')
    await store.selectBot('bot-1')
    await flushPromises()
    streamHandler?.(structuredClone(event))

    const restored = store.messages
      .flatMap(turn => turn.role === 'assistant' ? turn.messages : [])
      .find(block => block.type === 'tool'
        && block.userInput?.user_input_id === 'input-1'
        && block.userInput.status === 'submitted')
    if (restored?.type !== 'tool' || !restored.userInput) throw new Error('pending user input was not replayed after bot switch')
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toEqual([
      expect.objectContaining({ stream_id: responseStreamId, user_input_id: 'input-1' }),
      expect.objectContaining({ stream_id: responseStreamId, user_input_id: 'input-1' }),
    ])
    expect(await store.respondUserInput(restored.userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })).toBe(false)
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)
  })

  it('cancels user input over websocket and marks the block canceled', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{
      type: 'command_result',
      action_id: 'user_input_response',
      terminal: true,
    } as UIStreamEvent]
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

    expect(sentWSMessages.filter(message => message.type !== 'runtime_subscribe')).toHaveLength(0)
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

  it('rolls back user input optimistic state when websocket send throws', async () => {
    api.connectWebSocket.mockImplementationOnce((_botId: string, _onStreamEvent: UIStreamEventHandler) => ({
      get connected() {
        return true
      },
      send: vi.fn(() => {
        throw new Error('send failed')
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

    expect(toast.error).toHaveBeenCalledWith('send failed')
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
    expect(store.isSessionStreaming('bot-1', 'session-1')).toBe(true)

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

  it('keeps unknown background task snapshots non-active when hydrating messages', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'spawn_agent',
        input: { id: 'agent-a' },
        tool_call_id: 'call-spawn',
        running: false,
        background_task: {
          task_id: 'bg-1',
          status: 'unknown',
          agent_id: 'agent-a',
          agent_session_id: 'child-1',
        },
      }],
      timestamp: new Date().toISOString(),
    }])
    const store = useChatStore()

    await store.selectBot('bot-1')

    const tool = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(tool?.type).toBe('tool')
    if (tool?.type === 'tool') {
      expect(tool.backgroundTask?.status).toBe('unknown')
      expect(tool.running).toBe(false)
      expect(tool.done).toBe(true)
    }
  })

  it('hydrates skill activation user turns after a page refresh when kind is omitted', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'skill-user-1',
        role: 'user',
        text: '/flutter-adding-home-screen-widgets',
        skill_activation: {
          skills: [{
            name: 'flutter-adding-home-screen-widgets',
            display_name: 'Flutter adding home screen widgets',
            source_kind: 'managed',
            state: 'effective',
          }],
        },
        attachments: [],
        timestamp: '2026-07-03T00:00:00.000Z',
      },
      {
        id: 'skill-user-2',
        role: 'user',
        text: '/flutter-adding-home-screen-widgets please add widgets',
        skill_activation: {
          skills: [{
            name: 'flutter-adding-home-screen-widgets',
            display_name: 'Flutter adding home screen widgets',
            source_kind: 'managed',
            state: 'effective',
          }],
        },
        attachments: [],
        timestamp: '2026-07-03T00:00:01.000Z',
      },
      {
        id: 'skill-user-3',
        role: 'user',
        text: 'The user activated the following skill for this turn without an additional prompt: Flutter adding home screen widgets.',
        skill_activation: {
          skills: [{
            name: 'flutter-adding-home-screen-widgets',
            display_name: 'Flutter adding home screen widgets',
            source_kind: 'managed',
            state: 'effective',
          }],
        },
        attachments: [],
        timestamp: '2026-07-03T00:00:02.000Z',
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')

    expect(store.messages).toHaveLength(3)
    expect(store.messages[0]).toMatchObject({
      id: 'skill-user-1',
      role: 'user',
      text: '',
      userMessageKind: 'skill_activation',
      skillActivation: {
        skills: [{
          name: 'flutter-adding-home-screen-widgets',
          display_name: 'Flutter adding home screen widgets',
        }],
      },
    })
    expect(store.messages[1]).toMatchObject({
      id: 'skill-user-2',
      role: 'user',
      text: 'please add widgets',
      userMessageKind: 'skill_activation',
      skillActivation: {
        skills: [{
          name: 'flutter-adding-home-screen-widgets',
          display_name: 'Flutter adding home screen widgets',
        }],
      },
    })
    expect(store.messages[2]).toMatchObject({
      id: 'skill-user-3',
      role: 'user',
      text: '',
      userMessageKind: 'skill_activation',
    })
  })

  it('applies live background task output and completion events to existing tool blocks', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'assistant-1',
      role: 'assistant',
      messages: [{
        id: 1,
        type: 'tool',
        name: 'spawn_agent',
        input: { id: 'agent-a' },
        tool_call_id: 'call-spawn',
        running: true,
        background_task: {
          task_id: 'bg-1',
          status: 'running',
          agent_id: 'agent-a',
          agent_session_id: 'child-1',
        },
      }],
      timestamp: new Date().toISOString(),
    }])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    _sessionMessageHandler?.({
      type: 'background_task',
      event: 'output',
      session_id: 'session-1',
      task: {
        task_id: 'bg-1',
        status: 'running',
        session_id: 'session-1',
        agent_id: 'agent-a',
        agent_session_id: 'child-1',
        chunk: 'first line\n',
      },
    } as never)

    let tool = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(tool?.type).toBe('tool')
    if (tool?.type === 'tool') {
      expect(tool.backgroundTask?.status).toBe('running')
      expect(tool.backgroundTask?.outputTail).toBe('first line\n')
      expect(tool.background_task?.output_tail).toBe('first line\n')
      expect(tool.running).toBe(true)
      expect(tool.done).toBe(false)
    }

    _sessionMessageHandler?.({
      type: 'background_task',
      event: 'completed',
      session_id: 'session-1',
      task: {
        task_id: 'bg-1',
        status: 'completed',
        session_id: 'session-1',
        agent_id: 'agent-a',
        agent_session_id: 'child-1',
        output_tail: 'final line\n',
      },
    } as never)

    tool = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(tool?.type).toBe('tool')
    if (tool?.type === 'tool') {
      expect(tool.backgroundTask?.status).toBe('completed')
      expect(tool.backgroundTask?.outputTail).toBe('final line\n')
      expect(tool.background_task?.output_tail).toBe('final line\n')
      expect(tool.running).toBe(false)
      expect(tool.done).toBe(true)
    }
  })

  it('updates remembered hidden session summaries from title events reactively', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'parent-1', bot_id: 'bot-1', title: 'Parent', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValue([])
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-subagent',
      bot_id: 'bot-1',
      title: 'Initial subagent title',
      type: 'subagent',
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.selectSession('session-subagent')
    await flushPromises()

    _sessionMessageHandler?.({
      type: 'session_title_updated',
      session_id: 'session-subagent',
      title: 'Updated subagent title',
    } as never)
    await flushPromises()

    expect(store.knownSessionSummary('session-subagent')?.title).toBe('Updated subagent title')
    expect(store.knownSessions.find(session => session.id === 'session-subagent')?.title).toBe('Updated subagent title')
  })

  it('keeps remembered hidden session title updates after the visible session list refreshes', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'parent-1', bot_id: 'bot-1', title: 'Parent', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValue([])
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-subagent',
      bot_id: 'bot-1',
      title: 'Initial subagent title',
      type: 'subagent',
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.selectSession('session-subagent')
    await flushPromises()

    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'parent-1', bot_id: 'bot-1', title: 'Parent', type: 'chat' },
    ], nextCursor: null })
    await store.initialize()
    await flushPromises()

    _sessionMessageHandler?.({
      type: 'session_title_updated',
      session_id: 'session-subagent',
      title: 'Updated subagent title',
    } as never)
    await flushPromises()

    expect(store.knownSessionSummary('session-subagent')?.title).toBe('Updated subagent title')
    expect(store.knownSessions.find(session => session.id === 'session-subagent')?.title).toBe('Updated subagent title')
  })

  it('refreshes recents when a remembered hidden chat session receives activity', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-visible', bot_id: 'bot-1', title: 'Visible', type: 'chat' },
    ], nextCursor: null })
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-hidden',
      bot_id: 'bot-1',
      title: 'Hidden',
      type: 'chat',
      updated_at: '2026-06-01T00:00:00.000Z',
    })
    api.fetchMessagesUI.mockResolvedValue([])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await store.selectSession('session-hidden')
    await flushPromises()

    expect(store.sessions.map(session => session.id)).toEqual(['session-visible'])
    expect(store.knownSessionSummary('session-hidden')).toMatchObject({
      id: 'session-hidden',
      type: 'chat',
    })

    api.fetchSessions.mockResolvedValueOnce({ items: [
      {
        id: 'session-hidden',
        bot_id: 'bot-1',
        title: 'Hidden',
        type: 'chat',
        updated_at: '2026-06-23T10:00:00.000Z',
      },
      { id: 'session-visible', bot_id: 'bot-1', title: 'Visible', type: 'chat' },
    ], nextCursor: null })
    sessionsActivityHandler?.({
      type: 'session_touched',
      session_id: 'session-hidden',
      updated_at: '2026-06-23T10:00:00.000Z',
    })
    await flushPromises()

    expect(api.fetchSessions).toHaveBeenCalledTimes(2)
    expect(store.sessions.map(session => session.id)).toEqual(['session-hidden', 'session-visible'])
  })

  it('rolls back pending user input after a legacy side-band error', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await vi.waitFor(() => expect(api.fetchMessagesUI).toHaveBeenCalled())
    await flushPromises()
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))
    api.fetchMessagesUI.mockClear()

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    await flushPromises()
    await flushPromises()

    expect(api.fetchMessagesUI).not.toHaveBeenCalled()
    const block = store.messages[0]?.role === 'assistant'
      ? store.messages[0].messages[0]
      : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput?.status).toBe('pending')
      expect(block.userInput?.can_respond).toBe(true)
    }
  })

  it('rolls back user input when the side-band command fails without refreshing the session', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    await vi.waitFor(() => expect(api.fetchMessagesUI).toHaveBeenCalled())
    await flushPromises()
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))
    api.fetchMessagesUI.mockClear()

    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    const responseStreamId = sentWSMessages.at(-1)?.stream_id as string
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'response failed' },
    } as UIStreamEvent)
    await flushPromises()

    expect(api.fetchMessagesUI).not.toHaveBeenCalled()
    expect(store.messages).toHaveLength(1)
    const block = store.messages[0]?.role === 'assistant' ? store.messages[0].messages[0] : null
    expect(block?.type).toBe('tool')
    if (block?.type === 'tool') {
      expect(block.userInput).toMatchObject({ status: 'pending', can_respond: true })
    }
  })

  it('isolates the same user-input response stream id across sessions', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:user-input-a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:user-input-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()
    await flushPromises()
    const inputA = singleSelectUserInput('input-a')
    const inputB = singleSelectUserInput('input-b')
    store.chatView(targetA).transcript.messages.push(askUserTurn(inputA, 'call-a'))
    store.chatView(targetB).transcript.messages.push(askUserTurn(inputB, 'call-b'))
    const sharedStreamId = '00000000-0000-4000-8000-000000000001'
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(sharedStreamId)

    expect(await store.respondUserInput(inputA, { canceled: true }, targetA)).toBe(true)
    expect(await store.respondUserInput(inputB, { canceled: true }, targetB)).toBe(true)
    uuid.mockRestore()
    expect(sentWSMessages.filter(message => message.type === 'user_input_response')).toHaveLength(2)

    streamHandler?.({
      type: 'command_result',
      invocation_id: sharedStreamId,
      session_id: 'session-a',
      action_id: 'user_input_response',
      terminal: true,
    } as UIStreamEvent)
    streamHandler?.({
      type: 'error',
      stream_id: sharedStreamId,
      session_id: 'session-a',
      message: 'late completed user-input error',
    } as UIStreamEvent)
    streamHandler?.({
      type: 'command_error',
      invocation_id: sharedStreamId,
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'ambiguous late error' },
    } as UIStreamEvent)

    const blockB = store.chatView(targetB).transcript.messages[0]?.role === 'assistant'
      ? store.chatView(targetB).transcript.messages[0].messages[0]
      : undefined
    expect(blockB?.type === 'tool' ? blockB.userInput : undefined).toMatchObject({
      status: 'canceled',
      can_respond: false,
    })
    expect(toast.error).not.toHaveBeenCalled()

    streamHandler?.({
      type: 'command_error',
      invocation_id: sharedStreamId,
      session_id: 'session-b',
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'session B failed' },
    } as UIStreamEvent)
    expect(blockB?.type === 'tool' ? blockB.userInput : undefined).toMatchObject({
      status: 'pending',
      can_respond: true,
    })
    expect(toast.error).toHaveBeenCalledWith('session B failed')
  })

  it('does not classify an ordinary completed stream as a terminal user-input response', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent, { type: 'end' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const completedStreamId = '00000000-0000-4000-8000-000000000004'
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(completedStreamId)
    await expect(store.sendMessage('ordinary turn')).resolves.toMatchObject({ ok: true })

    sendEvents = []
    const userInput = singleSelectUserInput('reused-stream-input')
    store.messages.push(askUserTurn(userInput, 'reused-stream-call'))
    expect(await store.respondUserInput(userInput, { canceled: true })).toBe(true)
    uuid.mockRestore()
    streamHandler?.({
      type: 'command_error',
      invocation_id: completedStreamId,
      session_id: 'session-1',
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'reused stream response failed' },
    } as UIStreamEvent)

    const block = store.messages.at(-1)?.role === 'assistant'
      ? store.messages.at(-1)?.messages[0]
      : undefined
    expect(block?.type === 'tool' ? block.userInput : undefined).toMatchObject({
      status: 'pending',
      can_respond: true,
    })
    expect(toast.error).toHaveBeenCalledWith('reused stream response failed')
  })

  it('routes side-band results independently from a colliding runtime subscription id', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const subscriptionId = String(runtimeSubscribeMessages.at(-1)?.invocation_id ?? '')
    expect(subscriptionId).not.toBe('')
    const userInput = singleSelectUserInput('subscription-collision-input')
    store.messages.push(askUserTurn(userInput, 'subscription-collision-call'))
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(subscriptionId as `${string}-${string}-${string}-${string}-${string}`)
    expect(await store.respondUserInput(userInput, { canceled: true })).toBe(true)
    uuid.mockRestore()

    streamHandler?.({
      type: 'command_error',
      invocation_id: subscriptionId,
      session_id: 'session-1',
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'side-band collision failed' },
    } as UIStreamEvent)

    const block = store.messages.at(-1)?.role === 'assistant'
      ? store.messages.at(-1)?.messages[0]
      : undefined
    expect(block?.type === 'tool' ? block.userInput : undefined).toMatchObject({
      status: 'pending',
      can_respond: true,
    })
    expect(toast.error).toHaveBeenCalledWith('side-band collision failed')
  })

  it('ignores late user-input results after deleting their session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:delete-input-a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:delete-input-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()
    await flushPromises()
    const inputA = singleSelectUserInput('delete-input-a')
    const inputB = singleSelectUserInput('delete-input-b')
    store.chatView(targetA).transcript.messages.push(askUserTurn(inputA, 'delete-call-a'))
    store.chatView(targetB).transcript.messages.push(askUserTurn(inputB, 'delete-call-b'))
    const sharedStreamId = '00000000-0000-4000-8000-000000000002'
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(sharedStreamId)
    await store.respondUserInput(inputA, { canceled: true }, targetA)
    await store.respondUserInput(inputB, { canceled: true }, targetB)
    uuid.mockRestore()

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-a')
    streamHandler?.({
      type: 'command_error',
      invocation_id: sharedStreamId,
      session_id: 'session-a',
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'deleted session failed' },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'command_error',
      invocation_id: sharedStreamId,
      action_id: 'user_input_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'ambiguous deleted session failure' },
    } as UIStreamEvent)

    const blockB = store.chatView(targetB).transcript.messages[0]?.role === 'assistant'
      ? store.chatView(targetB).transcript.messages[0].messages[0]
      : undefined
    expect(blockB?.type === 'tool' ? blockB.userInput : undefined).toMatchObject({
      status: 'canceled',
      can_respond: false,
    })
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('ignores late approval results after deleting their session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const approval: UIToolApproval = {
      approval_id: 'delete-approval-a', short_id: 19, status: 'pending', can_approve: true,
    }
    store.messages.push(approvalTurn(approval))
    expect(await store.respondToolApproval(approval, 'approve')).toBe(true)
    const responseStreamId = String(sentWSMessages.at(-1)?.stream_id ?? '')
    expect(responseStreamId).not.toBe('')

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-a')
    streamHandler?.({
      type: 'command_error',
      invocation_id: responseStreamId,
      session_id: 'session-a',
      action_id: 'tool_approval_response',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'deleted approval failed' },
    } as UIStreamEvent)

    expect(store.sessionId).toBe('session-b')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('does not refresh a previous bot after user-input teardown', async () => {
    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot 1' },
      { id: 'bot-2', status: 'active', name: 'Bot 2' },
    ])
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' },
    ], nextCursor: null })
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const userInput = singleSelectUserInput()
    store.messages.push(askUserTurn(userInput))
    await store.respondUserInput(userInput, { answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }] })
    api.fetchMessagesUI.mockClear()

    await store.selectBot('bot-2')
    await flushPromises()

    expect(store.currentBotId).toBe('bot-2')
    expect(api.fetchMessagesUI).not.toHaveBeenCalledWith(
      'bot-1',
      'session-1',
      expect.anything(),
    )
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

  it('clears ACP runtime state with the authenticated user scope', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'acp-session-1', bot_id: 'bot-1', title: '', type: 'acp_agent' },
    ], nextCursor: null })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.ensureACPRuntime('acp-session-1')
    const key = store.acpRuntimeKey('bot-1', 'acp-session-1')
    expect(store.acpRuntimeStatuses[key]).toBeDefined()

    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))

    expect(store.acpRuntimeStatuses).toEqual({})
    expect(store.acpRuntimePending).toEqual({})
  })

  it('closes a staged ACP runtime with its owner bot on auth reset', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    await store.ensurePendingACPRuntime()
    expect(store.pendingACPRuntimeId).toBe('rt_warm')

    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))

    expect(api.closeACPRuntime).toHaveBeenCalledWith('bot-1', 'rt_warm')
    expect(store.pendingACPRuntimeId).toBe('')
  })

  it('aborts a colliding run in another session during auth reset', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:reset-a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:reset-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()
    await flushPromises()
    const sharedStreamId = '00000000-0000-4000-8000-000000000005'
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(sharedStreamId)
    const sending = store.sendMessage('run in B', undefined, { target: targetB })
    await flushPromises()
    const approval: UIToolApproval = {
      approval_id: 'approval-in-a', short_id: 21, status: 'pending', can_approve: true,
    }
    store.chatView(targetA).transcript.messages.push(approvalTurn(approval))
    expect(await store.respondToolApproval(approval, 'approve', targetA)).toBe(true)
    uuid.mockRestore()
    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }

    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))

    await expect(sending).resolves.toMatchObject({ ok: false })
    expect(websocket.abort).toHaveBeenCalledWith(sharedStreamId, 'session-b', '')
  })

  it('does not restore bots from an initialization response after auth reset', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    const oldBots = deferred<Array<{ id: string; status: string; name: string }>>()
    api.fetchBots.mockReturnValueOnce(oldBots.promise)
    const store = useChatStore()

    const initializing = store.initialize()
    await flushPromises()
    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))
    oldBots.resolve([{ id: 'old-user-bot', status: 'active', name: 'Old' }])
    await initializing

    expect(store.bots).toEqual([])
    expect(store.currentBotId).toBeNull()
    expect(api.fetchSessions).not.toHaveBeenCalledWith('old-user-bot')
  })

  it('does not apply a late bot refresh after auth reset', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const oldRefresh = deferred<Array<{ id: string; status: string; name: string }>>()
    api.fetchBots.mockReturnValueOnce(oldRefresh.promise)

    const refreshing = store.refreshBots()
    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))
    oldRefresh.resolve([{ id: 'old-user-bot', status: 'active', name: 'Old' }])
    await refreshing

    expect(store.bots).toEqual([])
    expect(store.currentBotId).toBeNull()
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

  it('aborts the active session stream through the websocket and settles the send', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('hello')
    await flushPromises()
    await flushPromises()

    const assistant = store.messages.find(turn => turn.role === 'assistant')
    const ws = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }
    expect(store.streaming).toBe(true)
    expect(assistant?.streaming).toBe(true)

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: lastSessionId,
      stream_id: lastStreamId,
      seq: 19,
      snapshot: runtimeSnapshotFromScript([], lastSessionId, lastStreamId, 'running', 19),
    } as UIStreamEvent)
    store.abort()

    const aborted = runtimeSnapshotFromScript([], lastSessionId, lastStreamId, 'aborted', 20)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: lastSessionId,
      stream_id: lastStreamId,
      seq: 20,
      snapshot: aborted,
    } as UIStreamEvent)

    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })
    expect(ws.abort).toHaveBeenCalledWith(lastStreamId, lastSessionId, `generation-${lastStreamId}`)
    expect(store.streaming).toBe(false)
    expect(assistant?.streaming).toBe(false)
  })

  it('ignores a trailing end after an error without dropping visible assistant output', async () => {
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

    const refreshCallsBefore = api.fetchMessagesUI.mock.calls.length
    streamHandler?.({ type: 'end', stream_id: lastStreamId, session_id: lastSessionId } as UIStreamEvent)
    await flushPromises()

    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(refreshCallsBefore)
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
  })

  it('waits for the server runtime operation before replacing a retried assistant', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const retry = store.retryLatestAssistant('assistant-old', { workspaceTargetId: 'computer-b' })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'retry_message',
      session_id: 'session-1',
      message_id: 'assistant-old',
      workspace_target_id: 'computer-b',
    })
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      streaming: false,
      messages: [{ type: 'text', content: 'old answer' }],
    })

    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer partial' }]))

    expect(store.messages.map(message => message.id)).not.toContain('assistant-old')
    expect(store.messages.map(message => message.role)).toEqual(['user', 'assistant'])
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      streaming: true,
      __optimistic: true,
      messages: [{ type: 'text', content: 'new answer partial' }],
    })

    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:02.000Z',
        streaming: false,
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11))
    await retry
    await flushPromises()

    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-new'])
  })

  it('keeps an initiating retry pending until runtime admission arrives', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-05-17T08:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    const retryStreamId = lastStreamId

    expect(store.streaming).toBe(true)
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])

    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    streamHandler?.(runtimeReplacementSnapshot(
      retryStreamId,
      operation,
      [{ id: 0, type: 'text', content: 'new answer' }],
      'running',
      1,
    ))
    expect(store.messages.map(message => message.id)).not.toContain('assistant-old')

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-05-17T08:00:00.000Z' },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:02.000Z',
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(
      retryStreamId,
      operation,
      [{ id: 0, type: 'text', content: 'new answer' }],
      'completed',
      2,
    ))

    await expect(retry).resolves.toMatchObject({ ok: true })
  })

  it('times out an unobserved send after disconnect instead of streaming forever', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    vi.useFakeTimers()
    try {
      const sending = store.sendMessage('possibly lost')
      await vi.advanceTimersByTimeAsync(0)
      const streamId = lastStreamId
      expect(streamId).not.toBe('')

      const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
        onClose?: (() => void) | null
        onOpen?: (() => void) | null
      }
      websocket.onClose?.()
      websocket.onOpen?.()
      streamHandler?.({
        type: 'runtime_snapshot',
        bot_id: 'bot-1',
        session_id: 'session-1',
        seq: 1,
        snapshot: { bot_id: 'bot-1', session_id: 'session-1', seq: 1, queue: [] },
      } as UIStreamEvent)

      await vi.advanceTimersByTimeAsync(29_999)
      expect(store.streaming).toBe(true)
      await vi.advanceTimersByTimeAsync(1)

      await expect(sending).resolves.toMatchObject({
        ok: false,
        stage: 'startup',
        error: 'runtime command was not acknowledged',
        restoreInput: 'possibly lost',
      })
      expect(store.streaming).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('moves fork divider anchor to the previous inherited assistant when retry replaces the fork anchor', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'fork-session',
        bot_id: 'bot-1',
        title: 'Fork',
        type: 'chat',
        created_at: '2026-05-17T08:00:05.000Z',
        metadata: {
          forked_from: {
            session_id: 'source-session',
            title: 'Source',
            message_id: 'source-assistant',
            fork_message_id: 'assistant-old',
          },
        },
      }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'first',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-prev',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'previous answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
      {
        id: 'user-2',
        role: 'user',
        text: 'second',
        attachments: [],
        timestamp: '2026-05-17T08:00:02.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:03.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-old',
    })

    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer partial' }], 'running', 10, 'fork-session'))

    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-prev',
    })

    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'first',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-prev',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'previous answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
      {
        id: 'user-2',
        role: 'user',
        text: 'second',
        attachments: [],
        timestamp: '2026-05-17T08:00:02.000Z',
      },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:06.000Z',
        streaming: false,
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11, 'fork-session'))
    await retry
    await flushPromises()

    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-prev',
    })
  })

  it('clears fork divider anchor when retry replaces the only inherited assistant', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'fork-session',
        bot_id: 'bot-1',
        title: 'Fork',
        type: 'chat',
        created_at: '2026-05-17T08:00:05.000Z',
        metadata: {
          forked_from: {
            session_id: 'source-session',
            title: 'Source',
            message_id: 'source-assistant',
            fork_message_id: 'assistant-old',
          },
        },
      }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    expect((store.activeChatTarget.metadata.forked_from as Record<string, unknown>).fork_message_id).toBe('assistant-old')

    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer partial' }], 'running', 10, 'fork-session'))

    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      session_id: 'source-session',
      message_id: 'source-assistant',
    })
    expect((store.activeChatTarget.metadata.forked_from as Record<string, unknown>).fork_message_id).toBeUndefined()

    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:06.000Z',
        streaming: false,
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11, 'fork-session'))
    await retry
    await vi.waitFor(() => {
      expect((store.activeChatTarget.metadata.forked_from as Record<string, unknown>).fork_message_id).toBeUndefined()
    })
  })

  it('moves fork divider anchor when edit replaces the fork anchor tail', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'fork-session',
        bot_id: 'bot-1',
        title: 'Fork',
        type: 'chat',
        created_at: '2026-05-17T08:00:05.000Z',
        metadata: {
          forked_from: {
            session_id: 'source-session',
            title: 'Source',
            message_id: 'source-assistant',
            fork_message_id: 'assistant-old',
          },
        },
      }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'first',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-prev',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'previous answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
      {
        id: 'user-2',
        role: 'user',
        text: 'second',
        attachments: [],
        timestamp: '2026-05-17T08:00:02.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:03.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const edit = store.editLatestUser('user-2', 'edited second')
    await flushPromises()
    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-old',
    })

    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'edit',
      replace_from_message_id: 'user-2',
      replacement_user_turn: {
        role: 'user',
        text: 'edited second',
        timestamp: '2026-05-17T08:00:06.000Z',
        platform: 'local',
      },
    }, [{ id: 0, type: 'text', content: 'new answer partial' }], 'running', 10, 'fork-session'))

    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-prev',
    })

    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'first',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-prev',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'previous answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
      {
        id: 'user-new',
        role: 'user',
        text: 'edited second',
        attachments: [],
        timestamp: '2026-05-17T08:00:06.000Z',
      },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:07.000Z',
        streaming: false,
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'edit',
      replace_from_message_id: 'user-2',
      replacement_user_turn: {
        role: 'user',
        text: 'edited second',
        timestamp: '2026-05-17T08:00:06.000Z',
        platform: 'local',
      },
    }, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11, 'fork-session'))
    await edit
    await flushPromises()

    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      fork_message_id: 'assistant-prev',
    })
  })

  it('restores the old assistant when retry fails before streaming starts', async () => {
    sendEvents = [{ type: 'error', message: 'model failed' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const result = await store.retryLatestAssistant('assistant-old')

    expect(result).toMatchObject({ ok: false, stage: 'startup', error: 'model failed' })
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])
  })

  it('does not restore a failed retry tail into a different active session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'session-a') {
        return Promise.resolve([
          {
            id: 'user-a',
            role: 'user',
            text: 'hello',
            attachments: [],
            timestamp: '2026-05-17T08:00:00.000Z',
          },
          {
            id: 'assistant-old',
            role: 'assistant',
            messages: [{ id: 1, type: 'text', content: 'old answer' }],
            timestamp: '2026-05-17T08:00:01.000Z',
            streaming: false,
          },
        ])
      }
      if (sessionId === 'session-b') {
        return Promise.resolve([
          {
            id: 'user-b',
            role: 'user',
            text: 'other chat',
            attachments: [],
            timestamp: '2026-05-17T09:00:00.000Z',
          },
        ])
      }
      return Promise.resolve([])
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    expect(store.messages.map(message => message.id)).toEqual(['user-a', 'assistant-old'])
    const retryStreamId = lastStreamId

    await store.selectSession('session-b')
    await flushPromises()
    expect(store.messages.map(message => message.id)).toEqual(['user-b'])

    streamHandler?.({
      type: 'error',
      stream_id: retryStreamId,
      session_id: 'session-a',
      message: 'model failed',
    } as UIStreamEvent)
    const result = await retry
    await flushPromises()

    expect(result).toMatchObject({ ok: false, stage: 'startup', error: 'model failed' })
    expect(store.sessionId).toBe('session-b')
    expect(store.messages.map(message => message.id)).toEqual(['user-b'])
  })

  it('waits for the server runtime operation before replacing an edited turn', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'old prompt',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const edit = store.editLatestUser('user-1', 'new prompt', { workspaceTargetId: 'computer-a' })
    await flushPromises()

    expect(sentWSMessages.at(-1)).toMatchObject({
      type: 'edit_message',
      session_id: 'session-1',
      message_id: 'user-1',
      text: 'new prompt',
      workspace_target_id: 'computer-a',
    })
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'old prompt' })

    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'edit',
      replace_from_message_id: 'user-1',
      replacement_user_turn: {
        role: 'user',
        text: 'new prompt',
        timestamp: '2026-05-17T08:00:02.000Z',
        platform: 'local',
      },
    }, [{ id: 0, type: 'text', content: 'new answer partial' }]))

    expect(store.messages.map(message => message.id)).not.toContain('user-1')
    expect(store.messages.map(message => message.id)).not.toContain('assistant-old')
    expect(store.messages.map(message => message.role)).toEqual(['user', 'assistant'])
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: 'new prompt',
      __optimistic: true,
    })
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      streaming: true,
      __optimistic: true,
    })

    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-new',
        role: 'user',
        text: 'new prompt',
        attachments: [],
        timestamp: '2026-05-17T08:00:02.000Z',
      },
      {
        id: 'assistant-new',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'new answer' }],
        timestamp: '2026-05-17T08:00:03.000Z',
        streaming: false,
      },
    ])
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, {
      kind: 'edit',
      replace_from_message_id: 'user-1',
      replacement_user_turn: {
        role: 'user',
        text: 'new prompt',
        timestamp: '2026-05-17T08:00:02.000Z',
        platform: 'local',
      },
    }, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11))
    await edit
    await vi.waitFor(() => {
      expect(store.messages).toMatchObject([
        { role: 'user', serverId: 'user-new', text: 'new prompt' },
        { role: 'assistant', serverId: 'assistant-new', streaming: false },
      ])
    })
  })

  it('restores the old latest turn tail when edit fails before streaming starts', async () => {
    sendEvents = [{ type: 'error', message: 'model failed' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'old prompt',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const result = await store.editLatestUser('user-1', 'new prompt')

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      error: 'model failed',
      restoreInput: 'new prompt',
    })
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])
  })

  it('does not restore a failed edit tail into a different active session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'session-a') {
        return Promise.resolve([
          {
            id: 'user-a',
            role: 'user',
            text: 'old prompt',
            attachments: [],
            timestamp: '2026-05-17T08:00:00.000Z',
          },
          {
            id: 'assistant-a',
            role: 'assistant',
            messages: [{ id: 1, type: 'text', content: 'old answer' }],
            timestamp: '2026-05-17T08:00:01.000Z',
            streaming: false,
          },
        ])
      }
      if (sessionId === 'session-b') {
        return Promise.resolve([
          {
            id: 'user-b',
            role: 'user',
            text: 'other chat',
            attachments: [],
            timestamp: '2026-05-17T09:00:00.000Z',
          },
        ])
      }
      return Promise.resolve([])
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const edit = store.editLatestUser('user-a', 'new prompt')
    await flushPromises()
    expect(store.messages.map(message => message.role)).toEqual(['user', 'assistant'])
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'old prompt' })
    const editStreamId = lastStreamId

    await store.selectSession('session-b')
    await flushPromises()
    expect(store.messages.map(message => message.id)).toEqual(['user-b'])

    streamHandler?.({
      type: 'error',
      stream_id: editStreamId,
      session_id: 'session-a',
      message: 'model failed',
    } as UIStreamEvent)
    const result = await edit
    await flushPromises()

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      error: 'model failed',
      restoreInput: 'new prompt',
    })
    expect(store.sessionId).toBe('session-b')
    expect(store.messages.map(message => message.id)).toEqual(['user-b'])
  })

  it('does not edit a latest user turn with attachments until attachment preservation is supported', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'old prompt',
        attachments: [{
          content_hash: 'hash-1',
          role: 'user',
          ordinal: 0,
          mime: 'image/png',
          size_bytes: 12,
          storage_key: 'asset-1',
          name: 'image.png',
        }],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'old answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const result = await store.editLatestUser('user-1', 'new prompt')

    expect(result).toMatchObject({ ok: false, stage: 'startup' })
    expect(sentWSMessages).toHaveLength(0)
    expect(store.messages.map(message => message.id)).toEqual(['user-1', 'assistant-old'])
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      attachments: [expect.objectContaining({ content_hash: 'hash-1' })],
    })
  })

  it('keeps fork source anchored to the copied message after switching to the fork session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' }],
      nextCursor: null,
    })
    const sourceTurns = [
      {
        id: 'source-user',
        role: 'user' as const,
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'source-assistant',
        role: 'assistant' as const,
        messages: [{ id: 1, type: 'text' as const, content: 'answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ]
    const forkTurns = [
      {
        id: 'fork-user',
        role: 'user' as const,
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'fork-assistant',
        role: 'assistant' as const,
        messages: [{ id: 1, type: 'text' as const, content: 'answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ]
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'source-session') return Promise.resolve(sourceTurns)
      if (sessionId === 'fork-session') return Promise.resolve(forkTurns)
      return Promise.resolve([])
    })
    api.forkSessionFromMessage.mockResolvedValueOnce({
      id: 'fork-session',
      bot_id: 'bot-1',
      title: 'Source fork',
      type: 'chat',
      metadata: {
        forked_from: {
          session_id: 'source-session',
          title: 'Source',
          message_id: 'source-assistant',
          fork_message_id: 'fork-final-raw-message',
        },
      },
    })
    api.fetchSessions.mockResolvedValueOnce({
      items: [{
        id: 'fork-session',
        bot_id: 'bot-1',
        title: 'Source fork',
        type: 'chat',
        metadata: {
          forked_from: {
            session_id: 'source-session',
            title: 'Source',
            message_id: 'source-assistant',
          },
        },
      }],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const ok = await store.forkMessage('source-assistant', { title: 'Custom fork name' })
    await applyLatestForkRequest(store)
    await flushPromises()

    expect(ok).toBe(true)
    expect(api.forkSessionFromMessage).toHaveBeenCalledWith('bot-1', 'source-session', 'source-assistant', { title: 'Custom fork name' })
    expect(store.sessionId).toBe('fork-session')
    expect(store.messages.map(message => message.id)).toEqual(['fork-user', 'fork-assistant'])
    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      session_id: 'source-session',
      message_id: 'source-assistant',
      fork_message_id: 'fork-assistant',
    })
  })

  it('keeps fork metadata when a stale session list refresh does not include the fork session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' }],
      nextCursor: null,
    })
    const sourceTurns = [
      {
        id: 'source-user',
        role: 'user' as const,
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'source-assistant',
        role: 'assistant' as const,
        messages: [{ id: 1, type: 'text' as const, content: 'answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ]
    const forkTurns = [
      {
        id: 'fork-user',
        role: 'user' as const,
        text: 'hello',
        attachments: [],
        timestamp: '2026-05-17T08:00:00.000Z',
      },
      {
        id: 'fork-assistant',
        role: 'assistant' as const,
        messages: [{ id: 1, type: 'text' as const, content: 'answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ]
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'source-session') return Promise.resolve(sourceTurns)
      if (sessionId === 'fork-session') return Promise.resolve(forkTurns)
      return Promise.resolve([])
    })
    api.forkSessionFromMessage.mockResolvedValueOnce({
      id: 'fork-session',
      bot_id: 'bot-1',
      title: 'Source fork',
      type: 'chat',
      metadata: {
        forked_from: {
          session_id: 'source-session',
          title: 'Source',
          message_id: 'source-assistant',
          fork_message_id: 'fork-assistant',
        },
      },
    })
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const ok = await store.forkMessage('source-assistant')
    await applyLatestForkRequest(store)
    await flushPromises()

    expect(ok).toBe(true)
    expect(store.sessionId).toBe('fork-session')
    expect(store.activeChatTarget.metadata.forked_from).toMatchObject({
      session_id: 'source-session',
      message_id: 'source-assistant',
      fork_message_id: 'fork-assistant',
    })
    expect(store.knownSessionSummary('fork-session')?.metadata?.forked_from).toMatchObject({
      fork_message_id: 'fork-assistant',
    })
  })

  it('keeps a forked ask-user continuation in one assistant turn when hydration resolves late', async () => {
    sendEvents = []
    const userInput = singleSelectUserInput()
    const forkTurns = [
      {
        id: 'fork-user', role: 'user' as const, text: 'use ask_user', attachments: [],
        timestamp: '2026-07-11T00:00:00.000Z',
      },
      {
        id: 'fork-assistant',
        role: 'assistant' as const,
        messages: [{
          id: 1,
          type: 'tool' as const,
          name: 'ask_user',
          input: { questions: [{ text: 'Which plan?', kind: 'single_select' }] },
          tool_call_id: 'call-ask',
          running: false,
          user_input: userInput,
        }],
        timestamp: '2026-07-11T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' }],
      nextCursor: null,
    })
    api.forkSessionFromMessage.mockResolvedValueOnce({
      id: 'fork-session', bot_id: 'bot-1', title: 'Source fork', type: 'chat',
    })
    const hydration = deferred<typeof forkTurns>()
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    api.fetchMessagesUI
      .mockResolvedValueOnce(forkTurns)
      .mockReturnValueOnce(hydration.promise)

    await store.forkMessage('source-assistant')
    await applyLatestForkRequest(store)
    await flushPromises()
    expect(store.messages.filter(message => message.role === 'assistant')).toHaveLength(1)

    await store.respondUserInput(userInput, {
      answers: [{ question_id: 'q1', option_ids: ['q1.o1'] }],
    })
    const responseStreamId = sentWSMessages.at(-1)?.stream_id as string
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'fork-session',
      stream_id: responseStreamId,
      seq: 1,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 2, type: 'text', content: 'continuation' } }],
        'fork-session',
        responseStreamId,
        'running',
        1,
      ),
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'fork-session',
      stream_id: responseStreamId,
      seq: 2,
      delta: {
        reset_messages: true,
        message_appends: [{ id: 2, type: 'text', content: 'continuation' }],
      },
    } as UIStreamEvent)

    hydration.resolve(forkTurns)
    await flushPromises()
    await flushPromises()

    const assistantTurns = store.messages.filter(message => message.role === 'assistant')
    expect(assistantTurns).toHaveLength(1)
    expect(assistantTurns[0]).toMatchObject({
      streaming: true,
      messages: [
        { type: 'tool', userInput: { user_input_id: 'input-1', status: 'submitted' } },
        { type: 'text', content: 'continuation' },
      ],
    })

    const completedForkTurns: UITurn[] = [
      forkTurns[0]!,
      {
        ...forkTurns[1]!,
        messages: [
          {
            ...forkTurns[1]!.messages[0]!,
            user_input: { ...userInput, status: 'submitted', can_respond: false },
          },
          { id: 2, type: 'text', content: 'continuation' },
        ],
      },
    ]
    api.fetchMessagesUI.mockResolvedValueOnce(completedForkTurns)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'fork-session',
      stream_id: responseStreamId,
      seq: 2,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 2, type: 'text', content: 'continuation' } }],
        'fork-session',
        responseStreamId,
        'completed',
        2,
      ),
    } as UIStreamEvent)
    await flushPromises()
    await flushPromises()

    const completedAssistantTurns = store.messages.filter(message => message.role === 'assistant')
    expect(completedAssistantTurns).toHaveLength(1)
    expect(completedAssistantTurns[0]).toMatchObject({
      streaming: false,
      messages: [
        { type: 'tool', userInput: { user_input_id: 'input-1', status: 'submitted' } },
        { type: 'text', content: 'continuation' },
      ],
    })
  })

  it('routes a late fork response to its origin view without changing the focused Session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' },
        { id: 'other-session', bot_id: 'bot-1', title: 'Other', type: 'chat' },
      ],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'source-session') {
        return Promise.resolve([
          {
            id: 'source-assistant',
            role: 'assistant' as const,
            messages: [{ id: 1, type: 'text' as const, content: 'answer' }],
            timestamp: '2026-05-17T08:00:01.000Z',
            streaming: false,
          },
        ])
      }
      if (sessionId === 'other-session') {
        return Promise.resolve([
          {
            id: 'other-user',
            role: 'user' as const,
            text: 'other',
            attachments: [],
            timestamp: '2026-05-17T09:00:00.000Z',
          },
        ])
      }
      return Promise.resolve([])
    })
    let resolveFork!: (session: unknown) => void
    api.forkSessionFromMessage.mockReturnValueOnce(new Promise(resolve => {
      resolveFork = resolve
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const targetA = { botId: 'bot-1', sessionId: 'source-session', viewId: 'chat:a' }
    const targetB = { botId: 'bot-1', sessionId: 'other-session', viewId: 'chat:b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.focusChatView(targetA.viewId)
    const fork = store.forkMessage('source-assistant', { target: targetA })
    await flushPromises()
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetB.viewId)
    await store.selectSession('other-session')
    await flushPromises()
    resolveFork({
      id: 'fork-session',
      bot_id: 'bot-1',
      title: 'Source fork',
      type: 'chat',
      metadata: {
        forked_from: {
          session_id: 'source-session',
          title: 'Source',
          message_id: 'source-assistant',
        },
      },
    })
    const ok = await fork
    await flushPromises()

    expect(ok).toBe(true)
    expect(store.sessionId).toBe('other-session')
    expect(store.messages.map(message => message.id)).toEqual(['other-user'])
    expect(store.knownSessionSummary('fork-session')).toMatchObject({ id: 'fork-session' })
    expect(api.fetchMessagesUI).toHaveBeenCalledWith('bot-1', 'fork-session', expect.anything())
    expect(store.forkedSessionRequested).toMatchObject({
      botId: 'bot-1',
      viewId: targetA.viewId,
      expectedSessionId: 'source-session',
      sessionId: 'fork-session',
      activate: true,
    })
  })

  it('drops a late Fork result after the authenticated scope resets', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'source-session', bot_id: 'bot-1', title: 'Source', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'source-assistant',
      role: 'assistant',
      messages: [{ id: 1, type: 'text', content: 'answer' }],
      timestamp: '2026-07-11T00:00:00Z',
      streaming: false,
    }])
    const response = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.forkSessionFromMessage.mockReturnValueOnce(response.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const fork = store.forkMessage('source-assistant')
    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))
    response.resolve({ id: 'old-fork', bot_id: 'bot-1', title: 'Old fork', type: 'chat' })

    await expect(fork).resolves.toBe(true)
    expect(store.forkedSessionRequested).toBeNull()
    expect(store.knownSessionSummary('old-fork')).toBeNull()
    expect(api.fetchMessagesUI).not.toHaveBeenCalledWith('bot-1', 'old-fork', expect.anything())
  })

  it('does not fork non-chat sessions', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'discuss-session', bot_id: 'bot-1', title: 'Discuss', type: 'discuss' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'assistant-1',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'answer' }],
        timestamp: '2026-05-17T08:00:01.000Z',
        streaming: false,
      },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const ok = await store.forkMessage('assistant-1')

    expect(ok).toBe(false)
    expect(api.forkSessionFromMessage).not.toHaveBeenCalled()
  })

  it('does not fork known non-canonical or non-assistant turns', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'Chat', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: new Date().toISOString() },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    expect(await store.forkMessage('user-1')).toBe(false)
    store.messages.push({
      id: 'ephemeral-error',
      role: 'assistant',
      messages: [{ id: 0, type: 'error', content: 'Response stopped' }],
      timestamp: new Date().toISOString(),
      streaming: false,
      __ephemeral: true,
    })
    expect(await store.forkMessage('ephemeral-error')).toBe(false)
    store.messages.push({
      id: 'optimistic-assistant',
      role: 'assistant',
      messages: [],
      timestamp: new Date().toISOString(),
      streaming: false,
      __optimistic: true,
    })
    expect(await store.forkMessage('optimistic-assistant')).toBe(false)
    expect(api.forkSessionFromMessage).not.toHaveBeenCalled()
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
        send: vi.fn((message: { type?: string; reasoning_effort?: string; stream_id?: string; session_id?: string }) => {
          if (message.type === 'runtime_subscribe' || message.type === 'runtime_unsubscribe') return
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
    expect(sent[0]!.reasoning_effort).toBe(REASONING_EFFORT_DISABLE)
  })

  it('keeps late quick action events scoped to the composer that sent them', async () => {
    api.fetchBots.mockResolvedValueOnce([
      { id: 'bot-1', status: 'active', name: 'Bot 1' },
      { id: 'bot-2', status: 'active', name: 'Bot 2' },
    ])
    let resolveQuickAction: (value: UIStreamEvent) => void = () => {}
    api.executeQuickAction.mockImplementationOnce(() => new Promise<UIStreamEvent>((resolve) => {
      resolveQuickAction = resolve
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('/help', undefined, { composerScope: 'bot-1:draft-a' })
    await flushPromises()

    api.fetchSession.mockResolvedValueOnce({
      id: 'session-b',
      bot_id: 'bot-1',
      title: 'Session B',
      type: 'chat',
    })
    await store.selectSession('session-b')
    resolveQuickAction({
      type: 'command_result',
      terminal: true,
      result: { kind: 'text', text: 'Help' },
    } as UIStreamEvent)
    await sendPromise

    const draftCommandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })
    expect(draftCommandEvent).toMatchObject({
      type: 'command_result',
      bot_id: 'bot-1',
      composer_scope: 'bot-1:draft-a',
    })
    expect(draftCommandEvent?.session_id).toBeUndefined()
    expect(store.commandEvent).toBeNull()
  })

  it('sends selected session ids as quick action capability context', async () => {
    api.executeQuickAction.mockResolvedValueOnce({
      type: 'command_result',
      terminal: true,
      result: { kind: 'text', text: 'Help' },
    } as UIStreamEvent)
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-a',
      bot_id: 'bot-1',
      title: 'Session A',
      type: 'chat',
    })
    const store = useChatStore()
    const onBeforeTurnAppend = vi.fn()
    const onTurnAppendAborted = vi.fn()

    await store.selectBot('bot-1')
    await store.selectSession('session-a')
    const result = await store.sendMessage('/help', undefined, {
      composerScope: 'bot-1:panel-a',
      onBeforeTurnAppend,
      onTurnAppendAborted,
    })

    expect(result).toMatchObject({ ok: true })
    expect(api.executeQuickAction).toHaveBeenCalledWith('bot-1', 'help', expect.objectContaining({
      composerScope: 'bot-1:panel-a',
      sessionId: 'session-a',
      skillActivationAllowed: true,
    }))
    expect(onBeforeTurnAppend).not.toHaveBeenCalled()
    expect(onTurnAppendAborted).not.toHaveBeenCalled()
  })

  it('keeps quick action transport failures as startup command errors', async () => {
    api.executeQuickAction.mockRejectedValueOnce(new Error('network unavailable'))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/help', undefined, { composerScope: 'bot-1:panel-a' })

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      restoreInput: '/help',
      error: 'network unavailable',
    })
    const commandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:panel-a' })
    expect(commandEvent).toMatchObject({
      type: 'command_error',
      composer_scope: 'bot-1:panel-a',
      error: { code: 'generic', message: 'network unavailable' },
    })
  })

  it('keeps direct skill slash websocket startup failures restorable', async () => {
    sendEvents = [
      { type: 'start' } as UIStreamEvent,
      { type: 'error', message: 'model failed' } as UIStreamEvent,
    ]
    const store = useChatStore()

    await store.selectBot('bot-1')
    const result = await store.sendMessage('/wat', undefined, { composerScope: 'bot-1:panel-a' })

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      restoreInput: '/wat',
      error: 'model failed',
    })
    expect(sentWSMessages[0]).toMatchObject({
      type: 'message',
      text: '/wat',
      composer_scope: 'bot-1:panel-a',
    })
    expect(sentWSMessages[0]?.requested_skills).toBeUndefined()
  })

  it('rejects direct skill activation in pending ACP drafts before sending websocket chat', async () => {
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const result = await store.sendMessage('/flutter-adding-home-screen-widgets', undefined, {
      composerScope: 'bot-1:draft-a',
    })

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      restoreInput: '/flutter-adding-home-screen-widgets',
    })
    expect(sentWSMessages).toHaveLength(0)
    expect(api.createSession).not.toHaveBeenCalled()
    expect(store.loading).toBe(false)
    const commandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })
    expect(commandEvent).toMatchObject({
      type: 'command_error',
      error: { code: 'unsupported_skill_slash_context' },
    })
  })

  it('rejects skill list quick action in pending ACP drafts without reading the catalog', async () => {
    api.executeQuickAction.mockResolvedValueOnce({
      type: 'command_error',
      terminal: true,
      composer_scope: 'bot-1:draft-a',
      error: { code: 'unsupported_skill_slash_context', message: 'unsupported' },
    } as UIStreamEvent)
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const result = await store.sendMessage('/skill list', undefined, {
      composerScope: 'bot-1:draft-a',
    })

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      restoreInput: '/skill list',
    })
    expect(api.executeQuickAction).toHaveBeenCalledWith('bot-1', 'skill.list', expect.objectContaining({
      composerScope: 'bot-1:draft-a',
      skillActivationAllowed: false,
    }))
    expect(sentWSMessages).toHaveLength(0)
    expect(store.loading).toBe(false)
    const commandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })
    expect(commandEvent).toMatchObject({
      type: 'command_error',
      error: { code: 'unsupported_skill_slash_context' },
    })
  })

  it('shows ACP help without skill entry points', async () => {
    api.executeQuickAction.mockResolvedValueOnce({
      type: 'command_result',
      terminal: true,
      composer_scope: 'bot-1:draft-a',
      result: {
        kind: 'list',
        items: [{ id: 'help', title: '/help', kind: 'quick_action' }],
        text: 'Available Web quick actions: /help.',
      },
    } as UIStreamEvent)
    const store = useChatStore()

    await store.selectBot('bot-1')
    store.stageACPSession({ agentId: 'codex' })
    const result = await store.sendMessage('/help', undefined, {
      composerScope: 'bot-1:draft-a',
    })

    expect(result).toMatchObject({ ok: true })
    expect(api.executeQuickAction).toHaveBeenCalledWith('bot-1', 'help', expect.objectContaining({
      composerScope: 'bot-1:draft-a',
      skillActivationAllowed: false,
    }))
    expect(sentWSMessages).toHaveLength(0)
    const commandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })
    expect(commandEvent).toMatchObject({
      type: 'command_result',
      result: {
        kind: 'list',
        items: [
          expect.objectContaining({ id: 'help' }),
        ],
      },
    })
    expect(commandEvent?.result?.items?.some(item => item.id === 'skill.list')).toBe(false)
    expect(commandEvent?.result?.text).not.toContain('/skill list')
    expect(store.loading).toBe(false)
  })

  it('sends only skill name in requested skill websocket payloads', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent, { type: 'end' } as UIStreamEvent]
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-a',
      bot_id: 'bot-1',
      title: 'Session A',
      type: 'chat',
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.selectSession('session-a')
    const result = await store.sendMessage('hello with skill', undefined, {
      requestedSkills: [{
        name: 'alpha',
        display_name: 'Alpha',
        description: 'Display-only description',
        source_kind: 'managed',
        state: 'effective',
      }],
      composerScope: 'bot-1:panel-a',
    })

    expect(result).toMatchObject({ ok: true })
    expect(sentWSMessages[0]?.requested_skills).toEqual([{
      name: 'alpha',
    }])
    expect(JSON.stringify(sentWSMessages[0]?.requested_skills)).not.toContain('Display-only description')
    expect(JSON.stringify(sentWSMessages[0]?.requested_skills)).not.toContain('managed')
  })

  it('inserts direct skill activation only after the server user_message ack', async () => {
    sendEvents = []
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-a',
      bot_id: 'bot-1',
      title: 'Session A',
      type: 'chat',
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await store.selectSession('session-a')
    const sendPromise = store.sendMessage('/flutter-adding-home-screen-widgets', undefined, {
      composerScope: 'bot-1:panel-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    expect(store.messages).toHaveLength(0)
    expect(sentWSMessages[0]).toMatchObject({
      type: 'message',
      text: '/flutter-adding-home-screen-widgets',
    })
    expect(sentWSMessages[0]?.requested_skills).toBeUndefined()

    streamHandler?.({
      type: 'user_message',
      stream_id: streamId,
      session_id: 'session-a',
      data: {
        id: 'msg-skill',
        role: 'user',
        text: '',
        user_message_kind: 'skill_activation',
        skill_activation: {
          skills: [{
            name: 'flutter-adding-home-screen-widgets',
            display_name: 'Flutter adding home screen widgets',
            description: 'Safe display summary',
            source_kind: 'managed',
            state: 'effective',
          }],
        },
        timestamp: '2026-07-03T00:00:00.000Z',
      },
    } as UIStreamEvent)
    await flushPromises()

    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: '',
      userMessageKind: 'skill_activation',
      skillActivation: {
        skills: [{
          name: 'flutter-adding-home-screen-widgets',
          display_name: 'Flutter adding home screen widgets',
        }],
      },
    })
    expect(store.messages[1]).toMatchObject({ role: 'assistant', streaming: true })

    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-a',
      data: { id: 1, type: 'text', content: 'Done' },
    } as UIStreamEvent)
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [{ type: 'text', content: 'Done' }],
      streaming: true,
    })

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-a' } as UIStreamEvent)
    await expect(sendPromise).resolves.toMatchObject({ ok: true })
  })

  it('blocks a second deferred draft send while the first stream is still unbound', async () => {
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    const firstSend = store.sendMessage('first activation', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    expect(store.streaming).toBe(true)
    const secondResult = await store.sendMessage('second activation', undefined, {
      requestedSkills: [{ name: 'beta' }],
      composerScope: 'bot-1:draft-a',
    })
    expect(secondResult).toMatchObject({ ok: false, stage: 'startup' })
    expect(sentWSMessages).toHaveLength(1)

    streamHandler?.({ type: 'end', stream_id: streamId } as UIStreamEvent)
    await expect(firstSend).resolves.toMatchObject({ ok: true })
    expect(store.streaming).toBe(false)
  })

  it('subscribes before routing abort when session_created binds a deferred draft stream', async () => {
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('activate', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    expect(runtimeSubscribeMessages).toEqual([])
    store.abort()
    expect(abortedWSStreams).not.toContain(streamId)

    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'session-1' } as UIStreamEvent)
    expect(abortedWSStreams).toContain(streamId)
    expect(runtimeSubscribeMessages).toEqual([expect.objectContaining({
      type: 'runtime_subscribe',
      session_id: 'session-1',
      invocation_id: expect.any(String),
    })])
    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: streamId,
      seq: 1,
      snapshot: runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 1),
    } as UIStreamEvent)
    expect(abortedWSStreams.filter(id => id === streamId)).toHaveLength(2)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: streamId,
      seq: 2,
      snapshot: runtimeSnapshotFromScript([], 'session-1', streamId, 'aborted', 2),
    } as UIStreamEvent)

    await expect(sending).resolves.toMatchObject({ ok: false })
    expect(store.streaming).toBe(false)
  })

  it('keeps the first created-session correlation when a stream receives a conflicting duplicate', async () => {
    sendEvents = []
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('activate', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'session-first' } as UIStreamEvent)
    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'session-conflict' } as UIStreamEvent)

    expect(store.sessionId).toBe('session-first')
    expect(store.knownSessionSummary('session-first')).not.toBeNull()
    expect(store.knownSessionSummary('session-conflict')).toBeNull()

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-first' } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: true })
  })

  it('ignores late messages for a terminal stream instead of resurrecting it', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent, { type: 'end' } as UIStreamEvent]
    const store = useChatStore()

    await store.selectBot('bot-1')
    await expect(store.sendMessage('hello')).resolves.toMatchObject({ ok: true })
    const messageCount = store.messages.length

    streamHandler?.({
      type: 'message',
      stream_id: lastStreamId,
      session_id: lastSessionId,
      data: { id: 1, type: 'text', content: 'late' },
    } as UIStreamEvent)

    expect(store.messages).toHaveLength(messageCount)
    expect(store.streaming).toBe(false)
  })

  it('does not select a late session_created event after the user switches sessions', async () => {
    sendEvents = []
    api.fetchSession.mockImplementation(async (_botId: string, sessionID: string) => ({
      id: sessionID,
      bot_id: 'bot-1',
      title: sessionID,
      type: 'chat',
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('hello with skill', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    await store.selectSession('session-b')
    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'created-session' } as UIStreamEvent)
    await flushPromises()

    expect(store.sessionId).toBe('session-b')
    // The hidden Draft view still owns this stream, so its new Session is
    // remembered without stealing global focus from session-b.
    expect(store.knownSessionSummary('created-session')).not.toBeNull()

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'created-session' } as UIStreamEvent)
    await sendPromise

    expect(store.sessionId).toBe('session-b')
  })

  it('deletes a deferred draft session when requested skill preflight fails after session_created', async () => {
    sendEvents = []
    const store = useChatStore()
    const requestedSkill = { name: 'alpha' }
    const attachment = {
      type: 'file',
      base64: 'data:text/plain;base64,aGVsbG8=',
      mime: 'text/plain',
      name: 'note.txt',
    }

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('hello with skill', [attachment], {
      requestedSkills: [requestedSkill],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'created-session' } as UIStreamEvent)
    await flushPromises()
    expect(store.sessionId).toBe('created-session')

    streamHandler?.({
      type: 'command_error',
      invocation_id: streamId,
      stream_id: streamId,
      session_id: 'created-session',
      composer_scope: 'bot-1:draft-a',
      terminal: true,
      error: {
        code: 'unsupported_skill_slash_context',
        message: 'Requested skills are not supported here.',
      },
    } as UIStreamEvent)
    const result = await sendPromise

    expect(result).toMatchObject({
      ok: false,
      stage: 'startup',
      restoreInput: 'hello with skill',
      restoreAttachments: [attachment],
      restoreRequestedSkills: [requestedSkill],
    })
    expect(api.deleteSession).toHaveBeenCalledWith('bot-1', 'created-session')
    expect(store.deletedSession).toEqual({
      id: 'created-session',
      botId: 'bot-1',
      seq: 1,
      composerScope: 'bot-1:draft-a',
    })
    expect(store.sessionId).toBeNull()
    expect(store.knownSessionSummary('created-session')).toBeNull()
    expect(store.messages).toHaveLength(0)
    const draftCommandEvent = store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })
    expect(draftCommandEvent).toMatchObject({
      type: 'command_error',
      bot_id: 'bot-1',
      composer_scope: 'bot-1:draft-a',
    })
    expect(draftCommandEvent?.session_id).toBeUndefined()
    expect(store.startupSendFailureFor({
      botId: 'bot-1',
      sessionId: null,
      viewId: 'draft-a',
    }, 'bot-1:draft-a')).toMatchObject({
      botId: 'bot-1',
      composerScope: 'bot-1:draft-a',
      restoreInput: 'hello with skill',
      restoreAttachments: [attachment],
      restoreRequestedSkills: [requestedSkill],
    })
  })

  it('keeps the current session when deferred draft failure arrives after a session switch', async () => {
    sendEvents = []
    api.fetchSession.mockImplementation(async (_botId: string, sessionID: string) => ({
      id: sessionID,
      bot_id: 'bot-1',
      title: sessionID,
      type: 'chat',
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('hello with skill', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    await store.selectSession('session-b')
    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'created-session' } as UIStreamEvent)
    streamHandler?.({
      type: 'command_error',
      invocation_id: streamId,
      stream_id: streamId,
      session_id: 'created-session',
      composer_scope: 'bot-1:draft-a',
      terminal: true,
      error: {
        code: 'unsupported_skill_slash_context',
        message: 'Requested skills are not supported here.',
      },
    } as UIStreamEvent)
    const result = await sendPromise

    expect(result).toMatchObject({ ok: false, stage: 'startup' })
    expect(api.deleteSession).toHaveBeenCalledWith('bot-1', 'created-session')
    expect(store.deletedSession).toEqual({
      id: 'created-session',
      botId: 'bot-1',
      seq: 1,
      composerScope: 'bot-1:draft-a',
    })
    expect(store.sessionId).toBe('session-b')
    expect(store.knownSessionSummary('created-session')).toBeNull()
  })

  it('keeps deferred draft websocket errors scoped away from the switched session', async () => {
    sendEvents = []
    api.fetchSession.mockImplementation(async (_botId: string, sessionID: string) => ({
      id: sessionID,
      bot_id: 'bot-1',
      title: sessionID,
      type: 'chat',
    }))
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sendPromise = store.sendMessage('hello with skill', undefined, {
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    await store.selectSession('session-b')
    streamHandler?.({ type: 'session_created', stream_id: streamId, session_id: 'created-session' } as UIStreamEvent)
    streamHandler?.({ type: 'error', stream_id: streamId, session_id: 'created-session', message: 'model failed' } as UIStreamEvent)
    const result = await sendPromise

    expect(result).toMatchObject({ ok: false, stage: 'startup', composerScope: 'bot-1:draft-a' })
    expect(api.deleteSession).toHaveBeenCalledWith('bot-1', 'created-session')
    expect(store.deletedSession).toEqual({
      id: 'created-session',
      botId: 'bot-1',
      seq: 1,
      composerScope: 'bot-1:draft-a',
    })
    expect(store.sessionId).toBe('session-b')
    expect(store.startupSendFailure).toBeNull()
  })

  it('keeps the current command event when an off-scope command event arrives', async () => {
    const store = useChatStore()

    await store.selectBot('bot-1')
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-b',
      bot_id: 'bot-1',
      title: 'Session B',
      type: 'chat',
    })
    await store.selectSession('session-b')

    store.showCommandError('current_error', 'Current error', {
      botId: 'bot-1',
      sessionId: 'session-b',
      composerScope: 'bot-1:draft-a',
    })
    store.showCommandError('late_error', 'Late draft error', {
      botId: 'bot-1',
      composerScope: 'bot-1:draft-a',
    })

    expect(store.commandEvent).toMatchObject({
      type: 'command_error',
      session_id: 'session-b',
      error: { code: 'current_error' },
    })
    expect(store.commandEventForScope({ botId: 'bot-1', composerScope: 'bot-1:draft-a' })).toMatchObject({
      type: 'command_error',
      error: { code: 'late_error' },
    })
  })

  it('ignores queued websocket events from a previous bot connection', async () => {
    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot 1' },
      { id: 'bot-2', status: 'active', name: 'Bot 2' },
    ])
    const handlers: Array<{ botId: string; handler: UIStreamEventHandler }> = []
    api.connectWebSocket.mockImplementation((botId: string, handler: UIStreamEventHandler) => {
      handlers.push({ botId, handler })
      return {
        get connected() {
          return true
        },
        send: vi.fn(),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const staleHandler = handlers.find(entry => entry.botId === 'bot-1')?.handler
    expect(staleHandler).toBeDefined()

    await store.selectBot('bot-2')
    staleHandler?.({ type: 'start', stream_id: 'old-stream', session_id: 'old-session' } as UIStreamEvent)
    staleHandler?.({
      type: 'message',
      stream_id: 'old-stream',
      session_id: 'old-session',
      data: { id: 0, type: 'text', content: 'late old-bot output' },
    } as UIStreamEvent)

    expect(store.currentBotId).toBe('bot-2')
    expect(store.isSessionStreaming('bot-1', 'old-session')).toBe(false)
    expect(store.messages).toHaveLength(0)
  })

  it('reattaches an active assistant stream after switching away and back', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
    ], nextCursor: null })
    let returningToSessionA = false
    api.fetchMessagesUI.mockImplementation(async (_botId: string, targetSessionId: string) => {
      if (returningToSessionA && targetSessionId === 'session-a') {
        return [{
          id: 'server-user-a',
          role: 'user',
          text: 'first',
          attachments: [],
          timestamp: '2026-07-10T00:00:00.000Z',
        }]
      }
      return []
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    const sending = store.sendMessage('first')
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-a',
      data: { id: 0, type: 'text', content: 'before switch' },
    } as UIStreamEvent)

    await store.selectSession('session-b')
    await flushPromises()
    expect(store.messages).toHaveLength(0)

    returningToSessionA = true
    await store.selectSession('session-a')
    await flushPromises()
    await flushPromises()
    expect(store.messages.map(turn => turn.role)).toEqual(['user', 'assistant'])
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [{ type: 'text', content: 'before switch' }],
      streaming: true,
    })

    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-a',
      data: { id: 1, type: 'text', content: 'after return' },
    } as UIStreamEvent)
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      messages: [
        { type: 'text', content: 'before switch' },
        { type: 'text', content: 'after return' },
      ],
    })

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-a' } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: true })
  })

  it('replaces the hydrated assistant twin when reattaching an active stream', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
    ], nextCursor: null })
    let returningToSessionA = false
    api.fetchMessagesUI.mockImplementation(async (_botId: string, targetSessionId: string) => {
      if (!returningToSessionA || targetSessionId !== 'session-a') return []
      return [
        { id: 'server-user-a', role: 'user', text: 'first', attachments: [], timestamp: '2026-07-10T00:00:00.000Z' },
        {
          id: 'server-assistant-a',
          role: 'assistant',
          messages: [{ id: 0, type: 'text', content: 'persisted' }],
          timestamp: '2026-07-10T00:00:01.000Z',
        },
      ]
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('first')
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({
      type: 'message', stream_id: streamId, session_id: 'session-a',
      data: { id: 0, type: 'text', content: 'live' },
    } as UIStreamEvent)

    await store.selectSession('session-b')
    returningToSessionA = true
    await store.selectSession('session-a')
    await flushPromises()
    await flushPromises()

    expect(store.messages.filter(turn => turn.role === 'assistant')).toHaveLength(1)
    expect(store.messages.at(-1)).toMatchObject({
      role: 'assistant',
      streaming: true,
      messages: [{ type: 'text', content: 'live' }],
    })

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-a' } as UIStreamEvent)
    await sending
  })

  it('reattaches an active stream even when return hydration fails', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
      { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
    ], nextCursor: null })
    let returningToSessionA = false
    api.fetchMessagesUI.mockImplementation(async (_botId: string, targetSessionId: string) => {
      if (returningToSessionA && targetSessionId === 'session-a') throw new Error('offline')
      return []
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    const sending = store.sendMessage('first')
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string
    streamHandler?.({
      type: 'message', stream_id: streamId, session_id: 'session-a',
      data: { id: 0, type: 'text', content: 'live' },
    } as UIStreamEvent)

    await store.selectSession('session-b')
    returningToSessionA = true
    await store.selectSession('session-a')
    await flushPromises()
    await flushPromises()

    // The keyed Session view survives the round trip, including its optimistic
    // user turn; a failed refresh no longer reconstructs only the assistant.
    expect(store.messages.map(turn => turn.role)).toEqual(['user', 'assistant'])
    expect(store.messages[1]).toMatchObject({ role: 'assistant', streaming: true })

    returningToSessionA = false
    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-a' } as UIStreamEvent)
    await sending
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

    const sent: Array<{ type?: string; stream_id?: string; session_id?: string }> = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { type?: string; stream_id?: string; session_id?: string }) => {
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
    await flushPromises()
    const second = store.sendMessage('second')
    await flushPromises()

    const streamA = sent.find(item => item.type === 'message' && item.stream_id && item.session_id === 'session-a')?.stream_id
    const streamB = sent.find(item => item.type === 'message' && item.stream_id && item.session_id === 'session-b')?.stream_id
    expect(streamA).toBeTruthy()
    expect(streamB).toBeTruthy()
    expect(store.isSessionStreaming('bot-1', 'session-a')).toBe(true)
    expect(store.isSessionStreaming('bot-1', 'session-b')).toBe(true)

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

  it('hydrates hidden subagent session summaries after selecting them', async () => {
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
    expect(store.knownSessionSummary('session-subagent')).toMatchObject({
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
    expect(store.knownSessionSummary('session-subagent')).toMatchObject({
      id: 'session-subagent',
      type: 'subagent',
    })
  })

  it('hydrates a missing summary even when selecting the already-persisted session id', async () => {
    api.fetchSession.mockResolvedValueOnce({
      id: 'acp-session-hidden',
      bot_id: 'bot-1',
      title: 'Codex',
      type: 'chat',
      session_mode: 'chat',
      runtime_type: 'acp_agent',
      runtime_metadata: {
        acp_agent_id: 'codex',
        project_path: '/data',
        acp_project_mode: 'project',
      },
    })
    api.fetchMessagesUI.mockResolvedValue([])
    const selection = useChatSelectionStore()
    selection.setBot('bot-1')
    selection.setSession('acp-session-hidden', { explicitSelection: true })

    const store = useChatStore()
    await store.selectSession('acp-session-hidden')

    expect(api.fetchSession).toHaveBeenCalledWith('bot-1', 'acp-session-hidden')
    expect(store.sessionId).toBe('acp-session-hidden')
    expect(store.hasExplicitSessionSelection).toBe(true)
    expect(store.activeSession).toMatchObject({
      id: 'acp-session-hidden',
      runtime_type: 'acp_agent',
      runtime_metadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    })
  })

  it('switches to hidden sessions before their summary hydration resolves', async () => {
    api.fetchSessions.mockResolvedValueOnce({ items: [
      { id: 'session-visible', bot_id: 'bot-1', title: 'Visible', type: 'chat' },
    ], nextCursor: null })
    api.fetchMessagesUI.mockResolvedValueOnce([{
      id: 'visible-message',
      role: 'user',
      text: 'visible',
      attachments: [],
      timestamp: '2026-06-23T09:00:00.000Z',
    }])
    let resolveFetchSession: (session: unknown) => void = () => {}
    api.fetchSession.mockImplementationOnce(() => new Promise((resolve) => {
      resolveFetchSession = resolve
    }))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const selection = store.selectSession('session-hidden')
    await flushPromises()

    expect(store.sessionId).toBe('session-hidden')
    expect(store.messages).toEqual([])

    resolveFetchSession({
      id: 'session-hidden',
      bot_id: 'bot-1',
      title: 'Hidden',
      type: 'subagent',
      parent_session_id: 'session-visible',
    })
    await selection

    expect(store.activeSession).toMatchObject({
      id: 'session-hidden',
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

    // Both copies would survive id-keyed dedup. Consolidation keeps the
    // optimistic render id stable so keyed chat DOM (including turn reserve)
    // does not remount, while recording the canonical server id separately.
    expect(store.messages.length).toBe(baseLength + 1)
    const justSent = store.messages.filter(m => m.role === 'user' && (m as { text?: string }).text === 'just sent')
    expect(justSent.length).toBe(1)
    expect(justSent[0]).toMatchObject({
      id: '1700000000000',
      serverId: 'server-user-1',
    })
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

  it('emits an explicit deleted-session signal after a session delete succeeds', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-2')

    expect(api.deleteSession).toHaveBeenCalledWith('bot-1', 'session-2')
    expect(store.deletedSession).toEqual({
      id: 'session-2',
      botId: 'bot-1',
      seq: 1,
    })
    expect(store.sessions.map(session => session.id)).toEqual(['session-1'])
    expect(store.sessionId).toBe('session-1')
  })

  it('aborts a deleted Session stream and ignores its late events in the focused Session', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const sending = store.sendMessage('stream in A')
    await flushPromises()
    const streamId = lastStreamId
    expect(streamId).not.toBe('')
    const runningSnapshot = runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 10)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: streamId,
      seq: 10,
      snapshot: runningSnapshot,
    } as UIStreamEvent)

    await store.selectSession('session-2')
    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-1')
    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })

    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-1',
      data: { id: 1, type: 'text', content: 'late A output' },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'end',
      stream_id: streamId,
      session_id: 'session-1',
    } as UIStreamEvent)
    await flushPromises()

    expect(abortedWSStreams).toContain(streamId)
    expect(store.sessionId).toBe('session-2')
    expect(store.messages).toEqual([])
    expect(store.sessions.map(session => session.id)).toEqual(['session-2'])
  })

  it('does not fall back to a hidden schedule session after deleting the active recent session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'schedule-1', bot_id: 'bot-1', title: 'Morning run', type: 'schedule' },
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    expect(store.sessionId).toBe('session-1')

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-1')

    expect(store.sessions.map(session => session.id)).toEqual(['schedule-1'])
    expect(store.sessionId).toBeNull()
    expect(store.messages).toEqual([])
  })

  it('falls back within the schedule sidebar mode when deleting an active schedule session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'schedule-1', bot_id: 'bot-1', title: 'Morning run', type: 'schedule' },
        { id: 'schedule-2', bot_id: 'bot-1', title: 'Evening run', type: 'schedule' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    expect(store.sessionId).toBe('schedule-1')

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('schedule-1', { fallbackMode: 'schedule' })

    expect(store.sessions.map(session => session.id)).toEqual(['schedule-2'])
    expect(store.sessionId).toBe('schedule-2')
  })

  it('does not mutate the active bot state when a delete resolves after switching bots', async () => {
    api.fetchBots.mockResolvedValue([
      { id: 'bot-1', status: 'active', name: 'Bot A' },
      { id: 'bot-2', status: 'active', name: 'Bot B' },
    ])
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'shared-session', bot_id: 'bot-1', title: 'A shared id', type: 'chat' },
        { id: 'session-a2', bot_id: 'bot-1', title: 'A2', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    let resolveDelete: () => void = () => {}
    api.deleteSession.mockImplementationOnce(() => new Promise<void>((resolve) => {
      resolveDelete = resolve
    }))
    const deletePromise = store.removeSession('shared-session')

    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'shared-session', bot_id: 'bot-2', title: 'B shared id', type: 'chat' },
        { id: 'session-b2', bot_id: 'bot-2', title: 'B2', type: 'chat' },
      ],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'bot-2-user',
        role: 'user',
        text: 'bot two prompt',
        timestamp: '2026-06-20T00:00:00.000Z',
      },
      {
        id: 'bot-2-assistant',
        role: 'assistant',
        messages: [{ id: 1, type: 'text', content: 'bot two reply' }],
        timestamp: '2026-06-20T00:00:01.000Z',
      },
    ])
    await store.selectBot('bot-2')
    await flushPromises()
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    const botTwoSend = store.sendMessage('keep bot two streaming')
    await flushPromises()
    const botTwoStreamId = lastStreamId

    expect(store.isChatViewStreaming({
      botId: 'bot-1',
      sessionId: 'shared-session',
      viewId: 'chat:bot-1',
    })).toBe(false)
    expect(store.isChatViewStreaming({
      botId: 'bot-2',
      sessionId: 'shared-session',
      viewId: 'chat:bot-2',
    })).toBe(true)

    resolveDelete()
    await deletePromise

    expect(store.currentBotId).toBe('bot-2')
    expect(store.sessions.map(session => session.id)).toEqual(['shared-session', 'session-b2'])
    expect(store.sessionId).toBe('shared-session')
    expect(store.messages.slice(0, 2).map(message => message.id)).toEqual(['bot-2-user', 'bot-2-assistant'])
    expect(abortedWSStreams).not.toContain(botTwoStreamId)
    expect(store.deletedSession).toEqual({
      id: 'shared-session',
      botId: 'bot-1',
      seq: 1,
    })
    streamHandler?.({
      type: 'end',
      stream_id: botTwoStreamId,
      session_id: 'shared-session',
    } as UIStreamEvent)
    await expect(botTwoSend).resolves.toMatchObject({ ok: true })
  })

  it('does not resurrect a deleted session when an older same-bot list refresh resolves late', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    let resolveRefresh: (value: { items: Array<{ id: string, bot_id: string, title: string, type: string }>, nextCursor: null }) => void = () => {}
    api.fetchSessions.mockImplementationOnce(() => new Promise((resolve) => {
      resolveRefresh = resolve
    }))
    sessionsActivityHandler?.({
      type: 'session_created',
      session_id: 'session-3',
      session_type: 'chat',
      title: 'C',
    })
    await flushPromises()

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-2')

    resolveRefresh({
      items: [
        { id: 'session-2', bot_id: 'bot-1', title: 'Deleted stale copy', type: 'chat' },
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-3', bot_id: 'bot-1', title: 'C', type: 'chat' },
      ],
      nextCursor: null,
    })
    await flushPromises()

    expect(store.sessions.map(session => session.id)).toEqual(['session-1', 'session-3'])
    expect(store.knownSessionSummary('session-2')).toBeNull()
  })

  it('does not select a tombstoned session from a stale initialize response', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-2')

    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-2', bot_id: 'bot-1', title: 'Deleted stale copy', type: 'chat' },
      ],
      nextCursor: null,
    })
    await store.initialize()

    expect(store.sessions).toEqual([])
    expect(store.sessionId).toBeNull()
    expect(store.knownSessionSummary('session-2')).toBeNull()
  })

  it('refreshes the current transcript when the session message stream reports dropped events', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'message-1', role: 'user', text: 'old', attachments: [], timestamp: '2026-06-20T00:00:00.000Z' },
    ])
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()
    expect(store.messages.map(message => message.id)).toEqual(['message-1'])

    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'message-2', role: 'user', text: 'refreshed', attachments: [], timestamp: '2026-06-20T00:00:01.000Z' },
    ])
    _sessionMessageHandler?.({ type: 'dropped', count: 3 })
    await flushPromises()

    expect(api.fetchMessagesUI).toHaveBeenLastCalledWith('bot-1', 'session-1', { limit: 30 })
    expect(store.messages.map(message => message.id)).toEqual(['message-2'])
  })

  it('refreshes the session list when the bot activity stream reports dropped events', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
      ],
      nextCursor: null,
    })
    sessionsActivityHandler?.({ type: 'dropped', count: 2 })
    await flushPromises()

    expect(store.sessions.map(session => session.id)).toEqual(['session-2', 'session-1'])
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

  it('keeps A and B transcripts independent while non-focused B receives Session SSE', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const latest = new Map([
      ['session-a', [{ id: 'a-1', role: 'user', text: 'from A', attachments: [], timestamp: '2026-07-11T00:00:00Z' }]],
      ['session-b', [{ id: 'b-1', role: 'user', text: 'from B', attachments: [], timestamp: '2026-07-11T00:00:01Z' }]],
    ])
    api.fetchMessagesUI.mockImplementation(async (_botId: string, targetSessionId: string) => latest.get(targetSessionId) ?? [])
    const store = useChatStore()

    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }
    store.bindChatView('chat:a', targetA, true)
    store.bindChatView('chat:b', targetB, true)
    await flushPromises()
    await flushPromises()

    expect(store.chatView(targetA).transcript.messages.map(message => message.id)).toEqual(['a-1'])
    expect(store.chatView(targetB).transcript.messages.map(message => message.id)).toEqual(['b-1'])
    expect(sessionMessageHandlers.has('bot-1:session-a')).toBe(true)
    expect(sessionMessageHandlers.has('bot-1:session-b')).toBe(true)

    store.focusChatView('chat:a')
    await store.selectSession('session-a')
    latest.set('session-b', [
      { id: 'b-1', role: 'user', text: 'from B', attachments: [], timestamp: '2026-07-11T00:00:01Z' },
      { id: 'b-2', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'B updated' }], timestamp: '2026-07-11T00:00:02Z' },
    ])
    sessionMessageHandlers.get('bot-1:session-b')?.({
      type: 'message_created',
      message: { id: 'b-2', session_id: 'session-b', created_at: '2026-07-11T00:00:02Z' },
    } as never)
    await new Promise(resolve => setTimeout(resolve, 150))
    await flushPromises()

    expect(store.sessionId).toBe('session-a')
    expect(store.chatView(targetA).transcript.messages.map(message => message.id)).toEqual(['a-1'])
    expect(store.chatView(targetB).transcript.messages.map(message => message.id)).toEqual(['b-1', 'b-2'])
  })

  it('subscribes every visible Session pane to runtime and unsubscribes the last hidden pane', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    runtimeSubscribeMessages = []
    store.bindChatView('chat:a', { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:a' }, true)
    store.bindChatView('chat:b', { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }, true)
    await flushPromises()

    expect(runtimeSubscribeMessages).toContainEqual(
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-b' }),
    )

    runtimeUnsubscribeMessages = []
    store.setChatViewVisible('chat:b', false)
    expect(runtimeUnsubscribeMessages).toContainEqual(expect.objectContaining({
      type: 'runtime_unsubscribe',
      session_id: 'session-b',
      invocation_id: expect.any(String),
      stream_id: expect.any(String),
    }))
    const unsubscribe = runtimeUnsubscribeMessages[0]
    expect(unsubscribe?.stream_id).toBe(unsubscribe?.invocation_id)
  })

  it('isolates the same runtime subscription invocation id across sessions', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
        { id: 'session-c', bot_id: 'bot-1', title: 'C', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []
    const sharedInvocationId = '00000000-0000-4000-8000-000000000003'
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(sharedInvocationId)
    store.bindChatView('chat:subscription-b', {
      botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:subscription-b',
    }, true)
    store.bindChatView('chat:subscription-c', {
      botId: 'bot-1', sessionId: 'session-c', viewId: 'chat:subscription-c',
    }, true)
    uuid.mockRestore()
    await flushPromises()
    await flushPromises()
    expect(runtimeSubscribeMessages).toEqual(expect.arrayContaining([
      expect.objectContaining({ invocation_id: sharedInvocationId, session_id: 'session-b' }),
      expect.objectContaining({ invocation_id: sharedInvocationId, session_id: 'session-c' }),
    ]))

    vi.useFakeTimers()
    try {
      streamHandler?.({
        type: 'command_error',
        invocation_id: sharedInvocationId,
        session_id: 'session-b',
        action_id: 'runtime_subscribe',
        terminal: true,
        error: { code: 'runtime_response_failed', message: 'session B subscription failed' },
      } as UIStreamEvent)
      streamHandler?.({
        type: 'command_result',
        invocation_id: sharedInvocationId,
        session_id: 'session-c',
        action_id: 'runtime_subscribe',
        terminal: true,
      } as UIStreamEvent)
      const subscribeCount = runtimeSubscribeMessages.length
      await vi.advanceTimersToNextTimerAsync()
      expect(runtimeSubscribeMessages).toHaveLength(subscribeCount + 1)
      expect(runtimeSubscribeMessages.at(-1)).toMatchObject({
        type: 'runtime_subscribe',
        session_id: 'session-b',
      })
    } finally {
      vi.useRealTimers()
      consoleError.mockRestore()
    }
  })

  it('hydrates a visible non-focused Session summary before it is activated', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchSession.mockResolvedValueOnce({
      id: 'session-b',
      bot_id: 'bot-1',
      title: 'Hidden subagent',
      type: 'subagent',
      runtime_type: 'acp_agent',
      runtime_metadata: { acp_agent_id: 'codex' },
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.focusChatView(targetA.viewId)

    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()

    expect(store.sessionId).toBe('session-a')
    expect(store.chatTargetFor(targetB)).toMatchObject({
      session: { id: 'session-b', type: 'subagent' },
      runtimeType: 'acp_agent',
      isACP: true,
    })
    expect(store.chatReadOnlyFor(targetB)).toBe(true)
    expect(api.fetchSession).toHaveBeenCalledWith('bot-1', 'session-b')
  })

  it('keeps an unknown real Session read-only until its summary confirms it is writable', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const summary = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.fetchSession.mockReturnValueOnce(summary.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }
    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()

    expect(store.chatReadOnlyFor(targetB)).toBe(true)
    await expect(store.sendMessage('must not send', undefined, { target: targetB })).resolves.toMatchObject({
      ok: false,
      stage: 'startup',
    })
    expect(sentWSMessages).toEqual([])

    summary.resolve({ id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' })
    await flushPromises()
    expect(store.chatReadOnlyFor(targetB)).toBe(false)
  })

  it('does not remember a visible Session summary that resolves after deletion', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const summary = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.fetchSession.mockReturnValueOnce(summary.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }
    store.bindChatView(targetB.viewId, targetB, true)
    await flushPromises()

    api.deleteSession.mockResolvedValueOnce(undefined)
    await store.removeSession('session-b')
    summary.resolve({ id: 'session-b', bot_id: 'bot-1', title: 'Deleted B', type: 'chat' })
    await flushPromises()

    expect(store.knownSessionSummary('session-b')).toBeNull()
    expect(store.sessions.some(session => session.id === 'session-b')).toBe(false)
  })

  it('routes an optimistic send and abort only to its explicit pane target', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([])
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:a' }
    const targetB = { botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:b' }
    store.bindChatView('chat:a', targetA, true)
    store.bindChatView('chat:b', targetB, true)
    store.focusChatView('chat:b')
    await store.selectSession('session-b')

    const sending = store.sendMessage('send to A', undefined, {
      target: targetA,
      composerScope: 'bot-1:chat:a',
    })
    await flushPromises()

    expect(store.chatView(targetA).transcript.messages.map(message => message.role)).toEqual(['user', 'assistant'])
    expect(store.chatView(targetB).transcript.messages).toEqual([])
    expect(sentWSMessages.at(-1)).toMatchObject({ session_id: 'session-a', composer_scope: 'bot-1:chat:a' })
    const runningSnapshot = runtimeSnapshotFromScript([], 'session-a', lastStreamId, 'running', 10)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-a',
      stream_id: lastStreamId,
      seq: 10,
      snapshot: runningSnapshot,
    } as UIStreamEvent)

    store.abort(targetA)
    const abortedSnapshot = runtimeSnapshotFromScript([], 'session-a', lastStreamId, 'aborted', 11, 'aborted')
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-a',
      stream_id: lastStreamId,
      seq: 11,
      snapshot: abortedSnapshot,
    } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })
    expect(store.sessionId).toBe('session-b')
    expect(store.chatView(targetB).transcript.messages).toEqual([])
  })

  it('shares one Session transcript and one Session SSE across two visible panes', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([
      { id: 'a-1', role: 'user', text: 'shared', attachments: [], timestamp: '2026-07-11T00:00:00Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    const first = store.bindChatView('chat:left', {
      botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:left',
    }, true)
    const second = store.bindChatView('chat:right', {
      botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:right',
    }, true)
    await flushPromises()

    expect(second).toBe(first)
    expect(first.transcript.messages.map(message => message.id)).toEqual(['a-1'])
    expect(api.streamSessionMessageEvents.mock.calls.filter(call => call[1] === 'session-a')).toHaveLength(1)
  })

  it('detaches the Session SSE when its last pane hides and reconnects from cache', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([
      { id: 'a-1', role: 'user', text: 'cached', attachments: [], timestamp: '2026-07-11T00:00:00Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    const target = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:left' }
    const cached = store.bindChatView('chat:left', target, true)
    store.bindChatView('chat:right', { ...target, viewId: 'chat:right' }, true)
    await flushPromises()

    expect(sessionMessageHandlers.has('bot-1:session-a')).toBe(true)
    store.setChatViewVisible('chat:left', false)
    expect(sessionMessageHandlers.has('bot-1:session-a')).toBe(true)

    store.setChatViewVisible('chat:right', false)
    expect(sessionMessageHandlers.has('bot-1:session-a')).toBe(false)
    expect(store.chatView(target)).toBe(cached)
    expect(cached.transcript.messages.map(message => message.id)).toEqual(['a-1'])

    store.setChatViewVisible('chat:left', true)
    await flushPromises()
    expect(sessionMessageHandlers.has('bot-1:session-a')).toBe(true)
    expect(store.chatView(target)).toBe(cached)
  })

  it('keeps Draft ACP Agent state scoped by pane and closes only the removed Draft runtime', async () => {
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)

    store.focusChatView(targetA.viewId)
    store.stageACPSession({ agentId: 'codex' }, {}, targetA)
    await store.ensurePendingACPRuntime(targetA)
    store.focusChatView(targetB.viewId)
    store.stageACPSession({ agentId: 'claude' }, {}, targetB)

    expect(store.pendingACPStateFor(targetA)).toMatchObject({
      metadata: { acp_agent_id: 'codex' },
      runtimeId: 'rt_warm',
    })
    expect(store.pendingACPStateFor(targetB)).toMatchObject({
      metadata: { acp_agent_id: 'claude' },
    })
    expect(api.closeACPRuntime).not.toHaveBeenCalled()

    store.focusChatView(targetA.viewId)
    expect(store.pendingACPSessionMetadata).toMatchObject({ acp_agent_id: 'codex' })
    store.unbindChatView(targetA.viewId)

    expect(api.closeACPRuntime).toHaveBeenCalledWith('bot-1', 'rt_warm')
    expect(store.pendingACPStateFor(targetB)).toMatchObject({ metadata: { acp_agent_id: 'claude' } })
  })

  it('does not let a late native Draft creation steal focus from another Draft', async () => {
    const creation = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.createSession.mockReturnValueOnce(creation.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)

    const sending = store.sendMessage('from A', undefined, { target: targetA })
    await flushPromises()
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })
    creation.resolve({ id: 'session-a', bot_id: 'bot-1', title: '', type: 'chat' })
    await sending

    expect(store.sessionId).toBeNull()
    expect(store.chatView({ ...targetA, sessionId: 'session-a' }).sessionId).toBe('session-a')
    expect(store.chatView(targetB).kind).toBe('draft')
    expect(store.userSentInSession).toMatchObject({ id: 'session-a', viewId: targetA.viewId })
  })

  it('drops a late Draft creation result after the authenticated scope resets', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    const creation = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.createSession.mockReturnValueOnce(creation.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const target = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    store.bindChatView(target.viewId, target, true)
    store.focusChatView(target.viewId)

    const sending = store.sendMessage('old user send', undefined, { target })
    await flushPromises()
    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))
    creation.resolve({ id: 'old-session', bot_id: 'bot-1', title: '', type: 'chat' })

    await expect(sending).resolves.toMatchObject({ ok: false })
    expect(store.sessions).toEqual([])
    expect(store.knownSessionSummary('old-session')).toBeNull()
    expect(store.currentBotId).toBeNull()
    expect(store.isChatViewCreatingSession(target)).toBe(false)
  })

  it('restores a failed ACP creation to its owning Draft after focus moves', async () => {
    const creation = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
    }>()
    api.createSession.mockReturnValueOnce(creation.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)
    store.stageACPSession({ agentId: 'codex' }, {}, targetA)

    const sending = store.sendMessage('from ACP A', undefined, { target: targetA })
    await flushPromises()
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })
    store.stageACPSession({ agentId: 'claude' }, {}, targetB)
    creation.reject(new Error('create failed'))
    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'startup' })

    expect(store.pendingACPStateFor(targetA)).toMatchObject({ metadata: { acp_agent_id: 'codex' } })
    expect(store.pendingACPStateFor(targetB)).toMatchObject({ metadata: { acp_agent_id: 'claude' } })
    expect(store.pendingACPSessionMetadata).toMatchObject({ acp_agent_id: 'claude' })
    expect(store.sessionId).toBeNull()
  })

  it('creates an explicit non-focused ACP Draft with its saved Agent and warm runtime', async () => {
    sendEvents = [{ type: 'start' } as UIStreamEvent]
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)
    store.stageACPSession({ agentId: 'codex' }, {}, targetA)
    await store.ensurePendingACPRuntime(targetA)
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })

    const sending = store.sendMessage('run in A', undefined, { target: targetA })
    await flushPromises()
    await flushPromises()

    expect(api.createSession).toHaveBeenLastCalledWith('bot-1', expect.objectContaining({
      runtimeType: 'acp_agent',
      acpRuntimeId: 'rt_warm',
      runtimeMetadata: expect.objectContaining({ acp_agent_id: 'codex' }),
    }))
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPStateFor(targetA)).toBeNull()
    expect(api.closeACPRuntime).not.toHaveBeenCalledWith('bot-1', 'rt_warm')
    const runningSnapshot = runtimeSnapshotFromScript([], 'session-1', lastStreamId, 'running', 10)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: lastStreamId,
      seq: 10,
      snapshot: runningSnapshot,
    } as UIStreamEvent)

    store.abort({ ...targetA, sessionId: 'session-1' })
    const abortedSnapshot = runtimeSnapshotFromScript([], 'session-1', lastStreamId, 'aborted', 11, 'aborted')
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: lastStreamId,
      seq: 11,
      snapshot: abortedSnapshot,
    } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })
  })

  it('does not let a late session_created event steal focus from another Draft', async () => {
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)
    const sending = store.sendMessage('activate A', undefined, {
      target: targetA,
      requestedSkills: [{ name: 'alpha' }],
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string

    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })
    streamHandler?.({
      type: 'session_created',
      stream_id: streamId,
      session_id: 'session-a',
    } as UIStreamEvent)

    expect(store.sessionId).toBeNull()
    expect(store.chatView({ ...targetA, sessionId: 'session-a' }).sessionId).toBe('session-a')
    expect(store.chatView(targetB).kind).toBe('draft')

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-a' } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: true })
  })

  it('cleans a non-focused deferred Session view and SSE after startup failure', async () => {
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)

    const sending = store.sendMessage('activate A', [{
      type: 'file',
      base64: 'data:text/plain;base64,aGVsbG8=',
      mime: 'text/plain',
      name: 'note.txt',
    }], {
      target: targetA,
      requestedSkills: [{ name: 'alpha' }],
      composerScope: 'bot-1:chat:draft-a',
    })
    await flushPromises()
    const streamId = sentWSMessages[0]?.stream_id as string
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })

    streamHandler?.({
      type: 'session_created',
      stream_id: streamId,
      session_id: 'created-a',
    } as UIStreamEvent)
    await flushPromises()
    expect(sessionMessageHandlers.has('bot-1:created-a')).toBe(true)

    streamHandler?.({
      type: 'command_error',
      invocation_id: streamId,
      stream_id: streamId,
      session_id: 'created-a',
      composer_scope: 'bot-1:chat:draft-a',
      terminal: true,
      error: { code: 'unsupported', message: 'preflight failed' },
    } as UIStreamEvent)

    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'startup' })
    expect(store.sessionId).toBeNull()
    expect(store.deletedSession).toMatchObject({ id: 'created-a', botId: 'bot-1' })
    expect(sessionMessageHandlers.has('bot-1:created-a')).toBe(false)
  })

  it('does not clear Draft B staging when Session A Agent update resolves late', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const update = deferred<{
      id: string
      bot_id: string
      title: string
      type: string
      runtime_type: string
      metadata: Record<string, unknown>
    }>()
    api.updateSessionAgent.mockReturnValueOnce(update.promise)
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:session-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)
    await store.selectSession('session-a')

    const updating = store.updateCurrentSessionAgent({ agentId: 'codex' }, targetA)
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })
    store.stageACPSession({ agentId: 'claude' }, {}, targetB)
    await store.ensurePendingACPRuntime(targetB)
    update.resolve({
      id: 'session-a',
      bot_id: 'bot-1',
      title: 'A',
      type: 'acp_agent',
      runtime_type: 'acp_agent',
      metadata: { acp_agent_id: 'codex' },
    })
    await updating

    expect(store.sessionId).toBeNull()
    expect(store.pendingACPStateFor(targetB)).toMatchObject({
      metadata: { acp_agent_id: 'claude' },
      runtimeId: 'rt_warm',
    })
    expect(api.closeACPRuntime).not.toHaveBeenCalledWith('bot-1', 'rt_warm')
  })

  it('routes a late /new Agent result back to its origin Draft request', async () => {
    const store = useChatStore()
    await store.selectBot('bot-1')
    const targetA = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    const targetB = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-b' }
    store.bindChatView(targetA.viewId, targetA, true)
    store.bindChatView(targetB.viewId, targetB, true)
    store.focusChatView(targetA.viewId)
    const settings = deferred<{ data: {
      chat_runtime: string
      chat_acp_agent_id: string
      chat_acp_project_path: string
      chat_acp_project_mode: string
    } }>()
    sdk.getBotsByBotIdSettings.mockReturnValueOnce(settings.promise)

    const command = store.sendMessage('/new codex', undefined, { target: targetA })
    await flushPromises()
    store.focusChatView(targetB.viewId)
    store.selectDraft({ explicitSelection: true })
    store.stageACPSession({ agentId: 'claude' }, {}, targetB)
    settings.resolve({ data: {
      chat_runtime: 'acp_agent',
      chat_acp_agent_id: 'codex',
      chat_acp_project_path: '/data/a',
      chat_acp_project_mode: 'project',
    } })

    await expect(command).resolves.toMatchObject({ ok: true })
    expect(store.sessionId).toBeNull()
    expect(store.pendingACPStateFor(targetB)).toMatchObject({ metadata: { acp_agent_id: 'claude' } })
    expect(store.draftViewRequested).toMatchObject({
      botId: 'bot-1',
      viewId: targetA.viewId,
      expectedSessionId: null,
      input: { agentId: 'codex', projectPath: '/data/a', projectMode: 'project' },
      activate: true,
    })
  })

  it('keeps the newest /new Agent choice when an older settings request resolves last', async () => {
    const codexSettings = deferred<{ data: {
      chat_runtime: string
      chat_acp_agent_id: string
      chat_acp_project_path: string
      chat_acp_project_mode: string
    } }>()
    const claudeSettings = deferred<{ data: {
      chat_runtime: string
      chat_acp_agent_id: string
      chat_acp_project_path: string
      chat_acp_project_mode: string
    } }>()
    const store = useChatStore()
    await store.selectBot('bot-1')
    sdk.getBotsByBotIdSettings
      .mockReturnValueOnce(codexSettings.promise)
      .mockReturnValueOnce(claudeSettings.promise)
    const target = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    store.bindChatView(target.viewId, target, true)
    store.focusChatView(target.viewId)

    const older = store.sendMessage('/new codex', undefined, { target })
    await flushPromises()
    const newer = store.sendMessage('/new claude-code', undefined, { target })
    await flushPromises()
    claudeSettings.resolve({ data: {
      chat_runtime: 'acp_agent',
      chat_acp_agent_id: 'claude-code',
      chat_acp_project_path: '/data/claude',
      chat_acp_project_mode: 'project',
    } })
    await newer
    const latestRequest = store.draftViewRequested
    expect(latestRequest).toMatchObject({
      viewId: target.viewId,
      input: { agentId: 'claude-code', projectPath: '/data/claude' },
    })

    codexSettings.resolve({ data: {
      chat_runtime: 'acp_agent',
      chat_acp_agent_id: 'codex',
      chat_acp_project_path: '/data/codex',
      chat_acp_project_mode: 'project',
    } })
    await older

    expect(store.draftViewRequested).toBe(latestRequest)
    expect(store.draftViewRequested?.input?.agentId).toBe('claude-code')
  })

  it('drops a late /new Agent result after the authenticated scope resets', async () => {
    const windowTarget = new EventTarget()
    vi.stubGlobal('window', windowTarget)
    const settings = deferred<{ data: {
      chat_runtime: string
      chat_acp_agent_id: string
      chat_acp_project_path: string
      chat_acp_project_mode: string
    } }>()
    const store = useChatStore()
    await store.selectBot('bot-1')
    sdk.getBotsByBotIdSettings.mockReturnValueOnce(settings.promise)
    const target = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    store.bindChatView(target.viewId, target, true)
    store.focusChatView(target.viewId)

    const command = store.sendMessage('/new codex', undefined, { target })
    await flushPromises()
    windowTarget.dispatchEvent(new CustomEvent(AUTH_SESSION_CLEARED_EVENT, {
      detail: { reason: 'logout' },
    }))
    settings.resolve({ data: {
      chat_runtime: 'acp_agent',
      chat_acp_agent_id: 'codex',
      chat_acp_project_path: '/data/a',
      chat_acp_project_mode: 'project',
    } })

    await expect(command).resolves.toMatchObject({ ok: true })
    expect(store.draftViewRequested).toBeNull()
    expect(store.currentBotId).toBeNull()
  })

  it('keeps a manual Draft Agent choice made after a deferred /new command', async () => {
    const settings = deferred<{ data: {
      chat_runtime: string
      chat_acp_agent_id: string
      chat_acp_project_path: string
      chat_acp_project_mode: string
    } }>()
    const store = useChatStore()
    await store.selectBot('bot-1')
    sdk.getBotsByBotIdSettings.mockReturnValueOnce(settings.promise)
    const target = { botId: 'bot-1', sessionId: null, viewId: 'chat:draft-a' }
    store.bindChatView(target.viewId, target, true)
    store.focusChatView(target.viewId)

    const command = store.sendMessage('/new codex', undefined, { target })
    await flushPromises()
    store.stageACPSession({ agentId: 'claude' }, {}, target)
    await store.ensurePendingACPRuntime(target)

    settings.resolve({ data: {
      chat_runtime: 'acp_agent',
      chat_acp_agent_id: 'codex',
      chat_acp_project_path: '/data/codex',
      chat_acp_project_mode: 'project',
    } })
    await expect(command).resolves.toMatchObject({ ok: true })

    expect(store.draftViewRequested).toBeNull()
    expect(store.pendingACPStateFor(target)).toMatchObject({
      metadata: { acp_agent_id: 'claude' },
      runtimeId: 'rt_warm',
    })
    expect(api.closeACPRuntime).not.toHaveBeenCalledWith('bot-1', 'rt_warm')
  })
  it('applies the rich active-run contract script to the current assistant turn', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const sendPromise = store.sendMessage('please inspect')
    await flushPromises()
    await flushPromises()
    const runtimeEvent = structuredClone(richActiveRunContractFixture.runtime_snapshot)
    runtimeEvent.stream_id = lastStreamId
    runtimeEvent.snapshot.current_run_view!.stream_id = lastStreamId
    streamHandler?.(runtimeEvent as UIStreamEvent)

    expect(sentWSMessages[0]).toMatchObject({
      type: 'message',
      text: 'please inspect',
      session_id: 'session-1',
    })

    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')

    expect(assistant.messages.find(block => block.type === 'reasoning')).toMatchObject({
      content: 'I need to inspect the workspace.',
    })
    expect(assistant.messages.find(block => block.type === 'text')).toMatchObject({
      content: 'I will check the current state.',
    })

    const execTool = assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-exec')
    expect(execTool).toMatchObject({
      type: 'tool',
      toolName: 'exec',
      done: true,
      running: false,
      progress: ['queued', { stdout: '/workspace\n' }],
    })

    const approvalTool = assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-approval')
    expect(approvalTool).toMatchObject({
      approval: {
        approval_id: 'approval-1',
        status: 'pending',
        can_approve: true,
      },
    })

    const askUserTool = assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-ask')
    expect(askUserTool).toMatchObject({
      userInput: {
        user_input_id: 'input-1',
        status: 'pending',
        can_respond: true,
        questions: [expect.objectContaining({ text: 'Continue?' })],
      },
    })

    api.fetchMessagesUI.mockResolvedValue([])
    const completed = structuredClone(runtimeEvent) as unknown as { type: string, seq: number, snapshot: SessionruntimeSnapshot }
    completed.type = 'runtime_snapshot'
    completed.seq += 1
    completed.snapshot.seq = completed.seq
    completed.snapshot.current_run_view!.status = 'completed'
    streamHandler?.(completed as UIStreamEvent)
    await sendPromise
  })

  it('records interrupted runtime streams as stream-stage failures after visible output', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const sendPromise = store.sendMessage('please run')
    await flushPromises()
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>(() => {}))
    const runtimeEvent = structuredClone(interruptedRunContractFixture.runtime_snapshot)
    runtimeEvent.stream_id = lastStreamId
    runtimeEvent.snapshot.current_run_view!.stream_id = lastStreamId
    streamHandler?.(runtimeEvent as UIStreamEvent)
    const result = await sendPromise

    expect(result).toMatchObject({
      ok: false,
      stage: 'stream',
      error: 'runtime interrupted',
    })
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.messages.some(block => block.type === 'text' && block.content === 'partial output')).toBe(true)
    expect(assistant.messages.some(block => block.type === 'error' && block.content === 'runtime interrupted')).toBe(true)
  })

  it('does not let stale active-run events for another session pollute the visible transcript', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchMessagesUI.mockResolvedValueOnce([])
    store.selectSession('session-2')
    await flushPromises()
    expect(store.sessionId).toBe('session-2')
    expect(store.messages).toEqual([])

    streamHandler?.({ type: 'start', stream_id: 'stream-old', session_id: 'session-1' } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: 'stream-old',
      session_id: 'session-1',
      data: { id: 0, type: 'text', content: 'old session output' },
    } as UIStreamEvent)

    expect(store.messages).toEqual([])

    streamHandler?.({ type: 'end', stream_id: 'stream-old', session_id: 'session-1' } as UIStreamEvent)
  })

  it('does not let stale runtime state for another session pollute the visible transcript', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchMessagesUI.mockResolvedValueOnce([])
    store.selectSession('session-2')
    await flushPromises()
    expect(store.sessionId).toBe('session-2')
    expect(store.messages).toEqual([])

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', 'stream-old-runtime'), 'session-1', 'stream-old-runtime', 'running', 11),
    } as UIStreamEvent)

    expect(store.streaming).toBe(false)
    expect(store.messages).toEqual([])
  })

  it('reattaches a still-running assistant when switching away and back', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const running = runtimeSnapshotFromScript(
      [{ type: 'message', data: { id: 0, type: 'text', content: 'still working' } }],
      'session-1',
      'stream-switch-back',
      'running',
      10,
    )
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 10, snapshot: running } as UIStreamEvent)
    expect(store.messages.some(turn => turn.role === 'assistant' && turn.messages.some(block => block.type === 'text' && block.content === 'still working'))).toBe(true)

    api.fetchMessagesUI.mockResolvedValue([])
    await store.selectSession('session-2')
    await flushPromises()
    await store.selectSession('session-1')
    await flushPromises()
    expect(store.messages.some(turn => turn.role === 'assistant' && turn.messages.some(block => block.type === 'text' && block.content === 'still working'))).toBe(true)

    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 11, snapshot: { ...running, seq: 11 } } as UIStreamEvent)
    expect(store.messages.some(turn => turn.role === 'assistant' && turn.messages.some(block => block.type === 'text' && block.content === 'still working'))).toBe(true)
  })

  it('does not duplicate persisted assistant history from a cold completed runtime snapshot', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([
      { id: 'assistant-persisted', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'persisted answer' }], timestamp: '2026-07-12T00:00:00Z', streaming: false },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 20,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'persisted answer' } }],
        'session-1',
        'stream-already-completed',
        'completed',
        20,
      ),
    } as UIStreamEvent)

    const assistants = store.messages.filter(turn => turn.role === 'assistant')
    expect(assistants).toHaveLength(1)
    expect(assistants[0]?.id).toBe('assistant-persisted')
  })

  it('starts the new session history request without waiting for the previous session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    let releaseSession2!: () => void
    const session2Pending = new Promise<UITurn[]>((resolve) => {
      releaseSession2 = () => resolve([])
    })
    const requested: string[] = []
    api.fetchMessagesUI.mockImplementation((_botId: string, sid: string) => {
      requested.push(sid)
      return sid === 'session-2' ? session2Pending : Promise.resolve([])
    })

    await store.selectSession('session-2')
    await flushPromises()
    await store.selectSession('session-1')
    await flushPromises()
    expect(requested).toEqual(expect.arrayContaining(['session-2', 'session-1']))

    releaseSession2()
    await flushPromises()
  })

  it('uses the websocket checkpoint without requesting REST runtime hydration', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
    expect(runtimeSubscribeMessages).toContainEqual(expect.objectContaining({
      type: 'runtime_subscribe', session_id: 'session-1',
    }))

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-new-epoch',
      seq: 1,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'new epoch' } }],
        'session-1',
        'stream-new-epoch',
        'running',
        1,
      ),
    } as UIStreamEvent)

    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-session-1',
      stream_id: 'stream-new-epoch',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: ' continued' }] },
    } as UIStreamEvent)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toContainEqual(expect.objectContaining({
      type: 'text',
      content: 'new epoch continued',
    }))
  })

  it('does not fail an active assistant when runtime subscription setup fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const subscription = runtimeSubscribeMessages.at(-1)
    const invocationId = String(subscription?.invocation_id ?? '')
    expect(invocationId).not.toBe('')

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-still-running',
      seq: 1,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'still running' } }],
        'session-1',
        'stream-still-running',
        'running',
        1,
      ),
    } as UIStreamEvent)
    vi.useFakeTimers()
    streamHandler?.({
      type: 'command_error',
      invocation_id: invocationId,
      action_id: 'runtime_subscribe',
      terminal: true,
      error: { code: 'runtime_response_failed', message: 'runtime backend unavailable' },
    } as UIStreamEvent)

    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(store.streaming).toBe(true)
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toContainEqual(expect.objectContaining({ content: 'still running' }))
    expect(assistant?.role === 'assistant' ? assistant.messages.some(block => block.type === 'error') : true).toBe(false)

    const subscribeCount = runtimeSubscribeMessages.length
    try {
      const firstRetryStartedAt = Date.now()
      await vi.advanceTimersToNextTimerAsync()
      const firstRetryDelay = Date.now() - firstRetryStartedAt
      expect(runtimeSubscribeMessages).toHaveLength(subscribeCount + 1)

      const retryInvocationId = String(runtimeSubscribeMessages.at(-1)?.invocation_id ?? '')
      expect(retryInvocationId).not.toBe('')
      streamHandler?.({
        type: 'command_error',
        invocation_id: retryInvocationId,
        action_id: 'runtime_subscribe',
        terminal: true,
        error: { code: 'runtime_response_failed', message: 'runtime backend still unavailable' },
      } as UIStreamEvent)
      const secondRetryStartedAt = Date.now()
      await vi.advanceTimersToNextTimerAsync()
      const secondRetryDelay = Date.now() - secondRetryStartedAt

      expect(secondRetryDelay).toBeGreaterThan(firstRetryDelay)
      expect(secondRetryDelay).toBeLessThanOrEqual(30_000)
      expect(runtimeSubscribeMessages).toHaveLength(subscribeCount + 2)
      expect(consoleError).toHaveBeenCalledWith('Runtime subscription failed:', 'runtime backend unavailable')
    } finally {
      consoleError.mockRestore()
      vi.useRealTimers()
    }
  })

  it('does not treat a colliding ordinary stream error as an old-server runtime rejection', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const subscriptionId = String(runtimeSubscribeMessages.at(-1)?.invocation_id ?? '')
    expect(subscriptionId).not.toBe('')
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(subscriptionId as `${string}-${string}-${string}-${string}-${string}`)
    const sending = store.sendMessage('colliding stream')
    await vi.waitFor(() => expect(lastStreamId).toBe(subscriptionId))
    uuid.mockRestore()

    streamHandler?.({
      type: 'error',
      stream_id: subscriptionId,
      session_id: 'session-1',
      message: 'model failed',
    } as UIStreamEvent)

    await expect(sending).resolves.toMatchObject({ ok: false, stage: 'startup' })
    expect(store.streaming).toBe(false)
  })

  it('keeps a long-running legacy stream alive after its start acknowledgement', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    vi.useFakeTimers()
    try {
      const sending = store.sendMessage('long legacy request')
      await vi.advanceTimersByTimeAsync(1)
      const streamId = lastStreamId
      expect(streamId).not.toBe('')
      streamHandler?.({ type: 'start', stream_id: streamId, session_id: 'session-1' } as UIStreamEvent)
      await vi.advanceTimersByTimeAsync(31_000)
      expect(store.streaming).toBe(true)

      api.fetchMessagesUI.mockResolvedValueOnce([
        { id: 'legacy-user', role: 'user', text: 'long legacy request', attachments: [], timestamp: new Date().toISOString() },
        {
          id: 'legacy-assistant', role: 'assistant',
          messages: [{ id: 0, type: 'text', content: 'eventually done' }],
          timestamp: new Date().toISOString(),
        },
      ])
      streamHandler?.({
        type: 'message', stream_id: streamId, session_id: 'session-1',
        data: { id: 0, type: 'text', content: 'eventually done' },
      } as UIStreamEvent)
      streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-1' } as UIStreamEvent)
      await expect(sending).resolves.toMatchObject({ ok: true })
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps retrying a failed runtime subscription until a websocket checkpoint arrives', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    vi.useFakeTimers()
    try {
      const store = useChatStore()
      await store.selectBot('bot-1')
      const initialSubscription = runtimeSubscribeMessages.at(-1)
      const invocationId = String(initialSubscription?.invocation_id ?? '')
      expect(invocationId).not.toBe('')

      streamHandler?.({
        type: 'command_error',
        invocation_id: invocationId,
        action_id: 'runtime_subscribe',
        terminal: true,
        error: { code: 'runtime_response_failed', message: 'runtime backend unavailable' },
      } as UIStreamEvent)
      const subscribeCount = runtimeSubscribeMessages.length

      await vi.advanceTimersToNextTimerAsync()

      expect(runtimeSubscribeMessages).toHaveLength(subscribeCount + 1)
      const checkpoint = runtimeSnapshotFromScript([], 'session-1', 'stream-ws-recovered', 'running', 1)
      streamHandler?.({
        type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', epoch: checkpoint.epoch,
        seq: 1, stream_id: 'stream-ws-recovered', snapshot: checkpoint,
      } as UIStreamEvent)
      expect(store.messages.some(turn => turn.id === 'runtime-stream-ws-recovered')).toBe(true)
      expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
    } finally {
      consoleError.mockRestore()
      vi.useRealTimers()
    }
  })

  it('subscribes the active session runtime when the session stream starts and resubscribes after drops', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()

    await store.selectBot('bot-1')
    await flushPromises()

    expect(runtimeSubscribeMessages).toContainEqual(expect.objectContaining({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    }))

    const beforeDrop = runtimeSubscribeMessages.length
    streamHandler?.({ type: 'runtime_dropped', bot_id: 'bot-1', session_id: 'session-1', message: 'subscriber overflow' } as UIStreamEvent)

    expect(runtimeSubscribeMessages).toHaveLength(beforeDrop + 1)
    expect(runtimeSubscribeMessages.at(-1)).toMatchObject({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    })
  })

  it('ignores dropped events after an inactive background runtime is unsubscribed', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    api.fetchMessagesUI.mockResolvedValueOnce([])
    store.selectSession('session-2')
    await flushPromises()
    expect(runtimeUnsubscribeMessages).toContainEqual(expect.objectContaining({
      type: 'runtime_unsubscribe', session_id: 'session-1',
    }))
    runtimeSubscribeMessages = []
    streamHandler?.({
      type: 'runtime_dropped',
      bot_id: 'bot-1',
      session_id: 'session-1',
      message: 'subscriber overflow',
    } as UIStreamEvent)

    expect(runtimeSubscribeMessages).toEqual([])
  })

  it('unsubscribes the previous session runtime when switching to a draft', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeUnsubscribeMessages = []

    store.selectDraft({ explicitSelection: true })

    expect(runtimeUnsubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_unsubscribe', session_id: 'session-1' }),
    ])
  })

  it('does not request REST runtime hydration while switching between a draft and session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.connectWebSocket.mockImplementationOnce((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return false
        },
        send: vi.fn(),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()

    store.selectDraft({ explicitSelection: true })
    await store.selectSession('session-1')
    await flushPromises()

    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
  })

  it('does not queue an abort while reconnecting and keeps the run active', async () => {
    const abort = vi.fn(() => {
      throw new Error('WebSocket is not connected')
    })
    api.connectWebSocket.mockImplementationOnce((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      streamHandler = onStreamEvent
      return {
        get connected() {
          return false
        },
        send: vi.fn(),
        abort,
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({ type: 'start', stream_id: 'stream-reconnecting-abort', session_id: 'session-1' } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: 'stream-reconnecting-abort',
      session_id: 'session-1',
      data: { id: 0, type: 'text', content: 'still running' },
    } as UIStreamEvent)
    expect(store.streaming).toBe(true)

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-reconnecting-abort',
      seq: 1,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-reconnecting-abort', 'running', 1),
    } as UIStreamEvent)
    store.abort()

    expect(abort).not.toHaveBeenCalled()
    expect(store.streaming).toBe(true)
    expect(toast.error).toHaveBeenCalledWith('WebSocket is not connected')

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 2,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-reconnecting-abort', 'aborted', 2),
    } as UIStreamEvent)
    expect(store.streaming).toBe(false)
    expect(store.messages.flatMap(turn =>
      turn.role === 'assistant' ? turn.messages.filter(block => block.type === 'error') : [],
    )).toContainEqual(expect.objectContaining({ content: 'Response stopped' }))
  })

  it('replays an unacknowledged abort after reconnecting to the same generation', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const sending = store.sendMessage('abort then reconnect')
    await flushPromises()
    const streamId = lastStreamId
    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      abort: ReturnType<typeof vi.fn>
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    const runtimeOutput = [{ type: 'message', data: { id: 0, type: 'text', content: 'partial answer' } } as UIStreamEvent]
    const running = runtimeSnapshotFromScript(runtimeOutput, 'session-1', streamId, 'running', 1)
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running,
    } as UIStreamEvent)
    store.abort()
    expect(websocket.abort).toHaveBeenCalledTimes(1)

    websocket.onClose?.()
    websocket.onOpen?.()
    const reconnected = runtimeSnapshotFromScript(runtimeOutput, 'session-1', streamId, 'running', 2)
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: reconnected,
    } as UIStreamEvent)
    expect(websocket.abort).toHaveBeenCalledTimes(2)
    expect(websocket.abort).toHaveBeenLastCalledWith(streamId, 'session-1', `generation-${streamId}`)

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-aborted-server', role: 'user', text: 'abort then reconnect', attachments: [], timestamp: new Date().toISOString(), external_message_id: streamId },
      { id: 'assistant-aborted-server', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'partial answer' }], timestamp: new Date().toISOString() },
    ])
    _sessionMessageHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: { id: 'assistant-aborted-server', bot_id: 'bot-1', session_id: 'session-1', role: 'assistant', content: 'partial answer', created_at: new Date().toISOString() },
    } as SessionMessageStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 3,
      snapshot: runtimeSnapshotFromScript(runtimeOutput, 'session-1', streamId, 'aborted', 3),
    } as UIStreamEvent)
    await expect(sending).resolves.toMatchObject({ ok: false })
    await vi.waitFor(() => {
      expect(store.messages[1]?.serverId ?? store.messages[1]?.id).toBe('assistant-aborted-server')
      const stopped = store.messages.flatMap(turn => turn.role === 'assistant'
        ? turn.messages.filter(block => block.type === 'error' && block.content === 'Response stopped')
        : [])
      expect(stopped).toHaveLength(1)
    })
  })

  it('reconciles committed aborted history in every subscribed store without message-created events', async () => {
    const runtimeHandlers: UIStreamEventHandler[] = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      runtimeHandlers.push(onStreamEvent)
      return {
        get connected() {
          return true
        },
        send: vi.fn((message: { stream_id?: string; session_id?: string }) => {
          if ((message as Record<string, unknown>).type === 'runtime_subscribe') return
          lastStreamId = message.stream_id ?? ''
          lastSessionId = message.session_id ?? ''
        }),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    api.fetchSessions.mockResolvedValue({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const first = useChatStore(createTestPinia())
    const second = useChatStore(createTestPinia())
    await first.selectBot('bot-1')
    await second.selectBot('bot-1')
    await flushPromises()
    expect(runtimeHandlers).toHaveLength(2)

    const sending = first.sendMessage('stop after partial output')
    await flushPromises()
    const streamId = lastStreamId
    const requestUserTurn: ConversationUiTurn = {
      role: 'user',
      text: 'stop after partial output',
      timestamp: new Date().toISOString(),
      platform: 'local',
      external_message_id: streamId,
    }
    const runtimeOutput = [{
      type: 'message',
      data: { id: 0, type: 'text', content: 'partial answer' },
    } as UIStreamEvent]
    const runningSnapshot = runtimeSnapshotFromScript(runtimeOutput, 'session-1', streamId, 'running', 1, '', requestUserTurn)
    const running = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: runningSnapshot.epoch,
      seq: 1,
      snapshot: runningSnapshot,
    } as UIStreamEvent
    for (const handler of runtimeHandlers) handler(structuredClone(running))

    api.fetchMessagesUI.mockResolvedValue([
      {
        id: 'user-aborted-server',
        role: 'user',
        text: 'stop after partial output',
        attachments: [],
        timestamp: new Date().toISOString(),
        external_message_id: streamId,
      },
      {
        id: 'assistant-aborted-server',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'partial answer' }],
        timestamp: new Date().toISOString(),
      },
    ])
    const fixtureTerminal = structuredClone(interruptedRunContractFixture.runtime_abort_stream ?? [])
      .find(event => event.type === 'runtime_delta' && event.delta.run?.status === 'aborted')
    if (fixtureTerminal?.type !== 'runtime_delta' || !fixtureTerminal.delta.run) {
      throw new Error('Go-generated committed abort terminal delta is missing')
    }
    expect(fixtureTerminal.delta.run).toMatchObject({
      status: 'aborted',
      history_committed: true,
      canonical_ready: true,
    })
    fixtureTerminal.bot_id = 'bot-1'
    fixtureTerminal.session_id = 'session-1'
    fixtureTerminal.epoch = runningSnapshot.epoch ?? ''
    fixtureTerminal.seq = 2
    fixtureTerminal.stream_id = streamId
    fixtureTerminal.delta.run.stream_id = streamId
    const aborted = fixtureTerminal as UIStreamEvent
    for (const handler of runtimeHandlers) handler(structuredClone(aborted))

    await expect(sending).resolves.toMatchObject({ ok: false })
    await vi.waitFor(() => {
      for (const store of [first, second]) {
        expect(store.messages[0]?.serverId ?? store.messages[0]?.id).toBe('user-aborted-server')
        expect(store.messages[1]?.serverId ?? store.messages[1]?.id).toBe('assistant-aborted-server')
        expect(store.messages.some(turn => turn.__optimistic)).toBe(false)
        expect(store.messages.some(turn => turn.__ephemeral)).toBe(false)
        expect(store.messages.every(turn => !turn.streaming)).toBe(true)
        expect(store.streaming).toBe(false)
        expect(store.loadingMessages).toBe(false)
      }
    })
    for (const store of [first, second]) {
      const stopped = store.messages.flatMap(turn => turn.role === 'assistant'
        ? turn.messages.filter(block => block.type === 'error' && block.content === 'Response stopped')
        : [])
      expect(stopped).toHaveLength(1)
    }
  })

  it('keeps a run active after abort until the runtime publishes its terminal state', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'user-1',
        role: 'user',
        text: 'hello',
        attachments: [],
        timestamp: '2026-07-12T00:00:00.000Z',
        external_message_id: 'stream-empty-retry',
      },
      { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    const retryStreamId = lastStreamId
    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    streamHandler?.(runtimeReplacementSnapshot(
      retryStreamId,
      operation,
      [{ id: 0, type: 'text', content: 'completed despite abort' }],
      'running',
      10,
    ))

    store.abort()
    expect(store.streaming).toBe(true)

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-new', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'completed despite abort' }], timestamp: '2026-07-12T00:00:02.000Z' },
    ])
    streamHandler?.(runtimeReplacementSnapshot(
      retryStreamId,
      operation,
      [{ id: 0, type: 'text', content: 'completed despite abort' }],
      'completed',
      11,
    ))

    await expect(retry).resolves.toMatchObject({ ok: true })
    expect(store.streaming).toBe(false)
  })

  it('resyncs and retries an abort when its terminal runtime update is missing', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    vi.useFakeTimers()
    try {
      const sending = store.sendMessage('stop this run')
      await vi.advanceTimersByTimeAsync(0)
      const streamId = lastStreamId
      const running = runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 1)
      streamHandler?.({
        type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', epoch: running.epoch,
        seq: 1, stream_id: streamId, snapshot: running,
      } as UIStreamEvent)

      store.abort()
      expect(abortedWSStreams.filter(id => id === streamId)).toHaveLength(1)
      const subscriptionsBeforeWatchdog = runtimeSubscribeMessages.length

      await vi.advanceTimersByTimeAsync(29_999)
      expect(runtimeSubscribeMessages).toHaveLength(subscriptionsBeforeWatchdog)
      await vi.advanceTimersByTimeAsync(1)

      expect(runtimeSubscribeMessages).toHaveLength(subscriptionsBeforeWatchdog + 1)
      expect(abortedWSStreams.filter(id => id === streamId)).toHaveLength(1)

      streamHandler?.({
        type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', epoch: running.epoch,
        seq: 1, stream_id: streamId, snapshot: running,
      } as UIStreamEvent)
      expect(abortedWSStreams.filter(id => id === streamId)).toHaveLength(2)

      const aborted = runtimeSnapshotFromScript([], 'session-1', streamId, 'aborted', 2)
      streamHandler?.({
        type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', epoch: aborted.epoch,
        seq: 2, stream_id: streamId, snapshot: aborted,
      } as UIStreamEvent)

      await expect(sending).resolves.toMatchObject({ ok: false, stage: 'stream' })
      expect(store.streaming).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps a run active when the abort command itself is rejected', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const send = store.sendMessage('hello')
    await flushPromises()
    const streamId = lastStreamId
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 10,
      snapshot: runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 10),
    } as UIStreamEvent)

    store.abort()
    streamHandler?.({
      type: 'error',
      stream_id: streamId,
      session_id: 'session-1',
      message: 'runtime abort command was not acknowledged',
    } as UIStreamEvent)

    expect(toast.error).toHaveBeenCalledWith('runtime abort command was not acknowledged')
    expect(store.streaming).toBe(true)

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-server', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-server', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'done' }], timestamp: '2026-07-12T00:00:01.000Z' },
    ])
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', stream_id: streamId, session_id: 'session-1', data: { id: 0, type: 'text', content: 'done' } } as UIStreamEvent],
        'session-1',
        streamId,
        'completed',
        11,
      ),
    } as UIStreamEvent)

    await expect(send).resolves.toMatchObject({ ok: true })
  })

  it('projects the same retry operation in two independently subscribed stores', async () => {
    const handlers: UIStreamEventHandler[] = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      handlers.push(onStreamEvent)
      return {
        get connected() {
          return true
        },
        send: vi.fn(),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    api.fetchSessions.mockResolvedValue({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ])

    const first = useChatStore(createTestPinia())
    const second = useChatStore(createTestPinia())
    await first.selectBot('bot-1')
    await second.selectBot('bot-1')
    await flushPromises()
    expect(handlers).toHaveLength(2)

    const retryOperation = replacementOperationsContractFixture.retry_snapshot.snapshot.current_run_view?.operation
    if (!retryOperation) throw new Error('missing generated retry operation fixture')
    const event = runtimeReplacementSnapshot('stream-shared-retry', retryOperation, [{ id: 0, type: 'text', content: 'shared partial' }])
    for (const handler of handlers) handler(structuredClone(event))

    const project = (store: ReturnType<typeof useChatStore>) => store.messages.map((turn) => {
      if (turn.role === 'user') return { role: turn.role, text: turn.text }
      if (turn.role === 'assistant') {
        return {
          role: turn.role,
          streaming: turn.streaming,
          blocks: turn.messages.map(block => ({ type: block.type, content: 'content' in block ? block.content : undefined })),
        }
      }
      return { role: turn.role }
    })
    expect(project(first)).toEqual(project(second))
    expect(project(first)).toEqual([
      { role: 'user', text: 'hello' },
      {
        role: 'assistant',
        streaming: true,
        blocks: [{ type: 'text', content: 'shared partial' }],
      },
    ])

    const replay = runtimeReplacementSnapshot('stream-shared-retry', {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }, [{ id: 0, type: 'text', content: 'shared partial updated' }], 'running', 11)
    for (const handler of handlers) handler(structuredClone(replay))
    expect(first.messages).toHaveLength(2)
    expect(second.messages).toHaveLength(2)
    expect(project(first)).toEqual(project(second))
  })

  it('projects the same ordinary request and assistant run in two independently subscribed stores', async () => {
    const handlers: UIStreamEventHandler[] = []
    const abortCommands: Array<ReturnType<typeof vi.fn>> = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      handlers.push(onStreamEvent)
      const abort = vi.fn()
      abortCommands.push(abort)
      return {
        get connected() {
          return true
        },
        send: vi.fn(),
        abort,
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    api.fetchSessions.mockResolvedValue({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([])

    const first = useChatStore(createTestPinia())
    const second = useChatStore(createTestPinia())
    await first.selectBot('bot-1')
    await second.selectBot('bot-1')
    await flushPromises()
    expect(handlers).toHaveLength(2)

    const event = structuredClone(richActiveRunContractFixture.runtime_snapshot) as UIStreamEvent
    for (const handler of handlers) handler(structuredClone(event))

    const project = (store: ReturnType<typeof useChatStore>) => store.messages.map((turn) => {
      if (turn.role === 'user') {
        return {
          role: turn.role,
          text: turn.text,
          externalMessageId: turn.externalMessageId,
          attachments: turn.attachments.map(attachment => attachment.name),
        }
      }
      if (turn.role === 'assistant') {
        return {
          role: turn.role,
          streaming: turn.streaming,
          blocks: turn.messages.map(block => block.type),
        }
      }
      return { role: turn.role }
    })
    expect(project(first)).toEqual(project(second))
    expect(project(first)).toEqual([
      {
        role: 'user',
        text: 'Inspect the workspace',
        externalMessageId: 'stream-rich',
        attachments: ['notes.txt'],
      },
      {
        role: 'assistant',
        streaming: true,
        blocks: ['reasoning', 'text', 'tool', 'tool', 'tool'],
      },
    ])

    for (const handler of handlers) handler(structuredClone(event))
    expect(first.messages).toHaveLength(2)
    expect(second.messages).toHaveLength(2)

    first.abort()
    expect(abortCommands[0]).toHaveBeenCalledTimes(1)
    expect(abortCommands[1]).not.toHaveBeenCalled()

    const aborted = structuredClone(event) as unknown as {
      type: string
      seq: number
      snapshot: SessionruntimeSnapshot
    }
    aborted.type = 'runtime_snapshot'
    aborted.seq = 12
    aborted.snapshot.seq = 12
    if (!aborted.snapshot.current_run_view) throw new Error('missing generated current run fixture')
    aborted.snapshot.current_run_view.status = 'aborted'
    for (const handler of handlers) handler(structuredClone(aborted) as UIStreamEvent)

    const abortBlocks = (store: ReturnType<typeof useChatStore>) => store.messages.flatMap(turn =>
      turn.role === 'assistant'
        ? turn.messages.filter(block => block.type === 'error').map(block => block.content)
        : [],
    )
    expect(abortBlocks(first)).toEqual(['Response stopped'])
    expect(abortBlocks(second)).toEqual(abortBlocks(first))
  })

  it('reconciles persisted ordinary turns in every subscribed store after terminal runtime', async () => {
    const runtimeHandlers: UIStreamEventHandler[] = []
    const historyHandlers: Array<(event: SessionMessageStreamEvent) => void> = []
    api.connectWebSocket.mockImplementation((_botId: string, onStreamEvent: UIStreamEventHandler) => {
      runtimeHandlers.push(onStreamEvent)
      return {
        get connected() {
          return true
        },
        send: vi.fn(),
        abort: vi.fn(),
        close: vi.fn(),
        onOpen: null,
        onClose: null,
      }
    })
    api.streamSessionMessageEvents.mockImplementation((_botId: string, _sessionId: string, signal: AbortSignal, onEvent: (event: SessionMessageStreamEvent) => void) => new Promise<void>((resolve) => {
      historyHandlers.push(onEvent)
      signal.addEventListener('abort', () => resolve(), { once: true })
    }))
    api.fetchSessions.mockResolvedValue({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValue([])

    const first = useChatStore(createTestPinia())
    const second = useChatStore(createTestPinia())
    await first.selectBot('bot-1')
    await second.selectBot('bot-1')
    await flushPromises()
    expect(runtimeHandlers).toHaveLength(2)
    expect(historyHandlers).toHaveLength(2)

    const streamId = 'stream-shared-completion'
    const running = runtimeSnapshotFromScript(
      [{ type: 'message', data: { id: 0, type: 'text', content: 'shared answer' } } as UIStreamEvent],
      'session-1',
      streamId,
      'running',
      10,
      '',
      {
        role: 'user',
        text: 'shared prompt',
        timestamp: new Date().toISOString(),
        external_message_id: streamId,
      },
    )
    for (const handler of runtimeHandlers) {
      handler({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 10, snapshot: structuredClone(running) } as UIStreamEvent)
    }

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-shared-server', role: 'user', text: 'shared prompt', attachments: [], timestamp: new Date().toISOString(), external_message_id: streamId },
      { id: 'assistant-shared-server', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'shared answer' }], timestamp: new Date().toISOString() },
    ])
    for (const handler of historyHandlers) {
      handler({
        type: 'message_created',
        bot_id: 'bot-1',
        message: { id: 'assistant-shared-server', bot_id: 'bot-1', session_id: 'session-1', role: 'assistant', content: 'shared answer', created_at: new Date().toISOString() },
      } as SessionMessageStreamEvent)
    }
    const completed = structuredClone(running)
    completed.seq = 11
    completed.current_run_view!.status = 'completed'
    for (const handler of runtimeHandlers) {
      handler({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 11, snapshot: structuredClone(completed) } as UIStreamEvent)
    }

    await vi.waitFor(() => {
      for (const store of [first, second]) {
        expect(store.messages[0]?.serverId ?? store.messages[0]?.id).toBe('user-shared-server')
        expect(store.messages[1]?.serverId ?? store.messages[1]?.id).toBe('assistant-shared-server')
        expect(store.messages.some(turn => turn.__optimistic)).toBe(false)
      }
    })
  })

  it('restores a replaced tail when a runtime operation aborts without visible output', async () => {
    const persistedTurns = [
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    streamHandler?.(runtimeReplacementSnapshot('stream-empty-retry', operation, [], 'running', 10, 'session-1', false, false))
    expect(store.messages.map(turn => turn.id)).toContain('assistant-old')

    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    streamHandler?.(runtimeReplacementSnapshot('stream-empty-retry', operation, [], 'aborted', 11, 'session-1', false, false))
    await flushPromises()

    expect(store.streaming).toBe(false)
    expect(store.messages).toHaveLength(2)
    expect(store.messages.map(turn => turn.id)).toEqual(['user-1', 'assistant-old'])
    const restoredAssistant = store.messages[1]
    expect(restoredAssistant).toMatchObject({ role: 'assistant', streaming: false })
    if (restoredAssistant?.role !== 'assistant') throw new Error('missing restored assistant turn')
    expect(restoredAssistant.messages).toEqual([{ id: 0, type: 'text', content: 'old answer' }])
    expect(store.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error')
      : [])).toEqual([])
  })

  it.each([
    { kind: 'retry', status: 'errored' },
    { kind: 'retry', status: 'interrupted' },
    { kind: 'edit', status: 'errored' },
    { kind: 'edit', status: 'interrupted' },
  ])('restores a replaced tail when an empty $kind operation becomes $status', async ({ kind, status }) => {
    const persistedTurns = [
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation = kind === 'edit'
      ? replacementOperationsContractFixture.edit_snapshot.snapshot.current_run_view?.operation
      : ({ kind: 'retry', replace_from_message_id: 'assistant-old' } satisfies SessionruntimeRunOperationView)
    if (!operation) throw new Error('missing replacement operation')
    const streamId = `stream-empty-${kind}-${status}`
    streamHandler?.(runtimeReplacementSnapshot(streamId, operation, [], 'running', 10, 'session-1', false, false))
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const terminal = runtimeReplacementSnapshot(streamId, operation, [], status, 11, 'session-1', false, false)
    if (terminal.type === 'runtime_snapshot' && terminal.snapshot?.current_run_view) {
      terminal.snapshot.current_run_view.error = `replacement ${status}`
    }
    streamHandler?.(terminal)
    await flushPromises()

    expect(store.streaming).toBe(false)
    expect(store.messages.slice(0, 2).map(turn => turn.id)).toEqual(['user-old', 'assistant-old'])
    expect(store.messages.at(-1)).toMatchObject({ role: 'assistant', streaming: false })
    expect(store.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error')
      : [])).toEqual([])
  })

  it.each([
    { kind: 'retry', status: 'errored' },
    { kind: 'retry', status: 'aborted' },
    { kind: 'edit', status: 'errored' },
    { kind: 'edit', status: 'aborted' },
  ])('settles an initiating $kind when an empty run becomes $status and history refresh fails', async ({ kind, status }) => {
    sendEvents = []
    const persistedTurns = [
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const pending = kind === 'retry'
      ? store.retryLatestAssistant('assistant-old')
      : store.editLatestUser('user-old', 'edited prompt')
    await flushPromises()
    const streamId = lastStreamId
    const operation = kind === 'edit'
      ? replacementOperationsContractFixture.edit_snapshot.snapshot.current_run_view?.operation
      : ({ kind: 'retry', replace_from_message_id: 'assistant-old' } satisfies SessionruntimeRunOperationView)
    if (!operation) throw new Error('missing replacement operation')
    streamHandler?.(runtimeReplacementSnapshot(streamId, operation))
    api.fetchMessagesUI.mockRejectedValueOnce(new Error('history unavailable'))
    const terminal = runtimeReplacementSnapshot(streamId, operation, [], status, 11)
    if (terminal.type === 'runtime_snapshot' && terminal.snapshot?.current_run_view) {
      terminal.snapshot.current_run_view.error = `replacement ${status}`
      terminal.snapshot.current_run_view.history_committed = true
    }
    streamHandler?.(terminal)

    await expect(pending).resolves.toMatchObject({ ok: false, stage: 'stream' })
    await flushPromises()
    expect(store.messages.some(turn => turn.id === 'assistant-old')).toBe(false)
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: kind === 'edit' ? 'edited prompt' : 'old prompt',
    })
    if (status === 'aborted') {
      expect(store.messages).toHaveLength(2)
      expect(store.messages.at(-1)).toMatchObject({
        role: 'assistant',
        streaming: false,
        messages: [expect.objectContaining({ type: 'error' })],
      })
      return
    }
    expect(store.messages.at(-1)).toMatchObject({
      role: 'assistant',
      streaming: false,
      messages: [expect.objectContaining({
        type: 'error',
        content: `replacement ${status}`,
      })],
    })
  })

  it('keeps a replacement projection and reports errors after visible partial output', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    const partial = [{ id: 0, type: 'text', content: 'new partial answer' }] as ConversationUiMessage[]
    streamHandler?.(runtimeReplacementSnapshot('stream-partial-retry', operation, partial))
    const refreshCallsBefore = api.fetchMessagesUI.mock.calls.length
    const failed = runtimeReplacementSnapshot('stream-partial-retry', operation, partial, 'errored', 11)
    if (failed.type === 'runtime_snapshot' && failed.snapshot?.current_run_view) {
      failed.snapshot.current_run_view.error = 'replacement failed'
    }
    streamHandler?.(failed)
    await flushPromises()

    expect(store.messages.map(turn => turn.id)).not.toContain('assistant-old')
    const assistant = store.messages.at(-1)
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing replacement assistant')
    expect(assistant.streaming).toBe(false)
    expect(assistant.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'text', content: 'new partial answer' }),
      expect.objectContaining({ type: 'error', content: 'replacement failed' }),
    ]))
    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(refreshCallsBefore)
  })

  it('completes a retry in the background without leaking loading into the active session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    let replacementCompleted = false
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => {
      if (sessionId === 'session-a') {
        return Promise.resolve(replacementCompleted
          ? [
              { id: 'user-a', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
              { id: 'assistant-new', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'new answer' }], timestamp: '2026-07-12T00:00:02.000Z' },
            ]
          : [
              { id: 'user-a', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
              { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
            ])
      }
      return Promise.resolve([
        { id: 'user-b', role: 'user', text: 'other session', attachments: [], timestamp: '2026-07-12T01:00:00.000Z' },
      ])
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    const retryStreamId = lastStreamId
    await store.selectSession('session-b')
    await flushPromises()
    expect(store.loading).toBe(false)

    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    streamHandler?.(runtimeReplacementSnapshot(retryStreamId, operation, [{ id: 0, type: 'text', content: 'partial' }], 'running', 10, 'session-a'))
    replacementCompleted = true
    streamHandler?.(runtimeReplacementSnapshot(retryStreamId, operation, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11, 'session-a'))

    await expect(retry).resolves.toMatchObject({ ok: true })
    expect(store.sessionId).toBe('session-b')
    expect(store.loading).toBe(false)
    expect(store.messages).toMatchObject([{ role: 'user', text: 'other session' }])

    await store.selectSession('session-a')
    await flushPromises()
    expect(store.messages.map(message => message.id)).toEqual(['user-a', 'assistant-new'])
  })

  it('keeps a completed retry successful when REST reconciliation fails', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
    ])
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, operation, [{ id: 0, type: 'text', content: 'new answer' }]))
    api.fetchMessagesUI.mockRejectedValueOnce(new Error('history unavailable'))
    streamHandler?.(runtimeReplacementSnapshot(lastStreamId, operation, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11))

    await expect(retry).resolves.toMatchObject({ ok: true })
    expect(store.messages.at(-1)).toMatchObject({
      role: 'assistant',
      messages: [{ type: 'text', content: 'new answer' }],
    })
    consoleError.mockRestore()
  })

  it.each(['retry', 'edit'] as const)('does not restore stale history when an empty completed %s cannot reconcile', async (kind) => {
    sendEvents = []
    const persistedTurns = [
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const pending = kind === 'retry'
      ? store.retryLatestAssistant('assistant-old')
      : store.editLatestUser('user-old', 'edited prompt')
    await flushPromises()
    const streamId = lastStreamId
    const operation = kind === 'retry'
      ? ({ kind: 'retry', replace_from_message_id: 'assistant-old' } satisfies SessionruntimeRunOperationView)
      : ({
          kind: 'edit',
          replace_from_message_id: 'user-old',
          replacement_user_turn: {
            role: 'user',
            text: 'edited prompt',
            timestamp: '2026-07-12T00:00:02.000Z',
            platform: 'local',
          },
        } satisfies SessionruntimeRunOperationView)
    streamHandler?.(runtimeReplacementSnapshot(streamId, operation))
    api.fetchMessagesUI.mockRejectedValue(new Error('history unavailable'))
    streamHandler?.(runtimeReplacementSnapshot(streamId, operation, [], 'completed', 11, 'session-1', true))

    await expect(pending).resolves.toMatchObject({ ok: true })
    await flushPromises()
    expect(store.messages.some(turn => turn.id === 'assistant-old')).toBe(false)
    expect(store.messages.some(turn => turn.role === 'assistant')).toBe(false)
    expect(store.messages).toHaveLength(1)
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: kind === 'edit' ? 'edited prompt' : 'old prompt',
    })
    consoleError.mockRestore()
  })

  it.each(['retry', 'edit'] as const)('projects an empty completed %s on a passive client before history reconciliation', async (kind) => {
    const persistedTurns = [
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ]
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce(structuredClone(persistedTurns))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation = kind === 'retry'
      ? ({ kind: 'retry', replace_from_message_id: 'assistant-old' } satisfies SessionruntimeRunOperationView)
      : ({
          kind: 'edit',
          replace_from_message_id: 'user-old',
          replacement_user_turn: {
            role: 'user',
            text: 'edited prompt',
            timestamp: '2026-07-12T00:00:02.000Z',
            platform: 'local',
          },
        } satisfies SessionruntimeRunOperationView)
    api.fetchMessagesUI.mockRejectedValue(new Error('history unavailable'))
    streamHandler?.(runtimeReplacementSnapshot(`stream-passive-empty-${kind}`, operation, [], 'completed', 11, 'session-1', true))
    await flushPromises()

    expect(store.messages).toHaveLength(1)
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: kind === 'edit' ? 'edited prompt' : 'old prompt',
    })
    expect(store.messages.some(turn => turn.id === 'assistant-old')).toBe(false)
    consoleError.mockRestore()
  })

  it.each(['retry', 'edit'] as const)('reconciles a committed empty aborted %s on a passive client', async (kind) => {
    const streamId = `stream-passive-abort-${kind}`
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const replacementText = kind === 'edit' ? 'edited prompt' : 'old prompt'
    const operation = kind === 'retry'
      ? ({ kind: 'retry', replace_from_message_id: 'assistant-old' } satisfies SessionruntimeRunOperationView)
      : ({
          kind: 'edit',
          replace_from_message_id: 'user-old',
          replacement_user_turn: {
            role: 'user',
            text: replacementText,
            timestamp: '2026-07-12T00:00:02.000Z',
            platform: 'local',
            external_message_id: streamId,
          },
        } satisfies SessionruntimeRunOperationView)
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-new', role: 'user', text: replacementText, attachments: [], timestamp: '2026-07-12T00:00:02.000Z', external_message_id: streamId },
    ])
    streamHandler?.(runtimeReplacementSnapshot(streamId, operation, [], 'aborted', 11))

    await vi.waitFor(() => {
      expect(store.messages.some(turn => turn.id === 'assistant-old')).toBe(false)
      expect(store.messages[0]?.serverId ?? store.messages[0]?.id).toBe('user-new')
      expect(store.messages[0]).toMatchObject({ role: 'user', text: replacementText })
      expect(store.messages.at(-1)).toMatchObject({
        role: 'assistant',
        streaming: false,
        messages: [expect.objectContaining({ type: 'error', content: 'Response stopped' })],
      })
    })
  })

  it('does not carry runtime ownership from a successful retry into a reused stream id', async () => {
    sendEvents = []
    const sharedStreamId = '00000000-0000-4000-8000-000000000099' as `${string}-${string}-${string}-${string}-${string}`
    const uuid = vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(sharedStreamId)
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    const first = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    streamHandler?.(runtimeReplacementSnapshot(sharedStreamId, operation, [{ id: 0, type: 'text', content: 'new answer' }]))
    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      { id: 'assistant-new', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'new answer' }], timestamp: '2026-07-12T00:00:02.000Z' },
    ])
    streamHandler?.(runtimeReplacementSnapshot(sharedStreamId, operation, [{ id: 0, type: 'text', content: 'new answer' }], 'completed', 11))
    await expect(first).resolves.toMatchObject({ ok: true })
    await flushPromises()

    sendEvents = [{ type: 'error', message: 'second command rejected' } as UIStreamEvent]
    await expect(store.retryLatestAssistant('assistant-new')).resolves.toMatchObject({
      ok: false,
      stage: 'startup',
    })
    expect(store.messages.map(turn => turn.id)).toEqual(['user-1', 'assistant-new'])
    uuid.mockRestore()
  })

  it('replays failed replacement partial output after switching back to the session', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    let replacementPersisted = false
    let replacementStreamId = ''
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => Promise.resolve(sessionId === 'session-a'
      ? replacementPersisted
        ? [
            { id: 'user-a', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z', external_message_id: replacementStreamId },
            { id: 'assistant-new', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'partial replacement' }], timestamp: '2026-07-12T00:00:02.000Z' },
          ]
        : [
            { id: 'user-a', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
            { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-12T00:00:01.000Z' },
          ]
      : [
          { id: 'user-b', role: 'user', text: 'other session', attachments: [], timestamp: '2026-07-12T01:00:00.000Z' },
        ]))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const retry = store.retryLatestAssistant('assistant-old')
    await flushPromises()
    const retryStreamId = lastStreamId
    replacementStreamId = retryStreamId
    const operation: SessionruntimeRunOperationView = { kind: 'retry', replace_from_message_id: 'assistant-old' }
    const partial = [{ id: 0, type: 'text', content: 'partial replacement' }] as ConversationUiMessage[]
    streamHandler?.(runtimeReplacementSnapshot(retryStreamId, operation, partial, 'running', 10, 'session-a'))
    await store.selectSession('session-b')
    await flushPromises()

    const failed = runtimeReplacementSnapshot(retryStreamId, operation, partial, 'errored', 11, 'session-a')
    if (failed.type === 'runtime_snapshot' && failed.snapshot?.current_run_view) {
      failed.snapshot.current_run_view.error = 'replacement failed'
      failed.snapshot.current_run_view.history_committed = true
    }
    replacementPersisted = true
    streamHandler?.(failed)
    await expect(retry).resolves.toMatchObject({ ok: false, stage: 'stream' })

    await store.selectSession('session-a')
    await flushPromises()
    const replay = structuredClone(failed)
    replay.type = 'runtime_snapshot'
    streamHandler?.(replay)
    await flushPromises()

    expect(store.messages.map(message => message.id)).toEqual(['user-a', 'assistant-new'])
    expect(store.messages.at(-1)).toMatchObject({
      role: 'assistant',
      messages: expect.arrayContaining([
        expect.objectContaining({ type: 'text', content: 'partial replacement' }),
        expect.objectContaining({ type: 'error', content: 'replacement failed' }),
      ]),
    })
  })

  it('hydrates an edit operation from a runtime snapshot and replays it idempotently', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    api.fetchMessagesUI.mockResolvedValueOnce([
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation = replacementOperationsContractFixture.edit_snapshot.snapshot.current_run_view?.operation
    if (!operation) throw new Error('missing generated edit operation fixture')
    const snapshot = runtimeReplacementSnapshot(
      'stream-snapshot-edit',
      operation,
      [{ id: 0, type: 'text', content: 'snapshot partial' }],
    )
    snapshot.type = 'runtime_snapshot'
    streamHandler?.(snapshot)

    expect(store.messages).toMatchObject([
      { role: 'user', text: 'edited prompt', __optimistic: true },
      {
        role: 'assistant',
        streaming: true,
        messages: [{ type: 'text', content: 'snapshot partial' }],
      },
    ])

    streamHandler?.(runtimeReplacementSnapshot(
      'stream-snapshot-edit',
      operation,
      [{ id: 0, type: 'text', content: 'snapshot partial updated' }],
      'running',
      11,
    ))
    expect(store.messages).toHaveLength(2)
    expect(store.messages).toMatchObject([
      { role: 'user', text: 'edited prompt' },
      { role: 'assistant', messages: [{ type: 'text', content: 'snapshot partial updated' }] },
    ])
  })

  it('replays an early runtime operation after session history hydration', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    let resolveHistory: ((turns: unknown[]) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveHistory = resolve
    }))
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const operation: SessionruntimeRunOperationView = {
      kind: 'retry',
      replace_from_message_id: 'assistant-old',
    }
    const earlySnapshot = runtimeReplacementSnapshot(
      'stream-early-retry',
      operation,
      [{ id: 0, type: 'text', content: 'early partial' }],
    )
    earlySnapshot.type = 'runtime_snapshot'
    streamHandler?.(earlySnapshot)
    expect(store.streaming).toBe(true)
    expect(store.messages).toEqual([])

    resolveHistory?.([
      { id: 'user-1', role: 'user', text: 'hello', attachments: [], timestamp: '2026-07-12T00:00:00.000Z' },
      {
        id: 'assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-12T00:00:01.000Z',
      },
    ])
    await flushPromises()
    await flushPromises()

    expect(store.messages).toHaveLength(2)
    expect(store.messages.map(turn => turn.id)).not.toContain('assistant-old')
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      streaming: true,
      messages: [{ type: 'text', content: 'early partial' }],
    })
  })

  it('reuses the in-flight A history before replaying an A to B to A runtime replacement', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-a', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-b', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const historyA1 = deferred<UITurn[]>()
    let sessionAFetches = 0
    api.fetchMessagesUI.mockImplementation((_botId: string, targetSessionId: string) => {
      if (targetSessionId === 'session-b') return Promise.resolve([])
      sessionAFetches += 1
      return historyA1.promise
    })
    const store = useChatStore()
    const selectionA1 = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(streamHandler).not.toBeNull()
      expect(sessionAFetches).toBeGreaterThan(0)
    })
    const sessionAFetchesBeforeSwitch = sessionAFetches

    streamHandler?.(runtimeReplacementSnapshot(
      'stream-a-b-a-retry',
      { kind: 'retry', replace_from_message_id: 'assistant-old' },
      [{ id: 0, type: 'text', content: 'replacement partial' }],
      'running',
      1,
      'session-a',
    ))
    await store.selectSession('session-b')
    const selectionA2 = store.selectSession('session-a')
    expect(sessionAFetches).toBe(sessionAFetchesBeforeSwitch)

    historyA1.resolve([
      { id: 'user-old', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-14T00:00:00Z' },
      { id: 'assistant-old', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-14T00:00:01Z' },
    ])
    await selectionA1
    await selectionA2
    await flushPromises()
    expect(store.streaming).toBe(true)

    expect(store.messages).toMatchObject([
      { role: 'user', text: 'old prompt' },
      { role: 'assistant', streaming: true, messages: [{ type: 'text', content: 'replacement partial' }] },
    ])
  })

  it('rejects an early runtime replacement stream when history hydration fails', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    let rejectHistory: ((reason?: unknown) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((_resolve, reject) => {
      rejectHistory = reject
    }))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChatStore()
    void store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.(runtimeReplacementSnapshot(
      'stream-hydration-failure',
      { kind: 'retry', replace_from_message_id: 'assistant-old' },
      [{ id: 0, type: 'text', content: 'early partial' }],
    ))
    expect(store.streaming).toBe(true)

    rejectHistory?.(new Error('history unavailable'))
    await flushPromises()
    await flushPromises()

    expect(store.streaming).toBe(false)
    expect(store.loading).toBe(false)
    expect(store.messages).toEqual([])
    consoleError.mockRestore()
  })

  it('completes an initiating send through runtime deltas without legacy stream frames', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const staleHistory = deferred<UITurn[]>()
    const historyCallsBeforeRace = api.fetchMessagesUI.mock.calls.length
    api.fetchMessagesUI.mockImplementationOnce(() => staleHistory.promise)
    _sessionMessageHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: { id: 'older-message', bot_id: 'bot-1', session_id: 'session-1', role: 'assistant', content: 'older', created_at: new Date().toISOString() },
    } as SessionMessageStreamEvent)
    await vi.waitFor(() => {
      expect(api.fetchMessagesUI).toHaveBeenCalledTimes(historyCallsBeforeRace + 1)
    })

    const send = store.sendMessage('hello runtime')
    await flushPromises()
    const optimisticUserId = store.messages[0]?.id
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: 'hello runtime',
      externalMessageId: lastStreamId,
    })
    const runtimeScript = [{
      type: 'message',
      stream_id: lastStreamId,
      session_id: 'session-1',
      data: { id: 0, type: 'text', content: 'runtime response' },
    }] as UIStreamEvent[]
    const requestUserTurn: ConversationUiTurn = {
      role: 'user',
      text: 'hello runtime',
      timestamp: new Date().toISOString(),
      platform: 'local',
      external_message_id: lastStreamId,
    }
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(runtimeScript, 'session-1', lastStreamId, 'running', 11, '', requestUserTurn),
    } as UIStreamEvent)

    expect(store.messages).toMatchObject([
      { role: 'user', text: 'hello runtime' },
      { role: 'assistant', streaming: true, messages: [{ type: 'text', content: 'runtime response' }] },
    ])
    expect(store.messages[0]?.id).toBe(optimisticUserId)
    expect(store.messages).toHaveLength(2)

    api.fetchMessagesUI.mockResolvedValue([
      { id: 'user-server', role: 'user', text: 'hello runtime', attachments: [], timestamp: new Date().toISOString(), external_message_id: lastStreamId },
      {
        id: 'assistant-server',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'runtime response' }],
        timestamp: new Date().toISOString(),
      },
    ])
    _sessionMessageHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'assistant-server',
        bot_id: 'bot-1',
        session_id: 'session-1',
        role: 'assistant',
        content: 'runtime response',
        created_at: new Date().toISOString(),
      },
    } as SessionMessageStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 12,
      snapshot: runtimeSnapshotFromScript(runtimeScript, 'session-1', lastStreamId, 'completed', 12, '', requestUserTurn),
    } as UIStreamEvent)
    staleHistory.resolve([])
    const result = await send
    await vi.waitFor(() => {
      expect(api.fetchMessagesUI).toHaveBeenCalledTimes(historyCallsBeforeRace + 2)
    })

    expect(result).toEqual({ ok: true })
    expect(store.streaming).toBe(false)
    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({ role: 'user' })
    expect(store.messages[0]?.serverId ?? store.messages[0]?.id).toBe('user-server')
    expect(store.messages[0]?.__optimistic).not.toBe(true)
    expect(store.messages.at(-1)).toMatchObject({
      role: 'assistant',
      streaming: false,
      messages: [{ type: 'text', content: 'runtime response' }],
    })
    expect(store.messages.at(-1)?.serverId ?? store.messages.at(-1)?.id).toBe('assistant-server')
    expect(store.messages.at(-1)?.__optimistic).not.toBe(true)
  })

  it('keeps a completed send successful when its terminal history refresh fails', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const send = store.sendMessage('successful runtime')
    await flushPromises()
    const streamId = lastStreamId
    const runtimeScript = [{
      type: 'message',
      data: { id: 0, type: 'text', content: 'completed answer' },
    }] as UIStreamEvent[]
    const requestUserTurn: ConversationUiTurn = {
      role: 'user',
      text: 'successful runtime',
      timestamp: new Date().toISOString(),
      external_message_id: streamId,
    }
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 1,
      snapshot: runtimeSnapshotFromScript(runtimeScript, 'session-1', streamId, 'running', 1, '', requestUserTurn),
    } as UIStreamEvent)

    const historyCallsBeforeTerminal = api.fetchMessagesUI.mock.calls.length
    api.fetchMessagesUI.mockRejectedValueOnce(new Error('history temporarily unavailable'))
    _sessionMessageHandler?.({
      type: 'message_created',
      bot_id: 'bot-1',
      message: { id: 'assistant-success-server', bot_id: 'bot-1', session_id: 'session-1', role: 'assistant', content: 'completed answer', created_at: new Date().toISOString() },
    } as SessionMessageStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 2,
      snapshot: runtimeSnapshotFromScript(runtimeScript, 'session-1', streamId, 'completed', 2, '', requestUserTurn),
    } as UIStreamEvent)

    await expect(send).resolves.toEqual({ ok: true })
    await vi.waitFor(() => {
      expect(api.fetchMessagesUI).toHaveBeenCalledTimes(historyCallsBeforeTerminal + 1)
    })
    expect(store.messages[0]?.serverId ?? store.messages[0]?.id).not.toBe('user-success-server')
    expect(store.messages[1]?.serverId ?? store.messages[1]?.id).not.toBe('assistant-success-server')
    expect(store.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error')
      : [])).toEqual([])
    consoleError.mockRestore()
  })

  it('subscribes a newly created session before waiting for its first runtime response', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({ items: [], nextCursor: null })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const send = store.sendMessage('first prompt')
    await flushPromises()

    expect(lastSessionId).toBe('session-1')
    expect(runtimeSubscribeMessages).toContainEqual(expect.objectContaining({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    }))
    const firstSessionTimeline = wsOutboundTimeline.filter(message => message.session_id === 'session-1')
    expect(firstSessionTimeline.map(message => message.type)).toEqual(['runtime_subscribe', 'message'])
    expect(runtimeSubscribeMessages).toHaveLength(1)
    expect(runtimeSubscribeMessages[0]?.invocation_id).toEqual(expect.any(String))
    expect(runtimeSubscribeMessages[0]?.invocation_id).not.toBe('')
    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-first-send',
      seq: 2,
      snapshot: {
        ...runtimeSnapshotFromScript([], 'session-1', lastStreamId, 'completed', 2),
        epoch: 'epoch-first-send',
      },
    } as UIStreamEvent)
    expect(await send).toEqual({ ok: true })
  })

  it('reconnect contract: hydrates rich active-run state from runtime snapshot', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 10,
      snapshot: richActiveRunRuntimeSnapshot(),
    } as UIStreamEvent)

    expect(store.streaming).toBe(true)
    expect(store.messages[0]).toMatchObject({
      role: 'user',
      text: 'Inspect the workspace',
      externalMessageId: 'stream-rich',
      attachments: [{ name: 'notes.txt', content_hash: 'sha256:notes' }],
    })
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')

    expect(assistant.messages.find(block => block.type === 'reasoning')).toMatchObject({
      content: 'I need to inspect the workspace.',
    })
    expect(assistant.messages.find(block => block.type === 'text')).toMatchObject({
      content: 'I will check the current state.',
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-exec')).toMatchObject({
      type: 'tool',
      toolName: 'exec',
      done: true,
      running: false,
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-approval')).toMatchObject({
      approval: {
        approval_id: 'approval-1',
        status: 'pending',
      },
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-ask')).toMatchObject({
      userInput: {
        user_input_id: 'input-1',
        status: 'pending',
      },
    })
  })

  it('does not carry an abort request into a reused stream id with a new generation', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-reused-generation'
    const oldSnapshot = runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 1)
    oldSnapshot.current_run_view!.generation = 'generation-old'
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: oldSnapshot,
    } as UIStreamEvent)
    const ws = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }
    store.abort()
    expect(ws.abort).toHaveBeenCalledTimes(1)
    expect(ws.abort).toHaveBeenCalledWith(streamId, 'session-1', 'generation-old')

    const newSnapshot = runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 2)
    newSnapshot.current_run_view!.generation = 'generation-new'
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: newSnapshot,
    } as UIStreamEvent)
    await flushPromises()

    expect(ws.abort).toHaveBeenCalledTimes(1)
    expect(store.streaming).toBe(true)
    expect(store.messages.some(turn => turn.role === 'assistant' && turn.id === `runtime-${streamId}` && turn.streaming)).toBe(true)
  })

  it('does not carry assistant output into a reused active stream generation', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-active-generation-rollover'
    const first = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'old generation text' } } as UIStreamEvent,
      { type: 'message', data: { id: 1, type: 'reasoning', content: 'old generation reasoning' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1)
    first.current_run_view!.generation = 'generation-old'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: first } as UIStreamEvent)

    const second = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'new generation text' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 2)
    second.current_run_view!.generation = 'generation-new'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: second } as UIStreamEvent)
    await flushPromises()

    const activeAssistant = store.messages.find(turn => turn.role === 'assistant' && turn.streaming)
    expect(activeAssistant?.role).toBe('assistant')
    if (activeAssistant?.role !== 'assistant') throw new Error('missing active assistant')
    expect(activeAssistant.messages).toEqual([
      { id: 0, type: 'text', content: 'new generation text' },
    ])
  })

  it('matches the initiating optimistic request after a delayed runtime timestamp', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    sendEvents = []
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const sendPromise = store.sendMessage('hello delayed runtime')
    await flushPromises()
    const streamId = lastStreamId
    const running = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'delayed partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1, '', {
      role: 'user',
      text: 'hello delayed runtime',
      timestamp: new Date(Date.now() + 6_000).toISOString(),
      external_message_id: streamId,
    })
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running } as UIStreamEvent)

    expect(store.messages.filter(turn => turn.role === 'user')).toHaveLength(1)
    expect(store.messages.filter(turn => turn.role === 'assistant')).toHaveLength(1)

    const completed = structuredClone(running)
    completed.seq = 2
    completed.current_run_view!.status = 'completed'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: completed } as UIStreamEvent)
    await sendPromise
  })

  it('observes a reused stream id generation after its reducer state was evicted', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-reused-after-eviction'
    const first = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'old generation text' } } as UIStreamEvent,
      { type: 'message', data: { id: 1, type: 'reasoning', content: 'old generation reasoning' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1)
    first.current_run_view!.generation = 'generation-1'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: first } as UIStreamEvent)
    const completed = structuredClone(first)
    completed.seq = 2
    completed.current_run_view!.status = 'completed'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: completed } as UIStreamEvent)
    await flushPromises()
    expect(store.streaming).toBe(false)

    await store.selectSession('session-2')
    await flushPromises()
    await store.selectSession('session-1')
    await flushPromises()

    const second = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'new generation text' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 3)
    second.current_run_view!.generation = 'generation-2'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 3, snapshot: second } as UIStreamEvent)
    await flushPromises()

    expect(store.streaming).toBe(true)
    const activeAssistant = store.messages.find(turn => turn.role === 'assistant' && turn.streaming)
    expect(activeAssistant?.role).toBe('assistant')
    if (activeAssistant?.role !== 'assistant') throw new Error('missing active assistant')
    expect(activeAssistant.messages).toEqual([
      { id: 0, type: 'text', content: 'new generation text' },
    ])
  })

  it('keeps persisted history separate from a reused stream id generation', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const streamId = 'stream-reused-persisted-history'
    const persistedHistory: UITurn[] = [
      {
        id: 'db-user-old',
        role: 'user',
        text: 'old prompt',
        attachments: [],
        timestamp: '2026-07-14T00:00:00Z',
        external_message_id: streamId,
      },
      {
        id: 'db-assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old final' }],
        timestamp: '2026-07-14T00:00:01Z',
      },
    ]
    let oldRunPersisted = false
    api.fetchMessagesUI.mockImplementation((_botId: string, sessionId: string) => Promise.resolve(
      sessionId === 'session-1' && oldRunPersisted ? structuredClone(persistedHistory) : [],
    ))

    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const first = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'old partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1, '', {
      role: 'user',
      text: 'old prompt',
      timestamp: '2026-07-14T00:00:00Z',
      external_message_id: streamId,
    })
    first.current_run_view!.generation = 'generation-old'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: first } as UIStreamEvent)

    oldRunPersisted = true
    const completed = structuredClone(first)
    completed.seq = 2
    completed.current_run_view!.status = 'completed'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: completed } as UIStreamEvent)
    await vi.waitFor(() => {
      expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
        'old prompt',
        'old final',
      ])
    })

    await store.selectSession('session-2')
    await store.selectSession('session-1')
    await flushPromises()

    const second = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'new partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 3, '', {
      role: 'user',
      text: 'new prompt',
      timestamp: '2026-07-14T00:01:00Z',
      external_message_id: streamId,
    })
    second.current_run_view!.generation = 'generation-new'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 3, snapshot: second } as UIStreamEvent)
    await flushPromises()

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'old prompt',
      'old final',
      'new prompt',
      'new partial',
    ])
    expect(store.messages[0]).toMatchObject({ serverId: 'db-user-old', externalMessageId: streamId })
    expect(store.messages[1]).toMatchObject({ serverId: 'db-assistant-old', streaming: false })
    expect(store.messages[3]).toMatchObject({ role: 'assistant', streaming: true })
  })

  it('separates a fresh client from persisted history that reused the stream id', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const streamId = 'stream-reused-fresh-client'
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'db-user-old',
        role: 'user',
        text: 'inspect',
        attachments: [{ type: 'file', name: 'old.txt', content_hash: 'sha256:old' }],
        timestamp: '2026-07-14T00:00:00Z',
        external_message_id: streamId,
      },
      {
        id: 'db-assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-14T00:00:01Z',
      },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const running = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'new partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1, '', {
      role: 'user',
      text: 'inspect',
      attachments: [{ type: 'file', name: 'new.txt', content_hash: 'sha256:new' }],
      timestamp: '2026-07-14T00:00:01Z',
      external_message_id: streamId,
    })
    running.current_run_view!.generation = 'generation-new'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running } as UIStreamEvent)
    await flushPromises()

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'inspect',
      'old answer',
      'inspect',
      'new partial',
    ])
    expect(store.messages[0]).toMatchObject({
      id: 'db-user-old',
      externalMessageId: streamId,
      attachments: [{ name: 'old.txt', content_hash: 'sha256:old' }],
    })
    expect(store.messages[1]).toMatchObject({ id: 'db-assistant-old', streaming: false })
    expect(store.messages[2]).toMatchObject({
      role: 'user',
      attachments: [{ name: 'new.txt', content_hash: 'sha256:new' }],
    })
  })

  it('places a fresh-client abort after the current reused-stream request', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const streamId = 'stream-reused-fresh-abort'
    api.fetchMessagesUI.mockResolvedValueOnce([
      {
        id: 'db-user-old',
        role: 'user',
        text: 'old prompt',
        attachments: [],
        timestamp: '2026-07-14T00:00:00Z',
        external_message_id: streamId,
      },
      {
        id: 'db-assistant-old',
        role: 'assistant',
        messages: [{ id: 0, type: 'text', content: 'old answer' }],
        timestamp: '2026-07-14T00:00:01Z',
      },
    ])
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const running = runtimeSnapshotFromScript([], 'session-1', streamId, 'running', 1, '', {
      role: 'user',
      text: 'new prompt',
      timestamp: '2026-07-14T00:01:00Z',
      external_message_id: streamId,
    })
    running.current_run_view!.generation = 'generation-new'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running } as UIStreamEvent)

    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<UITurn[]>(() => {}))
    const aborted = structuredClone(running)
    aborted.seq = 2
    aborted.current_run_view!.status = 'aborted'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: aborted } as UIStreamEvent)
    await flushPromises()

    expect(store.messages.map((turn) => {
      if (turn.role === 'user') return turn.text
      return turn.messages[0]?.content
    })).toEqual([
      'old prompt',
      'old answer',
      'new prompt',
      'Response stopped',
    ])
  })

  it('replays an empty terminal runtime failure after initial history hydration', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    let resolveHistory: ((turns: unknown[]) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveHistory = resolve
    }))
    const store = useChatStore()
    const selection = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(streamHandler).not.toBeNull()
      expect(store.sessionId).toBe('session-1')
    })

    const streamId = 'stream-empty-terminal-hydration'
    const terminal = runtimeSnapshotFromScript([], 'session-1', streamId, 'errored', 3, 'runtime failed before output', {
      role: 'user',
      text: 'hello runtime',
      timestamp: new Date().toISOString(),
      platform: 'local',
      external_message_id: streamId,
    })
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 3, snapshot: terminal,
    } as UIStreamEvent)
    await flushPromises()
    expect(store.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error' && block.content === 'runtime failed before output')
      : [])).toHaveLength(1)
    resolveHistory?.([
      { id: 'user-server', role: 'user', text: 'hello runtime', attachments: [], timestamp: new Date().toISOString(), external_message_id: streamId },
    ])
    await selection
    await flushPromises()

    const errors = store.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error' && block.content === 'runtime failed before output')
      : [])
    expect(errors).toHaveLength(1)
    const ephemeralTurn = store.messages.find(turn => turn.role === 'assistant' && turn.__ephemeral)
    expect(ephemeralTurn).toBeDefined()
    const sentBeforeRetry = sentWSMessages.length
    await expect(store.retryLatestAssistant(ephemeralTurn!.id)).resolves.toMatchObject({ ok: false, stage: 'startup' })
    expect(sentWSMessages).toHaveLength(sentBeforeRetry)
    expect(store.streaming).toBe(false)
  })

  it('reattaches the runtime request turn after older history hydration completes', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const history = deferred<UITurn[]>()
    api.fetchMessagesUI.mockImplementationOnce(() => history.promise)
    const store = useChatStore()
    const selection = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(streamHandler).not.toBeNull()
      expect(store.sessionId).toBe('session-1')
    })

    const streamId = 'stream-running-during-history'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 2,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'new partial answer' } } as UIStreamEvent],
        'session-1',
        streamId,
        'running',
        2,
        '',
        {
          role: 'user',
          text: 'new prompt',
          timestamp: '2026-07-14T00:01:00Z',
          platform: 'local',
          external_message_id: streamId,
        },
      ),
    } as UIStreamEvent)
    expect(store.messages).toMatchObject([
      { role: 'user', text: 'new prompt' },
      { role: 'assistant', streaming: true },
    ])

    history.resolve([
      { id: 'old-user', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-14T00:00:00Z' },
      { id: 'old-assistant', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'old answer' }], timestamp: '2026-07-14T00:00:10Z' },
    ])
    await selection
    await flushPromises()

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'old prompt',
      'old answer',
      'new prompt',
      'new partial answer',
    ])
    expect(store.streaming).toBe(true)
  })

  it('fetches fresh history after a completed snapshot races an older hydration', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const staleHistory = deferred<UITurn[]>()
    api.fetchMessagesUI
      .mockImplementationOnce(() => staleHistory.promise)
      .mockResolvedValueOnce([
        { id: 'persisted-user', role: 'user', text: 'completed prompt', attachments: [], timestamp: '2026-07-14T00:01:00Z' },
        { id: 'persisted-assistant', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'completed answer' }], timestamp: '2026-07-14T00:01:10Z' },
      ])
    const store = useChatStore()
    const selection = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(streamHandler).not.toBeNull()
      expect(api.fetchMessagesUI).toHaveBeenCalledTimes(1)
    })

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 3,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-completed-during-history', 'completed', 3),
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 4,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-completed-during-history', 'completed', 4),
    } as UIStreamEvent)
    staleHistory.resolve([
      { id: 'old-user', role: 'user', text: 'old prompt', attachments: [], timestamp: '2026-07-14T00:00:00Z' },
    ])
    await selection
    await flushPromises()

    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(2)
    expect(store.messages).toMatchObject([
      { role: 'user', text: 'completed prompt' },
      { role: 'assistant', messages: [{ type: 'text', content: 'completed answer' }] },
    ])
  })

  it('reprojects a newer run after an older terminal history refresh applies', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const terminalHistory = deferred<UITurn[]>()
    api.fetchMessagesUI.mockImplementationOnce(() => terminalHistory.promise)
    const firstRun = runtimeSnapshotFromScript(
      [{ type: 'message', data: { id: 0, type: 'text', content: 'first answer' } } as UIStreamEvent],
      'session-1',
      'stream-generation-1',
      'running',
      1,
      '',
      { role: 'user', text: 'first prompt', external_message_id: 'stream-generation-1' },
    )
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: firstRun } as UIStreamEvent)
    const completedFirstRun = structuredClone(firstRun)
    completedFirstRun.seq = 2
    completedFirstRun.current_run_view!.status = 'completed'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: completedFirstRun } as UIStreamEvent)
    await vi.waitFor(() => expect(api.fetchMessagesUI).toHaveBeenCalledTimes(2))

    const secondRun = runtimeSnapshotFromScript(
      [{ type: 'message', data: { id: 0, type: 'text', content: 'second partial' } } as UIStreamEvent],
      'session-1',
      'stream-generation-2',
      'running',
      3,
      '',
      { role: 'user', text: 'second prompt', external_message_id: 'stream-generation-2' },
    )
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 3, snapshot: secondRun } as UIStreamEvent)

    terminalHistory.resolve([
      { id: 'persisted-user-1', role: 'user', text: 'first prompt', attachments: [], timestamp: '2026-07-14T00:00:00Z', external_message_id: 'stream-generation-1' },
      { id: 'persisted-assistant-1', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'first answer' }], timestamp: '2026-07-14T00:00:01Z' },
    ])
    await flushPromises()
    await flushPromises()
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: secondRun.epoch,
      stream_id: 'stream-generation-2',
      seq: 4,
      delta: { message_appends: [{ id: 0, type: 'text', content: ' tail' }] },
    } as UIStreamEvent)

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'first prompt',
      'first answer',
      'second prompt',
      'second partial tail',
    ])
    expect(store.messages.at(-1)).toMatchObject({ role: 'assistant', streaming: true })
  })

  it('fetches fresh history when the hydration preceding a completed snapshot fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const failedHistory = deferred<UITurn[]>()
    api.fetchMessagesUI
      .mockImplementationOnce(() => failedHistory.promise)
      .mockResolvedValueOnce([
        { id: 'persisted-user', role: 'user', text: 'completed prompt', attachments: [], timestamp: '2026-07-14T00:01:00Z' },
        { id: 'persisted-assistant', role: 'assistant', messages: [{ id: 0, type: 'text', content: 'completed answer' }], timestamp: '2026-07-14T00:01:10Z' },
      ])
    const store = useChatStore()
    const selection = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(streamHandler).not.toBeNull()
      expect(api.fetchMessagesUI).toHaveBeenCalledTimes(1)
    })

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 3,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-completed-after-history-error', 'completed', 3),
    } as UIStreamEvent)
    failedHistory.reject(new Error('transient history failure'))
    await selection
    await flushPromises()

    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(2)
    expect(store.messages).toMatchObject([
      { role: 'user', text: 'completed prompt' },
      { role: 'assistant', messages: [{ type: 'text', content: 'completed answer' }] },
    ])
    consoleError.mockRestore()
  })

  it('keeps a queued terminal refresh scoped to its session after switching sessions', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const sessionAHistory = deferred<UITurn[]>()
    const sessionBHistory = deferred<UITurn[]>()
    api.fetchMessagesUI.mockImplementation((_botId: string, sid: string) => {
      return sid === 'session-1' ? sessionAHistory.promise : sessionBHistory.promise
    })
    const store = useChatStore()
    const selectionA = store.selectBot('bot-1')
    await vi.waitFor(() => {
      expect(api.fetchMessagesUI).toHaveBeenCalledWith('bot-1', 'session-1', expect.anything())
    })

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 3,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-completed-before-switch', 'completed', 3),
    } as UIStreamEvent)
    const selectionB = store.selectSession('session-2')
    await vi.waitFor(() => {
      expect(api.fetchMessagesUI).toHaveBeenCalledWith('bot-1', 'session-2', expect.anything())
    })

    sessionAHistory.resolve([])
    await selectionA
    await flushPromises()
    expect(api.fetchMessagesUI.mock.calls.filter(([, sid]) => sid === 'session-1')).toHaveLength(2)

    sessionBHistory.resolve([])
    await selectionB
    expect(store.sessionId).toBe('session-2')
    expect(store.messages).toEqual([])
  })

  it('reconnect contract: hydrates interrupted terminal state from the shared runtime snapshot', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const refreshCallsBefore = api.fetchMessagesUI.mock.calls.length
    streamHandler?.(structuredClone(interruptedRunContractFixture.runtime_snapshot) as UIStreamEvent)
    await flushPromises()

    expect(store.streaming).toBe(false)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.streaming).toBe(false)
    expect(assistant.messages.some(block => block.type === 'text' && block.content === 'partial output')).toBe(true)
    expect(assistant.messages.some(block => block.type === 'error' && block.content === 'runtime interrupted')).toBe(true)
    expect(api.fetchMessagesUI).toHaveBeenCalledTimes(refreshCallsBefore)
  })

  it('reconnect contract: empty runtime snapshot clears stale local pending streams', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      seq: 19,
      stream_id: 'stream-stale-runtime',
      session_id: 'session-1',
      snapshot: runtimeSnapshotFromScript(
        [{
          type: 'message',
          stream_id: 'stream-stale-runtime',
          session_id: 'session-1',
          data: { id: 0, type: 'text', content: 'stale local output' },
        } as UIStreamEvent],
        'session-1',
        'stream-stale-runtime',
        'running',
        19,
      ),
    } as UIStreamEvent)
    expect(store.streaming).toBe(true)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.streaming).toBe(true)

    let resolveRefresh: ((turns: unknown[]) => void) | undefined
    const refreshCallsBefore = api.fetchMessagesUI.mock.calls.length
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveRefresh = resolve
    }))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-session-1',
      seq: 20,
      snapshot: {
        bot_id: 'bot-1',
        session_id: 'session-1',
        epoch: 'epoch-session-1',
        seq: 20,
        queue: [],
      },
    } as UIStreamEvent)

    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    expect(api.fetchMessagesUI.mock.calls.length).toBeGreaterThanOrEqual(refreshCallsBefore)
    resolveRefresh?.([])
    await flushPromises()
  })

  it('reconnect contract: lower-seq empty runtime snapshot clears state after backend reset', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-runtime-reset'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'running', 11),
    } as UIStreamEvent)

    expect(store.streaming).toBe(true)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.streaming).toBe(true)

    let resolveRefresh: ((turns: unknown[]) => void) | undefined
    const refreshCallsBefore = api.fetchMessagesUI.mock.calls.length
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveRefresh = resolve
    }))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-runtime-reset',
      seq: 0,
      snapshot: {
        bot_id: 'bot-1',
        session_id: 'session-1',
        epoch: 'epoch-runtime-reset',
        seq: 0,
        queue: [],
      },
    } as UIStreamEvent)

    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    expect(api.fetchMessagesUI.mock.calls.length).toBe(refreshCallsBefore + 1)
    resolveRefresh?.([])
    await flushPromises()
  })

  it('reconnect contract: lower-seq authoritative snapshot starts a new runtime sequence epoch', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-old',
      seq: 100,
      snapshot: {
        ...runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', 'stream-old-epoch'), 'session-1', 'stream-old-epoch', 'running', 100),
        epoch: 'epoch-old',
      },
    } as UIStreamEvent)
    expect(store.streaming).toBe(true)

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-old',
      seq: 1,
      snapshot: {
        ...runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', 'stream-stale-same-epoch'), 'session-1', 'stream-stale-same-epoch', 'running', 1),
        epoch: 'epoch-old',
      },
    } as UIStreamEvent)
    expect(store.messages.some(turn => turn.role === 'assistant' && turn.id === 'runtime-stream-stale-same-epoch')).toBe(false)

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-new',
      seq: 1,
      snapshot: {
        ...runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', 'stream-new-epoch'), 'session-1', 'stream-new-epoch', 'running', 1),
        epoch: 'epoch-new',
      },
    } as UIStreamEvent)

    expect(store.streaming).toBe(true)
    const assistants = store.messages.filter(turn => turn.role === 'assistant')
    expect(assistants.some(turn => turn.id === 'runtime-stream-new-epoch' && turn.streaming)).toBe(true)
    expect(assistants.some(turn => turn.id === 'runtime-stream-old-epoch' && turn.streaming)).toBe(false)
  })

  it('applies a checkpoint that changes identity at the same sequence', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    const epoch = 'epoch-equal-seq-convergence'
    const oldSnapshot = runtimeSnapshotFromScript([], 'session-1', 'stream-old', 'running', 10)
    oldSnapshot.epoch = epoch
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-old',
      epoch,
      seq: 10,
      snapshot: oldSnapshot,
    } as UIStreamEvent)
    expect(store.messages.find(turn => turn.id === 'runtime-stream-old')).toMatchObject({ streaming: true })

    runtimeSubscribeMessages = []
    const newSnapshot = runtimeSnapshotFromScript([], 'session-1', 'stream-new', 'running', 10)
    newSnapshot.epoch = epoch
    const replacement = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-new',
      epoch,
      seq: 10,
      snapshot: newSnapshot,
    } as UIStreamEvent
    streamHandler?.(structuredClone(replacement))

    expect(runtimeSubscribeMessages).toHaveLength(0)
    expect(store.messages.some(turn => turn.id === 'runtime-stream-old' && turn.streaming)).toBe(false)
    expect(store.messages.find(turn => turn.id === 'runtime-stream-new')).toMatchObject({ streaming: true })
  })

  it('forces resync instead of applying an epochless event after the store establishes an epoch', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-current',
      seq: 1,
      snapshot: {
        ...runtimeSnapshotFromScript([], 'session-1', 'stream-current', 'running', 1),
        epoch: 'epoch-current',
      },
    } as UIStreamEvent)
    runtimeSubscribeMessages = []

    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-current',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'must not apply' }] },
    } as UIStreamEvent)

    expect(runtimeSubscribeMessages).toEqual([expect.objectContaining({
      type: 'runtime_subscribe',
      session_id: 'session-1',
    })])
    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
    const assistant = store.messages.find(turn => turn.role === 'assistant' && turn.id === 'runtime-stream-current')
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toEqual([])
  })

  it('does not recreate an assistant turn when runtime terminal state follows a legacy error', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    sendEvents = richActiveRunStoreScript('session-1', 'stream-terminal-order')
    const sendPromise = store.sendMessage('test terminal ordering')
    await flushPromises()
    streamHandler?.({ type: 'error', stream_id: lastStreamId, session_id: lastSessionId, message: 'runtime failed' } as UIStreamEvent)
    await sendPromise
    const assistantCount = store.messages.filter(turn => turn.role === 'assistant').length

    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>(() => {}))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 12,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', lastStreamId), 'session-1', lastStreamId, 'errored', 12, 'runtime failed'),
    } as UIStreamEvent)

    expect(store.messages.filter(turn => turn.role === 'assistant')).toHaveLength(assistantCount)
  })

  it('does not reuse a legacy terminal projection for a different runtime request', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-legacy-terminal-reused'
    store.messages.push({
      id: 'old-optimistic-user',
      role: 'user',
      text: 'old prompt',
      attachments: [],
      timestamp: new Date().toISOString(),
      externalMessageId: streamId,
      streaming: false,
      isSelf: true,
      __optimistic: true,
    })
    streamHandler?.({ type: 'start', stream_id: streamId, session_id: 'session-1' } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-1',
      data: { id: 0, type: 'text', content: 'old partial' },
    } as UIStreamEvent)
    expect(store.messages.find(turn => turn.role === 'assistant')).toMatchObject({
      role: 'assistant',
      messages: [expect.objectContaining({ type: 'text', content: 'old partial' })],
    })
    streamHandler?.({ type: 'error', stream_id: streamId, session_id: 'session-1', message: 'old runtime failed' } as UIStreamEvent)
    expect(store.messages).toHaveLength(2)
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'old prompt' })
    expect(store.messages[1]).toMatchObject({ role: 'assistant', streaming: false })

    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<UITurn[]>(() => {}))
    const running = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'new partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1, '', {
      role: 'user',
      text: 'new prompt',
      timestamp: new Date(Date.now() + 10_000).toISOString(),
      external_message_id: streamId,
    })
    running.current_run_view!.generation = 'generation-new'
    streamHandler?.({ type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running } as UIStreamEvent)
    await flushPromises()

    expect(store.messages).toHaveLength(4)
    expect(store.messages[0]).toMatchObject({ role: 'user', text: 'old prompt' })
    expect(store.messages[1]).toMatchObject({
      role: 'assistant',
      streaming: false,
      messages: [
        expect.objectContaining({ type: 'text', content: 'old partial' }),
        expect.objectContaining({ type: 'error', content: 'old runtime failed' }),
      ],
    })
    expect(store.messages[2]).toMatchObject({ role: 'user', text: 'new prompt' })
    expect(store.messages[3]).toMatchObject({
      role: 'assistant',
      streaming: true,
      messages: [{ id: 0, type: 'text', content: 'new partial' }],
    })
  })

  it('resubscribes active and background running sessions when the websocket reconnects', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({ type: 'start', stream_id: 'stream-background', session_id: 'session-1' } as UIStreamEvent)
    api.fetchMessagesUI.mockResolvedValueOnce([])
    store.selectSession('session-2')
    await flushPromises()
    runtimeSubscribeMessages = []

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as { onOpen?: (() => void) | null }
    websocket.onOpen?.()

    expect(runtimeSubscribeMessages).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-2' }),
    ]))
  })

  it('retries a failed runtime subscription immediately on a new websocket generation', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const failedSubscription = runtimeSubscribeMessages.at(-1)
    const failedInvocationId = String(failedSubscription?.invocation_id ?? '')
    expect(failedInvocationId).not.toBe('')
    streamHandler?.({
      type: 'command_error',
      invocation_id: failedInvocationId,
      error: { code: 'runtime_subscription_failed', message: 'temporary failure' },
    } as UIStreamEvent)
    runtimeSubscribeMessages = []

    const websocket = api.connectWebSocket.mock.results.at(-1)?.value as {
      onClose?: (() => void) | null
      onOpen?: (() => void) | null
    }
    websocket.onClose?.()
    websocket.onOpen?.()

    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])
    consoleError.mockRestore()
  })

  it('applies runtime snapshots for running and completed state through the store reducer', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-runtime-delta'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'running', 11),
    } as UIStreamEvent)

    expect(store.streaming).toBe(true)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.streaming).toBe(true)
    expect(assistant.messages.find(block => block.type === 'text')).toMatchObject({
      content: 'I will check the current state.',
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-approval')).toMatchObject({
      approval: {
        approval_id: 'approval-1',
        status: 'pending',
      },
    })

    let resolveRefresh: ((turns: unknown[]) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveRefresh = resolve
    }))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 12,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'completed', 12),
    } as UIStreamEvent)

    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    expect(assistant.messages.some(block => block.type === 'error')).toBe(false)
    resolveRefresh?.([])
    await flushPromises()
  })

  it('ignores out-of-order runtime state for the active session', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-runtime-ordering'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'running', 11),
    } as UIStreamEvent)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')

    let resolveRefresh: ((turns: unknown[]) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveRefresh = resolve
    }))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 12,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'completed', 12),
    } as UIStreamEvent)
    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    const messageCount = store.messages.length

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'running', 11),
    } as UIStreamEvent)

    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    expect(store.messages).toHaveLength(messageCount)
    resolveRefresh?.([])
    await flushPromises()
  })

  it.each([
    { status: 'aborted', error: '', wantError: 'Response stopped' },
    { status: 'errored', error: 'runtime failed', wantError: 'runtime failed' },
    { status: 'lost', error: 'runtime owner lease expired', wantError: 'runtime owner lease expired' },
  ])('applies runtime snapshot terminal state $status through the store reducer', async ({ status, error, wantError }) => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = `stream-runtime-${status}`
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 11,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, 'running', 11),
    } as UIStreamEvent)
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(assistant?.role).toBe('assistant')
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')

    let resolveRefresh: ((turns: unknown[]) => void) | undefined
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>((resolve) => {
      resolveRefresh = resolve
    }))
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 12,
      snapshot: runtimeSnapshotFromScript(richActiveRunStoreScript('session-1', streamId), 'session-1', streamId, status, 12, error),
    } as UIStreamEvent)
    await flushPromises()

    expect(store.streaming).toBe(false)
    expect(assistant.streaming).toBe(false)
    const errorBlocks = assistant.messages.filter(block => block.type === 'error')
    expect(errorBlocks).toContainEqual(expect.objectContaining({ content: wantError }))
    resolveRefresh?.([])
    await flushPromises()
  })

  it('applies the Go-generated rich runtime delta stream through successful completion', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const stream = structuredClone(richActiveRunContractFixture.runtime_stream)
    for (const event of stream) streamHandler?.(event)

    const user = store.messages.find(turn => turn.role === 'user')
    const assistant = store.messages.find(turn => turn.role === 'assistant')
    expect(user).toMatchObject({ role: 'user', text: 'Inspect the workspace' })
    expect(assistant).toMatchObject({ role: 'assistant', streaming: true })
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.messages.find(block => block.type === 'reasoning')).toMatchObject({
      content: 'I need to inspect the workspace.',
    })
    expect(assistant.messages.find(block => block.type === 'text')).toMatchObject({
      content: 'I will check the current state.',
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-exec')).toMatchObject({
      progress: ['queued', { stdout: '/workspace\n' }],
      done: true,
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-approval')).toMatchObject({
      approval: { approval_id: 'approval-1', status: 'pending' },
    })
    expect(assistant.messages.find(block => block.type === 'tool' && block.toolCallId === 'call-ask')).toMatchObject({
      userInput: { user_input_id: 'input-1', status: 'pending' },
    })

    const deltas = stream.filter(event => event.type === 'runtime_delta')
    expect(deltas).not.toHaveLength(0)
    for (const event of deltas) {
      expect(event).not.toHaveProperty('snapshot')
      expect(event).toHaveProperty('delta')
    }

    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>(() => {}))
    const terminalStream = structuredClone(richActiveRunContractFixture.runtime_terminal_stream ?? [])
    expect(terminalStream).not.toHaveLength(0)
    for (const event of terminalStream) streamHandler?.(event)
    await flushPromises()

    expect(terminalStream.every(event => event.type === 'runtime_delta')).toBe(true)
    expect(terminalStream.some(event => event.type === 'runtime_delta' && event.delta.run?.status === 'completed')).toBe(true)
    expect(assistant).toMatchObject({ role: 'assistant', streaming: false })
    expect(assistant.messages.some(block => block.type === 'error')).toBe(false)
  })

  it('recovers the chat projection from the Go-generated runtime checkpoint', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const fixture = structuredClone(runtimeRecoveryContractFixture)
    streamHandler?.(fixture.runtime_snapshot)
    streamHandler?.(fixture.gap_delta)
    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])

    streamHandler?.(fixture.delayed_delta)
    expect(runtimeSubscribeMessages).toHaveLength(1)

    streamHandler?.(fixture.runtime_checkpoint)
    streamHandler?.(fixture.post_recovery_delta)

    expect(store.messages.find(turn => turn.role === 'user')).toMatchObject({
      text: 'Inspect the workspace',
      externalMessageId: 'stream-recovery',
    })
    expect(store.messages.find(turn => turn.role === 'assistant')).toMatchObject({
      streaming: true,
      messages: [expect.objectContaining({ type: 'text', content: 'missing checkpoint continued' })],
    })
    expect(store.streaming).toBe(true)
  })

  it('replays the complete Go-generated generation rollover delta stream', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    api.fetchMessagesUI.mockImplementation(() => new Promise<UITurn[]>(() => {}))

    const stream = structuredClone(generationReuseContractFixture.runtime_stream)
    const terminalIndex = stream.findIndex(event =>
      event.type === 'runtime_delta' && event.delta.run?.status === 'completed')
    expect(terminalIndex).toBeGreaterThan(0)
    for (const event of stream.slice(0, terminalIndex + 1)) streamHandler?.(event)

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'old prompt',
      'old answer',
    ])
    expect(store.streaming).toBe(false)

    for (const event of stream.slice(terminalIndex + 1)) streamHandler?.(event)

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'old prompt',
      'old answer',
      'new prompt',
      'new partial',
    ])
    expect(store.messages[1]).toMatchObject({ id: 'runtime-stream-generation-reuse', streaming: false })
    expect(store.messages[3]).toMatchObject({
      id: 'runtime-stream-generation-reuse-generation-b',
      role: 'assistant',
      streaming: true,
    })
  })

  it('keeps persisted output separate when attaching to a Go-generated reused stream snapshot', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    store.messages.push({
      id: 'user-generation-a',
      role: 'user',
      text: 'old prompt',
      attachments: [],
      timestamp: '2026-07-10T00:00:00Z',
      externalMessageId: 'stream-generation-reuse',
      streaming: false,
      isSelf: true,
    }, {
      id: 'assistant-generation-a',
      role: 'assistant',
      messages: [{ id: 0, type: 'text', content: 'old answer' }],
      timestamp: '2026-07-10T00:00:10Z',
      streaming: false,
    })

    const stream = structuredClone(generationReuseContractFixture.runtime_stream)
    expect(stream.some(event => event.type === 'runtime_delta'
      && event.delta.current_run_view?.generation === 'generation-a')).toBe(true)
    expect(stream.some(event => event.type === 'runtime_delta'
      && event.delta.current_run_view?.generation === 'generation-b')).toBe(true)
    streamHandler?.(structuredClone(generationReuseContractFixture.runtime_snapshot) as UIStreamEvent)

    expect(store.messages.map(turn => turn.role === 'user' ? turn.text : turn.messages[0]?.content)).toEqual([
      'old prompt',
      'old answer',
      'new prompt',
      'new partial',
    ])
    expect(store.messages[1]).toMatchObject({ id: 'assistant-generation-a', streaming: false })
    expect(store.messages[3]).toMatchObject({
      id: 'runtime-stream-generation-reuse',
      role: 'assistant',
      streaming: true,
    })
  })

  it('projects the Go-generated admission checkpoints without requesting a resync', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const stream = structuredClone(richActiveRunContractFixture.runtime_admission_stream ?? [])
    expect(stream).toHaveLength(3)
    for (const event of stream) streamHandler?.(event)

    expect(runtimeSubscribeMessages).toEqual([])
    expect(store.streaming).toBe(true)
    expect(store.messages.find(turn => turn.role === 'user')).toMatchObject({
      text: 'Inspect the workspace',
      externalMessageId: 'stream-admission',
    })
    expect(store.messages.find(turn => turn.id === 'runtime-stream-admission')).toMatchObject({
      role: 'assistant',
      streaming: true,
    })
  })

  it('projects the Go-generated retry reset without retaining discarded output', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const stream = structuredClone(richActiveRunContractFixture.runtime_reset_stream ?? [])
    expect(stream).toHaveLength(4)
    for (const event of stream) streamHandler?.(event)

    expect(runtimeSubscribeMessages).toEqual([])
    const assistant = store.messages.find(turn => turn.id === 'runtime-stream-reset')
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toEqual([
      { id: 1, type: 'text', content: 'replacement draft' },
    ])
  })

  it('accepts the complete Go-generated steer lifecycle without requesting a resync', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const stream = structuredClone(richActiveRunContractFixture.runtime_steer_stream ?? [])
    expect(stream).toHaveLength(4)
    for (const event of stream) streamHandler?.(event)

    expect(runtimeSubscribeMessages).toEqual([])
    expect(store.messages.find(turn => turn.id === 'runtime-stream-steer')).toMatchObject({
      role: 'assistant',
      streaming: true,
    })
  })

  it('applies the Go-generated interrupted runtime delta stream as a terminal failure', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    api.fetchMessagesUI.mockImplementationOnce(() => new Promise<unknown[]>(() => {}))

    for (const event of structuredClone(interruptedRunContractFixture.runtime_stream)) {
      streamHandler?.(event)
    }
    await flushPromises()

    const assistant = store.messages.find(turn => turn.role === 'assistant' && turn.messages.some(block => block.type === 'text' && block.content === 'partial output'))
    expect(assistant).toMatchObject({ role: 'assistant', streaming: false })
    if (assistant?.role !== 'assistant') throw new Error('missing assistant turn')
    expect(assistant.messages.find(block => block.type === 'text')).toMatchObject({ content: 'partial output' })
    expect(assistant.messages.find(block => block.type === 'error')).toMatchObject({ content: 'runtime interrupted' })
  })

  it('waits for a checkpoint after a runtime delta gap and ignores delayed deltas', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const stream = structuredClone(richActiveRunContractFixture.runtime_stream)
    streamHandler?.(stream[0]!)
    runtimeSubscribeMessages = []
    for (const event of stream.slice(2, 5)) streamHandler?.(event)

    expect(runtimeSubscribeMessages).toEqual([expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' })])
    expect(api.fetchSessionRuntime).not.toHaveBeenCalled()
    const beforeRecovery = store.messages.find(turn => turn.role === 'assistant')
    expect(beforeRecovery?.role === 'assistant' ? beforeRecovery.messages : []).toEqual([])

    streamHandler?.(stream[1]!)
    const afterDelayedDelta = store.messages.find(turn => turn.role === 'assistant')
    expect(afterDelayedDelta?.role === 'assistant' ? afterDelayedDelta.messages : []).toEqual([])

    streamHandler?.(structuredClone(richActiveRunContractFixture.runtime_snapshot) as UIStreamEvent)
    const recovered = store.messages.find(turn => turn.role === 'assistant')
    expect(recovered?.role === 'assistant' ? recovered.messages.find(block => block.type === 'text') : null).toMatchObject({
      content: 'I will check the current state.',
    })
  })

  it('resyncs after a malformed runtime snapshot without poisoning later deltas', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    const malformed = {
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-malformed',
      epoch: 'epoch-session-1',
      seq: 1,
      snapshot: {
        bot_id: 'bot-1',
        session_id: 'session-1',
        epoch: 'epoch-session-1',
        seq: 1,
        queue: [],
        current_run_view: {
          stream_id: 'stream-malformed',
          status: 'running',
          messages: [null],
        },
      },
    } as unknown as UIStreamEvent
    expect(() => streamHandler?.(malformed)).not.toThrow()
    expect(runtimeSubscribeMessages).toEqual([expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' })])
    expect(store.messages).toEqual([])

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-recovered',
      epoch: 'epoch-session-1',
      seq: 2,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-recovered', 'running', 2),
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-recovered',
      epoch: 'epoch-session-1',
      seq: 3,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'recovered' }] },
    } as UIStreamEvent)

    const assistant = store.messages.find(turn => turn.id === 'runtime-stream-recovered')
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toContainEqual({
      id: 0,
      type: 'text',
      content: 'recovered',
    })
  })

  it('rejects a runtime snapshot whose envelope and payload target different sessions', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-wrong-session',
      seq: 1,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'must not render' } } as UIStreamEvent],
        'session-2',
        'stream-wrong-session',
        'running',
        1,
      ),
    } as UIStreamEvent)

    expect(store.messages).toEqual([])
    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])
  })

  it('rejects runtime state whose declared bot conflicts with the websocket source bot', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()
    runtimeSubscribeMessages = []
    const snapshot = runtimeSnapshotFromScript(
      [{ type: 'message', data: { id: 0, type: 'text', content: 'must not render' } } as UIStreamEvent],
      'session-1',
      'stream-wrong-bot',
      'running',
      1,
    )
    snapshot.bot_id = 'bot-2'

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-2',
      session_id: 'session-1',
      stream_id: 'stream-wrong-bot',
      seq: 1,
      snapshot,
    } as UIStreamEvent)

    expect(store.messages).toEqual([])
    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])
  })

  it('rejects an empty snapshot that carries a stream envelope without terminating the active run', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-active',
      seq: 1,
      snapshot: runtimeSnapshotFromScript([], 'session-1', 'stream-active', 'running', 1),
    } as UIStreamEvent)
    const assistant = store.messages.find(turn => turn.role === 'assistant' && turn.id === 'runtime-stream-active')
    expect(assistant).toMatchObject({ role: 'assistant', streaming: true })
    runtimeSubscribeMessages = []

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: 'stream-active',
      seq: 2,
      snapshot: { bot_id: 'bot-1', session_id: 'session-1', seq: 2, queue: [] },
    } as UIStreamEvent)

    expect(assistant).toMatchObject({ role: 'assistant', streaming: true })
    expect(store.streaming).toBe(true)
    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])
  })

  it('waits for an admission checkpoint when a current-run delta skips a sequence', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const epoch = 'epoch-session-1'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      seq: 0,
      snapshot: { bot_id: 'bot-1', session_id: 'session-1', epoch, seq: 0, queue: [] },
    } as UIStreamEvent)
    runtimeSubscribeMessages = []
    const baseline = richActiveRunContractFixture.runtime_stream[0]
    if (baseline?.type !== 'runtime_snapshot' || !baseline.snapshot?.current_run_view) {
      throw new Error('missing generated admission snapshot')
    }
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: baseline.snapshot.current_run_view.stream_id,
      seq: 2,
      delta: { current_run_view: structuredClone(baseline.snapshot.current_run_view) },
    } as UIStreamEvent)

    expect(runtimeSubscribeMessages).toEqual([
      expect.objectContaining({ type: 'runtime_subscribe', session_id: 'session-1' }),
    ])
    expect(store.streaming).toBe(false)

    const checkpoint = {
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      seq: 2,
      queue: [],
      current_run_view: structuredClone(baseline.snapshot.current_run_view),
    }
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: baseline.snapshot.current_run_view.stream_id,
      seq: 2,
      snapshot: checkpoint,
    } as UIStreamEvent)

    expect(store.streaming).toBe(true)
    expect(store.messages.find(turn => turn.role === 'user')).toMatchObject({ text: 'Inspect the workspace' })
  })

  it('removes stale assistant blocks from authoritative checkpoints and snapshots', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const baseline = structuredClone(richActiveRunContractFixture.runtime_snapshot)
    const run = baseline.snapshot.current_run_view
    if (!run) throw new Error('missing generated current run')
    streamHandler?.({
      type: 'start',
      stream_id: run.stream_id,
      session_id: 'session-1',
    } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: run.stream_id,
      session_id: 'session-1',
      data: { id: 99, type: 'text', content: 'discarded legacy output' },
    } as UIStreamEvent)
    const legacyAssistant = store.messages.find(turn => turn.role === 'assistant'
      && turn.messages.some(block => block.id === 99))
    if (!legacyAssistant || legacyAssistant.role !== 'assistant') throw new Error('missing legacy assistant turn')
    expect(legacyAssistant.messages.map(block => block.id)).toContain(99)

    streamHandler?.(baseline as UIStreamEvent)
    const assistant = legacyAssistant
    expect(assistant.messages.map(block => block.id)).not.toContain(99)
    expect(assistant.messages.map(block => block.id)).toContain(0)

    const checkpointRun = structuredClone(run)
    checkpointRun.messages = checkpointRun.messages?.filter(message => message.id !== 0)
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      stream_id: run.stream_id,
      epoch: baseline.epoch,
      seq: baseline.seq + 1,
      delta: { current_run_view: checkpointRun },
    } as UIStreamEvent)
    expect(assistant.messages.map(block => block.id)).not.toContain(0)
    expect(assistant.messages.map(block => block.id)).toContain(1)

    const snapshot = structuredClone(baseline)
    snapshot.seq = baseline.seq + 2
    snapshot.snapshot.seq = snapshot.seq
    snapshot.snapshot.current_run_view!.messages = checkpointRun.messages?.filter(message => message.id !== 1)
    streamHandler?.(snapshot as UIStreamEvent)
    expect(assistant.messages.map(block => block.id)).not.toContain(1)
  })

  it('ignores delayed deltas until a checkpoint recovers a gap and detects a later gap', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const epoch = 'epoch-session-1'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      seq: 0,
      snapshot: { bot_id: 'bot-1', session_id: 'session-1', epoch, seq: 0, queue: [] },
    } as UIStreamEvent)
    runtimeSubscribeMessages = []
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: 'stream-gap',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'unusable gap' }] },
    } as UIStreamEvent)
    expect(runtimeSubscribeMessages).toHaveLength(1)

    const active = structuredClone(richActiveRunContractFixture.runtime_snapshot.snapshot.current_run_view)
    if (!active) throw new Error('missing generated current run')
    runtimeSubscribeMessages = []
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: active.stream_id,
      seq: 1,
      delta: { current_run_view: active },
    } as UIStreamEvent)
    expect(runtimeSubscribeMessages).toEqual([])
    expect(store.streaming).toBe(false)

    const checkpoint = {
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      seq: 2,
      queue: [],
      current_run_view: active,
    }
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: active.stream_id,
      seq: 2,
      snapshot: checkpoint,
    } as UIStreamEvent)
    expect(store.streaming).toBe(true)

    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch,
      stream_id: active.stream_id,
      seq: 4,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'second gap' }] },
    } as UIStreamEvent)
    expect(runtimeSubscribeMessages).toHaveLength(1)
  })

  it('finalizes a superseded local stream when a new run checkpoint arrives', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 1,
      snapshot: runtimeSnapshotFromScript(
        [{ type: 'message', data: { id: 0, type: 'text', content: 'old output' } } as UIStreamEvent],
        'session-1',
        'stream-old-checkpoint',
        'running',
        1,
      ),
    } as UIStreamEvent)
    const oldAssistant = store.messages.find(turn => turn.role === 'assistant')
    expect(oldAssistant).toMatchObject({ role: 'assistant', streaming: true })

    const nextSnapshot = runtimeSnapshotFromScript(
      [],
      'session-1',
      'stream-new-checkpoint',
      'running',
      3,
    )
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: nextSnapshot.epoch,
      stream_id: 'stream-new-checkpoint',
      seq: 3,
      snapshot: nextSnapshot,
    } as UIStreamEvent)

    expect(oldAssistant).toMatchObject({ role: 'assistant', streaming: false })
    expect(store.messages.filter(turn => turn.role === 'assistant' && turn.streaming)).toHaveLength(1)
    expect(store.messages.find(turn => turn.id === 'runtime-stream-new-checkpoint')).toMatchObject({
      role: 'assistant',
      streaming: true,
    })
  })

  it('projects batched message and progress appends without duplicating newly created blocks', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const baseline = runtimeSnapshotFromScript([], 'session-1', 'stream-batch', 'running', 2)
    baseline.current_run_view!.messages = [{ id: 1, type: 'tool', name: 'exec', tool_call_id: 'call-1' }]
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: baseline.epoch,
      stream_id: 'stream-batch',
      seq: 2,
      snapshot: baseline,
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: baseline.epoch,
      stream_id: 'stream-batch',
      seq: 3,
      delta: {
        message_appends: [
          { id: 0, type: 'text', content: 'hello' },
          { id: 0, type: 'text', content: ' world' },
        ],
        progress_appends: [
          { id: 1, progress: 'queued' },
          { id: 1, progress: 'done' },
        ],
      },
    } as UIStreamEvent)

    const assistant = store.messages.find(turn => turn.id === 'runtime-stream-batch')
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toContainEqual({
      id: 0,
      type: 'text',
      content: 'hello world',
    })
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toContainEqual(expect.objectContaining({
      id: 1,
      type: 'tool',
      progress: ['queued', 'done'],
    }))
  })

  it('keeps canonical runtime ownership when a late legacy end arrives', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const send = store.sendMessage('keep running')
    await flushPromises()
    const streamId = lastStreamId
    const running = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'partial' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1)
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running,
    } as UIStreamEvent)

    streamHandler?.({ type: 'end', stream_id: streamId, session_id: 'session-1' } as UIStreamEvent)
    expect(store.streaming).toBe(true)
    expect(store.messages.find(turn => turn.role === 'assistant')).toMatchObject({ streaming: true })

    const aborted = structuredClone(running)
    aborted.seq = 2
    aborted.current_run_view!.status = 'aborted'
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 2, snapshot: aborted,
    } as UIStreamEvent)

    await expect(send).resolves.toMatchObject({ ok: false, stage: 'stream' })
    expect(store.streaming).toBe(false)
  })

  it('does not revive checkpoint-reset blocks from a late legacy message', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const streamId = 'stream-late-legacy-message'
    const running = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'canonical' } } as UIStreamEvent,
    ], 'session-1', streamId, 'running', 1)
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-1', seq: 1, snapshot: running,
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: running.epoch,
      stream_id: streamId,
      seq: 2,
      delta: { reset_messages: true },
    } as UIStreamEvent)
    streamHandler?.({
      type: 'message',
      stream_id: streamId,
      session_id: 'session-1',
      data: { id: 99, type: 'text', content: 'stale legacy output' },
    } as UIStreamEvent)

    const assistant = store.messages.find(turn => turn.id === `runtime-${streamId}`)
    expect(assistant?.role === 'assistant' ? assistant.messages : []).toEqual([])
    expect(assistant).toMatchObject({ streaming: true })
  })

  it('sends an early abort before runtime generation hydration', async () => {
    sendEvents = []
    api.fetchSessions.mockResolvedValueOnce({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' }],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    await flushPromises()

    const send = store.sendMessage('abort before admission')
    await flushPromises()
    const streamId = lastStreamId
    const ws = api.connectWebSocket.mock.results.at(-1)?.value as { abort: ReturnType<typeof vi.fn> }

    store.abort()

    expect(ws.abort).toHaveBeenCalledWith(streamId, 'session-1', '')
    expect(abortedWSStreams).toContain(streamId)
    expect(store.streaming).toBe(true)

    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 1,
      snapshot: runtimeSnapshotFromScript([], 'session-1', streamId, 'aborted', 1),
    } as UIStreamEvent)
    await expect(send).resolves.toMatchObject({ ok: false })
    expect(store.streaming).toBe(false)
  })

  it('isolates the same runtime stream id across two sessions', async () => {
    api.fetchSessions.mockResolvedValueOnce({
      items: [
        { id: 'session-1', bot_id: 'bot-1', title: 'A', type: 'chat' },
        { id: 'session-2', bot_id: 'bot-1', title: 'B', type: 'chat' },
      ],
      nextCursor: null,
    })
    const store = useChatStore()
    await store.selectBot('bot-1')
    store.bindChatView('chat:shared-runtime-b', {
      botId: 'bot-1', sessionId: 'session-2', viewId: 'chat:shared-runtime-b',
    }, true)
    await flushPromises()

    const streamId = 'shared-runtime-stream'
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-1',
      seq: 1,
      snapshot: runtimeSnapshotFromScript([
        { type: 'message', data: { id: 0, type: 'text', content: 'answer A' } } as UIStreamEvent,
      ], 'session-1', streamId, 'running', 1),
    } as UIStreamEvent)
    streamHandler?.({
      type: 'runtime_snapshot',
      bot_id: 'bot-1',
      session_id: 'session-2',
      seq: 1,
      snapshot: runtimeSnapshotFromScript([
        { type: 'message', data: { id: 0, type: 'text', content: 'answer B' } } as UIStreamEvent,
      ], 'session-2', streamId, 'running', 1),
    } as UIStreamEvent)

    expect(store.isSessionStreaming('bot-1', 'session-1')).toBe(true)
    expect(store.isSessionStreaming('bot-1', 'session-2')).toBe(true)
    expect(store.messages.flatMap(turn => turn.role === 'assistant' ? turn.messages : []))
      .toContainEqual(expect.objectContaining({ content: 'answer A' }))

    await store.selectSession('session-2')
    await flushPromises()
    expect(store.messages.flatMap(turn => turn.role === 'assistant' ? turn.messages : []))
      .toContainEqual(expect.objectContaining({ content: 'answer B' }))

    const completedB = runtimeSnapshotFromScript([
      { type: 'message', data: { id: 0, type: 'text', content: 'answer B' } } as UIStreamEvent,
    ], 'session-2', streamId, 'completed', 2)
    streamHandler?.({
      type: 'runtime_snapshot', bot_id: 'bot-1', session_id: 'session-2', seq: 2, snapshot: completedB,
    } as UIStreamEvent)
    expect(store.isSessionStreaming('bot-1', 'session-2')).toBe(false)
    expect(store.isSessionStreaming('bot-1', 'session-1')).toBe(true)
  })
})
