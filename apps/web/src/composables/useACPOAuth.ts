import { computed, onUnmounted, ref } from 'vue'
import {
  getBotsByBotIdAcpClaudeCodeOauthAuthorize,
  getBotsByBotIdAcpClaudeCodeOauthStatus,
  getBotsByBotIdAcpCodexOauthAuthorize,
  getBotsByBotIdAcpCodexOauthStatus,
  postBotsByBotIdAcpClaudeCodeOauthExchange,
  postBotsByBotIdAcpCodexOauthDeviceAuthorize,
  postBotsByBotIdAcpCodexOauthDeviceCancel,
  postBotsByBotIdAcpCodexOauthDevicePoll,
  type HandlersAcpClaudeCodeOAuthStatus,
  type HandlersAcpCodexOAuthDeviceAuthorizeResponse,
  type HandlersAcpCodexOAuthDeviceStatusResponse,
  type HandlersAcpCodexOAuthStatus,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

export interface ACPCodexOAuthStatus {
  configured: boolean
  has_token: boolean
  callback_url?: string
  account_id?: string
}

export interface ACPClaudeCodeOAuthStatus {
  configured: boolean
  has_token: boolean
}

interface OAuthStatusLoadOptions {
  silent?: boolean
}

interface AuthorizeCodexOptions {
  timeoutMs?: number
}

type ACPCodexOAuthDeviceState = 'pending' | 'writing' | 'success' | 'error' | 'cancelled' | 'expired'
export type ACPCodexDeviceOpenResult = 'opened' | 'popup_blocked' | 'copy_failed'

export interface ACPCodexOAuthDeviceSession {
  bot_id: string
  session_id: string
  verification_url: string
  user_code: string
  expires_at?: string
  interval_seconds: number
  next_poll_after?: string
  status: ACPCodexOAuthDeviceState
  has_token: boolean
  account_id?: string
  error?: string
}

/**
 * Bot-scoped ACP OAuth flows for Codex and Claude Code, shared by the
 * bot settings card and the onboarding wizard. All endpoints require a live
 * bot + managed workspace, so `getBotId` must resolve to an existing bot id.
 */
export function useACPOAuth(getBotId: () => string) {
  const codexStatus = ref<ACPCodexOAuthStatus | null>(null)
  const codexStatusBotId = ref('')
  const codexStatusLoading = ref(false)
  const authorizingCodex = ref(false)
  const authorizingCodexDevice = ref(false)
  const codexDeviceSession = ref<ACPCodexOAuthDeviceSession | null>(null)

  const claudeStatus = ref<ACPClaudeCodeOAuthStatus | null>(null)
  const claudeStatusBotId = ref('')
  const claudeStatusLoading = ref(false)
  const authorizingClaude = ref(false)
  const exchangingClaude = ref(false)
  const claudeSessionId = ref('')

  const codexDevicePending = computed(() => {
    const session = codexDeviceSession.value
    if (!session || session.bot_id !== getBotId()) return false
    const status = session.status
    return status === 'pending' || status === 'writing'
  })

  const codexDeviceVerificationReady = computed(() => codexDevicePending.value)

  const codexAuthorizing = computed(() =>
    authorizingCodex.value || authorizingCodexDevice.value || codexDevicePending.value,
  )

  function normalizeCodexStatus(data: HandlersAcpCodexOAuthStatus | undefined): ACPCodexOAuthStatus | null {
    if (!data) return null
    return {
      configured: Boolean(data.configured),
      has_token: Boolean(data.has_token),
      callback_url: data.callback_url,
      account_id: data.account_id,
    }
  }

  function normalizeClaudeStatus(data: HandlersAcpClaudeCodeOAuthStatus | undefined): ACPClaudeCodeOAuthStatus | null {
    if (!data) return null
    return {
      configured: Boolean(data.configured),
      has_token: Boolean(data.has_token),
    }
  }

  async function loadCodexStatus(options: OAuthStatusLoadOptions = {}): Promise<ACPCodexOAuthStatus | null> {
    const botId = getBotId()
    if (!botId) return null
    if (!options.silent) {
      if (codexStatusBotId.value !== botId) codexStatus.value = null
      codexStatusLoading.value = true
    }
    try {
      const { data } = await getBotsByBotIdAcpCodexOauthStatus({
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (botId !== getBotId()) return codexStatus.value
      codexStatus.value = normalizeCodexStatus(data)
      codexStatusBotId.value = botId
      return codexStatus.value
    } catch {
      if (botId !== getBotId()) return codexStatus.value
      if (!options.silent) {
        codexStatus.value = null
        codexStatusBotId.value = botId
      }
      return null
    } finally {
      if (!options.silent && botId === getBotId()) codexStatusLoading.value = false
    }
  }

  async function loadClaudeStatus(): Promise<ACPClaudeCodeOAuthStatus | null> {
    const botId = getBotId()
    if (!botId) return null
    if (claudeStatusBotId.value !== botId) claudeStatus.value = null
    claudeStatusLoading.value = true
    try {
      const { data } = await getBotsByBotIdAcpClaudeCodeOauthStatus({
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (botId !== getBotId()) return claudeStatus.value
      claudeStatus.value = normalizeClaudeStatus(data)
      claudeStatusBotId.value = botId
      return claudeStatus.value
    } catch {
      if (botId !== getBotId()) return claudeStatus.value
      claudeStatus.value = null
      claudeStatusBotId.value = botId
      return null
    } finally {
      if (botId === getBotId()) claudeStatusLoading.value = false
    }
  }

  // Teardown for an in-flight Codex authorize flow (listener + poll timer). Set
  // while a flow runs, invoked on a new flow, on finish, and on unmount so the
  // 120s poll and message listener never outlive the component.
  let cancelCodexFlow: (() => void) | null = null
  let codexDevicePollTimer: ReturnType<typeof globalThis.setTimeout> | null = null
  let codexDeviceGeneration = 0

  function clearCodexDevicePollTimer() {
    if (codexDevicePollTimer === null) return
    globalThis.clearTimeout(codexDevicePollTimer)
    codexDevicePollTimer = null
  }

  function replaceCodexDeviceSession(botId: string, response: HandlersAcpCodexOAuthDeviceAuthorizeResponse) {
    const sessionID = response.session_id?.trim()
    const verificationURL = response.verification_url?.trim()
    const userCode = response.user_code?.trim()
    if (!sessionID || !verificationURL || !userCode) {
      throw new Error('device authorization failed')
    }
    codexDeviceSession.value = {
      bot_id: botId,
      session_id: sessionID,
      verification_url: verificationURL,
      user_code: userCode,
      expires_at: response.expires_at,
      interval_seconds: Math.max(response.interval_seconds ?? 5, 1),
      status: 'pending',
      has_token: false,
    }
  }

  function applyCodexDeviceStatus(response: HandlersAcpCodexOAuthDeviceStatusResponse | undefined, botId?: string) {
    const current = codexDeviceSession.value
    if (!current || !response) return
    if (botId && current.bot_id !== botId) return
    const status = (response.status || current.status) as ACPCodexOAuthDeviceState
    codexDeviceSession.value = {
      ...current,
      status,
      has_token: Boolean(response.has_token),
      account_id: response.account_id || current.account_id,
      error: response.error || undefined,
      expires_at: response.expires_at || current.expires_at,
      interval_seconds: Math.max(response.interval_seconds ?? current.interval_seconds ?? 5, 1),
      next_poll_after: response.next_poll_after,
    }
  }

  function codexDevicePollDelayMs(session: ACPCodexOAuthDeviceSession): number {
    if (session.next_poll_after) {
      const delay = new Date(session.next_poll_after).getTime() - Date.now()
      if (Number.isFinite(delay) && delay > 0) return delay
    }
    return Math.max(session.interval_seconds, 1) * 1000
  }

  function scheduleCodexDevicePoll(generation: number) {
    clearCodexDevicePollTimer()
    const session = codexDeviceSession.value
    if (!session || session.bot_id !== getBotId() || !codexDevicePending.value) return
    codexDevicePollTimer = globalThis.setTimeout(() => {
      codexDevicePollTimer = null
      if (generation !== codexDeviceGeneration) return
      void pollCodexDeviceAuthorization(generation)
    }, codexDevicePollDelayMs(session))
  }

  async function pollCodexDeviceAuthorization(generation = codexDeviceGeneration): Promise<ACPCodexOAuthDeviceSession | null> {
    const botId = getBotId()
    const session = codexDeviceSession.value
    if (!botId || !session || session.bot_id !== botId) return null
    try {
      const { data } = await postBotsByBotIdAcpCodexOauthDevicePoll({
        path: { bot_id: botId },
        body: { session_id: session.session_id },
        throwOnError: true,
      })
      if (generation !== codexDeviceGeneration || botId !== getBotId()) return codexDeviceSession.value
      applyCodexDeviceStatus(data, botId)
      if (codexDeviceSession.value?.has_token || codexDeviceSession.value?.status === 'success') {
        codexStatus.value = {
          configured: true,
          has_token: true,
          callback_url: codexStatus.value?.callback_url,
          account_id: codexDeviceSession.value.account_id,
        }
        codexStatusBotId.value = botId
        await loadCodexStatus({ silent: true })
        clearCodexDevicePollTimer()
      } else if (codexDevicePending.value) {
        scheduleCodexDevicePoll(generation)
      } else {
        clearCodexDevicePollTimer()
      }
      return codexDeviceSession.value
    } catch (error) {
      if (generation !== codexDeviceGeneration || botId !== getBotId()) return codexDeviceSession.value
      const current = codexDeviceSession.value
      if (current) {
        codexDeviceSession.value = {
          ...current,
          status: 'error',
          has_token: false,
          error: resolveApiErrorMessage(error, 'device authorization failed'),
        }
      }
      clearCodexDevicePollTimer()
      return codexDeviceSession.value
    }
  }

  function clearCodexDeviceAuthorization() {
    codexDeviceGeneration += 1
    clearCodexDevicePollTimer()
    codexDeviceSession.value = null
    authorizingCodexDevice.value = false
  }

  async function cancelCodexDeviceSessionOnServer(session: ACPCodexOAuthDeviceSession): Promise<void> {
    if (session.status !== 'pending' && session.status !== 'writing') return
    try {
      await postBotsByBotIdAcpCodexOauthDeviceCancel({
        path: { bot_id: session.bot_id },
        body: { session_id: session.session_id },
        throwOnError: true,
      })
    } catch {
      // Best-effort cleanup only; the backend TTL still reclaims abandoned sessions.
    }
  }

  /** Opens the Codex authorize popup and polls status until a token is stored. */
  async function authorizeCodex(options: AuthorizeCodexOptions = {}): Promise<boolean> {
    const botId = getBotId()
    if (!botId) return false
    const timeoutMs = options.timeoutMs ?? 120_000
    cancelCodexFlow?.()
    clearCodexDeviceAuthorization()
    authorizingCodex.value = true
    try {
      const { data } = await getBotsByBotIdAcpCodexOauthAuthorize({
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (!data?.auth_url) throw new Error('authorize failed')
      if (botId !== getBotId()) {
        authorizingCodex.value = false
        return false
      }
      const popup = window.open(data.auth_url, 'acp-codex-oauth', 'width=600,height=720')
      if (!popup) {
        authorizingCodex.value = false
        return false
      }
      return await new Promise<boolean>((resolve) => {
        const startedAt = Date.now()
        let completed = false
        let timer = 0
        const teardown = () => {
          window.removeEventListener('message', listener)
          if (timer) window.clearTimeout(timer)
          cancelCodexFlow = null
        }
        const finish = async (success: boolean) => {
          if (completed) return
          completed = true
          teardown()
          popup?.close()
          const stillCurrent = botId === getBotId()
          if (success && stillCurrent) await loadCodexStatus()
          authorizingCodex.value = false
          resolve(success && stillCurrent)
        }
        // Abrupt cancel (component unmount / new flow): stop without a status
        // refetch and resolve false.
        cancelCodexFlow = () => {
          if (completed) return
          completed = true
          teardown()
          popup?.close()
          authorizingCodex.value = false
          resolve(false)
        }
        const poll = () => {
          timer = window.setTimeout(() => {
            void (async () => {
              if (completed) return
              if (botId !== getBotId()) {
                await finish(false)
                return
              }
              const status = await loadCodexStatus()
              if (botId !== getBotId()) {
                await finish(false)
                return
              }
              if (status?.has_token) {
                await finish(true)
                return
              }
              // The popup is gone (user closed it, or the success page closed
              // itself). Re-check status once more so we don't miss a token that
              // was stored right as the window closed, then stop polling.
              if (popup?.closed) {
                const finalStatus = await loadCodexStatus()
                if (botId !== getBotId()) {
                  await finish(false)
                  return
                }
                await finish(!!finalStatus?.has_token)
                return
              }
              if (Date.now() - startedAt < timeoutMs) poll()
              else await finish(false)
            })()
          }, 1_500)
        }
        const listener = (event: MessageEvent) => {
          // Only trust same-origin success pings; cross-origin (e.g. desktop)
          // still completes via the status poll above.
          if (event.origin !== window.location.origin) return
          if (event.data?.type === 'memoh-acp-codex-oauth-success' && event.data?.botId === botId) {
            void finish(true)
          }
        }
        window.addEventListener('message', listener)
        poll()
      })
    } catch {
      authorizingCodex.value = false
      return false
    }
  }

  async function authorizeCodexDevice(): Promise<boolean> {
    const botId = getBotId()
    if (!botId) return false
    cancelCodexFlow?.()
    clearCodexDeviceAuthorization()
    authorizingCodexDevice.value = true
    try {
      const { data } = await postBotsByBotIdAcpCodexOauthDeviceAuthorize({
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (botId !== getBotId()) return false
      replaceCodexDeviceSession(botId, data ?? {})
      codexDeviceGeneration += 1
      scheduleCodexDevicePoll(codexDeviceGeneration)
      return true
    } catch {
      return false
    } finally {
      authorizingCodexDevice.value = false
    }
  }

  async function cancelCodexDeviceAuthorization(): Promise<boolean> {
    const botId = getBotId()
    const session = codexDeviceSession.value
    if (!botId || !session || session.bot_id !== botId) {
      clearCodexDeviceAuthorization()
      return false
    }
    codexDeviceGeneration += 1
    clearCodexDevicePollTimer()
    try {
      const { data } = await postBotsByBotIdAcpCodexOauthDeviceCancel({
        path: { bot_id: botId },
        body: { session_id: session.session_id },
        throwOnError: true,
      })
      applyCodexDeviceStatus(data, botId)
      codexDeviceSession.value = null
      return true
    } catch {
      if (session.bot_id === getBotId()) codexDeviceSession.value = null
      return false
    }
  }

  async function openCodexDeviceVerification(copyText: (text: string) => Promise<boolean>): Promise<ACPCodexDeviceOpenResult> {
    const session = codexDeviceSession.value
    if (!session || session.bot_id !== getBotId() || !codexDeviceVerificationReady.value) return 'copy_failed'
    const userCode = session?.user_code?.trim()
    const verificationURL = session?.verification_url?.trim()
    if (!userCode || !verificationURL) return 'copy_failed'

    const popup = window.open('', 'acp-codex-device-oauth', 'width=960,height=720')
    const copied = await copyText(userCode)
    if (!copied) {
      popup?.close()
      return 'copy_failed'
    }
    if (popup) {
      popup.location.href = verificationURL
      popup.focus()
      return 'opened'
    }
    const fallback = window.open(verificationURL, '_blank', 'width=960,height=720')
    return fallback ? 'opened' : 'popup_blocked'
  }

  /** Opens the Claude Code authorize popup; the user then pastes the code into `exchangeClaude`. */
  async function authorizeClaude(): Promise<boolean> {
    const botId = getBotId()
    if (!botId) return false
    authorizingClaude.value = true
    try {
      const { data } = await getBotsByBotIdAcpClaudeCodeOauthAuthorize({
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (!data?.auth_url || !data.session_id) throw new Error('authorize failed')
      claudeSessionId.value = data.session_id
      window.open(data.auth_url, 'acp-claude-code-oauth', 'width=600,height=720')
      return true
    } catch {
      return false
    } finally {
      authorizingClaude.value = false
    }
  }

  async function exchangeClaude(code: string): Promise<boolean> {
    const botId = getBotId()
    const trimmed = code.trim()
    if (!botId || !trimmed || !claudeSessionId.value) return false
    exchangingClaude.value = true
    try {
      const { data } = await postBotsByBotIdAcpClaudeCodeOauthExchange({
        path: { bot_id: botId },
        body: {
          session_id: claudeSessionId.value,
          code: trimmed,
        },
        throwOnError: true,
      })
      claudeStatus.value = normalizeClaudeStatus(data) ?? { configured: true, has_token: true }
      claudeStatusBotId.value = botId
      claudeSessionId.value = ''
      return !!claudeStatus.value.has_token
    } catch {
      return false
    } finally {
      exchangingClaude.value = false
    }
  }

  onUnmounted(() => {
    cancelCodexFlow?.()
    const deviceSession = codexDeviceSession.value
    clearCodexDeviceAuthorization()
    if (deviceSession) void cancelCodexDeviceSessionOnServer(deviceSession)
  })

  return {
    codexStatus,
    codexStatusLoading,
    authorizingCodex,
    authorizingCodexDevice,
    codexAuthorizing,
    codexDeviceSession,
    codexDevicePending,
    codexDeviceVerificationReady,
    claudeStatus,
    claudeStatusLoading,
    authorizingClaude,
    exchangingClaude,
    claudeSessionId,
    loadCodexStatus,
    loadClaudeStatus,
    authorizeCodex,
    authorizeCodexDevice,
    cancelCodexDeviceAuthorization,
    clearCodexDeviceAuthorization,
    openCodexDeviceVerification,
    authorizeClaude,
    exchangeClaude,
  }
}
