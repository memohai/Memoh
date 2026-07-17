import { describe, expect, it, vi } from 'vitest'
import type { UIAssistantTurn, UIMessage, UITurn } from '@/composables/api/useChat.types'
import type { ChatAssistantTurn } from '../types'
import {
  findPersistedRuntimeMatch,
  findSettledAssistant,
  settleRuntimeTerminal,
  type RuntimeTerminalSettleDependencies,
  type RuntimeTerminalStatus,
} from './settle'

const settled: ChatAssistantTurn = {
  id: 'render-key',
  serverId: 'assistant-row',
  role: 'assistant',
  messages: [{ id: 0, stable_id: 'assistant-row', turn_position: 7, turn_message_seq: 2, type: 'text', content: 'answer' }],
  timestamp: '2026-07-16T00:00:00Z',
  streaming: false,
}

function textRow(
  stableId: string | undefined,
  turnPosition: number | undefined,
  turnMessageSeq: number | undefined,
  content: string,
): UIMessage {
  return {
    id: 0,
    stable_id: stableId,
    turn_position: turnPosition,
    turn_message_seq: turnMessageSeq,
    type: 'text',
    content,
  }
}

function assistantTurn(id: string, messages: UIMessage[]): UIAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages,
    timestamp: '2026-07-16T00:00:00Z',
  }
}

function dependencies(history: UITurn[], calls: string[] = []): RuntimeTerminalSettleDependencies {
  return {
    fetchHistory: vi.fn(async () => {
      calls.push('fetch')
      return history
    }),
    replacePersisted: vi.fn(async () => {
      calls.push('replace')
    }),
    removeOptimistic: vi.fn(async () => {
      calls.push('remove')
    }),
    restoreDraft: vi.fn(async () => {
      calls.push('restore')
    }),
    keepPartial: vi.fn(async () => {
      calls.push('keep')
    }),
  }
}

describe('runtime settle', () => {
  it('matches runtime and history only by stable row identity', () => {
    expect(findSettledAssistant([settled], [{
      id: 7,
      stable_id: 'assistant-row',
      turn_position: 7,
      turn_message_seq: 2,
      type: 'text',
      content: 'different',
    }])).toBe(settled)
  })

  it('does not fall back to content or transcript position', () => {
    expect(findSettledAssistant([settled], [{ id: 7, type: 'text', content: 'answer' }])).toBeNull()
  })

  it('matches persisted rows only when stable id and both sequence coordinates agree', () => {
    const runtime = textRow('row-1', 7, 2, 'runtime')
    const exact = assistantTurn('turn-exact', [textRow('row-1', 7, 2, 'postgres')])
    const history: UITurn[] = [
      assistantTurn('wrong-position', [textRow('row-1', 8, 2, 'runtime')]),
      assistantTurn('wrong-sequence', [textRow('row-1', 7, 3, 'runtime')]),
      assistantTurn('wrong-id', [textRow('row-2', 7, 2, 'runtime')]),
      exact,
    ]

    const match = findPersistedRuntimeMatch(history, [runtime])

    expect(match?.turn).toBe(exact)
    expect(match?.rows).toEqual([{
      identity: { stableId: 'row-1', turnPosition: 7, turnMessageSeq: 2 },
      runtime,
      persisted: exact.messages[0],
    }])
  })

  it('does not infer a persisted row from matching text or missing coordinates', () => {
    const history = [assistantTurn('persisted', [textRow('row-1', 7, 2, 'same text')])]

    expect(findPersistedRuntimeMatch(history, [textRow(undefined, 7, 2, 'same text')])).toBeNull()
    expect(findPersistedRuntimeMatch(history, [textRow('row-1', undefined, 2, 'same text')])).toBeNull()
    expect(findPersistedRuntimeMatch(history, [textRow('row-1', 7, undefined, 'same text')])).toBeNull()
  })

  it('deduplicates repeated blocks that name the same persisted row', () => {
    const runtime = textRow('row-1', 7, 2, 'runtime')
    const persisted = assistantTurn('persisted', [
      textRow('row-1', 7, 2, 'postgres-a'),
      textRow('row-1', 7, 2, 'postgres-b'),
    ])

    expect(findPersistedRuntimeMatch([persisted], [runtime])?.rows).toHaveLength(1)
  })

  it('requires every row in an aggregated runtime block to exist in one persisted turn', () => {
    const runtime = {
      ...textRow('assistant-row', 7, 2, 'runtime tool'),
      row_identities: [
        { stable_id: 'assistant-row', role: 'assistant', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 2 },
        { stable_id: 'tool-row', role: 'tool', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 3 },
      ],
    } satisfies UIMessage
    const complete = assistantTurn('complete', [{
      ...textRow('assistant-row', 7, 2, 'persisted tool'),
      row_identities: [
        { stable_id: 'assistant-row', role: 'assistant', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 2 },
        { stable_id: 'tool-row', role: 'tool', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 3 },
      ],
    }])
    const truncated = assistantTurn('truncated', [textRow('assistant-row', 7, 2, 'partial')])

    expect(findPersistedRuntimeMatch([truncated], [runtime])).toBeNull()
    expect(findPersistedRuntimeMatch([truncated, complete], [runtime])?.turn).toBe(complete)
  })

  it('requires invisible user rows from the runtime ledger to exist in history', () => {
    const runtime = textRow('assistant-row', 7, 2, 'runtime answer')
    const persisted = assistantTurn('assistant-row', [textRow('assistant-row', 7, 2, 'persisted answer')])
    const injected: UITurn = {
      id: 'injected-row',
      role: 'user',
      text: 'change direction',
      timestamp: '2026-07-16T00:00:00Z',
      turn_position: 7,
      turn_message_seq: 3,
    }
    const ledger = [
      { stable_id: 'assistant-row', role: 'assistant', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 2 },
      { stable_id: 'injected-row', role: 'user', turn_id: 'turn-7', turn_position: 7, turn_message_seq: 3 },
    ]

    expect(findPersistedRuntimeMatch([persisted], [runtime], ledger)).toBeNull()
    const match = findPersistedRuntimeMatch([persisted, injected], [runtime], ledger)
    expect(match?.turn).toBe(persisted)
    expect(match?.ledger.map(row => row.stableId)).toEqual(['assistant-row', 'injected-row'])
  })

  it.each<RuntimeTerminalStatus>(['completed', 'aborted'])('lets Postgres replace the %s runtime projection when the row persisted', async (status) => {
    const runtime = textRow('row-1', 7, 2, 'runtime version')
    const persisted = assistantTurn('persisted', [textRow('row-1', 7, 2, 'postgres version')])
    const calls: string[] = []
    const deps = dependencies([persisted], calls)

    const result = await settleRuntimeTerminal({ status, runtimeMessages: [runtime] }, deps)

    expect(result).toMatchObject({ status, persistence: 'persisted', action: 'replace_persisted' })
    expect(result.persistence === 'persisted' && result.match.turn).toBe(persisted)
    expect(result.persistence === 'persisted' && result.match.rows[0]?.persisted).toBe(persisted.messages[0])
    expect(calls).toEqual(['fetch', 'replace'])
    expect(deps.removeOptimistic).not.toHaveBeenCalled()
    expect(deps.restoreDraft).not.toHaveBeenCalled()
    expect(deps.keepPartial).not.toHaveBeenCalled()
  })

  it('uses the Postgres turn as the sole winner when T0 history overlaps the T1 runtime row', async () => {
    const runtime = textRow('row-1', 7, 2, 'new live content')
    const sameTextWrongRow = assistantTurn('unrelated', [textRow('other-row', 6, 1, 'new live content')])
    const persisted = assistantTurn('persisted', [textRow('row-1', 7, 2, 'committed content')])
    const deps = dependencies([sameTextWrongRow, persisted])

    await settleRuntimeTerminal({ status: 'completed', runtimeMessages: [runtime] }, deps)

    expect(deps.replacePersisted).toHaveBeenCalledOnce()
    const match = vi.mocked(deps.replacePersisted).mock.calls[0]![0]
    expect(match.turn).toBe(persisted)
    expect(match.history).toEqual([sameTextWrongRow, persisted])
  })

  it('removes vanished optimistic turns and restores the draft after abort', async () => {
    const calls: string[] = []
    const deps = dependencies([], calls)

    const result = await settleRuntimeTerminal({
      status: 'aborted',
      runtimeMessages: [textRow('row-1', 7, 2, 'partial')],
    }, deps)

    expect(result).toEqual({
      status: 'aborted',
      persistence: 'vanished',
      action: 'remove_optimistic_restore_draft',
    })
    expect(calls).toEqual(['fetch', 'remove', 'restore'])
    expect(deps.replacePersisted).not.toHaveBeenCalled()
    expect(deps.keepPartial).not.toHaveBeenCalled()
  })

  it('keeps errored partial output when no durable row exists', async () => {
    const calls: string[] = []
    const deps = dependencies([], calls)

    const result = await settleRuntimeTerminal({
      status: 'errored',
      runtimeMessages: [textRow('row-1', 7, 2, 'partial')],
    }, deps)

    expect(result).toEqual({
      status: 'errored',
      persistence: 'vanished',
      action: 'keep_partial',
    })
    expect(calls).toEqual(['fetch', 'keep'])
    expect(deps.replacePersisted).not.toHaveBeenCalled()
    expect(deps.removeOptimistic).not.toHaveBeenCalled()
    expect(deps.restoreDraft).not.toHaveBeenCalled()
    expect(deps.keepPartial).toHaveBeenCalledWith('errored')
  })

  it('keeps completed output if the persistence contract is unexpectedly missing', async () => {
    const deps = dependencies([])

    const result = await settleRuntimeTerminal({
      status: 'completed',
      runtimeMessages: [textRow('row-1', 7, 2, 'answer')],
    }, deps)

    expect(result).toMatchObject({ persistence: 'vanished', action: 'keep_partial' })
    expect(deps.keepPartial).toHaveBeenCalledWith('completed')
  })

  it('propagates history failures without mutating the transcript', async () => {
    const deps = dependencies([])
    vi.mocked(deps.fetchHistory).mockRejectedValueOnce(new Error('history unavailable'))

    await expect(settleRuntimeTerminal({
      status: 'errored',
      runtimeMessages: [textRow('row-1', 7, 2, 'partial')],
    }, deps)).rejects.toThrow('history unavailable')

    expect(deps.replacePersisted).not.toHaveBeenCalled()
    expect(deps.removeOptimistic).not.toHaveBeenCalled()
    expect(deps.restoreDraft).not.toHaveBeenCalled()
    expect(deps.keepPartial).not.toHaveBeenCalled()
  })
})
