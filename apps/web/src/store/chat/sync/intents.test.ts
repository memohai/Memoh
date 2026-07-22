import { describe, expect, it, vi } from 'vitest'
import {
  createChatIntents,
  type ChatIntentWireMessage,
  type ChatIntentsDeps,
  type PendingChatIntent,
} from './intents'

function makeHarness(streamIds = ['stream-1']) {
  const wires: Array<{ botId: string; message: ChatIntentWireMessage }> = []
  const claims: PendingChatIntent[] = []
  const persisted: PendingChatIntent[] = []
  const removed: PendingChatIntent[] = []
  const restoredReplacements: PendingChatIntent[] = []
  const restoredDrafts: Parameters<ChatIntentsDeps['restoreComposerDraft']>[0][] = []
  const ids = [...streamIds]
  const deps: ChatIntentsDeps = {
    createStreamId: () => ids.shift() ?? 'stream-fallback',
    dispatch: (botId, message) => {
      wires.push({ botId, message })
      return true
    },
    createPlaceholder: request => ({
      userTurnKey: request.kind === 'send' ? `user-${request.streamId}` : undefined,
      assistantTurnKey: `assistant-${request.streamId}`,
    }),
    rollbackPlaceholder: vi.fn(),
    removeVanished: intent => removed.push(intent),
    restoreReplacement: intent => restoredReplacements.push(intent),
    restoreComposerDraft: draft => restoredDrafts.push(draft),
    onClaim: intent => claims.push(intent),
    onPersisted: intent => persisted.push(intent),
  }
  const intents = createChatIntents(deps)
  return { intents, deps, wires, claims, persisted, removed, restoredReplacements, restoredDrafts }
}

const target = {
  botId: 'bot-1',
  sessionId: 'session-1',
  viewId: 'chat:1',
  composerScope: 'bot-1:chat:1',
}

describe('chat intents', () => {
  it('registers a send before dispatching its optimistic stream identity', () => {
    const { intents, wires } = makeHarness()

    const result = intents.send({ ...target, text: 'hello', requestedSkills: [{ name: 'review', display_name: 'Review' }] })

    expect(result).toMatchObject({
      ok: true,
      intent: {
        kind: 'send',
        streamId: 'stream-1',
        userTurnKey: 'user-stream-1',
        assistantTurnKey: 'assistant-stream-1',
      },
    })
    expect(wires).toEqual([{
      botId: 'bot-1',
      message: expect.objectContaining({
        type: 'message',
        stream_id: 'stream-1',
        invocation_id: 'stream-1',
        session_id: 'session-1',
        requested_skills: [{ name: 'review' }],
      }),
    }])
  })

  it('binds one generation and never claims the same stream from another session', () => {
    const { intents, claims } = makeHarness()
    intents.send({ ...target, text: 'hello' })

    expect(intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-1' })).toMatchObject({
      generation: 'generation-1',
    })
    expect(intents.claimRun({
      botId: 'bot-1',
      sessionId: 'session-2',
      streamId: 'stream-1',
      generation: 'generation-1',
    })).toBeNull()
    expect(intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-2' })).toBeNull()
    expect(claims).toHaveLength(1)
  })

  it('does not let a terminal generation claim a reused stream placeholder', () => {
    const { intents } = makeHarness(['shared-stream', 'shared-stream'])
    intents.send({ ...target, text: 'old request' })
    intents.terminal({
      ...target,
      streamId: 'shared-stream',
      generation: 'generation-old',
      status: 'completed',
      persistence: 'persisted',
    })
    intents.send({ ...target, text: 'new request' })

    expect(intents.claimRun({
      ...target,
      streamId: 'shared-stream',
      generation: 'generation-old',
    })).toBeNull()
    expect(intents.claimRun({
      ...target,
      streamId: 'shared-stream',
      generation: 'generation-new',
    })).toMatchObject({ text: 'new request', generation: 'generation-new' })
  })

  it('keeps strict abort fenced while allowing one explicit pre-admission cancellation', () => {
    const { intents, wires } = makeHarness()
    intents.send({ ...target, text: 'hello' })

    expect(intents.abortRun({ ...target, streamId: 'stream-1', generation: '' })).toBe(false)
    expect(intents.cancelPendingSend('bot-1', 'session-1', 'stream-1')).toBe(true)
    expect(intents.cancelPendingSend('bot-1', 'session-1', 'stream-1')).toBe(true)
    expect(wires.filter(wire => wire.message.type === 'abort')).toEqual([{
      botId: 'bot-1',
      message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1' },
    }])

    intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-1' })
    expect(wires.filter(wire => wire.message.type === 'abort')).toEqual([
      { botId: 'bot-1', message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1' } },
      { botId: 'bot-1', message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1', generation: 'generation-1' } },
    ])
  })

  it.each(['return false', 'throw'] as const)(
    'does not queue a bound pre-admission abort when dispatch chooses to %s',
    (failureMode) => {
      const { intents, deps, wires } = makeHarness()
      intents.send({ ...target, text: 'hello' })
      deps.dispatch = (botId, message) => {
        wires.push({ botId, message })
        if (message.type !== 'abort') return true
        if (failureMode === 'throw') throw new Error('transport closed')
        return false
      }

      expect(intents.cancelPendingSend('bot-1', 'session-1', 'stream-1')).toBe(false)
      expect(intents.get('bot-1', 'session-1', 'stream-1')).toMatchObject({
        abortRequested: false,
        cancelSent: false,
        abortSentGeneration: '',
      })

      intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-1' })
      expect(wires.filter(wire => wire.message.type === 'abort')).toHaveLength(1)
    },
  )

  it('does not replay a requested abort when the terminal checkpoint first supplies generation', () => {
    const { intents, wires } = makeHarness()
    intents.send({ ...target, text: 'hello' })
    intents.cancelPendingSend('bot-1', 'session-1', 'stream-1')

    intents.terminal({
      ...target,
      streamId: 'stream-1',
      generation: 'generation-1',
      status: 'aborted',
      persistence: 'vanished',
    })

    expect(wires.filter(wire => wire.message.type === 'abort')).toEqual([{
      botId: 'bot-1',
      message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1' },
    }])
  })

  it('replays a generation-fenced abort after its bot transport closes', () => {
    const { intents, wires } = makeHarness()
    const handle = { ...target, streamId: 'stream-1', generation: 'generation-1' }
    intents.send({ ...target, text: 'hello' })
    intents.claimRun(handle)

    expect(intents.abortRun(handle)).toBe(true)
    expect(intents.abortRun(handle)).toBe(true)
    expect(wires.filter(wire => wire.message.type === 'abort')).toHaveLength(1)

    intents.handleTransportClose('other-bot')
    intents.claimRun(handle)
    expect(wires.filter(wire => wire.message.type === 'abort')).toHaveLength(1)

    intents.handleTransportClose('bot-1')
    expect(intents.get('bot-1', 'session-1', 'stream-1')).toMatchObject({
      abortRequested: true,
      abortSentGeneration: '',
    })
    intents.claimRun(handle)

    expect(wires.filter(wire => wire.message.type === 'abort')).toEqual([
      { botId: 'bot-1', message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1', generation: 'generation-1' } },
      { botId: 'bot-1', message: { type: 'abort', stream_id: 'stream-1', session_id: 'session-1', generation: 'generation-1' } },
    ])
  })

  it('clears abort replay state when the server rejects that generation', () => {
    const { intents, wires } = makeHarness()
    const handle = { ...target, streamId: 'stream-1', generation: 'generation-1' }
    intents.send({ ...target, text: 'hello' })
    intents.claimRun(handle)
    intents.abortRun(handle)

    intents.rejectAbort({ ...handle, generation: 'stale-generation' })
    expect(intents.get('bot-1', 'session-1', 'stream-1')).toMatchObject({
      abortRequested: true,
      abortSentGeneration: 'generation-1',
    })

    intents.rejectAbort(handle)
    expect(intents.get('bot-1', 'session-1', 'stream-1')).toMatchObject({
      abortRequested: false,
      abortSentGeneration: '',
    })

    intents.handleTransportClose('bot-1')
    intents.claimRun(handle)
    expect(wires.filter(wire => wire.message.type === 'abort')).toHaveLength(1)
  })

  it('rebinds a deferred draft before a runtime generation can claim it', () => {
    const { intents } = makeHarness()
    intents.send({ ...target, sessionId: '', text: 'hello' })

    expect(intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-1' })).toBeNull()
    expect(intents.rebindSession('bot-1', 'stream-1', 'session-1')).toMatchObject({ sessionId: 'session-1' })
    expect(intents.claimRun({ ...target, streamId: 'stream-1', generation: 'generation-1' })).toMatchObject({
      generation: 'generation-1',
    })
  })

  it('rolls back and restores a send that fails before runtime admission', () => {
    const { intents, deps, restoredDrafts } = makeHarness()
    intents.send({ ...target, text: 'restore before admission' })

    expect(intents.failPending('bot-1', 'session-1', 'stream-1')).toMatchObject({
      kind: 'send',
      generation: '',
    })
    expect(deps.rollbackPlaceholder).toHaveBeenCalledOnce()
    expect(restoredDrafts).toEqual([expect.objectContaining({ text: 'restore before admission' })])
    expect(intents.get('bot-1', 'session-1', 'stream-1')).toBeNull()
  })

  it('sends steer only with a complete generation-fenced handle', () => {
    const { intents, wires } = makeHarness()

    expect(intents.steer({ ...target, streamId: 'stream-1', generation: '' }, 'adjust')).toBe(false)
    expect(intents.steer({ ...target, streamId: 'stream-1', generation: 'generation-1' }, ' adjust ')).toBe(true)
    expect(wires).toEqual([{
      botId: 'bot-1',
      message: {
        type: 'steer_current_run',
        stream_id: 'stream-1',
        session_id: 'session-1',
        generation: 'generation-1',
        text: 'adjust',
      },
    }])
  })

  it('hands persisted terminal state to settle without restoring a draft', () => {
    const { intents, persisted, restoredDrafts } = makeHarness()
    intents.send({ ...target, text: 'hello' })

    expect(intents.terminal({
      ...target,
      streamId: 'stream-1',
      generation: 'generation-1',
      status: 'completed',
      persistence: 'persisted',
      stableIds: ['assistant-row'],
    })).toMatchObject({ kind: 'send', generation: 'generation-1' })
    expect(persisted).toHaveLength(1)
    expect(restoredDrafts).toEqual([])
    expect(intents.get('bot-1', 'session-1', 'stream-1')).toBeNull()
  })

  it('removes a vanished send and restores its owning composer draft', () => {
    const { intents, removed, restoredDrafts } = makeHarness()
    intents.send({
      ...target,
      text: 'restore me',
      attachments: [{ type: 'file', base64: 'data', name: 'note.txt' }],
      requestedSkills: [{ name: 'review' }],
    })

    intents.terminal({
      ...target,
      streamId: 'stream-1',
      generation: 'generation-1',
      status: 'aborted',
      persistence: 'vanished',
    })

    expect(removed).toHaveLength(1)
    expect(restoredDrafts).toEqual([expect.objectContaining({
      botId: 'bot-1',
      sessionId: 'session-1',
      viewId: 'chat:1',
      composerScope: 'bot-1:chat:1',
      text: 'restore me',
    })])
  })

  it('restores the replaced tail instead of the composer when retry vanishes', () => {
    const { intents, restoredDrafts, restoredReplacements } = makeHarness()
    intents.retry({ ...target, messageId: 'assistant-old' })

    intents.terminal({
      ...target,
      streamId: 'stream-1',
      generation: 'generation-1',
      status: 'aborted',
      persistence: 'vanished',
    })

    expect(restoredReplacements).toHaveLength(1)
    expect(restoredReplacements[0]).toMatchObject({ kind: 'retry', replaceFromMessageId: 'assistant-old' })
    expect(restoredDrafts).toEqual([])
  })
})
