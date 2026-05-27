import { useStorage } from '@vueuse/core'
import { onAuthSessionCleared } from '@/lib/auth-session'

const pinnedBotIds = useStorage<string[]>('pinned-bot-ids', [])

onAuthSessionCleared(() => {
  pinnedBotIds.value = []
})

export function usePinnedBots() {
  function isPinned(botId: string) {
    return pinnedBotIds.value.includes(botId)
  }

  function togglePin(botId: string) {
    const idx = pinnedBotIds.value.indexOf(botId)
    if (idx >= 0) {
      pinnedBotIds.value.splice(idx, 1)
    } else {
      pinnedBotIds.value.push(botId)
    }
  }

  function sortBots<T extends { id?: string }>(bots: T[]): T[] {
    return [...bots].sort((a, b) => {
      const aPinned = isPinned(a.id ?? '')
      const bPinned = isPinned(b.id ?? '')
      if (aPinned && !bPinned) return -1
      if (!aPinned && bPinned) return 1
      return 0
    })
  }

  return { pinnedBotIds, isPinned, togglePin, sortBots }
}
