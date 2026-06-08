import type { MessageStreamEvent } from '@/composables/api/useChat'

export function upsertById<T extends { id: number }>(items: T[], incoming: T): T[] {
  const index = items.findIndex(item => item.id === incoming.id)
  if (index < 0) {
    items.push(incoming)
    items.sort((a, b) => a.id - b.id)
    return items
  }
  const target = items[index]
  for (const key of Object.keys(target)) {
    if (!(key in incoming)) delete (target as Record<string, unknown>)[key]
  }
  Object.assign(target, incoming)
  return items
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
