import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { handleRendererNavigate, isRendererNavigationTarget } from './renderer-navigation'

describe('isRendererNavigationTarget', () => {
  it('accepts desktop renderer routes', () => {
    expect(isRendererNavigationTarget('/settings')).toBe(true)
    expect(isRendererNavigationTarget('/settings/providers')).toBe(true)
    expect(isRendererNavigationTarget('/bot/demo')).toBe(true)
    expect(isRendererNavigationTarget('/chat/demo')).toBe(true)
  })

  it('rejects unrelated or prefix-lookalike routes', () => {
    expect(isRendererNavigationTarget('/login')).toBe(false)
    expect(isRendererNavigationTarget('/settings-old')).toBe(false)
    expect(isRendererNavigationTarget('https://example.com/settings')).toBe(false)
  })
})

describe('handleRendererNavigate', () => {
  it('pushes accepted navigation targets', () => {
    const router = {
      currentRoute: ref({ fullPath: '/' }),
      push: vi.fn(() => Promise.resolve()),
    }

    expect(handleRendererNavigate(router, '/settings/providers')).toBe(true)

    expect(router.push).toHaveBeenCalledWith('/settings/providers')
  })

  it('skips duplicate and rejected targets', () => {
    const router = {
      currentRoute: ref({ fullPath: '/settings/providers' }),
      push: vi.fn(() => Promise.resolve()),
    }

    expect(handleRendererNavigate(router, '/settings/providers')).toBe(false)
    expect(handleRendererNavigate(router, '/settings-old')).toBe(false)

    expect(router.push).not.toHaveBeenCalled()
  })

  it('handles rejected router pushes', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const error = new Error('navigation failed')
    const router = {
      currentRoute: ref({ fullPath: '/' }),
      push: vi.fn(() => Promise.reject(error)),
    }

    handleRendererNavigate(router, '/settings/providers')
    await Promise.resolve()

    expect(warn).toHaveBeenCalledWith('failed to navigate renderer route', error)
    warn.mockRestore()
  })
})
