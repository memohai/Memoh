import type { TagResolver } from './tag-extractor'

export interface SpeechItem {
  text: string
}

/**
 * Parse a `<speech>` block. The entire trimmed content is one synthesis request.
 */
export const speechResolver: TagResolver<SpeechItem> = {
  tag: 'speech',
  parse(content: string): SpeechItem[] {
    const text = content.trim()
    return text ? [{ text }] : []
  },
}
