const SERVER_PROBE_TIMEOUT_MS = 5000

export type ServerConnectionErrorCode =
  | 'required'
  | 'invalid-url'
  | 'unsupported-protocol'
  | 'timeout'
  | 'unreachable'
  | 'http-error'
  | 'invalid-response'

export type ServerConnectionResult =
  | { ok: true, baseUrl: string }
  | { ok: false, baseUrl: string, error: ServerConnectionErrorCode, status?: number }

export type ServerConnectResult =
  | { ok: true, baseUrl: string, changed: boolean }
  | { ok: false, baseUrl: string, error: ServerConnectionErrorCode, status?: number }

export interface DesktopBaseUrlCandidates {
  session?: string
  desktop?: string
  proxy?: string
  vite?: string
  profile?: string
  fallback: string
}

export function normalizeBaseUrl(raw: string): string {
  let value = raw.trim()
  if (!value) throw new Error('Server URL is required')
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(value)) {
    const localHost = /^(localhost|127\.|0\.0\.0\.0|\[::1\])(?::|\/|$)/i.test(value)
    value = `${localHost ? 'http' : 'https'}://${value}`
  }
  const url = new URL(value)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('Server URL must use http or https')
  }
  url.hash = ''
  url.search = ''
  return url.toString().replace(/\/$/, '')
}

export function resolveDesktopBaseUrl(candidates: DesktopBaseUrlCandidates): string {
  const configured = candidates.session?.trim()
    || candidates.desktop?.trim()
    || candidates.proxy?.trim()
    || candidates.vite?.trim()
    || candidates.profile?.trim()
    || candidates.fallback
  return normalizeBaseUrl(configured)
}

export function normalizeServerInput(raw: unknown): ServerConnectionResult {
  const value = typeof raw === 'string' ? raw.trim() : ''
  if (!value) return { ok: false, baseUrl: '', error: 'required' }
  try {
    return { ok: true, baseUrl: normalizeBaseUrl(value) }
  } catch (error) {
    const message = error instanceof Error ? error.message : ''
    return {
      ok: false,
      baseUrl: value,
      error: message.includes('http or https') ? 'unsupported-protocol' : 'invalid-url',
    }
  }
}

export async function probeServerBaseUrl(
  baseUrl: string,
  request: typeof fetch = globalThis.fetch,
  timeoutMs = SERVER_PROBE_TIMEOUT_MS,
): Promise<ServerConnectionResult> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), timeoutMs)
  try {
    let response: Response
    try {
      response = await request(`${baseUrl}/ping`, {
        signal: controller.signal,
        headers: { Accept: 'application/json' },
      })
    } catch (error) {
      return {
        ok: false,
        baseUrl,
        error: controller.signal.aborted || (error instanceof Error && error.name === 'AbortError')
          ? 'timeout'
          : 'unreachable',
      }
    }
    if (!response.ok) {
      return { ok: false, baseUrl, error: 'http-error', status: response.status }
    }
    const payload = await response.json().catch(() => null) as { status?: unknown } | null
    if (payload?.status !== 'ok') {
      return { ok: false, baseUrl, error: 'invalid-response' }
    }
    return { ok: true, baseUrl }
  } finally {
    clearTimeout(timeout)
  }
}
