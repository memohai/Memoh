import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

describe('chat-selection store', () => {
  let storage: Map<string, string>

  beforeEach(() => {
    vi.resetModules()
    storage = new Map<string, string>()
    const localStorageMock = {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    }
    vi.stubGlobal('localStorage', localStorageMock)
    vi.stubGlobal('document', {})
    vi.stubGlobal('window', {
      localStorage: localStorageMock,
      document: {},
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
    setActivePinia(createPinia())
  })

  it('does not migrate a legacy stored session id into an explicit selection', async () => {
    const { useChatSelectionStore } = await import('./chat-selection')
    localStorage.setItem('chat-session-id', 'history-session-1')

    const selection = useChatSelectionStore()

    expect(selection.sessionId).toBe('history-session-1')
    expect(selection.explicitSelection).toBe(false)
  })

  it('preserves an existing explicit selection flag', async () => {
    const { useChatSelectionStore } = await import('./chat-selection')
    localStorage.setItem('chat-session-id', 'manual-session-1')
    localStorage.setItem('chat-explicit-selection', 'true')

    const selection = useChatSelectionStore()

    expect(selection.sessionId).toBe('manual-session-1')
    expect(selection.explicitSelection).toBe(true)
  })
})
