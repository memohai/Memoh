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

function snapshotEvent(value: unknown, path: string): RuntimeContractFixture['runtime_snapshot'] {
  const event = record(value, path)
  if (event.type !== 'runtime_snapshot') throw new Error(`${path}.type must be runtime_snapshot`)
  const snapshot = record(event.snapshot, `${path}.snapshot`) as SessionruntimeSnapshot
  const reduction = reduceSessionRuntimeSnapshot(
    {},
    snapshot,
    typeof event.seq === 'number' ? event.seq : undefined,
    true,
    typeof event.epoch === 'string' ? event.epoch : undefined,
  )
  if (reduction.kind !== 'applied') throw new Error(`${path} is invalid: ${reduction.reason}`)
  if (event.bot_id !== snapshot.bot_id || event.session_id !== snapshot.session_id) {
    throw new Error(`${path} envelope does not match its snapshot target`)
  }
  return value as RuntimeContractFixture['runtime_snapshot']
}

function runtimeStream(
  value: unknown,
  path: string,
  initial: SessionRuntimeReducerState = {},
): SessionRuntimeReducerState {
  if (!Array.isArray(value)) throw new Error(`${path} must be an array`)
  let state = initial
  for (const [index, rawEvent] of value.entries()) {
    const eventPath = `${path}[${index}]`
    const event = record(rawEvent, eventPath)
    const botId = nonEmptyString(event.bot_id, `${eventPath}.bot_id`)
    const sessionId = nonEmptyString(event.session_id, `${eventPath}.session_id`)
    if (event.type === 'runtime_snapshot') {
      const snapshot = record(event.snapshot, `${eventPath}.snapshot`)
      if (snapshot.bot_id !== botId || snapshot.session_id !== sessionId) {
        throw new Error(`${eventPath} envelope does not match its snapshot target`)
      }
    } else if (event.type === 'runtime_delta') {
      nonEmptyString(event.stream_id, `${eventPath}.stream_id`)
    }
    const reduction = event.type === 'runtime_snapshot'
      ? reduceSessionRuntimeSnapshot(
          state,
          record(event.snapshot, `${eventPath}.snapshot`) as SessionruntimeSnapshot,
          typeof event.seq === 'number' ? event.seq : undefined,
          true,
          typeof event.epoch === 'string' ? event.epoch : undefined,
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

function optionalRuntimeStream(root: UnknownRecord, key: string) {
  const value = root[key]
  return value === undefined ? undefined : runtimeStream(value, key)
}

function parseRuntimeContract(value: unknown, scenario: RuntimeContractFixture['scenario']): RuntimeContractFixture {
  const root = record(value, scenario)
  version(root.version, scenario)
  if (root.scenario !== scenario) throw new Error(`${scenario}.scenario is invalid`)
  snapshotEvent(root.runtime_snapshot, `${scenario}.runtime_snapshot`)
  const activeState = runtimeStream(root.runtime_stream, `${scenario}.runtime_stream`)
  if (root.runtime_terminal_stream !== undefined) {
    runtimeStream(root.runtime_terminal_stream, `${scenario}.runtime_terminal_stream`, activeState)
  }
  optionalRuntimeStream(root, 'runtime_admission_stream')
  optionalRuntimeStream(root, 'runtime_reset_stream')
  optionalRuntimeStream(root, 'runtime_steer_stream')
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
  snapshotEvent(root.runtime_snapshot, 'generation_reuse.runtime_snapshot')
  runtimeStream(root.runtime_stream, 'generation_reuse.runtime_stream')
  return value as RuntimeGenerationReuseContractFixture
}

export const richActiveRunContractFixture = parseRuntimeContract(richActiveRunJSON, 'rich_active_run')
export const interruptedRunContractFixture = parseRuntimeContract(interruptedRunJSON, 'interrupted_run')
export const replacementOperationsContractFixture = parseReplacementContract(replacementOperationsJSON)
export const generationReuseContractFixture = parseGenerationReuseContract(generationReuseJSON)
