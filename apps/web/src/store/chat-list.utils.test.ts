import { describe, expect, it } from 'vitest'
import { reconcileById, shouldRefreshFromMessageCreated, upsertById } from './chat-list.utils'

describe('chat-list.utils', () => {
  it('replaces existing item with same id and preserves order', () => {
    const items = [
      { id: 2, content: 'second' },
      { id: 4, content: 'fourth' },
    ]

    expect(upsertById(items, { id: 2, content: 'updated' })).toEqual([
      { id: 2, content: 'updated' },
      { id: 4, content: 'fourth' },
    ])
  })

  it('inserts new item and sorts by id', () => {
    const items = [
      { id: 4, content: 'fourth' },
      { id: 8, content: 'eighth' },
    ]

    expect(upsertById(items, { id: 6, content: 'sixth' })).toEqual([
      { id: 4, content: 'fourth' },
      { id: 6, content: 'sixth' },
      { id: 8, content: 'eighth' },
    ])
  })

  it('updates an existing item in place, preserving array and item identity', () => {
    const original = { id: 2, content: 'second' }
    const items = [original, { id: 4, content: 'fourth' }]

    const result = upsertById(items, { id: 2, content: 'updated' })

    expect(result).toBe(items)
    expect(result[0]).toBe(original)
    expect(original.content).toBe('updated')
  })

  it('drops fields absent from the incoming snapshot when updating in place', () => {
    const original: { id: number; content: string; stale?: boolean } = {
      id: 2,
      content: 'second',
      stale: true,
    }
    const items = [original]

    upsertById(items, { id: 2, content: 'updated' })

    expect(original).toEqual({ id: 2, content: 'updated' })
  })

  it('reconcileById reuses matched items in place and follows incoming order', () => {
    const a = { id: 1, v: 'a' }
    const b = { id: 2, v: 'b' }
    const target = [a, b]

    const result = reconcileById(target, [
      { id: 2, v: 'b2' },
      { id: 1, v: 'a2' },
    ])

    expect(result).toBe(target)
    expect(result[0]).toBe(b)
    expect(result[1]).toBe(a)
    expect(a.v).toBe('a2')
    expect(b.v).toBe('b2')
    expect(result.map(x => x.id)).toEqual([2, 1])
  })

  it('reconcileById drops items absent from incoming and inserts new ones', () => {
    const a = { id: 1, v: 'a' }
    const target = [a, { id: 2, v: 'b' }]

    const result = reconcileById(target, [
      { id: 1, v: 'a' },
      { id: 3, v: 'c' },
    ])

    expect(result[0]).toBe(a)
    expect(result.map(x => x.id)).toEqual([1, 3])
  })

  it('reconcileById matches existing items via a custom key', () => {
    const optimistic = { id: 'client-1', serverId: 'server-1', v: 'old' }
    const target: Array<{ id: string; serverId?: string; v: string }> = [optimistic]

    reconcileById(target, [{ id: 'server-1', v: 'new' }], {
      keyOfExisting: item => item.serverId ?? item.id,
    })

    expect(target[0]).toBe(optimistic)
    expect(optimistic.v).toBe('new')
  })

  it('reconcileById applies a custom merge to matched items', () => {
    const a = { id: 1, items: ['x'] }
    const target = [a]

    reconcileById(target, [{ id: 1, items: ['x', 'y'] }], {
      merge: (cur, inc) => {
        cur.items = inc.items
      },
    })

    expect(target[0]).toBe(a)
    expect(a.items).toEqual(['x', 'y'])
  })

  it('refreshes only for current session message_created events', () => {
    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm1',
        bot_id: 'bot-1',
        session_id: 'session-1',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(true)

    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm2',
        bot_id: 'bot-1',
        session_id: 'session-2',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(false)

    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', null, {
      type: 'session_title_updated',
      bot_id: 'bot-1',
      session_id: 'session-1',
      title: 'new title',
    })).toBe(false)
  })

  it('does not refresh current session while a local stream is active', () => {
    expect(shouldRefreshFromMessageCreated('bot-1', 'session-1', 'session-1', {
      type: 'message_created',
      bot_id: 'bot-1',
      message: {
        id: 'm3',
        bot_id: 'bot-1',
        session_id: 'session-1',
        role: 'user',
        content: 'hello',
        created_at: '2026-04-10T10:00:00Z',
      },
    })).toBe(false)
  })
})
