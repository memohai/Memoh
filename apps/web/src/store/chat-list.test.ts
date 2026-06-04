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
  createACPRuntime: vi.fn(),
  setACPRuntimeModel: vi.fn(),
  setACPRuntimeModelByID: vi.fn(),
  closeACPRuntime: vi.fn(),
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
