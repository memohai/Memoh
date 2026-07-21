/**
 * Normalizes the HTTP/WebSocket spellings of the same Memoh server endpoint.
 */
export function normalizeRuntimeServerUrl(serverUrl: string): string {
  const url = new URL(serverUrl.trim())
  if (url.protocol === 'ws:') {
    url.protocol = 'http:'
  } else if (url.protocol === 'wss:') {
    url.protocol = 'https:'
  }
  url.pathname = url.pathname.replace(/\/+$/, '') || '/'
  url.search = ''
  url.hash = ''
  return url.href
}
