import type { PluginsInstallation } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { startOAuthPopupFlow, type OAuthPopupFlowController } from '@/utils/oauth/popup-flow'

export type PluginOAuthWaitResult = 'authorized' | 'cancelled' | 'uninstalled' | 'timeout'

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
  return waitForPluginOAuthPopup(options)
}

function waitForPluginOAuthPopup(options: PluginOAuthWaitOptions & { popup: Window }): Promise<PluginOAuthWaitResult> {
  return new Promise((resolve, reject) => {
    const cleanup = () => options.onCleanup?.()
    let pollErrorCount = 0
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
      isAuthorized: status => status?.status === 'ready' || !!status?.enabled,
      abortOnPollError: error => isPluginInstallationNotFoundError(error) ? 'cancelled' : false,
      failOnPollError: (error) => {
        pollErrorCount += 1
        return pollErrorCount >= PLUGIN_OAUTH_POLL_ERROR_LIMIT ? error : false
      },
      onAuthorized: () => {
        cleanup()
        resolve('authorized')
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
    let pollTimer: ReturnType<typeof window.setTimeout> | undefined
    let timeoutTimer: ReturnType<typeof window.setTimeout> | undefined

    const cleanup = () => {
      if (pollTimer) {
        window.clearTimeout(pollTimer)
        pollTimer = undefined
      }
      if (timeoutTimer) {
        window.clearTimeout(timeoutTimer)
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
          if (status.status === 'ready' || status.enabled) {
            finish('authorized')
            return
          }
          pollErrorCount = 0
          pollTimer = window.setTimeout(poll, 2_000)
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
          pollTimer = window.setTimeout(poll, 2_000)
        })
    }

    timeoutTimer = window.setTimeout(() => finish('timeout'), 120_000)
    options.onController?.({
      cancel: () => finish('cancelled'),
      dispose: () => finish('cancelled'),
    })
    poll()
  })
}
