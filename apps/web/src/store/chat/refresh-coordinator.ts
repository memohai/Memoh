import type { Ref } from 'vue'
import type { FetchSessionsResult } from '@/composables/api/useChat'

interface ChatRefreshCoordinatorDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  fetchSessions: (botId: string) => Promise<FetchSessionsResult>
  applySessionsSnapshot: (response: FetchSessionsResult) => void
  isSessionStreaming: (botId: string, sessionId: string) => boolean
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
  const sessionRefreshTimers = new Map<string, ReturnType<typeof setTimeout>>()
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

  function scheduleSessionRefresh(botId: string, targetSessionId: string, delay = 100) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    const key = `${bid}:${sid}`
    const previousTimer = sessionRefreshTimers.get(key)
    if (previousTimer) clearTimeout(previousTimer)

    const generation = scopeGeneration
    const timer = setTimeout(() => {
      if (sessionRefreshTimers.get(key) === timer) {
        sessionRefreshTimers.delete(key)
      }
      if (generation !== scopeGeneration) return
      if ((currentBotId.value ?? '').trim() !== bid) return
      if (isSessionStreaming(bid, sid)) return
      void refreshCurrentSession(bid, sid)
    }, delay)
    sessionRefreshTimers.set(key, timer)
  }

  // Compatibility for callers that still mean "the session focused right now".
  // Capture that target at schedule time; later focus changes must not redirect
  // or cancel the queued refresh.
  function scheduleRefreshCurrentSession(expectedSessionId?: string, delay = 100) {
    const bid = (currentBotId.value ?? '').trim()
    const sid = (sessionId.value ?? '').trim()
    if (!bid || !sid) return
    if (expectedSessionId?.trim() && expectedSessionId.trim() !== sid) return
    scheduleSessionRefresh(bid, sid, delay)
  }

  function resetRefreshCoordinator() {
    scopeGeneration += 1
    sessionListRequests.clear()
    for (const timer of sessionRefreshTimers.values()) {
      clearTimeout(timer)
    }
    sessionRefreshTimers.clear()
  }

  return {
    refreshSessionsList,
    scheduleSessionRefresh,
    scheduleRefreshCurrentSession,
    resetRefreshCoordinator,
  }
}
