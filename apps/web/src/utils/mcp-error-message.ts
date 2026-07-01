import { resolveApiErrorMessage } from '@/utils/api-error'

type Translate = (key: string) => string

function normalized(value: string): string {
  return value.trim().replace(/\s+/g, ' ')
}

function isAuthorizationError(lower: string): boolean {
  return lower.includes('authorization failed')
    || lower.includes('unauthorized')
    || lower.includes('unauthenticated')
    || lower.includes('401')
    || lower.includes('invalid or expired token')
    || lower.includes('invalid token')
    || lower.includes('expired token')
    || lower.includes('invalid api key')
    || lower.includes('invalid_api_key')
}

function isPermissionError(lower: string): boolean {
  return lower.includes('permission denied')
    || lower.includes('forbidden')
    || lower.includes('403')
}

function isTimeoutError(lower: string): boolean {
  return lower.includes('connection timed out')
    || lower.includes('timed out')
    || lower.includes('timeout')
    || lower.includes('deadline exceeded')
}

function isOAuthCancelled(lower: string): boolean {
  return lower.includes('access_denied')
    || lower.includes('consent denied')
    || lower.includes('authorization denied')
}

function isOAuthExchangeError(lower: string): boolean {
  return lower.includes('invalid_grant')
    || lower.includes('invalid_request')
    || lower.includes('invalid_client')
    || lower.includes('token exchange')
}

function pluginNotReadyMessage(lower: string, t: Translate): string {
  if (!lower.includes('plugin is not ready')) return ''
  if (lower.includes('needs_config')) return t('bots.plugins.notReadyNeedsConfig')
  if (lower.includes('needs_auth')) return t('bots.plugins.notReadyNeedsAuth')
  if (lower.includes('admin_required')) return t('bots.plugins.notReadyAdminSetup')
  return t('bots.plugins.notReady')
}

export function mcpConnectionErrorMessage(raw: string, t: Translate): string {
  const text = normalized(raw)
  if (!text) return t('bots.plugins.mcpConnectionFailed')
  const lower = text.toLowerCase()
  if (isAuthorizationError(lower)) return t('bots.plugins.mcpAuthorizationFailed')
  if (isPermissionError(lower)) return t('bots.plugins.mcpPermissionDenied')
  if (isTimeoutError(lower)) return t('bots.plugins.mcpConnectionTimedOut')
  if (
    lower.includes('missing a connection')
    || lower.includes('declares mcp resources but none are installed')
    || lower.includes('oauth mcp resource')
  ) {
    return t('bots.plugins.resourcesMissing')
  }
  return t('bots.plugins.mcpConnectionCheckFailed')
}

export function resolvePluginActionErrorMessage(error: unknown, fallback: string, t: Translate): string {
  const text = normalized(resolveApiErrorMessage(error, fallback))
  if (!text || text === fallback) return fallback
  const lower = text.toLowerCase()

  const notReady = pluginNotReadyMessage(lower, t)
  if (notReady) return notReady
  if (lower.includes('oauth client') && lower.includes('not configured')) return t('bots.plugins.notReadyAdminSetup')
  if (lower.includes('plugin is already installed')) return t('bots.plugins.alreadyInstalled')
  if (lower.includes('plugin is uninstalled')) return t('bots.plugins.alreadyUninstalled')
  if (lower.includes('managed mcp name conflict')) return t('bots.plugins.mcpConnectionNameConflict')
  if (isOAuthCancelled(lower)) return t('mcp.oauth.authCancelled')
  if (isOAuthExchangeError(lower)) return t('mcp.oauth.authFailed')
  if (
    lower.includes('plugin mcp probe failed')
    || lower.includes('plugin mcp resource')
    || lower.includes('calling "initialize"')
    || lower.includes('sending "initialize"')
    || lower.includes('mcp connection')
  ) {
    return mcpConnectionErrorMessage(text, t)
  }
  return text
}

export function resolveMCPOAuthErrorMessage(error: unknown, fallback: string, t: Translate): string {
  const text = normalized(resolveApiErrorMessage(error, fallback))
  if (!text || text === fallback) return fallback
  const lower = text.toLowerCase()
  if (isOAuthCancelled(lower)) return t('mcp.oauth.authCancelled')
  if (isAuthorizationError(lower) || isOAuthExchangeError(lower)) return t('mcp.oauth.authFailed')
  if (isPermissionError(lower)) return t('bots.plugins.mcpPermissionDenied')
  if (isTimeoutError(lower)) return t('bots.plugins.mcpConnectionTimedOut')
  return text
}
