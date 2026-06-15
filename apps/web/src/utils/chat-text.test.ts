import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '@/store/chat-list'
import { canRewriteRequest, cleanUserText, getMessageCopyText } from './chat-text'

const ASSISTANT_MESSAGE_ID = '11111111-1111-4111-8111-111111111111'
const USER_MESSAGE_ID = '22222222-2222-4222-8222-222222222222'

function userMessage(text: string, options: { id?: string; serverId?: string; streaming?: boolean } = {}): ChatMessage {
  return {
    id: options.id ?? 'user-1',
    serverId: options.serverId,
    role: 'user',
    text,
    attachments: [],
    timestamp: '2026-01-01T00:00:00Z',
    streaming: options.streaming ?? false,
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

  it('allows rewrite only for non-streaming persisted user requests with text', () => {
    expect(canRewriteRequest(userMessage('hello', { id: USER_MESSAGE_ID }))).toBe(true)
    expect(canRewriteRequest(userMessage('hello', { id: 'user-1', serverId: USER_MESSAGE_ID }))).toBe(true)
    expect(canRewriteRequest(userMessage('hello', { id: USER_MESSAGE_ID, streaming: true }))).toBe(false)
    expect(canRewriteRequest(userMessage('[attachment:file] notes.txt', { id: USER_MESSAGE_ID }))).toBe(false)
    expect(canRewriteRequest(assistantMessage([{ type: 'text', content: 'reply' }]))).toBe(false)
  })

  it('requires a persisted message id before showing rewrite', () => {
    expect(canRewriteRequest(userMessage('hello', { id: 'user-1' }))).toBe(false)
  })
})
