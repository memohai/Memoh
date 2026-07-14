import { describe, expect, it } from 'vitest'
import { captureChatPaneSendContext, matchesChatPaneSendContext } from './chat-pane-send'

describe('chat pane send context', () => {
  it('keeps the original target after an ephemeral pane is repointed', () => {
    const target = {
      botId: 'bot-1',
      sessionId: 'session-a',
      viewId: 'chat:1',
    }
    const context = captureChatPaneSendContext(target, 'bot-1:chat:1')

    target.sessionId = 'session-b'

    expect(context.target).toEqual({
      botId: 'bot-1',
      sessionId: 'session-a',
      viewId: 'chat:1',
    })
    expect(Object.isFrozen(context.target)).toBe(true)
  })

  it('restores a failed attachment conversion only into the original composer', () => {
    const context = captureChatPaneSendContext({
      botId: 'bot-1',
      sessionId: 'session-a',
      viewId: 'chat:1',
    }, 'bot-1:chat:1')

    expect(matchesChatPaneSendContext(context, {
      botId: 'bot-1',
      sessionId: 'session-a',
      viewId: 'chat:1',
    }, 'bot-1:chat:1')).toBe(true)
    expect(matchesChatPaneSendContext(context, {
      botId: 'bot-1',
      sessionId: 'session-b',
      viewId: 'chat:1',
    }, 'bot-1:chat:1')).toBe(false)
    expect(matchesChatPaneSendContext(context, {
      botId: 'bot-1',
      sessionId: 'session-a',
      viewId: 'chat:2',
    }, 'bot-1:chat:2')).toBe(false)
  })
})
