import type { MessageStreamEvent } from '@/composables/api/useChat'

export function assignInPlace<T extends object>(target: T, source: T): void {
  for (const key of Object.keys(target)) {
    if (!(key in source)) delete (target as Record<string, unknown>)[key]
  }
  Object.assign(target, source)
}

export function upsertById<T extends { id: number }>(items: T[], incoming: T): T[] {
  const existing = items.find(item => item.id === incoming.id)
  if (existing === undefined) {
    items.push(incoming)
    items.sort((a, b) => a.id - b.id)
    return items
  }
  assignInPlace(existing, incoming)
  return items
}

interface ReconcileByIdOptions<T> {
  keyOfExisting?: (item: T) => unknown
  keyOfIncoming?: (item: T) => unknown
  merge?: (current: T, incoming: T) => void
}

export function reconcileById<T extends { id: PropertyKey }>(
  target: T[],
  incoming: T[],
  options: ReconcileByIdOptions<T> = {},
): T[] {
  const keyOfExisting = options.keyOfExisting ?? ((item: T) => item.id)
  const keyOfIncoming = options.keyOfIncoming ?? ((item: T) => item.id)
  const merge = options.merge ?? assignInPlace
  const byKey = new Map<unknown, T>()
  for (const item of target) byKey.set(keyOfExisting(item), item)
  const next = incoming.map((item) => {
    const current = byKey.get(keyOfIncoming(item))
    if (current === undefined) return item
    merge(current, item)
    return current
  })
  target.splice(0, target.length, ...next)
  return target
}

export function sortByRecency<T extends { updated_at?: string; created_at?: string }>(items: T[]): T[] {
  const key = (item: T) => item.updated_at ?? item.created_at ?? ''
  return [...items].sort((a, b) => {
    const ka = key(a)
    const kb = key(b)
    return ka < kb ? 1 : ka > kb ? -1 : 0
  })
}

export function latestOutputLine(tail?: string): string {
  if (!tail) return ''
  for (const line of tail.split('\n').reverse()) {
    for (const segment of line.split('\r').reverse()) {
      const candidate = segment.trim()
      if (candidate) return candidate
    }
  }
  return ''
}

export type TurnSegment<T> =
  | { kind: 'rail'; key: string; blocks: T[] }
  | { kind: 'flow'; key: string; block: T }

const PROCESS_BLOCK_TYPES = new Set(['reasoning', 'tool'])

// Group a turn's blocks into segments by their immutable `type`: maximal runs of
// process blocks (reasoning/tool) become one recessed "rail"; text/error/attachment
// blocks break out as standalone "flow" segments. Keying by the segment's first
// block id keeps every segment stable as the turn streams (blocks only append at
// the tail), so no block ever reparents — which is what prevents remount/stall.
export function segmentTurnBlocks<T extends { id: number; type: string }>(blocks: T[]): TurnSegment<T>[] {
  const segments: TurnSegment<T>[] = []
  let rail: { kind: 'rail'; key: string; blocks: T[] } | null = null
  for (const block of blocks) {
    if (PROCESS_BLOCK_TYPES.has(block.type)) {
      if (rail === null) {
        rail = { kind: 'rail', key: `rail:${block.id}`, blocks: [] }
        segments.push(rail)
      }
      rail.blocks.push(block)
    } else {
      rail = null
      segments.push({ kind: 'flow', key: `flow:${block.id}`, block })
    }
  }
  return segments
}

export function shouldRefreshFromMessageCreated(
  targetBotId: string,
  currentSessionId: string | null,
  streamingSessionId: string | null,
  event: MessageStreamEvent,
): boolean {
  if ((event.type ?? '').toLowerCase() !== 'message_created') return false

  const raw = event.message
  if (!raw) return false

  const eventBotId = String(event.bot_id ?? '').trim()
  if (eventBotId && eventBotId !== targetBotId) return false

  const messageBotId = String(raw.bot_id ?? '').trim()
  if (messageBotId && messageBotId !== targetBotId) return false

  const messageSessionId = String(raw.session_id ?? '').trim()
  if (!currentSessionId) return false
  if (messageSessionId && messageSessionId !== currentSessionId) return false
  if (streamingSessionId && streamingSessionId === currentSessionId) return false

  return true
}
