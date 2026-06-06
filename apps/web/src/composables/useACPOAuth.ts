import { onUnmounted, ref } from 'vue'
import { client } from '@memohai/sdk/client'

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

interface ACPCodexOAuthAuthorizeResponse {
  auth_url: string
}

interface ACPClaudeCodeOAuthAuthorizeResponse {
  auth_url: string
  session_id: string
}

/**
 * Bot-scoped ACP OAuth flows for Codex (ChatGPT) and Claude Code, shared by the
 * bot settings card and the onboarding wizard. All endpoints require a live
 * bot + managed workspace, so `getBotId` must resolve to an existing bot id.
 */
export function useACPOAuth(getBotId: () => string) {
  const codexStatus = ref<ACPCodexOAuthStatus | null>(null)
  const codexStatusLoading = ref(false)
  const authorizingCodex = ref(false)

  const claudeStatus = ref<ACPClaudeCodeOAuthStatus | null>(null)
  const claudeStatusLoading = ref(false)
  const authorizingClaude = ref(false)
  const exchangingClaude = ref(false)
  const claudeSessionId = ref('')

  async function loadCodexStatus(): Promise<ACPCodexOAuthStatus | null> {
    const botId = getBotId()
    if (!botId) return null
    codexStatusLoading.value = true
    try {
      const { data } = await client.get<{ 200: ACPCodexOAuthStatus }, unknown, true>({
        url: '/bots/{bot_id}/acp/codex/oauth/status',
        path: { bot_id: botId },
        throwOnError: true,
      })
      codexStatus.value = data ?? null
      return codexStatus.value
    } catch {
      codexStatus.value = null
      return null
    } finally {
      codexStatusLoading.value = false
    }
  }

  async function loadClaudeStatus(): Promise<ACPClaudeCodeOAuthStatus | null> {
    const botId = getBotId()
    if (!botId) return null
    claudeStatusLoading.value = true
    try {
      const { data } = await client.get<{ 200: ACPClaudeCodeOAuthStatus }, unknown, true>({
        url: '/bots/{bot_id}/acp/claude-code/oauth/status',
        path: { bot_id: botId },
        throwOnError: true,
      })
      claudeStatus.value = data ?? null
      return claudeStatus.value
    } catch {
      claudeStatus.value = null
      return null
    } finally {
      claudeStatusLoading.value = false
    }
  }

  // Teardown for an in-flight Codex authorize flow (listener + poll timer). Set
  // while a flow runs, invoked on a new flow, on finish, and on unmount so the
  // 120s poll and message listener never outlive the component.
  let cancelCodexFlow: (() => void) | null = null

  /** Opens the Codex authorize popup and polls status until a token is stored. */
  async function authorizeCodex(): Promise<boolean> {
    const botId = getBotId()
    if (!botId) return false
    cancelCodexFlow?.()
    authorizingCodex.value = true
    try {
      const { data } = await client.get<{ 200: ACPCodexOAuthAuthorizeResponse }, unknown, true>({
        url: '/bots/{bot_id}/acp/codex/oauth/authorize',
        path: { bot_id: botId },
        throwOnError: true,
      })
      if (!data?.auth_url) throw new Error('authorize failed')
      const popup = window.open(data.auth_url, 'acp-codex-oauth', 'width=600,height=720')
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
          if (success) await loadCodexStatus()
          authorizingCodex.value = false
          resolve(success)
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
              const status = await loadCodexStatus()
              if (status?.has_token) {
                await finish(true)
                return
              }
              // The popup is gone (user closed it, or the success page closed
              // itself). Re-check status once more so we don't miss a token that
              // was stored right as the window closed, then stop polling.
              if (popup?.closed) {
                const finalStatus = await loadCodexStatus()
                await finish(!!finalStatus?.has_token)
                return
              }
              if (Date.now() - startedAt < 120_000) poll()
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

  /** Opens the Claude Code authorize popup; the user then pastes the code into `exchangeClaude`. */
  async function authorizeClaude(): Promise<boolean> {
    const botId = getBotId()
    if (!botId) return false
    authorizingClaude.value = true
    try {
      const { data } = await client.get<{ 200: ACPClaudeCodeOAuthAuthorizeResponse }, unknown, true>({
        url: '/bots/{bot_id}/acp/claude-code/oauth/authorize',
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
      const { data } = await client.post<{ 200: ACPClaudeCodeOAuthStatus }, unknown, true>({
        url: '/bots/{bot_id}/acp/claude-code/oauth/exchange',
        path: { bot_id: botId },
        body: {
          session_id: claudeSessionId.value,
          code: trimmed,
        },
        throwOnError: true,
      })
      claudeStatus.value = data ?? { configured: true, has_token: true }
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
  })

  return {
    codexStatus,
    codexStatusLoading,
    authorizingCodex,
    claudeStatus,
    claudeStatusLoading,
    authorizingClaude,
    exchangingClaude,
    claudeSessionId,
    loadCodexStatus,
    loadClaudeStatus,
    authorizeCodex,
    authorizeClaude,
    exchangeClaude,
  }
}
