import { describe, expect, it } from 'vitest'
import type { ChatMessage } from '@/store/chat-list'
import {
  activeAnchorIndex,
  animateScrollTo,
  buildMinimapAnchors,
  panelScrollTop,
  railActivePosition,
  sampleRailIndexes,
  tickWidth,
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

describe('sampleRailIndexes', () => {
  it('keeps every index when under the cap', () => {
    expect(sampleRailIndexes(3, 5)).toEqual([0, 1, 2])
  })

  it('samples evenly and keeps both ends', () => {
    const sampled = sampleRailIndexes(100, 5)
    expect(sampled).toEqual([0, 25, 50, 74, 99])
  })

  it('never repeats indexes', () => {
    const sampled = sampleRailIndexes(7, 5)
    expect(new Set(sampled).size).toBe(sampled.length)
    expect(sampled[0]).toBe(0)
    expect(sampled.at(-1)).toBe(6)
  })

  it('handles empty input', () => {
    expect(sampleRailIndexes(0, 5)).toEqual([])
  })
})

describe('railActivePosition', () => {
  it('returns the exact position when sampled', () => {
    expect(railActivePosition([0, 25, 50, 74, 99], 50)).toBe(2)
  })

  it('falls back to the previous mark between samples', () => {
    expect(railActivePosition([0, 25, 50, 74, 99], 40)).toBe(1)
  })

  it('clamps before the first mark', () => {
    expect(railActivePosition([5, 10], 2)).toBe(0)
  })

  it('handles empty samples', () => {
    expect(railActivePosition([], 3)).toBe(-1)
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

describe('animateScrollTo', () => {
  function createClock() {
    let time = 0
    const queue: FrameRequestCallback[] = []
    return {
      now: () => time,
      raf: (cb: FrameRequestCallback) => queue.push(cb),
      caf: (handle: number) => {
        queue.splice(handle - 1, 1)
      },
      tick(next: number) {
        time = next
        const pending = queue.splice(0, queue.length)
        for (const cb of pending) cb(next)
      },
    }
  }

  it('eases toward the target and snaps at the end', () => {
    const clock = createClock()
    const el = { scrollTop: 0 }
    animateScrollTo(el, () => 1000, { duration: 100, now: clock.now, raf: clock.raf, caf: clock.caf })
    clock.tick(50)
    expect(el.scrollTop).toBeCloseTo(1000 * (1 - 0.5 ** 5))
    clock.tick(100)
    expect(el.scrollTop).toBe(1000)
    clock.tick(150)
    expect(el.scrollTop).toBe(1000)
  })

  it('follows a target that moves mid-flight', () => {
    const clock = createClock()
    const el = { scrollTop: 0 }
    let target = 1000
    animateScrollTo(el, () => target, { duration: 100, now: clock.now, raf: clock.raf, caf: clock.caf })
    clock.tick(50)
    target = 2000
    clock.tick(100)
    expect(el.scrollTop).toBe(2000)
  })

  it('stops when cancelled', () => {
    const clock = createClock()
    const el = { scrollTop: 0 }
    const cancel = animateScrollTo(el, () => 1000, { duration: 100, now: clock.now, raf: clock.raf, caf: clock.caf })
    clock.tick(50)
    const frozen = el.scrollTop
    cancel()
    clock.tick(100)
    expect(el.scrollTop).toBe(frozen)
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
