import type { PluginsInstallation } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { startOAuthPopupFlow, type OAuthPopupFlowController } from '@/utils/oauth/popup-flow'

export type PluginOAuthWaitResult = 'authorized' | 'cancelled' | 'uninstalled' | 'timeout' | 'needs_config' | 'admin_required'

type Translate = (key: string) => string
const PLUGIN_OAUTH_POLL_ERROR_LIMIT = 3

interface PluginOAuthOpenResult {
  popup: Window | null
  external: boolean
}

interface PluginOAuthWaitOptions {
  botId: string
  installationId: string
  popup: Window | null
  external: boolean
  fetchStatus: (botId: string, installationId: string) => Promise<PluginsInstallation>
  t: Translate
  onController?: (controller: OAuthPopupFlowController) => void
  onCleanup?: () => void
}

export function isPluginInstallationNotFoundError(error: unknown): boolean {
  return resolveApiErrorMessage(error, '').trim().toLowerCase() === 'plugin installation not found'
}

export async function openPluginOAuthURL(url: string, popup?: Window | null): Promise<PluginOAuthOpenResult> {
  if (popup && !popup.closed) {
    popup.location.href = url
    return { popup, external: false }
  }

  const desktopOpenExternal = window.api?.desktop?.openExternalUrl
  if (desktopOpenExternal) {
    await desktopOpenExternal(url)
    return { popup: null, external: true }
  }

  return {
    popup: window.open(url, 'mcp-oauth', 'width=600,height=700'),
    external: false,
  }
}

export async function waitForPluginOAuth(options: PluginOAuthWaitOptions): Promise<PluginOAuthWaitResult> {
  if (options.external) {
    return waitForPluginOAuthWithoutPopup(options)
  }
  if (!options.popup) {
    throw new Error(options.t('mcp.oauth.flowInitFailed'))
  }
  return waitForPluginOAuthPopup({ ...options, popup: options.popup })
}

export function isPluginOAuthAuthorized(status: PluginsInstallation | null | undefined): boolean {
  const value = status?.status?.trim()
  if (value) return value === 'ready'
  return !!status?.enabled
}

function pluginOAuthWaitResult(status: PluginsInstallation | null | undefined): PluginOAuthWaitResult | null {
  if (status?.status === 'needs_config') return 'needs_config'
  if (status?.status === 'admin_required') return 'admin_required'
  if (status?.status === 'uninstalled') return 'uninstalled'
  if (isPluginOAuthAuthorized(status)) return 'authorized'
  return null
}

function waitForPluginOAuthPopup(options: PluginOAuthWaitOptions & { popup: Window }): Promise<PluginOAuthWaitResult> {
  return new Promise((resolve, reject) => {
    const cleanup = () => options.onCleanup?.()
    let pollErrorCount = 0
    let latestPollResult: PluginOAuthWaitResult | null = null
    const flow = startOAuthPopupFlow<PluginsInstallation>({
      popup: options.popup,
      target: window,
      expectedSource: options.popup,
      messageType: 'mcp-oauth-callback',
      messageMatches: event => event.data?.status === 'success',
      messageError: event => event.data?.status === 'error'
        ? new Error(String(event.data?.error || options.t('mcp.oauth.authFailed')))
        : null,
      pollIntervalMs: 2_000,
      timeoutMs: 120_000,
      pollStatus: () => options.fetchStatus(options.botId, options.installationId),
      isAuthorized: (status) => {
        pollErrorCount = 0
        latestPollResult = pluginOAuthWaitResult(status)
        return latestPollResult !== null
      },
      abortOnPollError: error => isPluginInstallationNotFoundError(error) ? 'cancelled' : false,
      failOnPollError: (error) => {
        pollErrorCount += 1
        return pollErrorCount >= PLUGIN_OAUTH_POLL_ERROR_LIMIT ? error : false
      },
      onAuthorized: () => {
        cleanup()
        resolve(latestPollResult ?? 'authorized')
      },
      onAborted: (reason) => {
        cleanup()
        resolve(reason === 'timeout' ? 'timeout' : 'cancelled')
      },
      onFailed: (error) => {
        cleanup()
        reject(error)
      },
    })
    options.onController?.({
      cancel: () => flow.cancel(),
      dispose: () => flow.cancel(),
    })
  })
}

function waitForPluginOAuthWithoutPopup(options: PluginOAuthWaitOptions): Promise<PluginOAuthWaitResult> {
  return new Promise((resolve, reject) => {
    let completed = false
    let pollErrorCount = 0
    let pollTimer: ReturnType<typeof globalThis.setTimeout> | undefined
    let timeoutTimer: ReturnType<typeof globalThis.setTimeout> | undefined

    const cleanup = () => {
      if (pollTimer) {
        globalThis.clearTimeout(pollTimer)
        pollTimer = undefined
      }
      if (timeoutTimer) {
        globalThis.clearTimeout(timeoutTimer)
        timeoutTimer = undefined
      }
      options.onCleanup?.()
    }

    const finish = (result: PluginOAuthWaitResult) => {
      if (completed) return
      completed = true
      cleanup()
      resolve(result)
    }

    const poll = () => {
      if (completed) return
      void Promise.resolve()
        .then(() => options.fetchStatus(options.botId, options.installationId))
        .then((status) => {
          if (completed) return
          const result = pluginOAuthWaitResult(status)
          if (result) {
            finish(result)
            return
          }
          pollErrorCount = 0
          pollTimer = globalThis.setTimeout(poll, 2_000)
        })
        .catch((error: unknown) => {
          if (completed) return
          if (isPluginInstallationNotFoundError(error)) {
            finish('uninstalled')
            return
          }
          pollErrorCount += 1
          if (pollErrorCount >= PLUGIN_OAUTH_POLL_ERROR_LIMIT) {
            completed = true
            cleanup()
            reject(error)
            return
          }
          pollTimer = globalThis.setTimeout(poll, 2_000)
        })
    }

    timeoutTimer = globalThis.setTimeout(() => finish('timeout'), 120_000)
    options.onController?.({
      cancel: () => finish('cancelled'),
      dispose: () => finish('cancelled'),
    })
    poll()
  })
}
