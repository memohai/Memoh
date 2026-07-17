import { describe, expect, it } from 'vitest'
import type { SessionruntimeSnapshot } from '@memohai/sdk'
import { createRuntimeSessionStore } from './runtime-session'

function snapshot(seq: number): SessionruntimeSnapshot {
  return {
    bot_id: 'bot-1',
    session_id: 'session-1',
    epoch: 'epoch-1',
    seq,
    queue: [],
  }
}

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

describe('runtime session store', () => {
  it('commits an applied snapshot under the session key', async () => {
    const store = createRuntimeSessionStore()

    const result = await store.enqueue('bot-1\0session-1', session =>
      session.applySnapshot(snapshot(1), 1, 'epoch-1'))

    expect(result?.reduction.kind).toBe('applied')
    expect(store.get('bot-1\0session-1')?.seq).toBe(1)
  })

  it('commits resync state when a delta has a sequence gap', async () => {
    const store = createRuntimeSessionStore()
    const key = 'bot-1\0session-1'
    await store.enqueue(key, session => session.applySnapshot(snapshot(1), 1, 'epoch-1'))

    const result = await store.enqueue(key, session => session.applyDelta({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 3,
      delta: { reset_messages: true },
    }, 'bot-1', 'session-1'))

    expect(result?.reduction).toMatchObject({ kind: 'resync', reason: 'sequence_gap' })
    expect(store.get(key)?.phase).toBe('awaiting_checkpoint')
  })

  it('preserves row identity metadata carried by text appends', async () => {
    const store = createRuntimeSessionStore()
    const key = 'bot-1\0session-1'
    await store.enqueue(key, session => session.applySnapshot({
      ...snapshot(1),
      current_run_view: {
        stream_id: 'stream-1',
        generation: 'generation-1',
        status: 'running',
        messages: [],
      },
    }, 1, 'epoch-1'))

    await store.enqueue(key, session => session.applyDelta({
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-1',
      stream_id: 'stream-1',
      seq: 2,
      delta: {
        message_appends: [{
          id: 0,
          type: 'text',
          content: 'hello',
          stable_id: 'assistant-row',
          turn_position: 4,
          turn_message_seq: 2,
          row_identities: [{
            stable_id: 'assistant-row',
            role: 'assistant',
            turn_id: 'turn-4',
            turn_position: 4,
            turn_message_seq: 2,
          }],
        }],
      },
    }, 'bot-1', 'session-1'))

    const message = store.get(key)?.snapshot?.current_run_view?.messages?.[0] as
      | { stable_id?: string, row_identities?: Array<{ stable_id: string }> }
      | undefined
    expect(message).toMatchObject({
      stable_id: 'assistant-row',
      row_identities: [{ stable_id: 'assistant-row' }],
    })
  })

  it('keeps the last snapshot while awaiting a new checkpoint', async () => {
    const store = createRuntimeSessionStore()
    const key = 'bot-1\0session-1'
    await store.enqueue(key, session => session.applySnapshot(snapshot(4), 4, 'epoch-1'))

    const state = await store.enqueue(key, session => session.awaitCheckpoint())

    expect(state.snapshot?.seq).toBe(4)
    expect(state.phase).toBe('awaiting_checkpoint')
  })

  it('serializes hydration, snapshot, delta, and checkpoint work per session', async () => {
    const store = createRuntimeSessionStore()
    const key = 'bot-1\0session-1'
    const hydration = deferred()
    const order: string[] = []

    const hydrate = store.enqueue(key, async (session) => {
      order.push('hydrate:start')
      await hydration.promise
      session.applySnapshot(snapshot(1), 1, 'epoch-1')
      order.push('hydrate:end')
    })
    const applyDelta = store.enqueue(key, (session) => {
      order.push('delta')
      session.applyDelta({
        type: 'runtime_delta',
        bot_id: 'bot-1',
        session_id: 'session-1',
        epoch: 'epoch-1',
        stream_id: 'stream-1',
        seq: 2,
        delta: {
          current_run_view: {
            generation: 'generation-1',
            stream_id: 'stream-1',
            status: 'running',
            messages: [],
          },
        },
      }, 'bot-1', 'session-1')
    })
    const checkpoint = store.enqueue(key, (session) => {
      order.push('checkpoint')
      session.awaitCheckpoint()
    })

    await Promise.resolve()
    expect(order).toEqual(['hydrate:start'])

    hydration.resolve()
    await Promise.all([hydrate, applyDelta, checkpoint])
    await store.flush(key)

    expect(order).toEqual(['hydrate:start', 'hydrate:end', 'delta', 'checkpoint'])
    expect(store.get(key)).toMatchObject({ seq: 2, phase: 'awaiting_checkpoint' })
  })

  it('lets different sessions dispatch independently', async () => {
    const store = createRuntimeSessionStore()
    const blocked = deferred()
    const order: string[] = []

    const first = store.enqueue('bot-1\0session-1', async () => {
      order.push('first:start')
      await blocked.promise
      order.push('first:end')
    })
    const second = store.enqueue('bot-1\0session-2', (session) => {
      order.push('second')
      session.applySnapshot({ ...snapshot(1), session_id: 'session-2' }, 1, 'epoch-1')
    })

    await second
    expect(order).toEqual(['first:start', 'second'])

    blocked.resolve()
    await first
    await store.flush()
    expect(order).toEqual(['first:start', 'second', 'first:end'])
  })

  it('invalidates running and queued work when a session is removed', async () => {
    const store = createRuntimeSessionStore()
    const key = 'bot-1\0session-1'
    const blocked = deferred()
    let staleQueuedRan = false

    const staleRunning = store.enqueue(key, async (session) => {
      await blocked.promise
      session.applySnapshot(snapshot(1), 1, 'epoch-1')
    })
    const staleQueued = store.enqueue(key, () => {
      staleQueuedRan = true
    })

    await Promise.resolve()
    store.remove(key)
    await store.enqueue(key, session => session.applySnapshot({ ...snapshot(8), epoch: 'epoch-2' }, 8, 'epoch-2'))
    blocked.resolve()
    await Promise.all([staleRunning, staleQueued])

    expect(staleQueuedRan).toBe(false)
    expect(store.get(key)).toMatchObject({ seq: 8, epoch: 'epoch-2' })
  })

  it('invalidates old work across all sessions when the store is cleared', async () => {
    const store = createRuntimeSessionStore()
    const firstKey = 'bot-1\0session-1'
    const secondKey = 'bot-1\0session-2'
    const blocked = deferred()

    const staleFirst = store.enqueue(firstKey, async (session) => {
      await blocked.promise
      session.applySnapshot(snapshot(1), 1, 'epoch-1')
    })
    const staleSecond = store.enqueue(secondKey, async (session) => {
      await blocked.promise
      session.applySnapshot({ ...snapshot(1), session_id: 'session-2' }, 1, 'epoch-1')
    })

    await Promise.resolve()
    store.clear()
    await store.enqueue(firstKey, session => session.applySnapshot({ ...snapshot(9), epoch: 'epoch-2' }, 9, 'epoch-2'))
    blocked.resolve()
    await Promise.all([staleFirst, staleSecond])

    expect(store.get(firstKey)).toMatchObject({ seq: 9, epoch: 'epoch-2' })
    expect(store.get(secondKey)).toBeUndefined()
  })
})
