import type { AssistantModelMessage, ModelMessage, TextPart } from 'ai'
import type { AgentAttachment, ContainerFileAttachment } from '../types/attachment'

const ATTACHMENTS_START = '<attachments>'
const ATTACHMENTS_END = '</attachments>'

/**
 * Get a unique key for deduplication of attachments.
 */
const getAttachmentKey = (a: AgentAttachment): string => {
  switch (a.type) {
    case 'file': return `file:${a.path}`
    case 'image': return `image:${(a.base64 ?? a.url ?? '').slice(0, 64)}`
  }
}

/**
 * Deduplicate attachments by their key.
 */
export const dedupeAttachments = (attachments: AgentAttachment[]): AgentAttachment[] => {
  return Array.from(new Map(attachments.map(a => [getAttachmentKey(a), a])).values())
}

/**
 * Parse attachment file paths from the inner content of an `<attachments>` block.
 * Each line should be formatted as `- /path/to/file`.
 */
export const parseAttachmentPaths = (content: string): string[] => {
  return content
    .split('\n')
    .map(line => line.trim())
    .map(line => {
      if (!line.startsWith('-')) return ''
      return line.slice(1).trim()
    })
    .filter(Boolean)
}

/**
 * Extract all `<attachments>...</attachments>` blocks from a text string.
 * Returns the cleaned text (blocks removed) and the parsed file attachments.
 */
export const extractAttachmentsFromText = (text: string): { cleanedText: string; attachments: ContainerFileAttachment[] } => {
  const paths: string[] = []
  const cleanedText = text.replace(
    /<attachments>([\s\S]*?)<\/attachments>/g,
    (_match, inner: string) => { 
      paths.push(...parseAttachmentPaths(inner))
      return ''
    }
  )
  const uniquePaths = Array.from(new Set(paths))
  return {
    cleanedText: cleanedText.replace(/\n{3,}/g, '\n\n').trim(),
    attachments: uniquePaths.map((path): ContainerFileAttachment => ({ type: 'file', path })),
  }
}

/**
 * Type guard: checks whether a content part is a TextPart.
 */
const isTextPart = (part: unknown): part is TextPart => {
  return (
    typeof part === 'object' &&
    part !== null &&
    (part as Record<string, unknown>).type === 'text' &&
    typeof (part as Record<string, unknown>).text === 'string'
  )
}

/**
 * Strip `<attachments>` blocks from all assistant messages in a message list.
 * Returns the cleaned messages and a deduplicated list of attachments found.
 */
export const stripAttachmentsFromMessages = (
  messages: ModelMessage[]
): { messages: ModelMessage[]; attachments: ContainerFileAttachment[] } => {
  const allAttachments: ContainerFileAttachment[] = []

  const stripped = messages.map((msg): ModelMessage => {
    if (msg.role !== 'assistant') return msg

    const assistantMsg = msg as AssistantModelMessage
    const { content } = assistantMsg

    if (typeof content === 'string') {
      const { cleanedText, attachments } = extractAttachmentsFromText(content)
      allAttachments.push(...attachments)
      return { ...assistantMsg, content: cleanedText }
    }

    if (Array.isArray(content)) {
      const newParts = content.map(part => {
        if (!isTextPart(part)) return part
        const { cleanedText, attachments } = extractAttachmentsFromText(part.text)
        allAttachments.push(...attachments)
        return { ...part, text: cleanedText }
      })
      return { ...assistantMsg, content: newParts }
    }

    return msg
  })

  return {
    messages: stripped,
    attachments: dedupeAttachments(allAttachments) as ContainerFileAttachment[],
  }
}

// ---------------------------------------------------------------------------
// Streaming extractor
// ---------------------------------------------------------------------------

export interface AttachmentsStreamResult {
  visibleText: string
  attachments: ContainerFileAttachment[]
}

/**
 * Incremental state-machine that intercepts `<attachments>...</attachments>`
 * blocks from a stream of text deltas. Text outside those blocks is passed
 * through as `visibleText`; completed blocks are emitted as `attachments`.
 */
export class AttachmentsStreamExtractor {
  private state: 'text' | 'attachments' = 'text'
  private buffer = ''
  private attachmentsBuffer = ''

  /**
   * Feed a new text delta into the extractor.
   */
  push(delta: string): AttachmentsStreamResult {
    this.buffer += delta
    let visible = ''
    const attachments: ContainerFileAttachment[] = []

    while (this.buffer.length > 0) {
      if (this.state === 'text') {
        const idx = this.buffer.indexOf(ATTACHMENTS_START)
        if (idx === -1) {
          // Emit everything except a small tail that could be the start of the opening tag.
          const keep = Math.min(this.buffer.length, ATTACHMENTS_START.length - 1)
          const emit = this.buffer.slice(0, this.buffer.length - keep)
          visible += emit
          this.buffer = this.buffer.slice(this.buffer.length - keep)
          break
        }

        visible += this.buffer.slice(0, idx)
        this.buffer = this.buffer.slice(idx + ATTACHMENTS_START.length)
        this.attachmentsBuffer = ''
        this.state = 'attachments'
        continue
      }

      // state === 'attachments'
      const endIdx = this.buffer.indexOf(ATTACHMENTS_END)
      if (endIdx === -1) {
        const keep = Math.min(this.buffer.length, ATTACHMENTS_END.length - 1)
        const take = this.buffer.slice(0, this.buffer.length - keep)
        this.attachmentsBuffer += take
        this.buffer = this.buffer.slice(this.buffer.length - keep)
        break
      }

      this.attachmentsBuffer += this.buffer.slice(0, endIdx)
      const paths = parseAttachmentPaths(this.attachmentsBuffer)
      for (const path of paths) {
        attachments.push({ type: 'file', path })
      }
      this.buffer = this.buffer.slice(endIdx + ATTACHMENTS_END.length)
      this.attachmentsBuffer = ''
      this.state = 'text'
    }

    return { visibleText: visible, attachments: dedupeAttachments(attachments) as ContainerFileAttachment[] }
  }

  /**
   * Flush any remaining buffered content. Call this when the stream ends.
   * If an `<attachments>` block was not properly closed, the raw text is
   * returned as `visibleText` to avoid data loss.
   */
  flushRemainder(): AttachmentsStreamResult {
    if (this.state === 'text') {
      const out = this.buffer
      this.buffer = ''
      return { visibleText: out, attachments: [] }
    }
    // Unclosed attachments block — treat it as literal text to avoid data loss.
    const out = `${ATTACHMENTS_START}${this.attachmentsBuffer}${this.buffer}`
    this.state = 'text'
    this.buffer = ''
    this.attachmentsBuffer = ''
    return { visibleText: out, attachments: [] }
  }
}

