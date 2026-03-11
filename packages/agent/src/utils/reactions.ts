import type { TagResolver } from './tag-extractor'

export interface ReactionItem {
  emoji: string
}

/**
 * Parse emoji entries from the inner content of a `<reactions>` block.
 * Each line should be formatted as `- 👍`.
 */
export const parseReactionEmojis = (content: string): string[] => {
  return content
    .split('\n')
    .map(line => line.trim())
    .map(line => {
      if (!line.startsWith('-')) return ''
      return line.slice(1).trim()
    })
    .filter(Boolean)
}

export const reactionsResolver: TagResolver<ReactionItem> = {
  tag: 'reactions',
  parse(content: string): ReactionItem[] {
    const emojis = Array.from(new Set(parseReactionEmojis(content)))
    return emojis.map((emoji): ReactionItem => ({ emoji }))
  },
}
