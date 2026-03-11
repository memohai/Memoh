/**
 * Generic extensible tag-interception system.
 *
 * Register TagResolver instances (e.g. attachments, reactions) and both the
 * batch extractor and the streaming state-machine will intercept the
 * corresponding `<tag>...</tag>` blocks, stripping them from visible text and
 * forwarding the parsed payload through {@link TagEvent} objects.
 */

// ---------------------------------------------------------------------------
// Public interfaces
// ---------------------------------------------------------------------------

export interface TagResolver<T = unknown> {
  tag: string
  parse(content: string): T[]
}

export interface TagEvent {
  tag: string
  data: unknown[]
}

export interface TagStreamResult {
  visibleText: string
  events: TagEvent[]
}

// ---------------------------------------------------------------------------
// Batch extractor
// ---------------------------------------------------------------------------

/**
 * Extract all registered tag blocks from a complete text string.
 * Returns the cleaned text (blocks removed) and a list of tag events.
 */
export function extractTagsFromText(
  text: string,
  resolvers: TagResolver[],
): { cleanedText: string; events: TagEvent[] } {
  const events: TagEvent[] = []
  let cleaned = text
  for (const resolver of resolvers) {
    const open = `<${resolver.tag}>`
    const close = `</${resolver.tag}>`
    const pattern = new RegExp(
      `${escapeRegExp(open)}([\\s\\S]*?)${escapeRegExp(close)}`,
      'g',
    )
    cleaned = cleaned.replace(pattern, (_match, inner: string) => {
      const parsed = resolver.parse(inner)
      if (parsed.length > 0) {
        events.push({ tag: resolver.tag, data: parsed })
      }
      return ''
    })
  }
  return {
    cleanedText: cleaned.replace(/\n{3,}/g, '\n\n').trim(),
    events,
  }
}

// ---------------------------------------------------------------------------
// Streaming extractor
// ---------------------------------------------------------------------------

interface ResolverMeta {
  resolver: TagResolver
  openTag: string
  closeTag: string
}

/**
 * Incremental state-machine that intercepts multiple `<tag>...</tag>` blocks
 * from a stream of text deltas.
 *
 * Text outside registered blocks is passed through as `visibleText`; completed
 * blocks are emitted as {@link TagEvent} entries.
 */
export class StreamTagExtractor {
  private metas: ResolverMeta[]
  private maxOpenLen: number
  private state: 'text' | 'inside' = 'text'
  private activeMeta: ResolverMeta | null = null
  private buffer = ''
  private tagBuffer = ''

  constructor(resolvers: TagResolver[]) {
    this.metas = resolvers.map((resolver) => ({
      resolver,
      openTag: `<${resolver.tag}>`,
      closeTag: `</${resolver.tag}>`,
    }))
    this.maxOpenLen = Math.max(...this.metas.map((m) => m.openTag.length), 0)
  }

  push(delta: string): TagStreamResult {
    this.buffer += delta
    let visible = ''
    const events: TagEvent[] = []

    while (this.buffer.length > 0) {
      if (this.state === 'text') {
        let earliestIdx = -1
        let matchedMeta: ResolverMeta | null = null

        for (const meta of this.metas) {
          const idx = this.buffer.indexOf(meta.openTag)
          if (idx !== -1 && (earliestIdx === -1 || idx < earliestIdx)) {
            earliestIdx = idx
            matchedMeta = meta
          }
        }

        if (earliestIdx === -1) {
          const keep = Math.min(this.buffer.length, this.maxOpenLen - 1)
          const emit = this.buffer.slice(0, this.buffer.length - keep)
          visible += emit
          this.buffer = this.buffer.slice(this.buffer.length - keep)
          break
        }

        visible += this.buffer.slice(0, earliestIdx)
        this.buffer = this.buffer.slice(earliestIdx + matchedMeta!.openTag.length)
        this.tagBuffer = ''
        this.activeMeta = matchedMeta
        this.state = 'inside'
        continue
      }

      // state === 'inside'
      const closeTag = this.activeMeta!.closeTag
      const endIdx = this.buffer.indexOf(closeTag)
      if (endIdx === -1) {
        const keep = Math.min(this.buffer.length, closeTag.length - 1)
        const take = this.buffer.slice(0, this.buffer.length - keep)
        this.tagBuffer += take
        this.buffer = this.buffer.slice(this.buffer.length - keep)
        break
      }

      this.tagBuffer += this.buffer.slice(0, endIdx)
      const parsed = this.activeMeta!.resolver.parse(this.tagBuffer)
      if (parsed.length > 0) {
        events.push({ tag: this.activeMeta!.resolver.tag, data: parsed })
      }
      this.buffer = this.buffer.slice(endIdx + closeTag.length)
      this.tagBuffer = ''
      this.activeMeta = null
      this.state = 'text'
    }

    return { visibleText: visible, events }
  }

  /**
   * Flush remaining buffered content. Call when the stream ends.
   * Unclosed tag blocks are returned as literal `visibleText` to avoid data loss.
   */
  flushRemainder(): TagStreamResult {
    if (this.state === 'text') {
      const out = this.buffer
      this.buffer = ''
      return { visibleText: out, events: [] }
    }
    const meta = this.activeMeta!
    const out = `${meta.openTag}${this.tagBuffer}${this.buffer}`
    this.state = 'text'
    this.buffer = ''
    this.tagBuffer = ''
    this.activeMeta = null
    return { visibleText: out, events: [] }
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
