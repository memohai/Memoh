import { afterEach, describe, expect, it, vi } from 'vitest'

const { registerHighlight } = vi.hoisted(() => ({
  registerHighlight: vi.fn(() => Promise.resolve()),
}))

vi.mock('stream-markdown', () => ({ registerHighlight }))

async function importFresh() {
  vi.resetModules()
  return await import('./warm-code-highlight')
}

describe('warmCodeHighlightOnIdle', () => {
  afterEach(() => {
    registerHighlight.mockClear()
    vi.unstubAllGlobals()
  })

  it('does nothing without a window', async () => {
    const { warmCodeHighlightOnIdle } = await importFresh()
    warmCodeHighlightOnIdle()
    expect(registerHighlight).not.toHaveBeenCalled()
  })

  it('warms the highlighter once via requestIdleCallback', async () => {
    const idleCallbacks: Array<() => void> = []
    vi.stubGlobal('window', {
      requestIdleCallback: (cb: () => void) => {
        idleCallbacks.push(cb)
        return 1
      },
    })

    const { warmCodeHighlightOnIdle } = await importFresh()
    warmCodeHighlightOnIdle()
    warmCodeHighlightOnIdle()

    expect(idleCallbacks).toHaveLength(1)
    idleCallbacks[0]?.()
    await vi.waitFor(() => expect(registerHighlight).toHaveBeenCalledTimes(1))
  })

  it('falls back to setTimeout when requestIdleCallback is unavailable', async () => {
    const timeouts: Array<() => void> = []
    vi.stubGlobal('window', {
      setTimeout: (cb: () => void) => {
        timeouts.push(cb)
        return 0
      },
    })

    const { warmCodeHighlightOnIdle } = await importFresh()
    warmCodeHighlightOnIdle()

    expect(timeouts).toHaveLength(1)
    timeouts[0]?.()
    await vi.waitFor(() => expect(registerHighlight).toHaveBeenCalledTimes(1))
  })
})
