import type { ChatMessage } from '@/store/chat-list'

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

export function isPersistentMessageId(id: string): boolean {
  return UUID_RE.test(id.trim())
}

export function cleanUserText(content?: string): string {
  if (!content) return ''
  return content
    .split('\n')
    .filter((line) => !/^\[attachment:\w+\]\s/.test(line.trim()))
    .join('\n')
    .trim()
}

export function getAssistantText(message: ChatMessage): string {
  if (message.role !== 'assistant') return ''
  return message.messages
    .filter(block => block.type === 'text' && !!block.content?.trim())
    .map(block => block.type === 'text' ? block.content.trim() : '')
    .filter(Boolean)
    .join('\n\n')
}

export function getMessageCopyText(message: ChatMessage): string {
  if (message.role === 'user') return cleanUserText(message.text).replace(/\s+$/g, '')
  if (message.role === 'assistant') return getAssistantText(message)
  return ''
}

export function canForkMessage(message: ChatMessage): boolean {
  return message.role === 'assistant'
    && !message.streaming
    && getAssistantText(message).length > 0
    && persistentMessageId(message).length > 0
}

export function canRewriteRequest(message: ChatMessage): boolean {
  return message.role === 'user'
    && !message.streaming
    && cleanUserText(message.text).length > 0
    && persistentMessageId(message).length > 0
}

export function persistentMessageId(message: ChatMessage): string {
  const id = message.serverId?.trim() || message.id.trim()
  return isPersistentMessageId(id) ? id : ''
}
