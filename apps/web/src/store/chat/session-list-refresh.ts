import type { Ref } from 'vue'
import type { FetchSessionsResult } from '@/composables/api/useChat'

interface SessionListRefreshDeps {
  currentBotId: Ref<string | null>
  fetchSessions: (botId: string) => Promise<FetchSessionsResult>
  applySnapshot: (response: FetchSessionsResult) => void
}

export function createSessionListRefresh({
  currentBotId,
  fetchSessions,
  applySnapshot,
}: SessionListRefreshDeps) {
  const requests = new Map<string, Promise<void>>()
  let generation = 0

  function refresh(botId: string): Promise<void> {
    const bid = botId.trim()
    if (!bid) return Promise.resolve()
    const existing = requests.get(bid)
    if (existing) return existing

    const requestGeneration = generation
    const promise = fetchSessions(bid)
      .then((response) => {
        if (requestGeneration !== generation) return
        if (requests.get(bid) !== promise) return
        if ((currentBotId.value ?? '').trim() !== bid) return
        applySnapshot(response)
      })
      .catch((error) => {
        console.error('Failed to refresh sessions:', error)
      })
      .finally(() => {
        if (requests.get(bid) === promise) requests.delete(bid)
      })

    requests.set(bid, promise)
    return promise
  }

  function reset() {
    generation += 1
    requests.clear()
  }

  return { refresh, reset }
}
