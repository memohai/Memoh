import type { SessionruntimeSnapshot } from '@memohai/sdk'
import {
  reduceSessionRuntimeDelta,
  reduceSessionRuntimeSnapshot,
  type SessionRuntimeDeltaEvent,
  type SessionRuntimeReducerState,
  type SessionRuntimeSnapshotEvent,
  type SessionRuntimeStateEvent,
} from '@memohai/sdk/session-runtime'
import generationReuseJSON from './__fixtures__/runtime-generation-reuse.contract.json'
import interruptedRunJSON from './__fixtures__/runtime-interrupted-run.contract.json'
import recoveryJSON from './__fixtures__/runtime-recovery.contract.json'
import replacementOperationsJSON from './__fixtures__/runtime-replacement-operations.contract.json'
import richActiveRunJSON from './__fixtures__/runtime-rich-active-run.contract.json'

export interface RuntimeContractFixture {
  version: number
  scenario: 'rich_active_run' | 'interrupted_run'
  runtime_snapshot: SessionRuntimeSnapshotEvent & {
    bot_id: string
    session_id: string
    seq: number
    snapshot: SessionruntimeSnapshot
  }
  runtime_stream: SessionRuntimeStateEvent[]
  runtime_terminal_stream?: SessionRuntimeStateEvent[]
  runtime_abort_stream?: SessionRuntimeStateEvent[]
  runtime_admission_stream?: SessionRuntimeStateEvent[]
  runtime_reset_stream?: SessionRuntimeStateEvent[]
  runtime_steer_stream?: SessionRuntimeStateEvent[]
}

export interface RuntimeReplacementContractFixture {
  version: number
  retry_snapshot: RuntimeContractFixture['runtime_snapshot']
  edit_snapshot: RuntimeContractFixture['runtime_snapshot']
}

export interface RuntimeGenerationReuseContractFixture {
  version: number
  scenario: 'generation_reuse'
  runtime_snapshot: RuntimeContractFixture['runtime_snapshot']
  runtime_stream: SessionRuntimeStateEvent[]
}

export interface RuntimeRecoveryContractFixture {
  version: number
  scenario: 'gap_checkpoint_recovery'
  runtime_snapshot: RuntimeContractFixture['runtime_snapshot']
  gap_delta: SessionRuntimeDeltaEvent
  delayed_delta: SessionRuntimeDeltaEvent
  runtime_checkpoint: RuntimeContractFixture['runtime_snapshot']
  post_recovery_delta: SessionRuntimeDeltaEvent
}

type UnknownRecord = Record<string, unknown>

function record(value: unknown, path: string): UnknownRecord {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`${path} must be an object`)
  }
  return value as UnknownRecord
}

function version(value: unknown, path: string): number {
  if (!Number.isSafeInteger(value) || Number(value) < 1) {
    throw new Error(`${path}.version must be a positive integer`)
  }
  return Number(value)
}

function nonEmptyString(value: unknown, path: string): string {
  if (typeof value !== 'string' || value.trim() === '') throw new Error(`${path} must be a non-empty string`)
  return value
}

function sequence(value: unknown, path: string): number {
  if (!Number.isSafeInteger(value) || Number(value) < 0) {
    throw new Error(`${path} must be a non-negative safe integer`)
  }
  return Number(value)
}

function runtimeEnvelope(event: UnknownRecord, path: string) {
  return {
    botId: nonEmptyString(event.bot_id, `${path}.bot_id`),
    sessionId: nonEmptyString(event.session_id, `${path}.session_id`),
    epoch: nonEmptyString(event.epoch, `${path}.epoch`),
    seq: sequence(event.seq, `${path}.seq`),
  }
}

function snapshotEvent(value: unknown, path: string): RuntimeContractFixture['runtime_snapshot'] {
  const event = record(value, path)
  if (event.type !== 'runtime_snapshot') throw new Error(`${path}.type must be runtime_snapshot`)
  const envelope = runtimeEnvelope(event, path)
  const rawSnapshot = record(event.snapshot, `${path}.snapshot`)
  const snapshotBotId = nonEmptyString(rawSnapshot.bot_id, `${path}.snapshot.bot_id`)
  const snapshotSessionId = nonEmptyString(rawSnapshot.session_id, `${path}.snapshot.session_id`)
  const snapshotEpoch = nonEmptyString(rawSnapshot.epoch, `${path}.snapshot.epoch`)
  const snapshotSeq = sequence(rawSnapshot.seq, `${path}.snapshot.seq`)
  if (
    envelope.botId !== snapshotBotId
    || envelope.sessionId !== snapshotSessionId
    || envelope.epoch !== snapshotEpoch
    || envelope.seq !== snapshotSeq
  ) {
    throw new Error(`${path} envelope does not match its snapshot target`)
  }
  const snapshot = rawSnapshot as SessionruntimeSnapshot
  const reduction = reduceSessionRuntimeSnapshot(
    {},
    snapshot,
    envelope.seq,
    envelope.epoch,
  )
  if (reduction.kind !== 'applied') throw new Error(`${path} is invalid: ${reduction.reason}`)
  return value as RuntimeContractFixture['runtime_snapshot']
}

function deltaEvent(
  value: unknown,
  path: string,
  target: { botId: string, sessionId: string },
): SessionRuntimeDeltaEvent {
  const event = record(value, path)
  if (event.type !== 'runtime_delta') throw new Error(`${path}.type must be runtime_delta`)
  const envelope = runtimeEnvelope(event, path)
  if (envelope.botId !== target.botId || envelope.sessionId !== target.sessionId) {
    throw new Error(`${path} target does not match the contract target`)
  }
  nonEmptyString(event.stream_id, `${path}.stream_id`)
  record(event.delta, `${path}.delta`)
  return value as SessionRuntimeDeltaEvent
}

function runtimeStream(
  value: unknown,
  path: string,
  initial: SessionRuntimeReducerState = {},
  target?: { botId: string, sessionId: string },
): SessionRuntimeReducerState {
  if (!Array.isArray(value)) throw new Error(`${path} must be an array`)
  let state = initial
  for (const [index, rawEvent] of value.entries()) {
    const eventPath = `${path}[${index}]`
    const event = record(rawEvent, eventPath)
    const { botId, sessionId, epoch, seq } = runtimeEnvelope(event, eventPath)
    if (target && (target.botId !== botId || target.sessionId !== sessionId)) {
      throw new Error(`${eventPath} target does not match the contract target`)
    }
    if (event.type === 'runtime_snapshot') {
      const snapshot = record(event.snapshot, `${eventPath}.snapshot`)
      if (
        nonEmptyString(snapshot.bot_id, `${eventPath}.snapshot.bot_id`) !== botId
        || nonEmptyString(snapshot.session_id, `${eventPath}.snapshot.session_id`) !== sessionId
        || nonEmptyString(snapshot.epoch, `${eventPath}.snapshot.epoch`) !== epoch
        || sequence(snapshot.seq, `${eventPath}.snapshot.seq`) !== seq
      ) {
        throw new Error(`${eventPath} envelope does not match its snapshot target`)
      }
    } else if (event.type === 'runtime_delta') {
      nonEmptyString(event.stream_id, `${eventPath}.stream_id`)
    } else {
      throw new Error(`${eventPath}.type must be runtime_snapshot or runtime_delta`)
    }
    const reduction = event.type === 'runtime_snapshot'
      ? reduceSessionRuntimeSnapshot(
          state,
          record(event.snapshot, `${eventPath}.snapshot`) as SessionruntimeSnapshot,
          seq,
          epoch,
        )
      : event.type === 'runtime_delta'
        ? reduceSessionRuntimeDelta(
            state,
            event as unknown as SessionRuntimeDeltaEvent,
            botId,
            sessionId,
          )
        : null
    if (!reduction || reduction.kind !== 'applied') {
      const reason = reduction && 'reason' in reduction ? reduction.reason : 'unsupported event type'
      throw new Error(`${eventPath} is invalid: ${reason}`)
    }
    state = reduction.state
  }
  return state
}

function optionalRuntimeStream(root: UnknownRecord, key: string, target: { botId: string, sessionId: string }) {
  const value = root[key]
  return value === undefined ? undefined : runtimeStream(value, key, {}, target)
}

export function parseRuntimeContract(value: unknown, scenario: RuntimeContractFixture['scenario']): RuntimeContractFixture {
  const root = record(value, scenario)
  version(root.version, scenario)
  if (root.scenario !== scenario) throw new Error(`${scenario}.scenario is invalid`)
  const baseline = snapshotEvent(root.runtime_snapshot, `${scenario}.runtime_snapshot`)
  const target = { botId: baseline.bot_id, sessionId: baseline.session_id }
  const activeState = runtimeStream(root.runtime_stream, `${scenario}.runtime_stream`, {}, target)
  if (root.runtime_terminal_stream !== undefined) {
    runtimeStream(root.runtime_terminal_stream, `${scenario}.runtime_terminal_stream`, activeState, target)
  }
  optionalRuntimeStream(root, 'runtime_abort_stream', target)
  optionalRuntimeStream(root, 'runtime_admission_stream', target)
  optionalRuntimeStream(root, 'runtime_reset_stream', target)
  optionalRuntimeStream(root, 'runtime_steer_stream', target)
  return value as RuntimeContractFixture
}

function parseReplacementContract(value: unknown): RuntimeReplacementContractFixture {
  const root = record(value, 'replacement_operations')
  version(root.version, 'replacement_operations')
  snapshotEvent(root.retry_snapshot, 'replacement_operations.retry_snapshot')
  snapshotEvent(root.edit_snapshot, 'replacement_operations.edit_snapshot')
  return value as RuntimeReplacementContractFixture
}

function parseGenerationReuseContract(value: unknown): RuntimeGenerationReuseContractFixture {
  const root = record(value, 'generation_reuse')
  version(root.version, 'generation_reuse')
  if (root.scenario !== 'generation_reuse') throw new Error('generation_reuse.scenario is invalid')
  const baseline = snapshotEvent(root.runtime_snapshot, 'generation_reuse.runtime_snapshot')
  runtimeStream(root.runtime_stream, 'generation_reuse.runtime_stream', {}, {
    botId: baseline.bot_id,
    sessionId: baseline.session_id,
  })
  return value as RuntimeGenerationReuseContractFixture
}

function parseRecoveryContract(value: unknown): RuntimeRecoveryContractFixture {
  const root = record(value, 'gap_checkpoint_recovery')
  version(root.version, 'gap_checkpoint_recovery')
  if (root.scenario !== 'gap_checkpoint_recovery') throw new Error('gap_checkpoint_recovery.scenario is invalid')

  const baseline = snapshotEvent(root.runtime_snapshot, 'gap_checkpoint_recovery.runtime_snapshot')
  const target = { botId: baseline.bot_id, sessionId: baseline.session_id }
  const initial = reduceSessionRuntimeSnapshot({}, baseline.snapshot, baseline.seq, baseline.epoch)
  if (initial.kind !== 'applied') throw new Error(`gap_checkpoint_recovery.runtime_snapshot is invalid: ${initial.reason}`)

  const gap = deltaEvent(root.gap_delta, 'gap_checkpoint_recovery.gap_delta', target)
  const gapReduction = reduceSessionRuntimeDelta(initial.state, gap, target.botId, target.sessionId)
  if (gapReduction.kind !== 'resync' || gapReduction.reason !== 'sequence_gap') {
    throw new Error('gap_checkpoint_recovery.gap_delta must create a sequence gap')
  }

  const delayed = deltaEvent(root.delayed_delta, 'gap_checkpoint_recovery.delayed_delta', target)
  const delayedReduction = reduceSessionRuntimeDelta(gapReduction.state, delayed, target.botId, target.sessionId)
  if (delayedReduction.kind !== 'ignored' || delayedReduction.state.phase !== 'awaiting_checkpoint') {
    throw new Error('gap_checkpoint_recovery.delayed_delta must remain behind the checkpoint barrier')
  }

  const checkpoint = snapshotEvent(root.runtime_checkpoint, 'gap_checkpoint_recovery.runtime_checkpoint')
  if (checkpoint.bot_id !== target.botId || checkpoint.session_id !== target.sessionId || checkpoint.seq < gap.seq) {
    throw new Error('gap_checkpoint_recovery.runtime_checkpoint does not cover the observed gap')
  }
  const checkpointReduction = reduceSessionRuntimeSnapshot(
    delayedReduction.state,
    checkpoint.snapshot,
    checkpoint.seq,
    checkpoint.epoch,
  )
  if (checkpointReduction.kind !== 'applied' || checkpointReduction.state.phase !== 'live') {
    throw new Error('gap_checkpoint_recovery.runtime_checkpoint must restore live state')
  }

  const postRecovery = deltaEvent(root.post_recovery_delta, 'gap_checkpoint_recovery.post_recovery_delta', target)
  const postRecoveryReduction = reduceSessionRuntimeDelta(
    checkpointReduction.state,
    postRecovery,
    target.botId,
    target.sessionId,
  )
  if (postRecoveryReduction.kind !== 'applied') {
    throw new Error('gap_checkpoint_recovery.post_recovery_delta must continue from the checkpoint')
  }
  return value as RuntimeRecoveryContractFixture
}

export const richActiveRunContractFixture = parseRuntimeContract(richActiveRunJSON, 'rich_active_run')
export const interruptedRunContractFixture = parseRuntimeContract(interruptedRunJSON, 'interrupted_run')
export const replacementOperationsContractFixture = parseReplacementContract(replacementOperationsJSON)
export const generationReuseContractFixture = parseGenerationReuseContract(generationReuseJSON)
export const runtimeRecoveryContractFixture = parseRecoveryContract(recoveryJSON)
