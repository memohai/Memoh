import type {
  UIAssistantTurn,
  UIMessage,
  UIRowIdentity,
  UITurn,
} from '@/composables/api/useChat.types'
import type { ChatAssistantTurn, ChatMessage } from '../types'

type Awaitable<T> = T | Promise<T>

export type RuntimeTerminalStatus = 'completed' | 'aborted' | 'errored'

export interface RuntimeRowIdentity {
  stableId: string
  turnPosition: number
  turnMessageSeq: number
}

export interface RuntimeSettledRow {
  identity: RuntimeRowIdentity
  runtime: UIMessage
  persisted: UIMessage
}

export interface RuntimePersistedMatch {
  history: UITurn[]
  turn: UIAssistantTurn
  rows: RuntimeSettledRow[]
  ledger: RuntimeRowIdentity[]
}

export interface RuntimeTerminalSettleInput {
  status: RuntimeTerminalStatus
  runtimeMessages: UIMessage[]
  rowLedger?: UIRowIdentity[]
}

export interface RuntimeTerminalSettleDependencies {
  fetchHistory: () => Promise<UITurn[]>
  replacePersisted: (match: RuntimePersistedMatch) => Awaitable<void>
  removeOptimistic: () => Awaitable<void>
  restoreDraft: () => Awaitable<void>
  keepPartial: (status: RuntimeTerminalStatus) => Awaitable<void>
}

export type RuntimeTerminalSettleResult =
  | {
      status: RuntimeTerminalStatus
      persistence: 'persisted'
      action: 'replace_persisted'
      match: RuntimePersistedMatch
    }
  | {
      status: RuntimeTerminalStatus
      persistence: 'vanished'
      action: 'remove_optimistic_restore_draft' | 'keep_partial'
    }
  | {
      status: RuntimeTerminalStatus
      persistence: 'unknown'
      action: 'keep_partial'
    }

function rowIdentity(message: UIMessage): RuntimeRowIdentity | null {
  const stableId = message.stable_id?.trim() ?? ''
  const turnPosition = message.turn_position
  const turnMessageSeq = message.turn_message_seq
  if (
    !stableId
    || !Number.isSafeInteger(turnPosition)
    || !Number.isSafeInteger(turnMessageSeq)
  ) return null
  return {
    stableId,
    turnPosition: turnPosition!,
    turnMessageSeq: turnMessageSeq!,
  }
}

function rowIdentities(message: UIMessage): RuntimeRowIdentity[] {
  const rows = (message.row_identities ?? []).flatMap((row) => {
    const stableId = row.stable_id?.trim() ?? ''
    if (
      !stableId
      || !Number.isSafeInteger(row.turn_position)
      || !Number.isSafeInteger(row.turn_message_seq)
    ) return []
    return [{
      stableId,
      turnPosition: row.turn_position,
      turnMessageSeq: row.turn_message_seq,
    }]
  })
  const primary = rowIdentity(message)
  if (primary && !rows.some(row => rowIdentityKey(row) === rowIdentityKey(primary))) rows.unshift(primary)
  return rows
}

function rowIdentityKey(identity: RuntimeRowIdentity): string {
  return `${identity.stableId}\u0000${identity.turnPosition}\u0000${identity.turnMessageSeq}`
}

function ledgerRowIdentity(row: UIRowIdentity): RuntimeRowIdentity | null {
  const stableId = row.stable_id?.trim() ?? ''
  if (!stableId || !Number.isSafeInteger(row.turn_position) || !Number.isSafeInteger(row.turn_message_seq)) return null
  return {
    stableId,
    turnPosition: row.turn_position,
    turnMessageSeq: row.turn_message_seq,
  }
}

function turnRowIdentity(turn: UITurn): RuntimeRowIdentity | null {
  const stableId = turn.id?.trim() ?? ''
  if (!stableId || !Number.isSafeInteger(turn.turn_position) || !Number.isSafeInteger(turn.turn_message_seq)) return null
  return {
    stableId,
    turnPosition: turn.turn_position!,
    turnMessageSeq: turn.turn_message_seq!,
  }
}

// Runtime and Postgres describe the same row only when the durable id and both
// ordering coordinates agree. Content, timestamps, and list position are not
// identities and therefore never participate in this handoff.
export function findPersistedRuntimeMatch(
  history: UITurn[],
  runtimeMessages: UIMessage[],
  rowLedger: UIRowIdentity[] = [],
): RuntimePersistedMatch | null {
  const runtimeRows = new Map<string, { identity: RuntimeRowIdentity, message: UIMessage }>()
  for (const message of runtimeMessages) {
    const identities = rowIdentities(message)
    if (identities.length === 0) return null
    for (const identity of identities) {
      runtimeRows.set(rowIdentityKey(identity), { identity, message })
    }
  }

  const declaredLedger = rowLedger.flatMap((row) => {
    const identity = ledgerRowIdentity(row)
    return identity ? [identity] : []
  })
  if (rowLedger.length > 0 && declaredLedger.length !== rowLedger.length) return null
  const required = new Map<string, RuntimeRowIdentity>()
  for (const identity of declaredLedger.length > 0
    ? declaredLedger
    : [...runtimeRows.values()].map(row => row.identity)) {
    required.set(rowIdentityKey(identity), identity)
  }
  if (required.size === 0) return null
  if (declaredLedger.length > 0 && [...runtimeRows.keys()].some(key => !required.has(key))) return null

  const persistedRows = new Map<string, { message?: UIMessage, turn?: UIAssistantTurn }>()
  for (const turn of history) {
    if (turn.role === 'assistant') {
      for (const message of turn.messages) {
        for (const identity of rowIdentities(message)) {
          persistedRows.set(rowIdentityKey(identity), { message, turn })
        }
      }
      continue
    }
    const identity = turnRowIdentity(turn)
    if (identity) persistedRows.set(rowIdentityKey(identity), {})
  }
  if ([...required.keys()].some(key => !persistedRows.has(key))) return null

  let assistantTurn: UIAssistantTurn | undefined
  const rows: RuntimeSettledRow[] = []
  for (const [key, runtime] of runtimeRows) {
    const persisted = persistedRows.get(key)
    if (!persisted?.message || !persisted.turn) return null
    assistantTurn ??= persisted.turn
    if (assistantTurn !== persisted.turn) return null
    rows.push({ identity: runtime.identity, runtime: runtime.message, persisted: persisted.message })
  }
  if (!assistantTurn) {
    for (const key of required.keys()) {
      const turn = persistedRows.get(key)?.turn
      if (turn) {
        assistantTurn = turn
        break
      }
    }
  }
  return assistantTurn ? { history, turn: assistantTurn, rows, ledger: [...required.values()] } : null
}

// Reconciles exactly once after a run becomes terminal. When the same row is
// visible in the runtime projection and REST history, the complete Postgres
// turn replaces the live view; the two views are never merged.
export async function settleRuntimeTerminal(
  input: RuntimeTerminalSettleInput,
  dependencies: RuntimeTerminalSettleDependencies,
): Promise<RuntimeTerminalSettleResult> {
  const history = await dependencies.fetchHistory()
  const match = findPersistedRuntimeMatch(history, input.runtimeMessages, input.rowLedger)
  if (match) {
    await dependencies.replacePersisted(match)
    return {
      status: input.status,
      persistence: 'persisted',
      action: 'replace_persisted',
      match,
    }
  }

  if (input.status === 'aborted' || (input.status === 'completed' && input.runtimeMessages.length === 0)) {
    await dependencies.removeOptimistic()
    await dependencies.restoreDraft()
    return {
      status: input.status,
      persistence: 'vanished',
      action: 'remove_optimistic_restore_draft',
    }
  }

  // Errored output is intentionally client-visible even when persistence
  // failed. A completed run should normally be persisted, but preserving its
  // projection is safer than deleting confirmed output if that contract drifts.
  await dependencies.keepPartial(input.status)
  return {
    status: input.status,
    persistence: 'vanished',
    action: 'keep_partial',
  }
}

// The durable row wins only when runtime and history name the same row. A
// missing stable id is an incomplete contract, not permission to guess by
// content, timestamp, or position in the transcript.
export function findSettledAssistant(
  transcript: ChatMessage[],
  runtimeMessages: UIMessage[],
): ChatAssistantTurn | null {
  const stableIds = new Set(runtimeMessages
    .flatMap(message => rowIdentities(message).map(identity => identity.stableId))
    .filter((id): id is string => Boolean(id)))
  if (stableIds.size === 0) return null
  return transcript.find((turn): turn is ChatAssistantTurn =>
    turn.role === 'assistant'
    && !turn.__optimistic
    && turn.messages.some(block => block.stable_id && stableIds.has(block.stable_id)),
  ) ?? null
}
