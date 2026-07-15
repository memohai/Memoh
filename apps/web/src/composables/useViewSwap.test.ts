// @vitest-environment jsdom
import { effectScope, nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useViewSwap } from './useViewSwap'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string | undefined> },
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

function setupSwap(key = 'provider') {
  const scope = effectScope()
  const swap = scope.run(() => useViewSwap(key))!
  return { scope, swap }
}

describe('useViewSwap', () => {
  beforeEach(() => {
    mocks.route.query = {}
    mocks.replace.mockReset()
  })

  it('writes the resource id into a page-owned query key without history', () => {
    mocks.route.query = { tab: 'kept' }
    const { scope, swap } = setupSwap('emailProvider')

    swap.openDetail('email-1')

    expect(swap.view.value).toBe('detail')
    expect(mocks.replace).toHaveBeenCalledWith({
      query: { tab: 'kept', emailProvider: 'email-1' },
    })
    scope.stop()
  })

  it('opens detail from a non-empty query value on mount', () => {
    mocks.route.query = { provider: 'abc' }
    const { scope, swap } = setupSwap('provider')

    expect(swap.view.value).toBe('detail')
    expect(swap.queryValue.value).toBe('abc')
    scope.stop()
  })

  it('clears only its own query key when returning to the list', async () => {
    mocks.route.query = { provider: 'abc', tab: 'kept' }
    const { scope, swap } = setupSwap('provider')

    swap.backToList()
    await nextTick()

    expect(swap.view.value).toBe('list')
    expect(mocks.replace).toHaveBeenCalledWith({ query: { tab: 'kept' } })
    scope.stop()
  })
})
