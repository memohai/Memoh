import { describe, expect, it, vi } from 'vitest'
import { fetchAllSessions, fetchSession, fetchSessions, fetchSessionsPage } from './useChat.chat-api'

const sdk = vi.hoisted(() => ({
  getBots: vi.fn(),
  getBotsByBotIdSessions: vi.fn(),
  getBotsByBotIdSessionsBySessionId: vi.fn(),
  postBotsByBotIdSessions: vi.fn(),
  patchBotsByBotIdSessionsBySessionId: vi.fn(),
  postBotsByBotIdSessionsBySessionIdAcpRuntime: vi.fn(),
  getBotsByBotIdSessionsBySessionIdAcpRuntime: vi.fn(),
  patchBotsByBotIdSessionsBySessionIdAcpRuntimeModel: vi.fn(),
  postBotsByBotIdAcpRuntimes: vi.fn(),
  patchBotsByBotIdAcpRuntimesByRuntimeIdModel: vi.fn(),
  getBotsByBotIdAcpRuntimesByRuntimeId: vi.fn(),
  deleteBotsByBotIdAcpRuntimesByRuntimeId: vi.fn(),
  deleteBotsByBotIdSessionsBySessionId: vi.fn(),
  deleteBotsByBotIdMessages: vi.fn(),
}))

vi.mock('@memohai/sdk', () => sdk)

describe('chat api sessions', () => {
  it('omits query params for the default session list', async () => {
    sdk.getBotsByBotIdSessions.mockResolvedValueOnce({ data: { items: [] } })

    await fetchSessions(' bot-1 ')

    expect(sdk.getBotsByBotIdSessions).toHaveBeenCalledWith({
      path: { bot_id: 'bot-1' },
      throwOnError: true,
    })
  })

  it('passes type and parent filters to the generated sessions endpoint', async () => {
    sdk.getBotsByBotIdSessions.mockResolvedValueOnce({ data: { items: [] } })

    await fetchSessions(' bot-1 ', {
      types: ['subagent'],
      parentSessionId: 'parent-1',
      limit: 100,
    })

    expect(sdk.getBotsByBotIdSessions).toHaveBeenCalledWith({
      path: { bot_id: 'bot-1' },
      query: {
        types: 'subagent',
        parent_session_id: 'parent-1',
        limit: 100,
      },
      throwOnError: true,
    })
  })

  it('returns next cursor for paged session lists', async () => {
    sdk.getBotsByBotIdSessions.mockResolvedValueOnce({
      data: {
        items: [{ id: 'session-1', bot_id: 'bot-1', title: 'One', type: 'chat' }],
        next_cursor: 'cursor-2',
      },
    })

    const page = await fetchSessionsPage('bot-1', { limit: 1 })

    expect(page).toEqual({
      items: [{ id: 'session-1', bot_id: 'bot-1', title: 'One', type: 'chat' }],
      nextCursor: 'cursor-2',
    })
  })

  it('follows next_cursor when fetching all sessions', async () => {
    sdk.getBotsByBotIdSessions
      .mockResolvedValueOnce({
        data: {
          items: [{ id: 'child-1', bot_id: 'bot-1', title: 'One', type: 'subagent' }],
          next_cursor: 'cursor-2',
        },
      })
      .mockResolvedValueOnce({
        data: {
          items: [{ id: 'child-2', bot_id: 'bot-1', title: 'Two', type: 'subagent' }],
        },
      })

    const items = await fetchAllSessions('bot-1', {
      types: ['subagent'],
      parentSessionId: 'parent-1',
      limit: 100,
    })

    expect(items.map(item => item.id)).toEqual(['child-1', 'child-2'])
    expect(sdk.getBotsByBotIdSessions).toHaveBeenLastCalledWith({
      path: { bot_id: 'bot-1' },
      query: {
        types: 'subagent',
        parent_session_id: 'parent-1',
        limit: 100,
        cursor: 'cursor-2',
      },
      throwOnError: true,
    })
  })

  it('fetches a single session summary by id', async () => {
    sdk.getBotsByBotIdSessionsBySessionId.mockResolvedValueOnce({
      data: { id: 'child-1', bot_id: 'bot-1', title: 'Child', type: 'subagent' },
    })

    const session = await fetchSession(' bot-1 ', ' child-1 ')

    expect(session.type).toBe('subagent')
    expect(sdk.getBotsByBotIdSessionsBySessionId).toHaveBeenCalledWith({
      path: { bot_id: 'bot-1', session_id: 'child-1' },
      throwOnError: true,
    })
  })
})
