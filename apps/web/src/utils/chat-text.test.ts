import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '@/store/chat-list'
import { canForkMessage, cleanUserText, getMessageCopyText } from './chat-text'

const ASSISTANT_MESSAGE_ID = '11111111-1111-4111-8111-111111111111'

function userMessage(text: string): ChatMessage {
  return {
    id: 'user-1',
    role: 'user',
    text,
    attachments: [],
    timestamp: '2026-01-01T00:00:00Z',
    streaming: false,
    isSelf: true,
  }
}

function assistantMessage(
  blocks: Array<{ type: 'text' | 'reasoning'; content: string }>,
  streaming = false,
  options: { id?: string; serverId?: string } = {},
): ChatMessage {
  return {
    id: options.id ?? ASSISTANT_MESSAGE_ID,
    serverId: options.serverId,
    role: 'assistant',
    messages: blocks.map((block, index) => ({
      id: index + 1,
      type: block.type,
      content: block.content,
      running: false,
    })),
    timestamp: '2026-01-01T00:00:00Z',
    streaming,
  }
}

describe('chat text actions', () => {
  it('strips attachment markers from copied user messages', () => {
    const message = userMessage([
      '[attachment:image] photo.png',
      'Please inspect this.',
      '  [attachment:file] notes.txt',
      'Thanks.',
    ].join('\n'))

    expect(cleanUserText(message.text)).toBe('Please inspect this.\nThanks.')
    expect(getMessageCopyText(message)).toBe('Please inspect this.\nThanks.')
  })

  it('joins copied assistant text blocks with blank lines', () => {
    const message = assistantMessage([
      { type: 'text', content: ' First block ' },
      { type: 'reasoning', content: 'hidden' },
      { type: 'text', content: '\nSecond block\n' },
    ])

    expect(getMessageCopyText(message)).toBe('First block\n\nSecond block')
  })

  it('allows fork only for non-streaming assistant messages with text', () => {
    expect(canForkMessage(assistantMessage([{ type: 'text', content: 'reply' }]))).toBe(true)
    expect(canForkMessage(assistantMessage([{ type: 'text', content: 'reply' }], true))).toBe(false)
    expect(canForkMessage(assistantMessage([{ type: 'reasoning', content: 'thinking' }]))).toBe(false)
    expect(canForkMessage(userMessage('hello'))).toBe(false)
  })

  it('requires a persisted message id before showing fork', () => {
    expect(canForkMessage(assistantMessage([{ type: 'text', content: 'reply' }], false, {
      id: 'assistant-1',
    }))).toBe(false)

    expect(canForkMessage(assistantMessage([{ type: 'text', content: 'reply' }], false, {
      id: 'assistant-1',
      serverId: ASSISTANT_MESSAGE_ID,
    }))).toBe(true)
  })
})
