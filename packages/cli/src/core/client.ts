import { client } from '@memoh/sdk/client'
import { readConfig, readToken, getBaseURL } from '../utils/store'

/**
 * Configure the SDK client with base URL and auth interceptor.
 * Call this once at CLI startup (before any API calls).
 */
export function setupClient() {
  const config = readConfig()
  client.setConfig({ baseUrl: getBaseURL(config) })

  // Add auth token to every request (read lazily from store)
  client.interceptors.request.use((request) => {
    const token = readToken()
    if (token?.access_token) {
      request.headers.set('Authorization', `Bearer ${token.access_token}`)
    }
    return request
  })
}

export { client }
