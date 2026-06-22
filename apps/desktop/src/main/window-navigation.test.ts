import { describe, expect, it, vi } from 'vitest'
import { dispatchRendererNavigate, RENDERER_NAVIGATE_CHANNEL } from './window-navigation'

function createWindow(options: { loading?: boolean, destroyed?: boolean } = {}) {
  return {
    isDestroyed: vi.fn(() => options.destroyed ?? false),
    webContents: {
      isLoading: vi.fn(() => options.loading ?? false),
      once: vi.fn(),
      send: vi.fn(),
    },
  }
}

describe('dispatchRendererNavigate', () => {
  it('sends navigation targets to ready renderers', () => {
    const window = createWindow()

    dispatchRendererNavigate(window, '/settings/providers')

    expect(window.webContents.send).toHaveBeenCalledWith(
      RENDERER_NAVIGATE_CHANNEL,
      '/settings/providers',
    )
    expect(window.webContents.once).not.toHaveBeenCalled()
  })

  it('waits for loading renderers before sending', () => {
    const window = createWindow({ loading: true })

    dispatchRendererNavigate(window, '/bot/demo')

    expect(window.webContents.send).not.toHaveBeenCalled()
    expect(window.webContents.once).toHaveBeenCalledWith('did-finish-load', expect.any(Function))

    const listener = window.webContents.once.mock.calls[0]?.[1] as (() => void)
    listener()

    expect(window.webContents.send).toHaveBeenCalledWith(RENDERER_NAVIGATE_CHANNEL, '/bot/demo')
  })

  it('drops pending navigation if the renderer is destroyed before load finishes', () => {
    const window = createWindow({ loading: true, destroyed: true })

    dispatchRendererNavigate(window, '/settings')

    const listener = window.webContents.once.mock.calls[0]?.[1] as (() => void)
    listener()

    expect(window.webContents.send).not.toHaveBeenCalled()
  })
})
