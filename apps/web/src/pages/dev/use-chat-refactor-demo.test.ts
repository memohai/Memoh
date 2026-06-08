import { describe, expect, it } from 'vitest'
import { useChatRefactorDemo } from './use-chat-refactor-demo'

type Demo = ReturnType<typeof useChatRefactorDemo>

function runCycles(demo: Demo, n: number) {
  let guard = 0
  while (demo.stats.cyclesCompleted < n && guard < 10000) {
    demo.tick()
    guard += 1
  }
}

describe('chat refactor demo engine', () => {
  it('NEW: optimistic turns keep their id (v-for key) across refetches → no remount', () => {
    const demo = useChatRefactorDemo()
    demo.mode.value = 'new'
    runCycles(demo, 3)
    const first = demo.messages.find(turn => turn.id === 'opt-a1')
    expect(first).toBeTruthy()
    expect(first?.serverId).toBe('srv-a1')
  })

  it('OLD: the just-sent turn is dropped and re-inserted under a new id → remount', () => {
    const demo = useChatRefactorDemo()
    demo.mode.value = 'old'
    runCycles(demo, 3)
    expect(demo.messages.find(turn => turn.id === 'opt-a1')).toBeFalsy()
    expect(demo.messages.find(turn => turn.id === 'srv-a1')).toBeTruthy()
  })

  it('NEW streams blocks with zero identity churn; OLD churns once per token', () => {
    const neu = useChatRefactorDemo()
    neu.mode.value = 'new'
    runCycles(neu, 3)

    const old = useChatRefactorDemo()
    old.mode.value = 'old'
    runCycles(old, 3)

    expect(neu.stats.blockChurn).toBe(0)
    expect(old.stats.blockChurn).toBeGreaterThan(20)
  })

  it('NEW sidebar bumps by server time without thrashing the array', () => {
    const demo = useChatRefactorDemo()
    demo.mode.value = 'new'
    demo.fireBackgroundEvent()
    expect(demo.sidebarStats.reorders).toBe(0)
    expect(demo.orderedSessions.value[0]?.id).toBe('s2')
  })

  it('OLD sidebar unshifts (jumps) on every event', () => {
    const demo = useChatRefactorDemo()
    demo.mode.value = 'old'
    demo.fireBackgroundEvent()
    expect(demo.sidebarStats.reorders).toBe(1)
    expect(demo.sessions[0]?.id).toBe('s2')
  })
})
