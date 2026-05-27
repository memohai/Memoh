export type AuthSessionClearReason = 'login' | 'logout' | 'token-cleared' | 'unauthorized'

export interface AuthSessionClearedDetail {
  reason: AuthSessionClearReason
}

export const AUTH_SESSION_CLEARED_EVENT = 'memoh:auth-session-cleared'

const USER_SCOPED_STORAGE_KEYS = [
  'chat-bot-id',
  'chat-session-id',
  'chat-input-drafts',
  'pinned-bot-ids',
  'workspace-tabs',
]

export function clearPersistedUserScopedState() {
  if (typeof localStorage === 'undefined') return

  for (const key of USER_SCOPED_STORAGE_KEYS) {
    localStorage.removeItem(key)
  }
}

export function notifyAuthSessionCleared(reason: AuthSessionClearReason) {
  clearPersistedUserScopedState()

  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<AuthSessionClearedDetail>(AUTH_SESSION_CLEARED_EVENT, {
    detail: { reason },
  }))
}

export function onAuthSessionCleared(callback: (detail: AuthSessionClearedDetail) => void) {
  if (typeof window === 'undefined') return () => {}

  const listener = (event: Event) => {
    callback((event as CustomEvent<AuthSessionClearedDetail>).detail)
  }
  window.addEventListener(AUTH_SESSION_CLEARED_EVENT, listener)
  return () => window.removeEventListener(AUTH_SESSION_CLEARED_EVENT, listener)
}
