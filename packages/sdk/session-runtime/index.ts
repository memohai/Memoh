import type {
  ConversationUiMessage,
  ConversationUiMessageType,
  SessionruntimeCurrentRunView,
  SessionruntimeSnapshot,
  SessionruntimeSteerState,
} from '../src/types.gen'

export interface SessionRuntimeSnapshotEvent {
  type: 'runtime_snapshot'
  bot_id: string
  session_id: string
  epoch: string
  stream_id?: string
  seq: number
  snapshot: SessionruntimeSnapshot
}

export interface SessionRuntimeDeltaEvent {
  type: 'runtime_delta'
  bot_id: string
  session_id: string
  epoch: string
  stream_id: string
  seq: number
  updated_at?: string
  delta: SessionRuntimeDelta
}

export interface SessionRuntimeDelta {
  current_run_view?: SessionruntimeCurrentRunView
  run?: SessionRuntimeRunPatch
  message_appends?: SessionRuntimeMessageAppend[]
  progress_appends?: SessionRuntimeProgressAppend[]
  message_upserts?: ConversationUiMessage[]
  reset_messages?: boolean
}

export interface SessionRuntimeRunPatch {
  stream_id: string
  status?: string
  error_code?: string
  error?: string
  history_committed?: boolean
  history_assistant_message_id?: string
  canonical_ready?: boolean
  steer?: SessionruntimeSteerState
  updated_at?: string
  owner_lease_expires_at?: string
}

export interface SessionRuntimeMessageAppend {
  id: number
  type: ConversationUiMessageType
  content: string
}

export interface SessionRuntimeProgressAppend {
  id: number
  progress: unknown
  input?: unknown
}

export interface SessionRuntimeDroppedEvent {
  type: 'runtime_dropped'
  bot_id?: string
  session_id?: string
  epoch?: string
  stream_id?: string
  seq?: number
  message?: string
}

export type SessionRuntimeStateEvent =
  | SessionRuntimeSnapshotEvent
  | SessionRuntimeDeltaEvent
  | SessionRuntimeDroppedEvent

export function parseSessionRuntimeStateEvent(value: unknown): SessionRuntimeStateEvent | undefined {
  if (!isRecord(value)) return undefined
  if (value.type === 'runtime_snapshot') {
    if (
      !isNonEmptyString(value.bot_id)
      || !isNonEmptyString(value.session_id)
      || !isNonEmptyString(value.epoch)
      || runtimeSequence(value.seq) === undefined
      || !isRuntimeSnapshotPayload(value.snapshot)
    ) return undefined
    const snapshot = value.snapshot
    if (
      trimmedString(snapshot.bot_id) !== trimmedString(value.bot_id)
      || trimmedString(snapshot.session_id) !== trimmedString(value.session_id)
      || trimmedString(snapshot.epoch) !== trimmedString(value.epoch)
      || runtimeSequence(snapshot.seq) !== runtimeSequence(value.seq)
    ) return undefined
    return value as unknown as SessionRuntimeSnapshotEvent
  }
  if (value.type === 'runtime_delta') {
    if (
      !isNonEmptyString(value.bot_id)
      || !isNonEmptyString(value.session_id)
      || !isNonEmptyString(value.epoch)
      || !isNonEmptyString(value.stream_id)
      || runtimeSequence(value.seq) === undefined
      || !isRuntimeDeltaPayload(value.delta)
    ) return undefined
    return value as unknown as SessionRuntimeDeltaEvent
  }
  if (value.type === 'runtime_dropped') {
    if (!isNonEmptyString(value.bot_id) || !isNonEmptyString(value.session_id)) return undefined
    return value as unknown as SessionRuntimeDroppedEvent
  }
  return undefined
}

export function isSessionRuntimeActiveStatus(status?: string): boolean {
  switch ((status ?? '').trim().toLowerCase()) {
    case 'admitting':
    case 'running':
    case 'aborting':
      return true
    default:
      return false
  }
}

export function isSessionRuntimeTerminalStatus(status?: string): boolean {
  switch ((status ?? '').trim().toLowerCase()) {
    case 'completed':
    case 'aborted':
    case 'errored':
    case 'interrupted':
    case 'lost':
      return true
    default:
      return false
  }
}

export interface SessionRuntimeReducerState {
  snapshot?: SessionruntimeSnapshot
  epoch?: string
  seq?: number
  phase?: 'awaiting_checkpoint' | 'live'
}

export type SessionRuntimeReduction =
  | { kind: 'applied', state: SessionRuntimeReducerState }
  | { kind: 'ignored', state: SessionRuntimeReducerState }
  | { kind: 'resync', state: SessionRuntimeReducerState, reason: 'invalid_payload' | 'invalid_seq' | 'sequence_gap' | 'epoch_mismatch' | 'missing_snapshot' | 'stream_mismatch' | 'missing_progress_target' }

type UnknownRecord = Record<string, unknown>

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === 'string'
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== ''
}

function isOptionalRecord(value: unknown): boolean {
  return value === undefined || isRecord(value)
}

const runtimeRunStatuses = new Set([
  'admitting',
  'running',
  'aborting',
  'completed',
  'aborted',
  'errored',
  'interrupted',
  'lost',
])

const runtimeSteerStatuses = new Set(['pending', 'queued', 'applied', 'rejected'])

function isRuntimeRunStatus(value: unknown): boolean {
  return typeof value === 'string' && runtimeRunStatuses.has(value.trim().toLowerCase())
}

function isRuntimeSteer(value: unknown): boolean {
  return value === undefined || (isRecord(value)
    && typeof value.id === 'string'
    && isRuntimeSteerStatus(value.status)
    && isOptionalString(value.error_code)
    && isOptionalString(value.error))
}

function isRuntimeSteerStatus(value: unknown): boolean {
  return typeof value === 'string' && runtimeSteerStatuses.has(value.trim().toLowerCase())
}

function isRuntimeQuestion(value: unknown): boolean {
  return isRecord(value)
    && (value.options === undefined || (Array.isArray(value.options) && value.options.every(isRecord)))
}

function isRuntimeUserInput(value: unknown): boolean {
  return isRecord(value)
    && (value.questions === undefined || (Array.isArray(value.questions) && value.questions.every(isRuntimeQuestion)))
}

function isRuntimeMessage(value: unknown): boolean {
  if (!isRecord(value)) return false
  if (typeof value.id !== 'number' || !Number.isFinite(value.id)) return false
  if (value.type !== 'text' && value.type !== 'reasoning' && value.type !== 'tool' && value.type !== 'attachments') return false
  return isOptionalString(value.content)
    && (value.attachments === undefined || (Array.isArray(value.attachments) && value.attachments.every(isRecord)))
    && (value.progress === undefined || Array.isArray(value.progress))
    && isOptionalRecord(value.approval)
    && (value.user_input === undefined || isRuntimeUserInput(value.user_input))
    && isOptionalRecord(value.background_task)
}

function isRuntimeTurn(value: unknown): boolean {
  return isRecord(value)
    && isOptionalString(value.role)
    && isOptionalString(value.text)
    && (value.attachments === undefined || (Array.isArray(value.attachments) && value.attachments.every(isRecord)))
    && (value.messages === undefined || (Array.isArray(value.messages) && value.messages.every(isRuntimeMessage)))
}

function isRuntimeOperation(value: unknown): boolean {
  return isRecord(value)
    && (value.kind === 'retry' || value.kind === 'edit')
    && typeof value.replace_from_message_id === 'string'
    && (value.replacement_user_turn === undefined || isRuntimeTurn(value.replacement_user_turn))
}

function isResolvedDecision(value: unknown): boolean {
  if (!isRecord(value)) return false
  const kind = trimmedString(value.kind)
  const status = trimmedString(value.status).toLowerCase()
  const id = trimmedString(value.id)
  if (!kind || !id || !status) return false
  if (kind === 'user_input') return status === 'submitted' || status === 'canceled'
  if (kind === 'tool_approval') return status === 'approved' || status === 'rejected'
  return false
}

function isCurrentRunView(value: unknown): boolean {
  if (!isRecord(value)) return false
  return typeof value.stream_id === 'string'
    && value.stream_id.trim() !== ''
    && typeof value.generation === 'string'
    && value.generation.trim() !== ''
    && isRuntimeRunStatus(value.status)
    && (value.history_committed === undefined || typeof value.history_committed === 'boolean')
    && isOptionalString(value.history_assistant_message_id)
    && (value.canonical_ready === undefined || typeof value.canonical_ready === 'boolean')
    && isOptionalString(value.error_code)
    && isOptionalString(value.error)
    && (value.messages === undefined || (Array.isArray(value.messages) && value.messages.every(isRuntimeMessage)))
    && (value.operation === undefined || isRuntimeOperation(value.operation))
    && (value.request_user_turn === undefined || isRuntimeTurn(value.request_user_turn))
    && (value.resolved_decision === undefined || isResolvedDecision(value.resolved_decision))
    && isRuntimeSteer(value.steer)
}

function isRuntimeSnapshotPayload(value: unknown): value is SessionruntimeSnapshot {
  if (!isRecord(value)) return false
  return isNonEmptyString(value.bot_id)
    && isNonEmptyString(value.session_id)
    && isOptionalString(value.epoch)
    && runtimeSequence(value.seq) !== undefined
    && (value.queue === undefined || (Array.isArray(value.queue) && value.queue.every(item => isRecord(item) && typeof item.stream_id === 'string')))
    && (value.current_run_view === undefined || isCurrentRunView(value.current_run_view))
}

function isRunPatch(value: unknown): boolean {
  return isRecord(value)
    && typeof value.stream_id === 'string'
    && value.stream_id.trim() !== ''
    && (value.status === undefined || isRuntimeRunStatus(value.status))
    && isOptionalString(value.error_code)
    && isOptionalString(value.error)
    && (value.history_committed === undefined || typeof value.history_committed === 'boolean')
    && isOptionalString(value.history_assistant_message_id)
    && (value.canonical_ready === undefined || typeof value.canonical_ready === 'boolean')
    && isOptionalString(value.updated_at)
    && isOptionalString(value.owner_lease_expires_at)
    && isRuntimeSteer(value.steer)
}

function isMessageAppend(value: unknown): boolean {
  return isRecord(value)
    && typeof value.id === 'number'
    && Number.isFinite(value.id)
    && (value.type === 'text' || value.type === 'reasoning' || value.type === 'tool' || value.type === 'attachments')
    && typeof value.content === 'string'
}

function isProgressAppend(value: unknown): boolean {
  return isRecord(value)
    && typeof value.id === 'number'
    && Number.isFinite(value.id)
    && Object.hasOwn(value, 'progress')
}

function isRuntimeDeltaPayload(value: unknown): value is SessionRuntimeDelta {
  if (!isRecord(value)) return false
  return (value.current_run_view === undefined || isCurrentRunView(value.current_run_view))
    && (value.run === undefined || isRunPatch(value.run))
    && (value.message_appends === undefined || (Array.isArray(value.message_appends) && value.message_appends.every(isMessageAppend)))
    && (value.progress_appends === undefined || (Array.isArray(value.progress_appends) && value.progress_appends.every(isProgressAppend)))
    && (value.message_upserts === undefined || (Array.isArray(value.message_upserts) && value.message_upserts.every(isRuntimeMessage)))
    && (value.reset_messages === undefined || typeof value.reset_messages === 'boolean')
}

function runtimeSequence(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined
}

function trimmedString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function reducerStateEpoch(state: SessionRuntimeReducerState): string {
  return trimmedString(state.epoch) || trimmedString(state.snapshot?.epoch)
}

function sameRuntimeSnapshotTarget(left: SessionruntimeSnapshot, right: SessionruntimeSnapshot): boolean {
  return trimmedString(left.bot_id) === trimmedString(right.bot_id)
    && trimmedString(left.session_id) === trimmedString(right.session_id)
}

function resync(state: SessionRuntimeReducerState, reason: Extract<SessionRuntimeReduction, { kind: 'resync' }>['reason']): SessionRuntimeReduction {
  const epoch = reducerStateEpoch(state)
  return {
    kind: 'resync',
    state: { snapshot: state.snapshot, epoch: epoch || undefined, seq: state.seq, phase: 'awaiting_checkpoint' },
    reason,
  }
}

export function awaitSessionRuntimeCheckpoint(state: SessionRuntimeReducerState): SessionRuntimeReducerState {
  const epoch = reducerStateEpoch(state)
  return { snapshot: state.snapshot, epoch: epoch || undefined, seq: state.seq, phase: 'awaiting_checkpoint' }
}

function cloneSnapshotForDelta(snapshot: SessionruntimeSnapshot): SessionruntimeSnapshot {
  const currentRun = snapshot.current_run_view
  return {
    ...snapshot,
    current_run_view: currentRun
      ? {
          ...currentRun,
          messages: [...(currentRun.messages ?? [])],
        }
      : undefined,
  }
}

export function reduceSessionRuntimeSnapshot(
  state: SessionRuntimeReducerState,
  snapshot: SessionruntimeSnapshot,
  eventSeq?: number,
  eventEpoch?: string,
): SessionRuntimeReduction {
  const envelopeSeq = runtimeSequence(eventSeq)
  const payloadSeq = runtimeSequence(snapshot.seq)
  if (
    payloadSeq === undefined
    || (eventSeq !== undefined && envelopeSeq === undefined)
    || (envelopeSeq !== undefined && payloadSeq !== undefined && envelopeSeq !== payloadSeq)
  ) return resync(state, 'invalid_seq')
  if (!isRuntimeSnapshotPayload(snapshot)) return resync(state, 'invalid_payload')
  if (state.snapshot && !sameRuntimeSnapshotTarget(state.snapshot, snapshot)) return resync(state, 'stream_mismatch')
  const seq = envelopeSeq ?? payloadSeq
  const envelopeEpoch = trimmedString(eventEpoch)
  const snapshotEpoch = trimmedString(snapshot.epoch)
  if (envelopeEpoch && snapshotEpoch && envelopeEpoch !== snapshotEpoch) return resync(state, 'epoch_mismatch')
  const epoch = envelopeEpoch || snapshotEpoch
  if (!epoch) return resync(state, 'epoch_mismatch')
  const currentEpoch = reducerStateEpoch(state)
  const sameEpoch = Boolean(epoch && currentEpoch && epoch === currentEpoch)
  if (seq !== undefined && state.seq !== undefined) {
    if (sameEpoch && seq < state.seq) return { kind: 'ignored', state }
  }

  const nextSnapshot = structuredClone(snapshot)
  if (epoch) nextSnapshot.epoch = epoch
  if (seq !== undefined) nextSnapshot.seq = seq
  return {
    kind: 'applied',
    state: {
      snapshot: nextSnapshot,
      epoch,
      seq,
      phase: 'live',
    },
  }
}

export function reduceSessionRuntimeDelta(
  state: SessionRuntimeReducerState,
  event: SessionRuntimeDeltaEvent,
  botId: string,
  sessionId: string,
): SessionRuntimeReduction {
  const seq = runtimeSequence(event.seq)
  if (seq === undefined) return resync(state, 'invalid_seq')
  const delta = event.delta
  if (!isRuntimeDeltaPayload(delta)) return resync(state, 'invalid_payload')
  if (state.snapshot && !isRuntimeSnapshotPayload(state.snapshot)) {
    return resync({}, 'invalid_payload')
  }
  if (
    state.snapshot
    && (
      trimmedString(state.snapshot.bot_id) !== botId.trim()
      || trimmedString(state.snapshot.session_id) !== sessionId.trim()
    )
  ) return resync(state, 'stream_mismatch')
  if (state.phase === 'awaiting_checkpoint') return { kind: 'ignored', state }
  const eventEpoch = trimmedString(event.epoch)
  const currentEpoch = reducerStateEpoch(state)
  if (currentEpoch && !eventEpoch) return resync(state, 'epoch_mismatch')
  if (currentEpoch && eventEpoch !== currentEpoch) return resync(state, 'epoch_mismatch')
  if (state.seq !== undefined && seq <= state.seq) return { kind: 'ignored', state }

  if (state.seq === undefined || seq !== state.seq + 1) {
    return resync(state, 'sequence_gap')
  }

  const snapshot = state.snapshot ? cloneSnapshotForDelta(state.snapshot) : undefined
  if (!snapshot) return resync(state, 'missing_snapshot')

  snapshot.seq = seq
  const epoch = eventEpoch || currentEpoch || trimmedString(snapshot.epoch)
  if (epoch) snapshot.epoch = epoch
  if (event.updated_at) snapshot.updated_at = event.updated_at
  const eventStreamId = trimmedString(event.stream_id)
  if (delta.current_run_view) {
    const checkpointStreamId = trimmedString(delta.current_run_view.stream_id)
    if (!eventStreamId || !checkpointStreamId || eventStreamId !== checkpointStreamId) {
      return resync(state, 'stream_mismatch')
    }
    snapshot.current_run_view = structuredClone(delta.current_run_view)
    return { kind: 'applied', state: { snapshot, epoch: epoch || undefined, seq, phase: 'live' } }
  }

  const run = snapshot.current_run_view
  if (!run) return resync(state, 'missing_snapshot')
  const runStreamId = trimmedString(run.stream_id)
  if (!eventStreamId || !runStreamId || eventStreamId !== runStreamId) {
    return resync(state, 'stream_mismatch')
  }
  const patchStreamId = trimmedString(delta.run?.stream_id)
  if (patchStreamId && patchStreamId !== runStreamId) {
    return resync(state, 'stream_mismatch')
  }

  const messages: ConversationUiMessage[] = run.messages ?? (run.messages = [])
  if (delta.reset_messages) run.messages = []
  for (const append of delta.message_appends ?? []) {
    const targetMessages: ConversationUiMessage[] = run.messages ?? (run.messages = [])
    const index = targetMessages.findIndex(candidate => candidate.id === append.id && candidate.type === append.type)
    let message: ConversationUiMessage
    if (index >= 0) {
      message = { ...targetMessages[index]! }
      targetMessages[index] = message
    } else {
      message = { id: append.id, type: append.type, content: '' }
      targetMessages.push(message)
    }
    message.content = `${message.content ?? ''}${append.content}`
  }
  for (const append of delta.progress_appends ?? []) {
    const targetMessages: ConversationUiMessage[] = run.messages ?? messages
    const index = targetMessages.findIndex(candidate => candidate.id === append.id)
    if (index < 0) return resync(state, 'missing_progress_target')
    const message = { ...targetMessages[index]! }
    targetMessages[index] = message
    message.progress = [...(message.progress ?? []), append.progress]
    if ('input' in append) message.input = append.input
  }
  for (const incoming of delta.message_upserts ?? []) {
    const targetMessages: ConversationUiMessage[] = run.messages ?? (run.messages = [])
    const toolCallId = incoming.type === 'tool' ? incoming.tool_call_id : undefined
    const hasToolIdentity = typeof toolCallId === 'string' && toolCallId.trim() !== ''
    const index = targetMessages.findIndex(message =>
      (hasToolIdentity && message.type === 'tool' && message.tool_call_id === toolCallId)
      || message.id === incoming.id,
    )
    if (index >= 0) targetMessages[index] = structuredClone(incoming)
    else targetMessages.push(structuredClone(incoming))
  }

  const patch = delta.run
  if (patch) {
    if ('status' in patch) run.status = patch.status
    if ('error_code' in patch) run.error_code = patch.error_code
    if ('error' in patch) run.error = patch.error
    if ('history_committed' in patch) run.history_committed = patch.history_committed
    if ('history_assistant_message_id' in patch) run.history_assistant_message_id = patch.history_assistant_message_id
    if ('canonical_ready' in patch) run.canonical_ready = patch.canonical_ready
    if ('steer' in patch) run.steer = patch.steer ? structuredClone(patch.steer) : patch.steer
    if ('updated_at' in patch) run.updated_at = patch.updated_at
    if ('owner_lease_expires_at' in patch) run.owner_lease_expires_at = patch.owner_lease_expires_at
  }
  return { kind: 'applied', state: { snapshot, epoch: epoch || undefined, seq, phase: 'live' } }
}
