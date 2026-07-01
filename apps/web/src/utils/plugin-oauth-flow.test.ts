// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { waitForPluginOAuth } from './plugin-oauth-flow'
import type { OAuthPopupFlowController } from './oauth/popup-flow'
import type { PluginsInstallation } from '@memohai/sdk'

function readyInstallation(): PluginsInstallation {
  return { id: 'install-1', status: 'ready', enabled: false }
}

function pendingInstallation(): PluginsInstallation {
  return { id: 'install-1', status: 'needs_auth', enabled: false }
}

function messageEvent(data: unknown, source?: MessageEventSource | null): MessageEvent {
  const event = new Event('message') as MessageEvent
  Object.defineProperty(event, 'data', { value: data })
  if (source !== undefined) Object.defineProperty(event, 'source', { value: source })
  return event
}

describe('waitForPluginOAuth', () => {
  const t = (key: string) => key
  let originalWindowApi: typeof window.api

  beforeEach(() => {
    vi.useFakeTimers()
    originalWindowApi = window.api
  })

  afterEach(() => {
    window.api = originalWindowApi
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('lets desktop external OAuth waits be cancelled', async () => {
    let controller: OAuthPopupFlowController | undefined
    const onCleanup = vi.fn()
    const fetchStatus = vi.fn(async () => pendingInstallation())

    const result = waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: null,
      external: true,
      fetchStatus,
      t,
      onController: value => {
        controller = value
      },
      onCleanup,
    })

    controller?.cancel()

    await expect(result).resolves.toBe('cancelled')
    expect(onCleanup).toHaveBeenCalledTimes(1)
  })

  it('reports uninstalled when external polling sees a removed installation', async () => {
    const fetchStatus = vi.fn(async () => {
      throw new Error('plugin installation not found')
    })

    await expect(waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: null,
      external: true,
      fetchStatus,
      t,
    })).resolves.toBe('uninstalled')
  })

  it('rejects repeated external polling errors instead of waiting for timeout', async () => {
    const fetchStatus = vi.fn(async () => {
      throw new Error('server unreachable')
    })

    const result = waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: null,
      external: true,
      fetchStatus,
      t,
    })
    const rejected = expect(result).rejects.toThrow('server unreachable')

    await vi.advanceTimersByTimeAsync(4_000)
    await rejected
    expect(fetchStatus).toHaveBeenCalledTimes(3)
  })

  it('settles popup waits when disposed by the plugin wrapper', async () => {
    let controller: OAuthPopupFlowController | undefined
    const popup = { closed: false, close: vi.fn(() => { popup.closed = true }) }
    const fetchStatus = vi.fn(async () => pendingInstallation())

    const result = waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: popup as unknown as Window,
      external: false,
      fetchStatus,
      t,
      onController: value => {
        controller = value
      },
    })

    controller?.dispose()

    await expect(result).resolves.toBe('cancelled')
    expect(popup.close).toHaveBeenCalledTimes(1)
  })

  it('authorizes popup waits from the matching popup callback', async () => {
    const popup = { closed: false, close: vi.fn(() => { popup.closed = true }) }
    const fetchStatus = vi.fn(async () => pendingInstallation())

    const result = waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: popup as unknown as Window,
      external: false,
      fetchStatus,
      t,
    })

    window.dispatchEvent(messageEvent(
      { type: 'mcp-oauth-callback', status: 'success' },
      popup as unknown as MessageEventSource,
    ))

    await expect(result).resolves.toBe('authorized')
  })

  it('authorizes external waits when polling returns ready', async () => {
    const fetchStatus = vi.fn(async () => readyInstallation())

    await expect(waitForPluginOAuth({
      botId: 'bot-1',
      installationId: 'install-1',
      popup: null,
      external: true,
      fetchStatus,
      t,
    })).resolves.toBe('authorized')
  })
})
