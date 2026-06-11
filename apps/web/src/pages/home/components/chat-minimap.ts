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
const DIRECT_SCROLL_VIEWPORTS = 2.5
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

export function planJump(from: number, to: number, viewportHeight: number): { pre: number | null } {
  if (Math.abs(to - from) <= viewportHeight * DIRECT_SCROLL_VIEWPORTS) return { pre: null }
  return { pre: to > from ? to - viewportHeight : to + viewportHeight }
}

export function tickWidth(textLength: number): number {
  const ratio = Math.min(Math.sqrt(textLength / TICK_FULL_LENGTH), 1)
  return Math.round(TICK_MIN_WIDTH + (TICK_MAX_WIDTH - TICK_MIN_WIDTH) * ratio)
}

export function viewportIndicator(view: ViewMetrics): { topPercent: number, heightPercent: number } {
  if (view.scrollHeight <= view.clientHeight) return { topPercent: 0, heightPercent: 100 }
  const clampPercent = (value: number) => Math.min(100, Math.max(0, value))
  return {
    topPercent: clampPercent((view.scrollTop / view.scrollHeight) * 100),
    heightPercent: clampPercent((view.clientHeight / view.scrollHeight) * 100),
  }
}

export function panelScrollTop(input: { itemTop: number, itemHeight: number, viewTop: number, viewHeight: number }): number | null {
  const { itemTop, itemHeight, viewTop, viewHeight } = input
  if (itemTop >= viewTop && itemTop + itemHeight <= viewTop + viewHeight) return null
  return Math.max(0, itemTop - viewHeight / 2 + itemHeight / 2)
}
