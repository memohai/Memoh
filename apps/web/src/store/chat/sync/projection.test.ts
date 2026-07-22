import { describe, expect, it, vi } from 'vitest'
import {
  createSynchronizedTranscript,
  disposeSynchronizedTranscript,
  projectRuntimeMessages,
  promoteSynchronizedTranscript,
  readSynchronizedTranscript,
} from './projection'

function transcript(sessionId: string | null) {
  return createSynchronizedTranscript({ botId: 'bot-1', sessionId, viewId: 'chat' }, {
    rememberBackgroundTask: task => task,
    applyPendingBackgroundEventsToTool: vi.fn(),
    mergeBackgroundTaskIntoMatchingTools: vi.fn(),
    bumpFsChangedAtIfFsMutation: vi.fn(),
    fetchMessages: vi.fn(async () => []),
    locateMessage: vi.fn(async () => ({ items: [] })),
  }, {
    onSnapshot: vi.fn(),
    onRefreshApplied: vi.fn(),
  })
}

describe('runtime projection', () => {
  it('preserves durable row identity and ordering on every derived block', () => {
    const messages = projectRuntimeMessages([{
      id: 0,
      stable_id: 'assistant-row',
      turn_position: 8,
      turn_message_seq: 2,
      type: 'text',
      content: 'answer',
    }])

    expect(messages).toEqual([{
      id: 0,
      stable_id: 'assistant-row',
      turn_position: 8,
      turn_message_seq: 2,
      type: 'text',
      content: 'answer',
    }])
  })

  it('promotes drafts by durable identity without content matching', () => {
    const target = transcript('session-1')
    const source = transcript(null)
    target.appendToView({
      id: 'persisted-render',
      serverId: 'row-1',
      turnPosition: 3,
      turnMessageSeq: 1,
      role: 'user',
      text: 'same text',
      attachments: [],
      timestamp: '2026-07-16T00:00:00Z',
      streaming: false,
      isSelf: true,
    })
    source.appendToView(
      {
        id: 'optimistic-twin',
        serverId: 'row-1',
        turnPosition: 3,
        turnMessageSeq: 1,
        role: 'user',
        text: 'different projection',
        attachments: [],
        timestamp: '2026-07-16T00:00:01Z',
        streaming: false,
        isSelf: true,
      },
      {
        id: 'different-row',
        role: 'user',
        text: 'same text',
        attachments: [],
        timestamp: '2026-07-16T00:00:02Z',
        streaming: false,
        isSelf: true,
      },
    )

    promoteSynchronizedTranscript(target, source)

    expect(target.messages.map(turn => turn.id)).toEqual(['persisted-render', 'different-row'])
  })

  it('disposes transcript state through the sync owner', () => {
    const target = transcript('session-1')
    target.appendToView({
      id: 'turn-1',
      role: 'user',
      text: 'hello',
      attachments: [],
      timestamp: '2026-07-16T00:00:00Z',
      streaming: false,
      isSelf: true,
    })

    disposeSynchronizedTranscript(target)

    expect(target.messages).toEqual([])
  })

  it('exposes a readonly transcript view to render consumers', () => {
    const target = transcript('session-1')
    const view = readSynchronizedTranscript(target)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    view.messages.push({
      id: 'forbidden',
      role: 'user',
      text: 'must not be inserted',
      attachments: [],
      timestamp: '2026-07-16T00:00:00Z',
      streaming: false,
      isSelf: true,
    })

    expect(target.messages).toEqual([])
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })
})
