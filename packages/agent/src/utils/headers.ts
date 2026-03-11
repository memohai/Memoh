import { AgentAuthContext, IdentityContext } from '../types'

export interface BuildHeadersOptions {
  isSubagent?: boolean
}

export const buildIdentityHeaders = (identity: IdentityContext, auth: AgentAuthContext, options?: BuildHeadersOptions) => {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${auth.bearer}`,
  }
  if (identity.channelIdentityId) {
    headers['X-Memoh-Channel-Identity-Id'] = identity.channelIdentityId
  }
  if (identity.sessionToken) {
    headers['X-Memoh-Session-Token'] = identity.sessionToken
  }
  if (identity.currentPlatform) {
    headers['X-Memoh-Current-Platform'] = identity.currentPlatform
  }
  if (identity.replyTarget) {
    headers['X-Memoh-Reply-Target'] = identity.replyTarget
  }
  if (options?.isSubagent) {
    headers['X-Memoh-Is-Subagent'] = 'true'
  }
  return headers
}