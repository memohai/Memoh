import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ensureOnboarding } from './onboarding'

const mockStore = vi.hoisted(() => ({
  state: {
    onboardingCompleted: false,
    fetchMe: vi.fn<() => Promise<boolean>>(),
  },
}))

vi.mock('@/store/user', () => ({
  useUserStore: () => mockStore.state,
}))

vi.mock('@/pages/onboarding/constants', () => ({
  ONBOARDING_KEYS: {
    forceOnboarding: 'memoh:onboarding:force',
  },
}))

const storage = new Map<string, string>()

Object.defineProperty(globalThis, 'localStorage', {
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
  configurable: true,
})

describe('ensureOnboarding', () => {
  beforeEach(() => {
    storage.clear()
    mockStore.state.onboardingCompleted = false
    mockStore.state.fetchMe.mockReset()
  })

  it('returns true without fetching when persisted state is completed', async () => {
    mockStore.state.onboardingCompleted = true

    await expect(ensureOnboarding()).resolves.toBe(true)
    expect(mockStore.state.fetchMe).not.toHaveBeenCalled()
  })

  it('returns false when server confirms onboarding is incomplete', async () => {
    mockStore.state.fetchMe.mockResolvedValue(true)

    await expect(ensureOnboarding()).resolves.toBe(false)
  })

  it('returns true on network failure to avoid blocking the user', async () => {
    mockStore.state.fetchMe.mockResolvedValue(false)

    await expect(ensureOnboarding()).resolves.toBe(true)
  })
})
