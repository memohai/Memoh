// Best-effort wrappers around Web Storage.
//
// Some browsers and privacy modes throw when accessing or writing storage
// (e.g. `SecurityError` when site data is blocked, `QuotaExceededError` on
// write). The onboarding flow only uses storage for non-critical UX state, so
// a storage failure must never break the flow — it is swallowed and logged.

function warn(action: string, key: string, error: unknown) {
  console.warn(`[safe-storage] ${action} failed for "${key}"`, error)
}

export function safeSessionSet(key: string, value: string): boolean {
  try {
    sessionStorage.setItem(key, value)
    return true
  } catch (error) {
    warn('sessionStorage.setItem', key, error)
    return false
  }
}

export function safeSessionGet(key: string): string | null {
  try {
    return sessionStorage.getItem(key)
  } catch (error) {
    warn('sessionStorage.getItem', key, error)
    return null
  }
}

export function safeSessionRemove(key: string): void {
  try {
    sessionStorage.removeItem(key)
  } catch (error) {
    warn('sessionStorage.removeItem', key, error)
  }
}

export function safeLocalSet(key: string, value: string): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch (error) {
    warn('localStorage.setItem', key, error)
    return false
  }
}

export function safeLocalGet(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch (error) {
    warn('localStorage.getItem', key, error)
    return null
  }
}

export function safeLocalRemove(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch (error) {
    warn('localStorage.removeItem', key, error)
  }
}
