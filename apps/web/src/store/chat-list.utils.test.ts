import { describe, expect, it } from 'vitest'
import { shouldRefreshFromMessageCreated, upsertById } from './chat-list.utils'

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
