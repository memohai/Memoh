import { describe, expect, it, vi } from 'vitest'
import type { UITurn } from '@/composables/api/useChat.types'
import { createPersistedHistoryReconciler } from './history'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

const ref = { botId: 'bot-1', sessionId: 'session-1' }
const history: UITurn[] = [{ id: 'row-1', role: 'user', text: 'hello', timestamp: '2026-07-16T00:00:00Z' }]

describe('persisted history reconciler', () => {
  it('coalesces concurrent notifications and reruns once when dirtied', async () => {
    const first = deferred<UITurn[]>()
    const fetchLatest = vi.fn()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce(history)
    const apply = vi.fn()
    const reconciler = createPersistedHistoryReconciler({ fetchLatest, apply })

    const a = reconciler.reconcile(ref)
    const b = reconciler.reconcile(ref)
    first.resolve(history)
    await Promise.all([a, b])

    expect(fetchLatest).toHaveBeenCalledTimes(2)
    expect(apply).toHaveBeenCalledTimes(2)
  })

  it('invalidates in-flight work on reset', async () => {
    const request = deferred<UITurn[]>()
    const apply = vi.fn()
    const reconciler = createPersistedHistoryReconciler({ fetchLatest: () => request.promise, apply })

    const pending = reconciler.reconcile(ref)
    reconciler.reset()
    request.resolve(history)
    await pending

    expect(apply).not.toHaveBeenCalled()
  })

  it('cancels one in-flight fetch without invalidating a newer reconcile for that session', async () => {
    const stale = deferred<UITurn[]>()
    const fresh = deferred<UITurn[]>()
    const freshHistory: UITurn[] = [{
      id: 'row-2',
      role: 'user',
      text: 'fresh',
      timestamp: '2026-07-16T00:00:01Z',
    }]
    const fetchLatest = vi.fn()
      .mockReturnValueOnce(stale.promise)
      .mockReturnValueOnce(fresh.promise)
    const apply = vi.fn()
    const reconciler = createPersistedHistoryReconciler({ fetchLatest, apply })

    const staleReconcile = reconciler.reconcile(ref)
    reconciler.cancel({ botId: ' bot-1 ', sessionId: ' session-1 ' })
    const freshReconcile = reconciler.reconcile(ref)
    stale.resolve(history)
    fresh.resolve(freshHistory)
    await Promise.all([staleReconcile, freshReconcile])

    expect(fetchLatest).toHaveBeenCalledTimes(2)
    expect(apply).toHaveBeenCalledOnce()
    expect(apply).toHaveBeenCalledWith(ref, freshHistory)
  })

  it('does not run a dirty follow-up after cancellation while apply is pending', async () => {
    const applying = deferred<void>()
    const fetchLatest = vi.fn().mockResolvedValue(history)
    const apply = vi.fn(() => applying.promise)
    const reconciler = createPersistedHistoryReconciler({ fetchLatest, apply })

    const first = reconciler.reconcile(ref)
    await vi.waitFor(() => expect(apply).toHaveBeenCalledOnce())
    const dirty = reconciler.reconcile(ref)
    reconciler.cancel(ref)
    applying.resolve()
    await Promise.all([first, dirty])

    expect(fetchLatest).toHaveBeenCalledOnce()
    expect(apply).toHaveBeenCalledOnce()
  })

  it('reports and propagates fetch failures to callers that must fence a runtime operation', async () => {
    const error = new Error('history unavailable')
    const onError = vi.fn()
    const reconciler = createPersistedHistoryReconciler({
      fetchLatest: () => Promise.reject(error),
      apply: vi.fn(),
      onError,
    })

    await expect(reconciler.reconcile(ref)).rejects.toBe(error)
    expect(onError).toHaveBeenCalledWith(ref, error)
  })
})
