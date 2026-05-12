import { client } from '@memohai/sdk/client'

export interface SetupApiClientOptions {
  baseUrl?: string
  // Called after the access token is cleared on a 401. Hosts (web / desktop
  // chat window / desktop settings window) decide what to do — usually a
  // router redirect to the login screen, but desktop satellite windows may
  // prefer to close themselves and let the chat window take over auth.
  onUnauthorized?: () => void
}

interface SdkUrlOptions {
  url: string
  path?: Record<string, unknown>
  query?: Record<string, unknown>
}

export function sdkAuthQuery(): { token?: string } {
  try {
    if (typeof localStorage === 'undefined') return {}
    const token = localStorage.getItem('token')?.trim()
    return token ? { token } : {}
  } catch {
    return {}
  }
}

export function sdkApiUrl(options: SdkUrlOptions): string {
  return client.buildUrl({
    ...client.getConfig(),
    ...options,
  })
}

function browserOrigin(): string {
  const { protocol, host, origin } = window.location
  return origin || `${protocol}//${host}`
}

export function sdkWebSocketUrl(options: SdkUrlOptions): string {
  const url = new URL(sdkApiUrl(options), browserOrigin())
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

/**
 * Configure the SDK client with base URL, auth interceptor, and 401 handling.
 * Call this once at app startup (main.ts).
 */
export function setupApiClient(options: SetupApiClientOptions = {}) {
  const apiBaseUrl = options.baseUrl?.trim() || import.meta.env.VITE_API_URL?.trim() || '/api'
  const agentBaseUrl = import.meta.env.VITE_AGENT_URL?.trim() || '/agent'
  void agentBaseUrl

  client.setConfig({ baseUrl: apiBaseUrl })

  // Add auth token to every request
  client.interceptors.request.use((request) => {
    const token = localStorage.getItem('token')
    if (token) {
      request.headers.set('Authorization', `Bearer ${token}`)
    }
    return request
  })

  // Handle 401 responses globally
  client.interceptors.response.use((response) => {
    if (response.status === 401) {
      localStorage.removeItem('token')
      options.onUnauthorized?.()
    }
    return response
  })
}
