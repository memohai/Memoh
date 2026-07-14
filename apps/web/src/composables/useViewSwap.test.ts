// @vitest-environment jsdom
import { effectScope, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useRoutedViewSwap } from './useViewSwap'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

interface Item {
  routeValue: string
}

function setupSwap() {
  const items = ref<Item[]>([])
  const selected = ref<Item>()
  const loading = ref(false)
  const ready = ref(false)
  const scope = effectScope()
  const swap = scope.run(() => useRoutedViewSwap({
    key: 'provider',
    items: () => items.value,
    selected: () => selected.value,
    select: item => selected.value = item,
    getRouteValue: item => item.routeValue,
    isLoading: () => loading.value,
    isReady: () => ready.value,
  }))!

  return { items, selected, loading, ready, scope, swap }
}

describe('useRoutedViewSwap', () => {
  beforeEach(() => {
    mocks.route.query = {}
    mocks.replace.mockReset()
  })

  it('waits for fresh data before resolving a typed resource key', async () => {
    mocks.route.query = { provider: 'fetch:two' }
    const state = setupSwap()
    state.items.value = [{ routeValue: 'search:one' }]
    state.loading.value = true
    state.ready.value = true
    await nextTick()

    expect(state.selected.value).toBeUndefined()
    expect(mocks.replace).not.toHaveBeenCalled()

    state.items.value = [
      { routeValue: 'search:one' },
      { routeValue: 'fetch:two' },
    ]
    state.loading.value = false
    await nextTick()

    expect(state.selected.value?.routeValue).toBe('fetch:two')
    state.scope.stop()
  })

  it('removes an invalid resource key once its source is ready', async () => {
    mocks.route.query = { provider: 'search:missing', tab: 'voice' }
    const state = setupSwap()
    state.ready.value = true
    await nextTick()

    expect(mocks.replace).toHaveBeenCalledWith({ query: { tab: 'voice' } })
    state.scope.stop()
  })

  it('writes the selected resource key without creating history', () => {
    mocks.route.query = { tab: 'voice' }
    const state = setupSwap()

    state.swap.openDetail({ routeValue: 'speech:one' })

    expect(state.selected.value?.routeValue).toBe('speech:one')
    expect(mocks.replace).toHaveBeenCalledWith({
      query: { tab: 'voice', provider: 'speech:one' },
    })
    state.scope.stop()
  })
})
