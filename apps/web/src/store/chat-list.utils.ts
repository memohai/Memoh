import type { MessageStreamEvent } from '@/composables/api/useChat'

export function upsertById<T extends { id: number }>(items: T[], incoming: T): T[] {
  const next = [...items]
  const index = next.findIndex(item => item.id === incoming.id)
  if (index >= 0) {
    next[index] = incoming
  } else {
    next.push(incoming)
    next.sort((a, b) => a.id - b.id)
  }
  return next
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
