import { describe, expect, it } from 'vitest'
import {
  canTriggerHistorySentinel,
  historyPrependAnchorDelta,
} from './chat-sentinel-readiness'

describe('canTriggerHistorySentinel', () => {
  const ready = {
    isVisible: true,
    loadingMessages: false,
    sessionLandingPending: false,
    userRequestedHistory: true,
    loadingOlder: false,
    hasMoreOlder: true,
    messageCount: 3,
  }

  it('allows prefetch after bottom landing and upward user intent', () => {
    expect(canTriggerHistorySentinel(ready)).toBe(true)
  })

  it('blocks while initial messages are loading', () => {
    expect(canTriggerHistorySentinel({ ...ready, loadingMessages: true })).toBe(false)
  })

  it('blocks while scroll is locked for session landing', () => {
    expect(canTriggerHistorySentinel({ ...ready, sessionLandingPending: true })).toBe(false)
  })

  it('blocks an intersecting sentinel without upward user intent', () => {
    expect(canTriggerHistorySentinel({ ...ready, userRequestedHistory: false })).toBe(false)
  })
})

describe('historyPrependAnchorDelta', () => {
  it('returns the existing row viewport displacement', () => {
    expect(historyPrependAnchorDelta({
      capturedAnchorTop: 24,
      currentAnchorTop: 424,
      userIntervenedDuringLoad: false,
    })).toBe(400)
  })

  it('does not mistake unrelated document growth for a prepend', () => {
    expect(historyPrependAnchorDelta({
      capturedAnchorTop: 24,
      currentAnchorTop: 24,
      userIntervenedDuringLoad: false,
    })).toBeNull()
  })

  it('skips when the user intervenes during the load', () => {
    expect(historyPrependAnchorDelta({
      capturedAnchorTop: 24,
      currentAnchorTop: 424,
      userIntervenedDuringLoad: true,
    })).toBeNull()
  })

  it('ignores sub-pixel layout noise', () => {
    expect(historyPrependAnchorDelta({
      capturedAnchorTop: 24,
      currentAnchorTop: 24.25,
      userIntervenedDuringLoad: false,
    })).toBeNull()
  })
})
