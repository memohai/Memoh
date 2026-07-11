import type { Ref } from 'vue'
import type { FetchSessionsResult } from '@/composables/api/useChat'

interface ChatRefreshCoordinatorDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  fetchSessions: (botId: string) => Promise<FetchSessionsResult>
  applySessionsSnapshot: (response: FetchSessionsResult) => void
  isSessionStreaming: (sessionId: string) => boolean
  refreshCurrentSession: (botId: string, sessionId: string) => Promise<void>
}

export function createChatRefreshCoordinator({
  currentBotId,
  sessionId,
  fetchSessions,
  applySessionsSnapshot,
  isSessionStreaming,
  refreshCurrentSession,
}: ChatRefreshCoordinatorDeps) {
  const sessionListRequests = new Map<string, Promise<void>>()
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let scheduledRefreshKey = ''
  let scopeGeneration = 0

  function refreshSessionsList(botId: string): Promise<void> {
    const bid = botId.trim()
    if (!bid) return Promise.resolve()
    const existing = sessionListRequests.get(bid)
    if (existing) return existing

    const generation = scopeGeneration
    const promise = fetchSessions(bid)
      .then((response) => {
        if (generation !== scopeGeneration) return
        if (sessionListRequests.get(bid) !== promise) return
        if ((currentBotId.value ?? '').trim() !== bid) return
        applySessionsSnapshot(response)
      })
      .catch((error) => {
        console.error('Failed to refresh sessions:', error)
      })
      .finally(() => {
        if (sessionListRequests.get(bid) === promise) sessionListRequests.delete(bid)
      })

    sessionListRequests.set(bid, promise)
    return promise
  }

  function scheduleRefreshCurrentSession(expectedSessionId?: string, delay = 100) {
    const bid = (currentBotId.value ?? '').trim()
    const sid = (sessionId.value ?? '').trim()
    if (!bid || !sid) return
    if (expectedSessionId?.trim() && expectedSessionId.trim() !== sid) return
    const key = `${bid}:${sid}`
    if (refreshTimer && scheduledRefreshKey === key) return
    if (refreshTimer) clearTimeout(refreshTimer)

    const generation = scopeGeneration
    scheduledRefreshKey = key
    refreshTimer = setTimeout(() => {
      refreshTimer = null
      scheduledRefreshKey = ''
      if (generation !== scopeGeneration) return
      if ((currentBotId.value ?? '').trim() !== bid || (sessionId.value ?? '').trim() !== sid) return
      if (isSessionStreaming(sid)) return
      void refreshCurrentSession(bid, sid)
    }, delay)
  }

  function resetRefreshCoordinator() {
    scopeGeneration += 1
    sessionListRequests.clear()
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
    scheduledRefreshKey = ''
  }

  return {
    refreshSessionsList,
    scheduleRefreshCurrentSession,
    resetRefreshCoordinator,
  }
}
