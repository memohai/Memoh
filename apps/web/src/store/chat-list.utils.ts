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
