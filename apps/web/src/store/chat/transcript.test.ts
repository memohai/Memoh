import { describe, expect, it, vi } from 'vitest'
import { ref, toRaw } from 'vue'
import type { UIMessage, UITurn } from '@/composables/api/useChat.types'
import { isOptimisticTurn } from '@/store/chat-list.normalize'
import { createBackgroundTaskTracker } from './background-tasks'
import { createTranscriptController } from './transcript'
import type { ChatAssistantTurn, ChatUserTurn, ToolCallBlock } from './types'

vi.mock('@/store/user', () => ({
  useUserStore: () => ({ userInfo: { id: 'user-1' } }),
}))

function rawUser(id: string, text = 'hello', timestamp = '2026-01-01T00:00:00.000Z'): UITurn {
  return { id, role: 'user', text, timestamp, platform: 'local' }
}

function rawAssistant(id: string, messages: UIMessage[] = [], timestamp = '2026-01-01T00:00:01.000Z'): UITurn {
  return { id, role: 'assistant', messages, timestamp }
}

function assistant(id: string, messages: ChatAssistantTurn['messages'] = []): ChatAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages,
    timestamp: '2026-01-01T00:00:01.000Z',
    streaming: true,
    __optimistic: true,
  }
}

function makeTranscript() {
  const currentBotId = ref<string | null>('bot-1')
  const sessionId = ref<string | null>('session-1')
  const backgroundTasks = createBackgroundTaskTracker()
  const bumpFsChangedAtIfFsMutation = vi.fn()
  const fetchMessages = vi.fn().mockResolvedValue([])
  const locateMessage = vi.fn().mockResolvedValue({ items: [] })
  const transcript = createTranscriptController({
    currentBotId,
    sessionId,
    rememberBackgroundTask: backgroundTasks.rememberBackgroundTask,
    applyPendingBackgroundEventsToTool: backgroundTasks.applyPendingBackgroundEventsToTool,
    bumpFsChangedAtIfFsMutation,
    fetchMessages,
    locateMessage,
  })
  return { transcript, currentBotId, sessionId, bumpFsChangedAtIfFsMutation, fetchMessages, locateMessage }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

function approvalMessage(status = 'pending'): UIMessage {
  return {
    id: 1,
    type: 'tool',
    name: 'exec',
    input: { command: 'pwd' },
    tool_call_id: 'call-1',
    running: false,
    approval: { approval_id: 'approval-1', status, can_approve: status === 'pending' },
  }
}

describe('chat transcript controller', () => {
  it('is the single context gate for appending active-session turns', () => {
    const { transcript } = makeTranscript()
    const turn = assistant('assistant-1')

    transcript.appendTurnToSession('bot-1', 'other-session', turn)
    expect(transcript.messages).toHaveLength(0)

    transcript.appendTurnToSession('bot-1', 'session-1', turn)
    expect(toRaw(transcript.messages[0])).toBe(turn)
  })

  it('owns server-id lookup and latest visible turn queries', () => {
    const { transcript } = makeTranscript()
    transcript.replaceMessages([
      rawUser('user-1'),
      rawAssistant('assistant-1'),
      rawUser('user-2'),
      rawAssistant('assistant-2'),
    ], 'session-1')
    const latestUser = transcript.findTurnByServerId('user-2')!
    const latestAssistant = transcript.findTurnByServerId('assistant-2')!
    const optimisticUser: ChatUserTurn = {
      id: 'optimistic-user',
      role: 'user',
      text: 'pending',
      attachments: [],
      timestamp: '2026-01-01T00:00:03.000Z',
      streaming: false,
      isSelf: true,
      __optimistic: true,
    }
    transcript.appendToView(optimisticUser)

    expect(transcript.hasTurn(optimisticUser)).toBe(true)
    expect(transcript.findTurnByServerId('missing')).toBeNull()
    expect(transcript.isLatestVisibleUserTurn(latestUser)).toBe(true)
    expect(transcript.isLatestVisibleAssistantTurn(latestAssistant)).toBe(true)
    expect(transcript.isLatestVisibleUserTurn(optimisticUser)).toBe(false)
    expect(transcript.isLatestVisibleUserTurn(transcript.findTurnByServerId('user-1')!)).toBe(false)
  })

  it('preserves optimistic render identity when a server snapshot replaces the view', () => {
    const { transcript } = makeTranscript()
    const snapshotHook = vi.fn()
    transcript.setSnapshotHook(snapshotHook)
    const optimistic: ChatUserTurn = {
      id: 'local-user',
      role: 'user',
      text: 'hello',
      attachments: [],
      timestamp: '2026-01-01T00:00:00.000Z',
      streaming: false,
      isSelf: true,
      __optimistic: true,
    }
    transcript.appendToView(optimistic)

    transcript.replaceMessages([rawUser('server-user')], 'session-1')

    expect(snapshotHook).toHaveBeenCalledWith('session-1', expect.any(Array))
    expect(transcript.messages[0]).toMatchObject({ id: 'local-user', serverId: 'server-user' })
  })

  it('rolls an optimistic tail back only while its original context is active', () => {
    const { transcript, sessionId } = makeTranscript()
    transcript.replaceMessages([
      rawUser('user-1'),
      rawAssistant('assistant-1', [{ id: 1, type: 'text', content: 'old' }]),
    ], 'session-1')
    const target = transcript.messages[1]!
    const optimistic = assistant('assistant-local')
    const replaced = transcript.replaceTailFromTurn(target, [optimistic])

    sessionId.value = 'session-2'
    transcript.restoreTailFromOptimistic('bot-1', 'session-1', null, optimistic, replaced)
    expect(toRaw(transcript.messages[1])).toBe(optimistic)

    sessionId.value = 'session-1'
    transcript.restoreTailFromOptimistic('bot-1', 'session-1', null, optimistic, replaced)
    expect(transcript.messages[1]?.id).toBe('assistant-1')
  })

  it('keeps a local approval decision when a stale pending tool snapshot arrives', () => {
    const { transcript } = makeTranscript()
    transcript.replaceMessages([rawAssistant('assistant-1', [approvalMessage()])], 'session-1')
    const turn = transcript.messages[0] as ChatAssistantTurn
    const snapshots = transcript.snapshotToolApprovalStates('approval-1')

    transcript.markToolApprovalDecision('approval-1', 'approved')
    transcript.upsertAssistantUIMessage(turn, approvalMessage('pending'))
    const block = turn.messages[0] as ToolCallBlock
    expect(block.approval?.status).toBe('approved')

    transcript.restoreToolApprovalStates(snapshots)
    expect(block.approval?.status).toBe('pending')
  })

  it('replaces an authoritative assistant snapshot without losing matching local tool state', () => {
    const { transcript } = makeTranscript()
    const turn = assistant('assistant-1')
    transcript.appendToView(turn)
    transcript.upsertAssistantUIMessage(turn, { id: 0, type: 'text', content: 'stale' })
    transcript.upsertAssistantUIMessage(turn, approvalMessage('pending'))
    transcript.markToolApprovalDecision('approval-1', 'approved')

    transcript.replaceAssistantUIMessageSnapshot(turn, [
      { ...approvalMessage('pending'), id: 9 },
      { id: 2, type: 'text', content: 'canonical' },
    ], new Set([0, 1]))

    expect(turn.messages.map(block => block.id)).toEqual([1, 2])
    const tool = turn.messages[0]
    expect(tool?.type).toBe('tool')
    if (tool?.type === 'tool') expect(tool.approval?.status).toBe('approved')

    transcript.replaceAssistantUIMessageSnapshot(turn, [], new Set([2]))
    expect(turn.messages.map(block => block.id)).toEqual([1])
  })

  it('snapshots and restores optimistic user-input state', () => {
    const { transcript } = makeTranscript()
    const userInput = {
      user_input_id: 'input-1',
      status: 'pending',
      can_respond: true,
      questions: [{ id: 'q1', text: 'Pick', kind: 'text' as const }],
    }
    const message: UIMessage = {
      id: 1,
      type: 'tool',
      name: 'ask_user',
      input: {},
      tool_call_id: 'call-input',
      running: false,
      user_input: userInput,
    }
    transcript.replaceMessages([rawAssistant('assistant-1', [message])], 'session-1')
    const block = (transcript.messages[0] as ChatAssistantTurn).messages[0] as ToolCallBlock
    const snapshots = transcript.snapshotUserInputStates('input-1')
    transcript.markUserInputDecision('input-1', 'submitted')
    expect(block.userInput).toMatchObject({ status: 'submitted', can_respond: false })

    transcript.restoreUserInputStates(snapshots)

    expect(block.userInput).toMatchObject({ status: 'pending', can_respond: true })
  })

  it('replays ephemeral stream errors after refresh until user-scope reset', () => {
    const { transcript } = makeTranscript()
    transcript.replaceMessages([rawUser('user-1')], 'session-1')
    const failed = assistant('assistant-local', [{ id: 1, type: 'text', content: 'partial' }])
    transcript.appendToView(failed)
    transcript.finalizeStreamFailure(failed, 'bot-1', 'session-1', new Error('stream failed'))

    transcript.replaceMessages([rawUser('user-1')], 'session-1')
    expect(transcript.messages.some(turn =>
      turn.role === 'assistant' && turn.messages.some(block => block.type === 'error' && block.content === 'stream failed'),
    )).toBe(true)

    transcript.resetUserScope()
    transcript.replaceMessages([rawUser('user-1')], 'session-1')
    expect(transcript.messages).toHaveLength(1)
  })

  it('keeps the canonical user as retry target when an aborted turn has no assistant row', () => {
    const { transcript } = makeTranscript()
    transcript.replaceMessages([rawUser('user-aborted')], 'session-1')
    const failed = assistant('assistant-local')
    transcript.appendToView(failed)
    transcript.appendAssistantError(failed, 'session-1', 'Response stopped', false, {
      streamId: 'stream-aborted',
      generation: 'generation-aborted',
    })

    expect(failed.retryTargetId).toBe('user-aborted')
    transcript.replaceMessages([rawUser('user-aborted')], 'session-1')

    const replayed = transcript.messages.find((turn): turn is ChatAssistantTurn => turn.role === 'assistant')
    expect(replayed).toMatchObject({
      __ephemeral: true,
      retryTargetId: 'user-aborted',
    })
  })

  it('keeps identical runtime errors distinct by stream generation across refreshes', () => {
    const { transcript } = makeTranscript()
    const userA = { ...rawUser('user-a', 'same prompt'), external_message_id: 'stream-reused' }
    const userB = { ...rawUser('user-b', 'same prompt', '2026-01-01T00:01:00.000Z'), external_message_id: 'stream-reused' }
    const identityA = { streamId: 'stream-reused', generation: 'generation-a' }
    const identityB = { streamId: 'stream-reused', generation: 'generation-b' }

    transcript.replaceMessages([userA], 'session-1')
    const failedA = assistant('assistant-a')
    transcript.appendToView(failedA)
    transcript.appendAssistantError(failedA, 'session-1', 'Response stopped', true, identityA)

    transcript.replaceMessages([userA, userB], 'session-1')
    const failedB = assistant('assistant-b')
    transcript.appendToView(failedB)
    transcript.appendAssistantError(failedB, 'session-1', 'Response stopped', true, identityB)

    transcript.replaceMessages([userA, userB], 'session-1')

    const errors = transcript.messages.filter((turn): turn is ChatAssistantTurn =>
      turn.role === 'assistant'
      && turn.messages.some(block => block.type === 'error' && block.content === 'Response stopped'))
    expect(errors).toHaveLength(2)
    expect(errors.every(turn => turn.__ephemeral === true)).toBe(true)
    expect(transcript.isLatestVisibleAssistantTurn(errors[1]!)).toBe(false)
    expect(errors[0]?.id).not.toBe(errors[1]?.id)
    expect(transcript.assistantTurnForRuntimeError('session-1', identityA)).toBe(errors[0])
    expect(transcript.assistantTurnForRuntimeError('session-1', identityB)).toBe(errors[1])
  })

  it('does not accumulate identical terminal errors on one persisted assistant', () => {
    const { transcript } = makeTranscript()
    const user = { ...rawUser('user-1'), external_message_id: 'stream-reused' }
    const persisted = rawAssistant('assistant-1', [{ id: 0, type: 'text', content: 'partial' }])
    transcript.replaceMessages([user, persisted], 'session-1')
    const assistantTurn = transcript.messages[1]
    if (assistantTurn?.role !== 'assistant') throw new Error('missing assistant')

    transcript.appendAssistantError(assistantTurn, 'session-1', 'Response stopped', false, {
      streamId: 'stream-reused',
      generation: 'generation-a',
    })
    transcript.appendAssistantError(assistantTurn, 'session-1', 'Response stopped', false, {
      streamId: 'stream-reused',
      generation: 'generation-b',
    })
    transcript.replaceMessages([user, persisted], 'session-1')

    const stopped = transcript.messages.flatMap(turn => turn.role === 'assistant'
      ? turn.messages.filter(block => block.type === 'error' && block.content === 'Response stopped')
      : [])
    expect(stopped).toHaveLength(1)
  })

  it('routes completed tool messages through the fs mutation beacon', () => {
    const { transcript, bumpFsChangedAtIfFsMutation } = makeTranscript()
    const turn = assistant('assistant-1')
    const tool = approvalMessage()

    transcript.upsertAssistantUIMessage(turn, tool)

    expect(bumpFsChangedAtIfFsMutation).toHaveBeenCalledWith(tool)
  })

  it('normalizes and reconciles background-task turns without leaking tracker state', () => {
    const { transcript } = makeTranscript()
    const turns = transcript.normalizeTurns([
      rawAssistant('assistant-1', [{
        id: 1,
        type: 'tool',
        name: 'background',
        input: {},
        tool_call_id: 'call-bg',
        running: true,
        background_task: { task_id: 'task-1', status: 'running' },
      }]),
      {
        id: 'system-1',
        role: 'system',
        kind: 'background_task',
        timestamp: '2026-01-01T00:00:02.000Z',
        background_task: { task_id: 'task-1', status: 'completed' },
      },
    ] as UITurn[])

    const tool = (turns[0] as ChatAssistantTurn).messages[0] as ToolCallBlock
    expect(tool.backgroundTask?.status).toBe('completed')
    expect(tool.done).toBe(true)
  })

  it('keeps optimistic turns the initial-history snapshot does not contain yet', async () => {
    // First send from a draft: promoteDraftChatView starts the per-session
    // SSE, whose prepare step refreshes history. That fetch can resolve
    // AFTER the optimistic user+assistant turns were appended but BEFORE the
    // server persisted the send — the snapshot comes back empty. The refresh
    // replace must carry the unmatched optimistic turns over instead of
    // blanking the user's own message until the stream-end refresh.
    const { transcript, fetchMessages } = makeTranscript()
    const pending = deferred<UITurn[]>()
    fetchMessages.mockReturnValueOnce(pending.promise)

    const loading = transcript.loadInitialMessages('bot-1', 'session-1')
    const assistantTurn = transcript.createOptimisticAssistantTurn()
    const userTurn = transcript.createOptimisticUserTurn('hello first')
    transcript.appendToView(userTurn, assistantTurn)

    pending.resolve([])
    await loading
    // The user turn is carried over by the replace itself; the streaming
    // assistant is restored by the caller's reattach step (chat-list's
    // prepareSessionMessages finally block), mirrored here.
    transcript.reattachTurnToSession('bot-1', 'session-1', assistantTurn)

    expect(transcript.messages.map(turn => turn.role)).toEqual(['user', 'assistant'])
    expect(transcript.messages[0]).toMatchObject({ role: 'user', text: 'hello first' })
  })

  it('adopts carried-over optimistic turns once the snapshot contains their server twins', async () => {
    const { transcript, fetchMessages } = makeTranscript()
    // Round 1: optimistic turns survive an empty snapshot (as above).
    fetchMessages.mockResolvedValueOnce([])
    const loading = transcript.loadInitialMessages('bot-1', 'session-1')
    const assistantTurn = transcript.createOptimisticAssistantTurn()
    const userTurn = transcript.createOptimisticUserTurn('hello first')
    transcript.appendToView(userTurn, assistantTurn)
    await loading

    // Round 2 (stream-end refresh): the server now returns persisted twins —
    // no duplicates, the optimistic pair collapses into the server rows.
    // Server timestamps sit next to the optimistic client clock (the twin
    // match tolerates 5s of skew), unlike the fixed dates used elsewhere.
    const now = new Date().toISOString()
    fetchMessages.mockResolvedValueOnce([
      rawUser('server-user', 'hello first', now),
      rawAssistant('server-assistant', [], now),
    ])
    await transcript.refreshCurrentSession('bot-1', 'session-1')

    expect(transcript.messages.map(turn => turn.role)).toEqual(['user', 'assistant'])
    expect(transcript.messages.filter(turn => turn.role === 'user')).toHaveLength(1)
  })

  it('does not let an older persisted twin swallow a re-sent optimistic prompt', async () => {
    // The user sends the SAME text again in a session whose history already
    // contains that prompt. The racing snapshot returns only the OLD row —
    // the new send is not persisted yet. A text-set match would treat the
    // new optimistic turn as "already present" and drop it; the count-based
    // match reserves the old row for the old (non-optimistic) local turn.
    const { transcript, fetchMessages } = makeTranscript()
    transcript.replaceHistoryView([
      rawUser('server-user-old', 'same prompt', '2026-01-01T00:00:00.000Z'),
      rawAssistant('server-assistant-old', [], '2026-01-01T00:00:01.000Z'),
    ], 'session-1')

    const pending = deferred<UITurn[]>()
    fetchMessages.mockReturnValueOnce(pending.promise)
    const loading = transcript.loadInitialMessages('bot-1', 'session-1')
    const assistantTurn = transcript.createOptimisticAssistantTurn()
    const userTurn = transcript.createOptimisticUserTurn('same prompt')
    transcript.appendToView(userTurn, assistantTurn)

    pending.resolve([
      rawUser('server-user-old', 'same prompt', '2026-01-01T00:00:00.000Z'),
      rawAssistant('server-assistant-old', [], '2026-01-01T00:00:01.000Z'),
    ])
    await loading
    transcript.reattachTurnToSession('bot-1', 'session-1', assistantTurn)

    expect(transcript.messages.map(turn => `${turn.role}${isOptimisticTurn(turn) ? '*' : ''}`))
      .toEqual(['user', 'assistant', 'user*', 'assistant*'])
  })

  it('owns refresh state and reports the latest applied timestamp', async () => {
    const { transcript, fetchMessages } = makeTranscript()
    const onRefreshApplied = vi.fn()
    transcript.setRefreshAppliedHook(onRefreshApplied)
    fetchMessages.mockResolvedValueOnce([
      rawUser('user-1'),
      rawAssistant('assistant-1', [], '2026-01-01T00:00:02.000Z'),
    ])

    await transcript.loadInitialMessages('bot-1', 'session-1')

    expect(transcript.loadingMessages.value).toBe(false)
    expect(transcript.hasMoreOlder.value).toBe(true)
    expect(onRefreshApplied).toHaveBeenCalledWith('bot-1', 'session-1', '2026-01-01T00:00:02.000Z')
  })

  it('drops an older-page response that resolves after the active session changes', async () => {
    const { transcript, sessionId, fetchMessages } = makeTranscript()
    transcript.replaceHistoryView([rawUser('session-1-user')], 'session-1')
    const pending = deferred<UITurn[]>()
    fetchMessages.mockReturnValueOnce(pending.promise)

    const loading = transcript.loadOlderMessages()
    sessionId.value = 'session-2'
    transcript.clearHistoryView()
    transcript.replaceHistoryView([rawUser('session-2-user')], 'session-2')
    pending.resolve([rawUser('session-1-older', 'old', '2025-01-01T00:00:00.000Z')])

    expect(await loading).toBe(0)
    expect(transcript.messages.map(turn => turn.id)).toEqual(['session-2-user'])
    expect(transcript.loadingOlder.value).toBe(false)
  })

  it('drops a locate response that resolves after the active session changes', async () => {
    const { transcript, sessionId, locateMessage } = makeTranscript()
    const pending = deferred<{ items: UITurn[]; target_id?: string }>()
    locateMessage.mockReturnValueOnce(pending.promise)

    const locating = transcript.locateMessageByExternalId('external-1')
    sessionId.value = 'session-2'
    transcript.clearHistoryView()
    transcript.replaceHistoryView([rawUser('session-2-user')], 'session-2')
    pending.resolve({
      items: [{ ...rawUser('session-1-target'), external_message_id: 'external-1' } as UITurn],
      target_id: 'session-1-target',
    })

    expect(await locating).toBeNull()
    expect(transcript.messages.map(turn => turn.id)).toEqual(['session-2-user'])
    expect(transcript.hasLoadedOlder.value).toBe(false)
  })
})
