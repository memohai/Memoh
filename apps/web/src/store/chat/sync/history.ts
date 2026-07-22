import type { UITurn } from '@/composables/api/useChat.types'

export interface PersistedHistoryRef {
  botId: string
  sessionId: string
}

interface PersistedHistoryReconcilerDeps {
  fetchLatest: (ref: PersistedHistoryRef) => Promise<UITurn[]>
  apply: (ref: PersistedHistoryRef, turns: UITurn[]) => void | Promise<void>
  onError?: (ref: PersistedHistoryRef, error: unknown) => void
}

interface PendingHistoryReconcile {
  dirty: boolean
  promise: Promise<void>
}

function normalize(ref: PersistedHistoryRef): PersistedHistoryRef | null {
  const botId = ref.botId.trim()
  const sessionId = ref.sessionId.trim()
  return botId && sessionId ? { botId, sessionId } : null
}

function keyOf(ref: PersistedHistoryRef) {
  return `${ref.botId}\u0000${ref.sessionId}`
}

// Reconciles persisted-only events such as channel messages. Runtime-owned
// turns never enter this path; their one-time authority handoff is settle.ts.
export function createPersistedHistoryReconciler({
  fetchLatest,
  apply,
  onError,
}: PersistedHistoryReconcilerDeps) {
  const pending = new Map<string, PendingHistoryReconcile>()
  let generation = 0

  function reconcile(input: PersistedHistoryRef): Promise<void> {
    const ref = normalize(input)
    if (!ref) return Promise.resolve()
    const key = keyOf(ref)
    const existing = pending.get(key)
    if (existing) {
      existing.dirty = true
      return existing.promise
    }

    const requestGeneration = generation
    const entry: PendingHistoryReconcile = {
      dirty: false,
      promise: Promise.resolve(),
    }
    entry.promise = (async () => {
      do {
        entry.dirty = false
        const turns = await fetchLatest(ref)
        if (requestGeneration !== generation || pending.get(key) !== entry) return
        await apply(ref, turns)
        if (requestGeneration !== generation || pending.get(key) !== entry) return
      } while (entry.dirty)
    })().catch((error) => {
      onError?.(ref, error)
      throw error
    }).finally(() => {
      if (pending.get(key) === entry) pending.delete(key)
    })
    pending.set(key, entry)
    return entry.promise
  }

  function cancel(input: PersistedHistoryRef) {
    const ref = normalize(input)
    if (!ref) return
    pending.delete(keyOf(ref))
  }

  function reset() {
    generation += 1
    pending.clear()
  }

  return { reconcile, cancel, reset }
}
