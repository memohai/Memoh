import type { ChatMessage } from '@/store/chat-list'

export interface MinimapAnchor {
  id: string
  role: 'user' | 'assistant'
  preview: string
}

export interface ViewMetrics {
  scrollTop: number
  clientHeight: number
  scrollHeight: number
}

const PREVIEW_MAX_LENGTH = 100
const PROBE_RATIO = 0.25
const TICK_MIN_WIDTH = 5
const TICK_MAX_WIDTH = 16
const TICK_FULL_LENGTH = 80

function previewText(message: ChatMessage): string {
  const raw = message.role === 'user'
    ? message.text
    : message.role === 'assistant'
      ? message.messages.find(block => block.type === 'text')?.content
      : ''
  return raw?.trim().replace(/\s+/g, ' ').slice(0, PREVIEW_MAX_LENGTH) ?? ''
}

function anchorsForRole(messages: ChatMessage[], role: 'user' | 'assistant'): MinimapAnchor[] {
  const anchors: MinimapAnchor[] = []
  for (const message of messages) {
    if (message.role !== role) continue
    const preview = previewText(message)
    if (preview) anchors.push({ id: message.id, role, preview })
  }
  return anchors
}

export function buildMinimapAnchors(messages: ChatMessage[]): MinimapAnchor[] {
  const userAnchors = anchorsForRole(messages, 'user')
  return userAnchors.length ? userAnchors : anchorsForRole(messages, 'assistant')
}

export function activeAnchorIndex(tops: number[], view: ViewMetrics): number {
  if (!tops.length) return -1
  if (view.scrollTop + view.clientHeight >= view.scrollHeight - 1) return tops.length - 1
  const probe = view.scrollTop + view.clientHeight * PROBE_RATIO
  let low = 0
  let high = tops.length - 1
  let active = 0
  while (low <= high) {
    const mid = (low + high) >> 1
    if (tops[mid]! <= probe) {
      active = mid
      low = mid + 1
    } else {
      high = mid - 1
    }
  }
  return active
}

export function tickWidth(textLength: number): number {
  const ratio = Math.min(Math.sqrt(textLength / TICK_FULL_LENGTH), 1)
  return Math.round(TICK_MIN_WIDTH + (TICK_MAX_WIDTH - TICK_MIN_WIDTH) * ratio)
}

export function sampleRailIndexes(total: number, max: number): number[] {
  if (total <= 0 || max <= 0) return []
  if (total <= max) return Array.from({ length: total }, (_, index) => index)
  return Array.from({ length: max }, (_, position) => Math.round(position * (total - 1) / (max - 1)))
}

export function railActivePosition(sampled: number[], activeIndex: number): number {
  if (!sampled.length) return -1
  let position = 0
  for (let i = 0; i < sampled.length; i += 1) {
    if (sampled[i]! > activeIndex) break
    position = i
  }
  return position
}

export interface ScrollTweenOptions {
  duration?: number
  now?: () => number
  raf?: (cb: FrameRequestCallback) => number
  caf?: (handle: number) => void
}

export function animateScrollTo(
  el: { scrollTop: number },
  getTarget: () => number,
  options: ScrollTweenOptions = {},
): () => void {
  const duration = options.duration ?? 450
  const now = options.now ?? (() => performance.now())
  const raf = options.raf ?? (cb => requestAnimationFrame(cb))
  const caf = options.caf ?? (handle => cancelAnimationFrame(handle))
  const start = el.scrollTop
  const startedAt = now()
  let cancelled = false
  let handle = 0
  const frame = () => {
    if (cancelled) return
    const progress = duration > 0 ? Math.min(1, (now() - startedAt) / duration) : 1
    const eased = 1 - (1 - progress) ** 5
    el.scrollTop = start + (getTarget() - start) * eased
    if (progress < 1) handle = raf(frame)
  }
  handle = raf(frame)
  return () => {
    if (cancelled) return
    cancelled = true
    caf(handle)
  }
}

export function panelScrollTop(input: { itemTop: number, itemHeight: number, viewTop: number, viewHeight: number }): number | null {
  const { itemTop, itemHeight, viewTop, viewHeight } = input
  if (itemTop >= viewTop && itemTop + itemHeight <= viewTop + viewHeight) return null
  return Math.max(0, itemTop - viewHeight / 2 + itemHeight / 2)
}
