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

  it('binds the compatibility refresh to its original bot and session', async () => {
    vi.useFakeTimers()
    const { coordinator, sessionId, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleRefreshCurrentSession('session-1')
    sessionId.value = 'session-2'
    await vi.runAllTimersAsync()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-1')
  })

  it('schedules transcript refreshes independently for different sessions', async () => {
    vi.useFakeTimers()
    const { coordinator, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    coordinator.scheduleSessionRefresh('bot-1', 'session-b')
    await vi.runAllTimersAsync()

    expect(refreshCurrentSession).toHaveBeenCalledTimes(2)
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-a')
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-b')
  })

  it('debounces each session key without delaying other session keys', async () => {
    vi.useFakeTimers()
    const { coordinator, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    coordinator.scheduleSessionRefresh('bot-1', 'session-b')
    await vi.advanceTimersByTimeAsync(50)
    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    await vi.advanceTimersByTimeAsync(50)

    expect(refreshCurrentSession).toHaveBeenCalledOnce()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-b')

    await vi.advanceTimersByTimeAsync(50)
    expect(refreshCurrentSession).toHaveBeenCalledTimes(2)
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-a')
  })

  it('cancels all scheduled session refreshes on reset', async () => {
    vi.useFakeTimers()
    const { coordinator, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    coordinator.scheduleSessionRefresh('bot-1', 'session-b')
    coordinator.resetRefreshCoordinator()
    await vi.runAllTimersAsync()

    expect(refreshCurrentSession).not.toHaveBeenCalled()
  })

  it('skips only the target sessions that are still streaming', async () => {
    vi.useFakeTimers()
    const { coordinator, isSessionStreaming, refreshCurrentSession } = makeCoordinator()

    vi.mocked(isSessionStreaming).mockImplementation((botId, session) =>
      botId === 'bot-1' && session === 'session-a',
    )
    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    coordinator.scheduleSessionRefresh('bot-1', 'session-b')
    await vi.runAllTimersAsync()

    expect(refreshCurrentSession).toHaveBeenCalledOnce()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'session-b')
  })

  it('does not confuse the same session id across different bots', async () => {
    vi.useFakeTimers()
    const { coordinator, isSessionStreaming, refreshCurrentSession } = makeCoordinator()

    vi.mocked(isSessionStreaming).mockImplementation((botId, session) =>
      botId === 'bot-2' && session === 'shared-session',
    )
    coordinator.scheduleSessionRefresh('bot-1', 'shared-session')
    await vi.runAllTimersAsync()

    expect(isSessionStreaming).toHaveBeenCalledWith('bot-1', 'shared-session')
    expect(refreshCurrentSession).toHaveBeenCalledOnce()
    expect(refreshCurrentSession).toHaveBeenCalledWith('bot-1', 'shared-session')
  })

  it('drops a scheduled session refresh when its bot is no longer active', async () => {
    vi.useFakeTimers()
    const { coordinator, currentBotId, refreshCurrentSession } = makeCoordinator()

    coordinator.scheduleSessionRefresh('bot-1', 'session-a')
    currentBotId.value = 'bot-2'
    await vi.runAllTimersAsync()

    expect(refreshCurrentSession).not.toHaveBeenCalled()
  })
})
