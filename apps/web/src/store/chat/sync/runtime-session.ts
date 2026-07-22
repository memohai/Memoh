import {
  awaitSessionRuntimeCheckpoint,
  reduceSessionRuntimeDelta,
  reduceSessionRuntimeSnapshot,
  type SessionRuntimeDeltaEvent,
  type SessionRuntimeReducerState,
  type SessionRuntimeReduction,
} from '@memohai/sdk/session-runtime'
import type { SessionruntimeSnapshot } from '@memohai/sdk'

export interface RuntimeSessionReduction {
  previous: SessionRuntimeReducerState
  reduction: SessionRuntimeReduction
}

export interface RuntimeSessionQueueHandle {
  isCurrent: () => boolean
  get: () => SessionRuntimeReducerState | undefined
  applySnapshot: (
    snapshot: SessionruntimeSnapshot,
    eventSeq?: number,
    eventEpoch?: string,
  ) => RuntimeSessionReduction | undefined
  applyDelta: (
    event: SessionRuntimeDeltaEvent,
    botId: string,
    sessionId: string,
  ) => RuntimeSessionReduction | undefined
  awaitCheckpoint: () => SessionRuntimeReducerState | undefined
}

// Owns reducer state for every session. Callers receive reductions and decide
// which UI effects to run, but cannot feed the reducer without committing the
// resulting state through this owner.
export function createRuntimeSessionStore() {
  const states = new Map<string, SessionRuntimeReducerState>()
  const queueTails = new Map<string, Promise<void>>()
  const keyGenerations = new Map<string, number>()
  let storeGeneration = 0

  interface QueueToken {
    storeGeneration: number
    keyGeneration: number
  }

  function queueToken(key: string): QueueToken {
    return {
      storeGeneration,
      keyGeneration: keyGenerations.get(key) ?? 0,
    }
  }

  function isCurrentQueueToken(key: string, token: QueueToken): boolean {
    return token.storeGeneration === storeGeneration
      && token.keyGeneration === (keyGenerations.get(key) ?? 0)
  }

  function get(key: string): SessionRuntimeReducerState | undefined {
    return states.get(key)
  }

  function applySnapshot(
    key: string,
    snapshot: SessionruntimeSnapshot,
    eventSeq?: number,
    eventEpoch?: string,
  ): RuntimeSessionReduction {
    const previous = states.get(key) ?? {}
    const reduction = reduceSessionRuntimeSnapshot(previous, snapshot, eventSeq, eventEpoch)
    states.set(key, reduction.state)
    return { previous, reduction }
  }

  function applyDelta(
    key: string,
    event: SessionRuntimeDeltaEvent,
    botId: string,
    sessionId: string,
  ): RuntimeSessionReduction {
    const previous = states.get(key) ?? {}
    const reduction = reduceSessionRuntimeDelta(previous, event, botId, sessionId)
    states.set(key, reduction.state)
    return { previous, reduction }
  }

  function awaitCheckpoint(key: string): SessionRuntimeReducerState {
    const next = awaitSessionRuntimeCheckpoint(states.get(key) ?? {})
    states.set(key, next)
    return next
  }

  function queueHandle(key: string, token: QueueToken): RuntimeSessionQueueHandle {
    const isCurrent = () => isCurrentQueueToken(key, token)
    return {
      isCurrent,
      get: () => isCurrent() ? states.get(key) : undefined,
      applySnapshot: (snapshot, eventSeq, eventEpoch) => isCurrent()
        ? applySnapshot(key, snapshot, eventSeq, eventEpoch)
        : undefined,
      applyDelta: (event, botId, sessionId) => isCurrent()
        ? applyDelta(key, event, botId, sessionId)
        : undefined,
      awaitCheckpoint: () => isCurrent() ? awaitCheckpoint(key) : undefined,
    }
  }

  // The callback owns one serialized session operation, including any async
  // hydration it performs before committing through the guarded handle.
  function enqueue<T>(
    key: string,
    operation: (session: RuntimeSessionQueueHandle) => T | Promise<T>,
  ): T | Promise<T | undefined> | undefined {
    const token = queueToken(key)
    const previous = queueTails.get(key)
    if (previous) {
      const result = previous.then(async () => {
        if (!isCurrentQueueToken(key, token)) return undefined
        return operation(queueHandle(key, token))
      })
      const tail = result.then(
        () => undefined,
        () => undefined,
      )
      queueTails.set(key, tail)
      void tail.then(() => {
        if (queueTails.get(key) === tail) queueTails.delete(key)
      })
      return result
    }

    if (!isCurrentQueueToken(key, token)) return undefined
    const immediate = operation(queueHandle(key, token))
    if (!immediate || typeof (immediate as Promise<T>).then !== 'function') return immediate

    const result = Promise.resolve(immediate)
    const tail = result.then(
      () => undefined,
      () => undefined,
    )
    queueTails.set(key, tail)
    void tail.then(() => {
      if (queueTails.get(key) === tail) queueTails.delete(key)
    })
    return result
  }

  async function flush(key?: string): Promise<void> {
    if (key !== undefined) {
      for (;;) {
        const tail = queueTails.get(key)
        if (!tail) return
        await tail
        if (queueTails.get(key) === tail) return
      }
    }

    for (;;) {
      const tails = [...queueTails.values()]
      if (tails.length === 0) return
      await Promise.all(tails)
    }
  }

  function remove(key: string) {
    states.delete(key)
    keyGenerations.set(key, (keyGenerations.get(key) ?? 0) + 1)
    queueTails.delete(key)
  }

  function clear() {
    storeGeneration += 1
    states.clear()
    keyGenerations.clear()
    queueTails.clear()
  }

  function keys(): IterableIterator<string> {
    return states.keys()
  }

  return {
    get,
    enqueue,
    flush,
    remove,
    clear,
    keys,
  }
}
