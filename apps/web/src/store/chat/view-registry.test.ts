import { computed } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createChatViewRegistry,
  type ChatTranscriptHooks,
  type ChatTranscriptReader,
} from './view-registry'
import type { ChatAssistantTurn, ChatMessage } from './types'

interface FakeTranscript extends ChatTranscriptReader {
  id: string
  messages: ChatMessage[]
  hooks: ChatTranscriptHooks
}

function pendingApprovalTurn(id: string): ChatAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages: [{
      id: 1,
      type: 'tool',
      name: 'exec',
      input: {},
      tool_call_id: `call-${id}`,
      running: false,
      toolCallId: `call-${id}`,
      toolName: 'exec',
      result: null,
      done: true,
      approval: {
        approval_id: `approval-${id}`,
        status: 'pending',
        can_approve: true,
      },
    }],
    timestamp: '2026-01-01T00:00:01.000Z',
    streaming: false,
  }
}

function pendingUserInputTurn(id: string): ChatAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages: [{
      id: 1,
      type: 'tool',
      name: 'ask_user',
      input: {},
      tool_call_id: `call-${id}`,
      running: false,
      toolCallId: `call-${id}`,
      toolName: 'ask_user',
      result: null,
      done: true,
      userInput: {
        user_input_id: `input-${id}`,
        status: 'pending',
        questions: [],
        can_respond: true,
      },
    }],
    timestamp: '2026-01-01T00:00:01.000Z',
    streaming: false,
  }
}

function makeRegistry(options: { cacheLimit?: number, streaming?: Set<string> } = {}) {
  const createTranscript = vi.fn((target, hooks: ChatTranscriptHooks): FakeTranscript => ({
    id: target.sessionId ? `session:${target.sessionId}` : `draft:${target.viewId}`,
    messages: [],
    hooks,
  }))
  const onPromoteTranscript = vi.fn()
  const onDisposeTranscript = vi.fn()
  const onEvict = vi.fn()
  const onSnapshot = vi.fn()
  const onRefreshApplied = vi.fn()
  const registry = createChatViewRegistry({
    cacheLimit: options.cacheLimit,
    isSessionStreaming: (botId, sessionId) => options.streaming?.has(`${botId}:${sessionId}`) === true,
    createTranscript,
    onPromoteTranscript,
    onDisposeTranscript,
    onEvict,
    onSnapshot,
    onRefreshApplied,
  })
  return {
    registry,
    createTranscript,
    onPromoteTranscript,
    onDisposeTranscript,
    onEvict,
    onSnapshot,
    onRefreshApplied,
  }
}

describe('chat view registry', () => {
  it('shares one transcript reader for a session and isolates different sessions', () => {
    const { registry, createTranscript } = makeRegistry()
    const first = registry.getOrCreate({ botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:1' })
    const duplicate = registry.getOrCreate({ botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:2' })
    const second = registry.getOrCreate({ botId: 'bot-1', sessionId: 'session-b', viewId: 'chat:3' })

    expect(duplicate).toBe(first)
    expect(second.transcript).not.toBe(first.transcript)
    expect(createTranscript).toHaveBeenCalledTimes(2)
  })

  it('passes normalized targets and forwards transcript hooks without owning writes', () => {
    const { registry, createTranscript, onSnapshot, onRefreshApplied } = makeRegistry()
    const view = registry.getOrCreate({ botId: ' bot-1 ', sessionId: ' session-a ', viewId: ' chat:1 ' })
    const target = createTranscript.mock.calls[0]![0]

    expect(target).toEqual({ botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:1' })
    view.transcript.hooks.onSnapshot('session-a', [])
    view.transcript.hooks.onRefreshApplied('bot-1', 'session-a', '2026-01-01T00:00:00.000Z')

    expect(onSnapshot).toHaveBeenCalledWith(view, 'session-a', [])
    expect(onRefreshApplied).toHaveBeenCalledWith(view, 'session-a', '2026-01-01T00:00:00.000Z')
    expect(view.initialized).toBe(true)
  })

  it('promotes a draft through the injected transcript callback without mutating messages', () => {
    const { registry, onPromoteTranscript, onDisposeTranscript } = makeRegistry()
    const draft = registry.getOrCreate({ botId: 'bot-1', sessionId: null, viewId: 'chat:1' })
    const immutableMessages = Object.freeze<ChatMessage[]>([])
    draft.transcript.messages = immutableMessages as unknown as ChatMessage[]
    registry.bindPanel('chat:1', { botId: 'bot-1', sessionId: null, viewId: 'chat:1' }, true)

    const promoted = registry.promoteDraft('bot-1', 'chat:1', 'session-a')

    expect(onPromoteTranscript).toHaveBeenCalledOnce()
    expect(onPromoteTranscript).toHaveBeenCalledWith(promoted.transcript, draft.transcript)
    expect(promoted.transcript.messages).toEqual([])
    expect(draft.transcript.messages).toBe(immutableMessages)
    expect(onDisposeTranscript).toHaveBeenCalledWith(draft.transcript)
    expect(registry.getPanel('chat:1')).toBe(promoted)
    expect(registry.getDraft('bot-1', 'chat:1')).toBeUndefined()
  })

  it('promotes into an existing session without merging transcript content itself', () => {
    const { registry, onPromoteTranscript, onDisposeTranscript } = makeRegistry()
    const session = registry.getOrCreate({ botId: 'bot-1', sessionId: 'session-a', viewId: 'session-panel' })
    const draft = registry.getOrCreate({ botId: 'bot-1', sessionId: null, viewId: 'draft-panel' })
    session.transcript.messages.push(pendingApprovalTurn('settled'))
    draft.transcript.messages.push(pendingUserInputTurn('optimistic'))
    registry.bindPanel('draft-panel', { botId: 'bot-1', sessionId: null, viewId: 'draft-panel' }, true)

    const promoted = registry.promoteDraft('bot-1', 'draft-panel', 'session-a')

    expect(promoted).toBe(session)
    expect(session.transcript.messages).toHaveLength(1)
    expect(draft.transcript.messages).toHaveLength(1)
    expect(onPromoteTranscript).toHaveBeenCalledWith(session.transcript, draft.transcript)
    expect(onDisposeTranscript).toHaveBeenCalledWith(draft.transcript)
    expect(registry.getPanel('draft-panel')).toBe(session)
  })

  it('carries draft workspace selection metadata into the promoted session', () => {
    const { registry } = makeRegistry()
    const draft = registry.getOrCreate({ botId: 'bot-1', sessionId: null, viewId: 'chat:1' })
    draft.workspaceTargetId.value = 'computer-b'
    draft.workspaceTargetSnapshot.value = {
      target_id: 'computer-b',
      kind: 'remote',
      name: 'Computer B',
    }
    draft.workspaceTargetSelectionSource.value = 'user'

    const promoted = registry.promoteDraft('bot-1', 'chat:1', 'session-a')

    expect(promoted.workspaceTargetId.value).toBe('computer-b')
    expect(promoted.workspaceTargetSnapshot.value?.name).toBe('Computer B')
    expect(promoted.workspaceTargetSelectionSource.value).toBe('user')
  })

  it('reference-counts visible panels for a shared session', () => {
    const { registry } = makeRegistry()
    const target = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:1' }

    const first = registry.bindPanel('chat:1', target, true)
    const second = registry.bindPanel('chat:2', { ...target, viewId: 'chat:2' }, true)
    const stillVisible = registry.setPanelVisible('chat:1', false)
    const hidden = registry.setPanelVisible('chat:2', false)

    expect(first.activatedSession?.sessionId).toBe('session-a')
    expect(second.activatedSession).toBeNull()
    expect(stillVisible?.deactivatedSession).toBeNull()
    expect(hidden?.deactivatedSession?.sessionId).toBe('session-a')
  })

  it('keeps a shared session entry when one panel closes', () => {
    const { registry, onDisposeTranscript } = makeRegistry()
    const target = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:left' }
    const shared = registry.bindPanel('chat:left', target, true).view
    registry.bindPanel('chat:right', { ...target, viewId: 'chat:right' }, true)

    registry.unbindPanel('chat:left')

    expect(registry.getPanel('chat:left')).toBeUndefined()
    expect(registry.getPanel('chat:right')).toBe(shared)
    expect(shared.visiblePanelIds).toEqual(new Set(['chat:right']))
    expect(onDisposeTranscript).not.toHaveBeenCalled()
  })

  it('evicts by notifying transcript disposal without invoking a mutation method', () => {
    const { registry, onDisposeTranscript, onEvict } = makeRegistry({ cacheLimit: 0 })
    const target = { botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:1' }
    const attached = registry.bindPanel('chat:1', target, true).view
    Object.freeze(attached.transcript.messages)

    registry.setPanelVisible('chat:1', false)
    registry.prune()

    expect(registry.getPanel('chat:1')).toBeUndefined()
    expect(onDisposeTranscript).toHaveBeenCalledOnce()
    expect(onDisposeTranscript).toHaveBeenCalledWith(attached.transcript)
    expect(onEvict).toHaveBeenCalledOnce()
    expect(onEvict).toHaveBeenCalledWith(attached)
  })

  it('notifies eviction when the last panel releases a draft view', () => {
    const { registry, onDisposeTranscript, onEvict } = makeRegistry()
    const draft = registry.bindPanel('draft:1', {
      botId: 'bot-1',
      sessionId: null,
      viewId: 'draft:1',
    }, true).view

    registry.unbindPanel('draft:1')

    expect(registry.getDraft('bot-1', 'draft:1')).toBeUndefined()
    expect(onDisposeTranscript).toHaveBeenCalledWith(draft.transcript)
    expect(onEvict).toHaveBeenCalledOnce()
    expect(onEvict).toHaveBeenCalledWith(draft)
  })

  it('counts attached hidden sessions against the cache limit', () => {
    const { registry } = makeRegistry({ cacheLimit: 1 })
    registry.bindPanel('chat:1', {
      botId: 'bot-1',
      sessionId: 'session-attached',
      viewId: 'chat:1',
    }, false)
    const detached = registry.getOrCreate({
      botId: 'bot-1',
      sessionId: 'session-detached',
      viewId: 'chat:2',
    })

    registry.prune()

    expect(registry.getPanel('chat:1')).toBeUndefined()
    expect(registry.getSession('bot-1', 'session-attached')).toBeUndefined()
    expect(registry.getSession('bot-1', 'session-detached')).toBe(detached)
  })

  it('invalidates a cached pane lookup when its hidden attached view is evicted', () => {
    const { registry } = makeRegistry({ cacheLimit: 0 })
    const target = { botId: 'bot-1', sessionId: 'session-attached', viewId: 'chat:1' }
    const paneView = computed(() => {
      void registry.revision.value
      return registry.getOrCreate(target)
    })
    const first = paneView.value
    registry.bindPanel('chat:1', target, true)

    registry.setPanelVisible('chat:1', false)

    expect(paneView.value).not.toBe(first)
    expect(paneView.value.transcript).not.toBe(first.transcript)
  })

  it('evicts least-recent hidden sessions but protects visible, streaming, and pending views', () => {
    const streaming = new Set(['bot-1:streaming'])
    const { registry, onDisposeTranscript } = makeRegistry({ cacheLimit: 4, streaming })
    registry.bindPanel('chat:visible', { botId: 'bot-1', sessionId: 'visible', viewId: 'chat:visible' }, true)
    registry.getOrCreate({ botId: 'bot-1', sessionId: 'streaming', viewId: 'chat:streaming' })
    const pending = registry.getOrCreate({ botId: 'bot-1', sessionId: 'pending', viewId: 'chat:pending' })
    pending.transcript.messages.push(pendingApprovalTurn('pending'))
    const asking = registry.getOrCreate({ botId: 'bot-1', sessionId: 'asking', viewId: 'chat:asking' })
    asking.transcript.messages.push(pendingUserInputTurn('asking'))
    const oldest = registry.getOrCreate({ botId: 'bot-1', sessionId: 'oldest', viewId: 'chat:oldest' })
    registry.getOrCreate({ botId: 'bot-1', sessionId: 'newest', viewId: 'chat:newest' })

    registry.prune()

    expect(registry.getSession('bot-1', 'visible')).toBeDefined()
    expect(registry.getSession('bot-1', 'streaming')).toBeDefined()
    expect(registry.getSession('bot-1', 'pending')).toBeDefined()
    expect(registry.getSession('bot-1', 'asking')).toBeDefined()
    expect(registry.getSession('bot-1', 'oldest')).toBeUndefined()
    expect(onDisposeTranscript).toHaveBeenCalledWith(oldest.transcript)
  })

  it('disposes every owned transcript on reset without mutating transcript data', () => {
    const { registry, onDisposeTranscript } = makeRegistry()
    const first = registry.getOrCreate({ botId: 'bot-1', sessionId: 'session-a', viewId: 'chat:1' })
    const second = registry.getOrCreate({ botId: 'bot-2', sessionId: 'session-b', viewId: 'chat:2' })
    const firstMessages = first.transcript.messages
    const secondMessages = second.transcript.messages

    registry.resetAll()

    expect(onDisposeTranscript).toHaveBeenCalledTimes(2)
    expect(first.transcript.messages).toBe(firstMessages)
    expect(second.transcript.messages).toBe(secondMessages)
  })
})
