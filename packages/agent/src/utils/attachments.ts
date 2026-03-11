import type { AssistantModelMessage, ModelMessage, TextPart } from 'ai'
import type {
  AgentAttachment,
  ContainerFileAttachment,
} from '../types/attachment'
import type { TagResolver } from './tag-extractor'
import { StreamTagExtractor, extractTagsFromText } from './tag-extractor'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// TagResolver for <attachments>
// ---------------------------------------------------------------------------

export const attachmentsResolver: TagResolver<ContainerFileAttachment> = {
  tag: 'attachments',
  parse(content: string): ContainerFileAttachment[] {
    const paths = Array.from(new Set(parseAttachmentPaths(content)))
    return paths.map((path): ContainerFileAttachment => ({ type: 'file', path }))
  },
}

// ---------------------------------------------------------------------------
// Batch extraction (backward-compatible wrapper)
// ---------------------------------------------------------------------------

/**
 * Extract all `<attachments>...</attachments>` blocks from a text string.
 * Returns the cleaned text (blocks removed) and the parsed file attachments.
 */
export const extractAttachmentsFromText = (text: string): { cleanedText: string; attachments: ContainerFileAttachment[] } => {
  const { cleanedText, events } = extractTagsFromText(text, [attachmentsResolver])
  const attachments = events
    .filter((e) => e.tag === 'attachments')
    .flatMap((e) => e.data as ContainerFileAttachment[])
  return {
    cleanedText,
    attachments: dedupeAttachments(attachments) as ContainerFileAttachment[],
  }
}

// ---------------------------------------------------------------------------
// Message-level stripping
// ---------------------------------------------------------------------------

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
 * Strip all registered tag blocks from assistant messages in a message list.
 * Accepts additional resolvers to strip beyond `<attachments>` (e.g. `<reactions>`).
 * Returns the cleaned messages and a deduplicated list of attachments found.
 */
export const stripAttachmentsFromMessages = (
  messages: ModelMessage[],
  extraResolvers: TagResolver[] = [],
): { messages: ModelMessage[]; attachments: ContainerFileAttachment[] } => {
  const allAttachments: ContainerFileAttachment[] = []
  const resolvers: TagResolver[] = [attachmentsResolver, ...extraResolvers]

  const cleanText = (text: string): string => {
    const { cleanedText, events } = extractTagsFromText(text, resolvers)
    const attachments = events
      .filter((e) => e.tag === 'attachments')
      .flatMap((e) => e.data as ContainerFileAttachment[])
    allAttachments.push(...attachments)
    return cleanedText
  }

  const stripped = messages.map((msg): ModelMessage => {
    if (msg.role !== 'assistant') return msg

    const assistantMsg = msg as AssistantModelMessage
    const { content } = assistantMsg

    if (typeof content === 'string') {
      return { ...assistantMsg, content: cleanText(content) }
    }

    if (Array.isArray(content)) {
      const newParts = content.map(part => {
        if (!isTextPart(part)) return part
        return { ...part, text: cleanText(part.text) }
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
// Streaming extractor (backward-compatible wrapper)
// ---------------------------------------------------------------------------

export interface AttachmentsStreamResult {
  visibleText: string
  attachments: ContainerFileAttachment[]
}

/**
 * Backward-compatible streaming extractor that delegates to {@link StreamTagExtractor}.
 * Intercepts `<attachments>...</attachments>` blocks from a stream of text deltas.
 */
export class AttachmentsStreamExtractor {
  private inner: StreamTagExtractor

  constructor() {
    this.inner = new StreamTagExtractor([attachmentsResolver])
  }

  push(delta: string): AttachmentsStreamResult {
    const { visibleText, events } = this.inner.push(delta)
    const attachments = events
      .filter((e) => e.tag === 'attachments')
      .flatMap((e) => e.data as ContainerFileAttachment[])
    return {
      visibleText,
      attachments: dedupeAttachments(attachments) as ContainerFileAttachment[],
    }
  }

  flushRemainder(): AttachmentsStreamResult {
    const { visibleText, events } = this.inner.flushRemainder()
    const attachments = events
      .filter((e) => e.tag === 'attachments')
      .flatMap((e) => e.data as ContainerFileAttachment[])
    return {
      visibleText,
      attachments: dedupeAttachments(attachments) as ContainerFileAttachment[],
    }
  }
}
