import type { Message } from './useChat.types'

const yamlHeaderRe = /^---\n[\s\S]*?\n---\n?/

export function extractToolCalls(
  message: Message,
): Array<{ id: string; name: string; input: unknown }> {
  const parts = getContentParts(message)
  if (!parts) return []
  return parts
    .filter((p) => String((p as Record<string, unknown>).type ?? '').toLowerCase() === 'tool-call')
    .map((p) => {
      const part = p as Record<string, unknown>
      return {
        id: String(part.toolCallId ?? ''),
        name: String(part.toolName ?? ''),
        input: part.input ?? null,
      }
    })
}

export function extractToolCallId(message: Message): string {
  const raw = message.content
  if (!raw || typeof raw !== 'object') return ''
  const obj = raw as Record<string, unknown>
  if (typeof obj.tool_call_id === 'string') return obj.tool_call_id.trim()
  const parts = getContentParts(message)
  if (!parts) return ''
  for (const p of parts) {
    const part = p as Record<string, unknown>
    if (String(part.type ?? '').toLowerCase() === 'tool-result' && typeof part.toolCallId === 'string') {
      return part.toolCallId.trim()
    }
  }
  return ''
}

export function extractAllToolResults(
  message: Message,
): Array<{ toolCallId: string; output: unknown }> {
  const parts = getContentParts(message)
  if (!parts) return []
  return parts
    .filter((p) => String((p as Record<string, unknown>).type ?? '').toLowerCase() === 'tool-result')
    .map((p) => {
      const part = p as Record<string, unknown>
      return {
        toolCallId: String(part.toolCallId ?? ''),
        output: part.output ?? null,
      }
    })
}

function getContentParts(message: Message): unknown[] | null {
  const raw = message.content
  if (!raw || typeof raw !== 'object') return null
  const obj = raw as Record<string, unknown>
  if (Array.isArray(obj.content)) return obj.content
  if (typeof obj.content === 'string') {
    try {
      const parsed = JSON.parse(obj.content)
      if (Array.isArray(parsed)) return parsed
    } catch {
      return null
    }
  }
  return null
}

export function extractMessageText(message: Message): string {
  const raw = message.content
  if (!raw) return ''

  let text: string
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      text = extractTextFromContent(parsed?.content ?? parsed).trim()
    } catch {
      text = raw.trim()
    }
  } else if (typeof raw === 'object') {
    const obj = raw as Record<string, unknown>
    if ('content' in obj && obj.content !== undefined && obj.content !== null) {
      text = extractTextFromContent(obj.content).trim()
    } else {
      text = extractTextFromContent(raw).trim()
    }
  } else {
    text = extractTextFromContent(raw).trim()
  }

  if (message.role === 'user') {
    text = stripYAMLHeader(text)
  }
  return text
}

export function stripYAMLHeader(text: string): string {
  return text.replace(yamlHeaderRe, '').trim()
}

export function extractTextFromContent(content: unknown): string {
  if (typeof content === 'string') return content.trim()

  if (Array.isArray(content)) {
    return content
      .map((part) => {
        if (!part || typeof part !== 'object') return ''
        const value = part as Record<string, unknown>
        const partType = String(value.type ?? '').toLowerCase()
        if (partType === 'text' && typeof value.text === 'string') return value.text.trim()
        if (partType === 'link' && typeof value.url === 'string') return value.url.trim()
        if (partType === 'emoji' && typeof value.emoji === 'string') return value.emoji.trim()
        if (typeof value.text === 'string') return value.text.trim()
        return ''
      })
      .filter(Boolean)
      .join('\n')
      .trim()
  }

  if (content && typeof content === 'object') {
    const value = content as Record<string, unknown>
    if (typeof value.text === 'string') return value.text.trim()
  }

  return ''
}
