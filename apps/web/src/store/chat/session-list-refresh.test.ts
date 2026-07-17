import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { FetchSessionsResult } from '@/composables/api/useChat'
import { createSessionListRefresh } from './session-list-refresh'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

function page(id: string): FetchSessionsResult {
  return { items: [{ id, bot_id: 'bot-1', title: id, type: 'chat' }], nextCursor: null }
}

describe('session list refresh', () => {
  it('deduplicates requests and applies only the active bot response', async () => {
    const currentBotId = ref<string | null>('bot-1')
    const request = deferred<FetchSessionsResult>()
    const fetchSessions = vi.fn(() => request.promise)
    const applySnapshot = vi.fn()
    const refresh = createSessionListRefresh({ currentBotId, fetchSessions, applySnapshot })

    const first = refresh.refresh('bot-1')
    const second = refresh.refresh('bot-1')
    request.resolve(page('session-1'))
    await Promise.all([first, second])

    expect(fetchSessions).toHaveBeenCalledOnce()
    expect(applySnapshot).toHaveBeenCalledWith(page('session-1'))
  })

  it('invalidates an in-flight response on reset', async () => {
    const currentBotId = ref<string | null>('bot-1')
    const request = deferred<FetchSessionsResult>()
    const applySnapshot = vi.fn()
    const refresh = createSessionListRefresh({
      currentBotId,
      fetchSessions: () => request.promise,
      applySnapshot,
    })

    const pending = refresh.refresh('bot-1')
    refresh.reset()
    request.resolve(page('stale'))
    await pending

    expect(applySnapshot).not.toHaveBeenCalled()
  })
})
