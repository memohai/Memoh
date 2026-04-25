import { client } from '@memohai/sdk/client'

export interface SetupApiClientOptions {
  // Called after the access token is cleared on a 401. Hosts (web / desktop
  // chat window / desktop settings window) decide what to do — usually a
  // router redirect to the login screen, but desktop satellite windows may
  // prefer to close themselves and let the chat window take over auth.
  onUnauthorized?: () => void
}

/**
 * Configure the SDK client with base URL, auth interceptor, and 401 handling.
 * Call this once at app startup (main.ts).
 */
export function setupApiClient(options: SetupApiClientOptions = {}) {
  const apiBaseUrl = import.meta.env.VITE_API_URL?.trim() || '/api'
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
