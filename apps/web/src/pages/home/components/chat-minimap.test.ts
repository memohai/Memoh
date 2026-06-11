import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '@/store/chat-list'
import {
  activeAnchorIndex,
  buildMinimapAnchors,
  panelScrollTop,
  planJump,
  tickWidth,
  viewportIndicator,
} from './chat-minimap'

function userTurn(id: string, text: string): ChatMessage {
  return {
    id,
    role: 'user',
    text,
    attachments: [],
    timestamp: '',
    streaming: false,
    isSelf: true,
  }
}

function assistantTurn(id: string, text: string): ChatMessage {
  return {
    id,
    role: 'assistant',
    messages: text ? [{ type: 'text', content: text }] : [],
    timestamp: '',
    streaming: false,
  } as ChatMessage
}

describe('buildMinimapAnchors', () => {
  it('anchors each user turn with text', () => {
    const anchors = buildMinimapAnchors([
      userTurn('u1', 'first question'),
      assistantTurn('a1', 'answer one'),
      userTurn('u2', 'second question'),
    ])
    expect(anchors.map(anchor => anchor.id)).toEqual(['u1', 'u2'])
    expect(anchors[0]?.preview).toBe('first question')
  })

  it('skips user turns without text and normalizes whitespace', () => {
    const anchors = buildMinimapAnchors([
      userTurn('u1', '  '),
      userTurn('u2', 'multi\n  line\ttext  '),
    ])
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ id: 'u2', preview: 'multi line text' })
  })

  it('clamps long previews', () => {
    const anchors = buildMinimapAnchors([userTurn('u1', 'x'.repeat(500))])
    expect(anchors[0]?.preview).toHaveLength(100)
  })

  it('falls back to assistant turns when there are no user anchors', () => {
    const anchors = buildMinimapAnchors([
      assistantTurn('a1', 'heartbeat output'),
      assistantTurn('a2', ''),
      assistantTurn('a3', 'second output'),
    ])
    expect(anchors.map(anchor => anchor.id)).toEqual(['a1', 'a3'])
    expect(anchors[0]?.role).toBe('assistant')
  })
})

describe('activeAnchorIndex', () => {
  const view = { clientHeight: 400, scrollHeight: 4000 }

  it('returns -1 when there are no anchors', () => {
    expect(activeAnchorIndex([], { scrollTop: 0, ...view })).toBe(-1)
  })

  it('activates the last anchor above the probe line', () => {
    const tops = [0, 1000, 2000]
    expect(activeAnchorIndex(tops, { scrollTop: 0, ...view })).toBe(0)
    expect(activeAnchorIndex(tops, { scrollTop: 950, ...view })).toBe(1)
    expect(activeAnchorIndex(tops, { scrollTop: 2500, ...view })).toBe(2)
  })

  it('keeps the first anchor active before its top', () => {
    expect(activeAnchorIndex([500, 1000], { scrollTop: 0, ...view })).toBe(0)
  })

  it('forces the last anchor at the bottom', () => {
    expect(activeAnchorIndex([0, 3950], { scrollTop: 3600, ...view })).toBe(1)
  })
})

describe('planJump', () => {
  it('scrolls directly for short distances', () => {
    expect(planJump(0, 800, 400)).toEqual({ pre: null })
  })

  it('pre-jumps one viewport short when far below', () => {
    expect(planJump(0, 5000, 400)).toEqual({ pre: 4600 })
  })

  it('pre-jumps one viewport past when far above', () => {
    expect(planJump(5000, 0, 400)).toEqual({ pre: 400 })
  })
})

describe('tickWidth', () => {
  it('grows with prompt length between bounds', () => {
    expect(tickWidth(0)).toBe(5)
    expect(tickWidth(20)).toBeGreaterThan(5)
    expect(tickWidth(20)).toBeLessThan(tickWidth(60))
    expect(tickWidth(80)).toBe(16)
    expect(tickWidth(10000)).toBe(16)
  })
})

describe('viewportIndicator', () => {
  it('maps the visible window to percentages', () => {
    expect(viewportIndicator({ scrollTop: 1000, clientHeight: 500, scrollHeight: 5000 }))
      .toEqual({ topPercent: 20, heightPercent: 10 })
  })

  it('clamps degenerate content height', () => {
    expect(viewportIndicator({ scrollTop: 0, clientHeight: 500, scrollHeight: 0 }))
      .toEqual({ topPercent: 0, heightPercent: 100 })
  })
})

describe('panelScrollTop', () => {
  it('returns null when the item is visible', () => {
    expect(panelScrollTop({ itemTop: 100, itemHeight: 28, viewTop: 0, viewHeight: 300 })).toBeNull()
  })

  it('centers items below the view', () => {
    expect(panelScrollTop({ itemTop: 600, itemHeight: 28, viewTop: 0, viewHeight: 300 })).toBe(464)
  })

  it('centers items above the view', () => {
    expect(panelScrollTop({ itemTop: 100, itemHeight: 28, viewTop: 400, viewHeight: 300 })).toBe(0)
  })
})
