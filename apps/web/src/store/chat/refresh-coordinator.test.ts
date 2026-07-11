import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { FetchSessionsResult } from '@/composables/api/useChat'
import { createChatRefreshCoordinator } from './refresh-coordinator'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

function page(id: string): FetchSessionsResult {
  return {
    items: [{ id, bot_id: 'bot-1', title: id, type: 'chat' }],
    nextCursor: null,
  }
}

function makeCoordinator() {
  const currentBotId = ref<string | null>('bot-1')
  const sessionId = ref<string | null>('session-1')
  const fetchSessions = vi.fn<(_botId: string) => Promise<FetchSessionsResult>>()
  const applySessionsSnapshot = vi.fn()
  const isSessionStreaming = vi.fn(() => false)
  const refreshCurrentSession = vi.fn(async () => {})
  return {
    currentBotId,
    sessionId,
    fetchSessions,
    applySessionsSnapshot,
    isSessionStreaming,
    refreshCurrentSession,
    coordinator: createChatRefreshCoordinator({
      currentBotId,
      sessionId,
      fetchSessions,
      applySessionsSnapshot,
      isSessionStreaming,
      refreshCurrentSession,
    }),
  }
}

afterEach(() => {
  vi.useRealTimers()
})

describe('chat refresh coordinator', () => {
  it('deduplicates session-list refreshes for the same bot', async () => {
    const { coordinator, fetchSessions, applySessionsSnapshot } = makeCoordinator()
    const request = deferred<FetchSessionsResult>()
    fetchSessions.mockReturnValue(request.promise)

    const first = coordinator.refreshSessionsList('bot-1')
    const second = coordinator.refreshSessionsList('bot-1')
    expect(fetchSessions).toHaveBeenCalledOnce()

    request.resolve(page('session-current'))
    await Promise.all([first, second])
    expect(applySessionsSnapshot).toHaveBeenCalledOnce()
  })

  it('invalidates old responses and lets a new scope request win', async () => {
    const { coordinator, fetchSessions, applySessionsSnapshot } = makeCoordinator()
    const oldRequest = deferred<FetchSessionsResult>()
    const newRequest = deferred<FetchSessionsResult>()
    fetchSessions
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(newRequest.promise)

    const oldRefresh = coordinator.refreshSessionsList('bot-1')
    coordinator.resetRefreshCoordinator()
    const newRefresh = coordinator.refreshSessionsList('bot-1')
    newRequest.resolve(page('session-new'))
    await newRefresh
    oldRequest.resolve(page('session-old'))
    await oldRefresh

    expect(applySessionsSnapshot).toHaveBeenCalledOnce()
    expect(applySessionsSnapshot).toHaveBeenCalledWith(page('session-new'))
  })

  it('ignores a response when its bot is no longer active', async () => {
    const { coordinator, currentBotId, fetchSessions, applySessionsSnapshot } = makeCoordinator()
    const request = deferred<FetchSessionsResult>()
    fetchSessions.mockReturnValue(request.promise)

    const refresh = coordinator.refreshSessionsList('bot-1')
    currentBotId.value = 'bot-2'
    request.resolve(page('session-old'))
    await refresh

    expect(applySessionsSnapshot).not.toHaveBeenCalled()
  })

  it('binds a debounced transcript refresh to its original bot and session', async () => {
    vi.useFakeTimers()
    const { coordinator, sessionId, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleRefreshCurrentSession('session-1')
    sessionId.value = 'session-2'
    await vi.runAllTimersAsync()
    expect(refreshCurrentSession).not.toHaveBeenCalled()

    sessionId.value = 'session-1'
    coordinator.scheduleRefreshCurrentSession('session-1')
    await vi.runAllTimersAsync()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-1')
  })

  it('replaces an old-session debounce when the active session changes', async () => {
    vi.useFakeTimers()
    const { coordinator, sessionId, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleRefreshCurrentSession('session-1')
    sessionId.value = 'session-2'
    coordinator.scheduleRefreshCurrentSession('session-2')
    await vi.runAllTimersAsync()

    expect(refreshCurrentSession).toHaveBeenCalledOnce()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-2')
  })

  it('cancels scheduled refreshes and skips sessions that are still streaming', async () => {
    vi.useFakeTimers()
    const { coordinator, isSessionStreaming, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleRefreshCurrentSession('session-1')
    coordinator.resetRefreshCoordinator()
    await vi.runAllTimersAsync()
    expect(refreshCurrentSession).not.toHaveBeenCalled()

    vi.mocked(isSessionStreaming).mockReturnValue(true)
    coordinator.scheduleRefreshCurrentSession('session-1')
    await vi.runAllTimersAsync()
    expect(refreshCurrentSession).not.toHaveBeenCalled()
  })
})
