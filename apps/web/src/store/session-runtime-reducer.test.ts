import { describe, expect, it } from 'vitest'
import type { SessionruntimeSnapshot } from '@memohai/sdk'
import {
  reduceSessionRuntimeDelta,
  reduceSessionRuntimeSnapshot,
  type SessionRuntimeStateEvent,
  type SessionRuntimeReducerState,
} from '@memohai/sdk/session-runtime'
import {
  parseRuntimeContract,
  richActiveRunContractFixture as runtimeStateMachineContractFixture,
  runtimeRecoveryContractFixture,
} from './runtime-contract-fixtures.test-support'

function reduceContractStream(events: SessionRuntimeStateEvent[]): SessionRuntimeReducerState {
  let state: SessionRuntimeReducerState = {}
  for (const event of structuredClone(events)) {
    const reduction = event.type === 'runtime_snapshot'
      ? reduceSessionRuntimeSnapshot(state, event.snapshot, event.seq, event.epoch)
      : event.type === 'runtime_delta'
        ? reduceSessionRuntimeDelta(state, event, event.bot_id, event.session_id)
        : { kind: 'ignored' as const, state }
    expect(reduction.kind).toBe('applied')
    state = reduction.state
  }
  return state
}

function runningSnapshot(seq = 1, epoch = ''): SessionruntimeSnapshot {
  return {
    bot_id: 'bot-1',
    session_id: 'session-1',
    ...(epoch ? { epoch } : {}),
    seq,
    queue: [],
    current_run_view: {
      stream_id: 'stream-1',
      generation: 'generation-1',
      status: 'running',
      messages: [],
      row_ledger: [],
    },
  }
}

describe('session runtime reducer', () => {
  it('replays the Go-generated gap, delayed delta, checkpoint, and recovery delta', () => {
    const fixture = structuredClone(runtimeRecoveryContractFixture)
    const baseline = reduceSessionRuntimeSnapshot(
      {},
      fixture.runtime_snapshot.snapshot,
      fixture.runtime_snapshot.seq,
      fixture.runtime_snapshot.epoch,
    )
    expect(baseline).toMatchObject({ kind: 'applied', state: { phase: 'live', seq: fixture.runtime_snapshot.seq } })

    const gap = reduceSessionRuntimeDelta(
      baseline.state,
      fixture.gap_delta,
      fixture.gap_delta.bot_id,
      fixture.gap_delta.session_id,
    )
    expect(gap).toMatchObject({ kind: 'resync', reason: 'sequence_gap', state: { phase: 'awaiting_checkpoint' } })

    const delayed = reduceSessionRuntimeDelta(
      gap.state,
      fixture.delayed_delta,
      fixture.delayed_delta.bot_id,
      fixture.delayed_delta.session_id,
    )
    expect(delayed).toMatchObject({ kind: 'ignored', state: { phase: 'awaiting_checkpoint' } })

    const checkpoint = reduceSessionRuntimeSnapshot(
      delayed.state,
      fixture.runtime_checkpoint.snapshot,
      fixture.runtime_checkpoint.seq,
      fixture.runtime_checkpoint.epoch,
    )
    expect(checkpoint).toMatchObject({
      kind: 'applied',
      state: {
        phase: 'live',
        seq: fixture.runtime_checkpoint.seq,
        snapshot: { current_run_view: { messages: [{ type: 'text', content: 'missing checkpoint' }] } },
      },
    })

    const recovered = reduceSessionRuntimeDelta(
      checkpoint.state,
      fixture.post_recovery_delta,
      fixture.post_recovery_delta.bot_id,
      fixture.post_recovery_delta.session_id,
    )
    expect(recovered).toMatchObject({
      kind: 'applied',
      state: {
        phase: 'live',
        seq: fixture.post_recovery_delta.seq,
        snapshot: { current_run_view: { messages: [{ type: 'text', content: 'missing checkpoint continued' }] } },
      },
    })
  })

  it('replays Go-generated admission, reset, and steer state machines', () => {
    const admission = reduceContractStream(runtimeStateMachineContractFixture.runtime_admission_stream ?? [])
    expect(admission.snapshot?.current_run_view).toMatchObject({
      stream_id: 'stream-admission',
      status: 'running',
      request_user_turn: { text: 'Inspect the workspace' },
    })

    const reset = reduceContractStream(runtimeStateMachineContractFixture.runtime_reset_stream ?? [])
    expect(reset.snapshot?.current_run_view?.messages).toEqual([
      { id: 1, type: 'text', content: 'replacement draft' },
    ])

    const steer = reduceContractStream(runtimeStateMachineContractFixture.runtime_steer_stream ?? [])
    expect(steer.snapshot?.current_run_view?.steer).toMatchObject({
      id: 'steer-runtime-contract',
      status: 'applied',
      text: 'adjust course',
    })
  })

  it('accepts a lower sequence only when the runtime epoch changes', () => {
    const current = reduceSessionRuntimeSnapshot({}, runningSnapshot(100, 'epoch-1'), 100, 'epoch-1')
    expect(current.kind).toBe('applied')

    const reset = reduceSessionRuntimeSnapshot(current.state, runningSnapshot(1, 'epoch-2'), 1, 'epoch-2')
    expect(reset).toMatchObject({ kind: 'applied', state: { epoch: 'epoch-2', seq: 1 } })
  })

  it('ignores a lower authoritative snapshot from the same epoch', () => {
    const current = reduceSessionRuntimeSnapshot({}, runningSnapshot(100), 100, 'epoch-1')
    const stale = reduceSessionRuntimeSnapshot(current.state, runningSnapshot(1), 1, 'epoch-1')

    expect(stale).toMatchObject({ kind: 'ignored', state: { epoch: 'epoch-1', seq: 100 } })
  })

  it('reapplies an equal-sequence authoritative snapshot for the same runtime identity', () => {
    const initial = runningSnapshot(10, 'epoch-1')
    initial.current_run_view!.messages = [{ id: 0, type: 'text', content: 'pending' }]
    const current = reduceSessionRuntimeSnapshot({}, initial, 10, 'epoch-1')
    const refreshed = runningSnapshot(10, 'epoch-1')
    refreshed.current_run_view!.messages = [{ id: 0, type: 'text', content: 'server truth' }]

    const result = reduceSessionRuntimeSnapshot(current.state, refreshed, 10, 'epoch-1')

    expect(result).toMatchObject({
      kind: 'applied',
      state: {
        epoch: 'epoch-1',
        seq: 10,
        snapshot: { current_run_view: { messages: [{ content: 'server truth' }] } },
      },
    })
  })

  it('accepts an equal-sequence checkpoint that changes runtime identity', () => {
    const current = reduceSessionRuntimeSnapshot({}, runningSnapshot(10, 'epoch-1'), 10, 'epoch-1')
    const differentStream = runningSnapshot(10, 'epoch-1')
    differentStream.current_run_view!.stream_id = 'stream-2'
    const differentGeneration = runningSnapshot(10, 'epoch-1')
    differentGeneration.current_run_view!.generation = 'generation-2'

    expect(reduceSessionRuntimeSnapshot(current.state, differentStream, 10, 'epoch-1')).toMatchObject({
      kind: 'applied',
      state: { seq: 10, snapshot: { current_run_view: { stream_id: 'stream-2' } } },
    })

    expect(reduceSessionRuntimeSnapshot(current.state, differentGeneration, 10, 'epoch-1')).toMatchObject({
      kind: 'applied',
      state: { seq: 10, snapshot: { current_run_view: { generation: 'generation-2' } } },
    })
  })

  it('rejects snapshot target changes across sequence and epoch transitions', () => {
    const current = reduceSessionRuntimeSnapshot({}, runningSnapshot(1, 'epoch-1'), 1, 'epoch-1')
    const higherSequence = runningSnapshot(2, 'epoch-1')
    higherSequence.bot_id = 'bot-2'
    const newEpoch = runningSnapshot(1, 'epoch-2')
    newEpoch.session_id = 'session-2'

    expect(reduceSessionRuntimeSnapshot(current.state, higherSequence, 2, 'epoch-1')).toMatchObject({
      kind: 'resync',
      reason: 'stream_mismatch',
    })
    expect(reduceSessionRuntimeSnapshot(current.state, newEpoch, 1, 'epoch-2')).toMatchObject({
      kind: 'resync',
      reason: 'stream_mismatch',
    })
  })

  it('ignores stale snapshots and deltas', () => {
    const state: SessionRuntimeReducerState = { snapshot: runningSnapshot(5, 'epoch-1'), epoch: 'epoch-1', seq: 5, phase: 'live' }
    expect(reduceSessionRuntimeSnapshot(state, runningSnapshot(4, 'epoch-1'), 4, 'epoch-1')).toMatchObject({ kind: 'ignored' })
    expect(reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 5,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'old' }] },
    }, 'bot-1', 'session-1')).toMatchObject({ kind: 'ignored' })
  })

  it('requests resync for sequence gaps without mutating the current snapshot', () => {
    const snapshot = runningSnapshot(1)
    const state: SessionRuntimeReducerState = { snapshot, seq: 1 }
    const result = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 3,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'gap' }] },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason: 'sequence_gap' })
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it('preserves the accepted sequence while awaiting a checkpoint and ignores delayed deltas', () => {
    const snapshot = runningSnapshot(10, 'epoch-1')
    const gap = reduceSessionRuntimeDelta({ snapshot, epoch: 'epoch-1', seq: 10 }, {
      type: 'runtime_delta',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 12,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'gap' }] },
    }, 'bot-1', 'session-1')

    expect(gap).toMatchObject({ kind: 'resync', reason: 'sequence_gap', state: { seq: 10, phase: 'awaiting_checkpoint' } })
    expect(reduceSessionRuntimeDelta(gap.state, {
      type: 'runtime_delta',
      epoch: 'epoch-other',
      stream_id: 'stream-1',
      seq: 11,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'delayed' }] },
    }, 'bot-1', 'session-1')).toMatchObject({ kind: 'ignored', state: { seq: 10 } })
  })

  it('requests resync for a delta from a different runtime epoch', () => {
    const snapshot = runningSnapshot(5, 'epoch-current')
    const result = reduceSessionRuntimeDelta({ snapshot, epoch: 'epoch-current', seq: 5 }, {
      type: 'runtime_delta',
      epoch: 'epoch-stale',
      stream_id: 'stream-1',
      seq: 6,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'stale' }] },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason: 'epoch_mismatch', state: { epoch: 'epoch-current' } })
    expect(result.state.seq).toBe(5)
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it('requests resync when an established epoch is missing from a snapshot or delta', () => {
    const snapshot = runningSnapshot(5, 'epoch-current')
    const state: SessionRuntimeReducerState = { snapshot, epoch: 'epoch-current', seq: 5 }

    const snapshotResult = reduceSessionRuntimeSnapshot(state, runningSnapshot(6), 6)
    expect(snapshotResult).toMatchObject({
      kind: 'resync',
      reason: 'epoch_mismatch',
      state: { epoch: 'epoch-current', snapshot },
    })
    expect(snapshotResult.state.seq).toBe(5)

    const deltaResult = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 6,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'epochless' }] },
    }, 'bot-1', 'session-1')
    expect(deltaResult).toMatchObject({
      kind: 'resync',
      reason: 'epoch_mismatch',
      state: { epoch: 'epoch-current', snapshot },
    })
    expect(deltaResult.state.seq).toBe(5)
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it('requests resync when the snapshot envelope and payload epochs conflict', () => {
    const current = reduceSessionRuntimeSnapshot({}, runningSnapshot(1, 'epoch-current'), 1, 'epoch-current')
    const result = reduceSessionRuntimeSnapshot(current.state, runningSnapshot(2, 'epoch-payload'), 2, 'epoch-envelope')

    expect(result).toMatchObject({
      kind: 'resync',
      reason: 'epoch_mismatch',
      state: { epoch: 'epoch-current' },
    })
  })

  it('requires checkpoints to establish an epoch', () => {
    const initial = reduceSessionRuntimeSnapshot({}, runningSnapshot(1), 1)
    expect(initial).toMatchObject({ kind: 'resync', reason: 'epoch_mismatch', state: { phase: 'awaiting_checkpoint' } })
  })

  it('applies compact message, progress, upsert, and run patches', () => {
    let state: SessionRuntimeReducerState = { snapshot: runningSnapshot(1), seq: 1 }
    const append = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'hello' }] },
    }, 'bot-1', 'session-1')
    expect(append.kind).toBe('applied')
    state = append.state

    const tool = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 3,
      delta: { message_upserts: [{ id: 1, type: 'tool', name: 'exec', tool_call_id: 'call-1' }] },
    }, 'bot-1', 'session-1')
    expect(tool.kind).toBe('applied')
    state = tool.state

    const finish = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 4,
      delta: {
        progress_appends: [{ id: 1, progress: 'done' }],
        run: { stream_id: 'stream-1', status: 'completed', history_committed: true, canonical_ready: true },
      },
    }, 'bot-1', 'session-1')

    expect(finish).toMatchObject({
      kind: 'applied',
      state: {
        seq: 4,
        snapshot: {
          current_run_view: {
            status: 'completed',
            history_committed: true,
            canonical_ready: true,
            messages: [
              { id: 0, type: 'text', content: 'hello' },
              { id: 1, type: 'tool', progress: ['done'] },
            ],
          },
        },
      },
    })
  })

  it('replaces tool upserts by tool call identity when the block id changes', () => {
    const snapshot = runningSnapshot(1)
    snapshot.current_run_view!.messages = [
      { id: 1, type: 'tool', name: 'exec', tool_call_id: 'call-1', progress: ['running'] },
    ]
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: {
        message_upserts: [
          { id: 9, type: 'tool', name: 'exec', tool_call_id: 'call-1', progress: ['done'] },
        ],
      },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({
      kind: 'applied',
      state: {
        snapshot: {
          current_run_view: {
            messages: [
              { id: 9, type: 'tool', tool_call_id: 'call-1', progress: ['done'] },
            ],
          },
        },
      },
    })
  })

  it('uses structural sharing without mutating the previous snapshot', () => {
    const snapshot = runningSnapshot(1)
    snapshot.current_run_view!.messages = [
      { id: 0, type: 'text', content: 'hello' },
      { id: 1, type: 'reasoning', content: 'stable' },
    ]
    const stableMessage = snapshot.current_run_view!.messages[1]
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: ' world' }] },
    }, 'bot-1', 'session-1')

    expect(result.kind).toBe('applied')
    expect(snapshot.current_run_view?.messages[0]?.content).toBe('hello')
    expect(result.state.snapshot?.current_run_view?.messages?.[0]?.content).toBe('hello world')
    expect(result.state.snapshot?.current_run_view?.messages?.[1]).toBe(stableMessage)
  })

  it.each([
    {
      reason: 'stream_mismatch',
      delta: { run: { stream_id: 'another-stream', status: 'completed' } },
    },
    {
      reason: 'missing_progress_target',
      delta: { progress_appends: [{ id: 99, progress: 'missing' }] },
    },
  ])('requests resync for $reason without advancing the sequence', ({ reason, delta }) => {
    const snapshot = runningSnapshot(1)
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta,
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason, state: { snapshot } })
    expect(result.state.seq).toBe(1)
  })

  it('rejects message deltas whose top-level stream does not own the current run', () => {
    const snapshot = runningSnapshot(1)
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'another-stream',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'wrong run' }] },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason: 'stream_mismatch' })
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it('requests resync instead of throwing when a wire delta omits its stream id', () => {
    const snapshot = runningSnapshot(1)
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: undefined as unknown as string,
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'unowned' }] },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason: 'stream_mismatch' })
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it('rejects malformed snapshot and delta payloads without poisoning current state', () => {
    const snapshot = runningSnapshot(1)
    const state: SessionRuntimeReducerState = { snapshot, seq: 1 }
    const malformedSnapshot = runningSnapshot(2)
    malformedSnapshot.current_run_view!.messages = [null] as unknown as NonNullable<typeof malformedSnapshot.current_run_view>['messages']

    const snapshotResult = reduceSessionRuntimeSnapshot(state, malformedSnapshot, 2)
    expect(snapshotResult).toMatchObject({ kind: 'resync', reason: 'invalid_payload', state: { snapshot } })
    expect(snapshotResult.state.seq).toBe(1)

    const deltaResult = reduceSessionRuntimeDelta(state, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { message_appends: [null] } as unknown as Parameters<typeof reduceSessionRuntimeDelta>[1]['delta'],
    }, 'bot-1', 'session-1')
    expect(deltaResult).toMatchObject({ kind: 'resync', reason: 'invalid_payload', state: { snapshot } })
    expect(deltaResult.state.seq).toBe(1)
    expect(snapshot.current_run_view?.messages).toEqual([])
  })

  it.each([-1, 1.5, Number.MAX_SAFE_INTEGER + 1])('rejects invalid runtime sequence %s', (seq) => {
    const snapshot = runningSnapshot(1)
    const malformedSnapshot = runningSnapshot(1)
    malformedSnapshot.seq = seq

    expect(reduceSessionRuntimeSnapshot({}, malformedSnapshot, seq)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_seq',
    })
    expect(reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'invalid' }] },
    }, 'bot-1', 'session-1')).toMatchObject({ kind: 'resync', reason: 'invalid_seq' })
  })

  it('rejects conflicting snapshot envelope and payload sequences', () => {
    expect(reduceSessionRuntimeSnapshot({}, runningSnapshot(2), 3)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_seq',
    })
  })

  it('requires a snapshot payload sequence even when the envelope provides one', () => {
    const missingPayloadSequence = runningSnapshot(1)
    delete missingPayloadSequence.seq

    expect(reduceSessionRuntimeSnapshot({}, missingPayloadSequence, undefined)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_seq',
    })
    expect(reduceSessionRuntimeSnapshot({}, missingPayloadSequence, 1)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_seq',
    })
    expect(reduceSessionRuntimeSnapshot({}, runningSnapshot(1, 'epoch-1'), undefined, 'epoch-1')).toMatchObject({
      kind: 'applied',
      state: { seq: 1 },
    })
  })

  it('rejects snapshots with a missing runtime target', () => {
    const missingBot = runningSnapshot(1)
    missingBot.bot_id = '  '
    const missingSession = runningSnapshot(1)
    missingSession.session_id = undefined

    expect(reduceSessionRuntimeSnapshot({}, missingBot, 1)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_payload',
    })
    expect(reduceSessionRuntimeSnapshot({}, missingSession, 1)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_payload',
    })
  })

  it('requests resync when an active run omits its generation', () => {
    const malformed = runningSnapshot(2)
    delete (malformed.current_run_view as { generation?: string }).generation

    expect(reduceSessionRuntimeSnapshot({}, malformed, 2)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_payload',
    })
  })

  it('rejects snapshots and patches with an empty stream identity', () => {
    const malformedSnapshot = runningSnapshot(2)
    malformedSnapshot.current_run_view!.stream_id = '   '
    expect(reduceSessionRuntimeSnapshot({}, malformedSnapshot, 2)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_payload',
    })

    const snapshot = runningSnapshot(1)
    expect(reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { run: { stream_id: '', status: 'completed' } },
    }, 'bot-1', 'session-1')).toMatchObject({ kind: 'resync', reason: 'invalid_payload' })
  })

  it('rejects unknown run and steer statuses in snapshots and patches', () => {
    const malformedSnapshot = runningSnapshot(2)
    malformedSnapshot.current_run_view!.status = 'paused'
    expect(reduceSessionRuntimeSnapshot({}, malformedSnapshot, 2)).toMatchObject({
      kind: 'resync',
      reason: 'invalid_payload',
    })

    const snapshot = runningSnapshot(1)
    const malformedRunPatch = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { run: { stream_id: 'stream-1', status: 'paused' } },
    }, 'bot-1', 'session-1')
    expect(malformedRunPatch).toMatchObject({ kind: 'resync', reason: 'invalid_payload' })

    const malformedSteerPatch = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: {
        run: {
          stream_id: 'stream-1',
          steer: { id: 'steer-1', status: 'paused' },
        },
      },
    }, 'bot-1', 'session-1')
    expect(malformedSteerPatch).toMatchObject({ kind: 'resync', reason: 'invalid_payload' })
  })

  it('applies multiple appends and progress updates in one delta', () => {
    const snapshot = runningSnapshot(1)
    snapshot.current_run_view!.messages = [{ id: 1, type: 'tool', name: 'exec', tool_call_id: 'call-1' }]
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
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
    }, 'bot-1', 'session-1')

    expect(result.state.snapshot?.current_run_view?.messages).toEqual([
      { id: 1, type: 'tool', name: 'exec', tool_call_id: 'call-1', progress: ['queued', 'done'] },
      { id: 0, type: 'text', content: 'hello world' },
    ])
  })

  it('resets canonical messages before applying replacement blocks', () => {
    const snapshot = runningSnapshot(1)
    snapshot.current_run_view!.messages = [{ id: 0, type: 'text', content: 'old' }]
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: {
        reset_messages: true,
        message_upserts: [{ id: 0, type: 'text', content: 'new' }],
      },
    }, 'bot-1', 'session-1')

    expect(result.state.snapshot?.current_run_view?.messages).toEqual([{ id: 0, type: 'text', content: 'new' }])
    expect(snapshot.current_run_view?.messages).toEqual([{ id: 0, type: 'text', content: 'old' }])
  })

  it('upserts runtime ledger rows and replaces them at the terminal handoff', () => {
    const snapshot = runningSnapshot(1)
    snapshot.current_run_view!.row_ledger = [{
      stable_id: 'request-row',
      role: 'user',
      turn_id: 'turn-4',
      turn_position: 4,
      turn_message_seq: 1,
    }]

    const live = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 2,
      delta: {
        row_ledger_upserts: [{
          stable_id: 'assistant-row',
          role: 'assistant',
          turn_id: 'turn-4',
          turn_position: 4,
          turn_message_seq: 2,
        }],
      },
    }, 'bot-1', 'session-1')
    expect(live.state.snapshot?.current_run_view?.row_ledger?.map(row => row.stable_id))
      .toEqual(['request-row', 'assistant-row'])

    const settled = reduceSessionRuntimeDelta(live.state, {
      type: 'runtime_delta',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 3,
      delta: {
        reset_row_ledger: true,
        row_ledger_upserts: [{
          stable_id: 'committed-row',
          role: 'assistant',
          turn_id: 'turn-4',
          turn_position: 4,
          turn_message_seq: 2,
        }],
      },
    }, 'bot-1', 'session-1')
    expect(settled.state.snapshot?.current_run_view?.row_ledger?.map(row => row.stable_id))
      .toEqual(['committed-row'])
    expect(snapshot.current_run_view?.row_ledger?.map(row => row.stable_id))
      .toEqual(['request-row'])
  })

  it('rejects a self-contained delta across an unpublished sequence', () => {
    const result = reduceSessionRuntimeDelta({ snapshot: { bot_id: 'bot-1', session_id: 'session-1', seq: 0, queue: [] }, seq: 0 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { current_run_view: runningSnapshot(2).current_run_view },
    }, 'bot-1', 'session-1')

    expect(result).toMatchObject({ kind: 'resync', reason: 'sequence_gap', state: { seq: 0, phase: 'awaiting_checkpoint' } })
  })

  it('rejects malformed sequence values in generated wire fixtures', () => {
    const malformed = structuredClone(runtimeStateMachineContractFixture) as unknown as {
      runtime_stream: Array<{ seq: unknown }>
    }
    malformed.runtime_stream[0]!.seq = '2'

    expect(() => parseRuntimeContract(malformed, 'rich_active_run'))
      .toThrow('runtime_stream[0].seq must be a non-negative safe integer')
  })

  it('rejects empty epochs in generated wire fixtures', () => {
    const malformed = structuredClone(runtimeStateMachineContractFixture) as unknown as {
      runtime_snapshot: { epoch: unknown }
    }
    malformed.runtime_snapshot.epoch = ''

    expect(() => parseRuntimeContract(malformed, 'rich_active_run'))
      .toThrow('rich_active_run.runtime_snapshot.epoch must be a non-empty string')
  })

  it('rejects a generated wire stream that changes session target', () => {
    const malformed = structuredClone(runtimeStateMachineContractFixture) as unknown as {
      runtime_stream: Array<{ session_id: string }>
    }
    malformed.runtime_stream[1]!.session_id = 'session-other'

    expect(() => parseRuntimeContract(malformed, 'rich_active_run'))
      .toThrow('runtime_stream[1] target does not match the contract target')
  })

  it('resyncs when a delta target differs from its hydrated snapshot', () => {
    const snapshot = runningSnapshot(1)
    const result = reduceSessionRuntimeDelta({ snapshot, seq: 1 }, {
      type: 'runtime_delta',
      stream_id: 'stream-1',
      seq: 2,
      delta: { message_appends: [{ id: 0, type: 'text', content: 'wrong target' }] },
    }, 'bot-1', 'session-other')

    expect(result).toMatchObject({ kind: 'resync', reason: 'stream_mismatch' })
  })
})
