import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./auth-session', () => ({
  notifyAuthSessionCleared: vi.fn(),
}))

let client: typeof import('@memohai/sdk/client')['client']
let setupApiClient: typeof import('./api-client')['setupApiClient']
let mockedNotifyAuthSessionCleared: ReturnType<typeof vi.fn>

function stubLocalStorage() {
  const store = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, value)
    }),
    removeItem: vi.fn((key: string) => {
      store.delete(key)
    }),
    clear: vi.fn(() => {
      store.clear()
    }),
  })
}

async function drain<T>(stream: AsyncGenerator<T, unknown, unknown>) {
  for await (const _event of stream) {
  }
}

describe('setupApiClient auth handling', () => {
  beforeEach(async () => {
    vi.resetModules()
    stubLocalStorage()
    localStorage.clear()
    const sdkClientModule = await import('@memohai/sdk/client')
    const apiClientModule = await import('./api-client')
    const authSession = await import('./auth-session')
    client = sdkClientModule.client
    setupApiClient = apiClientModule.setupApiClient
    mockedNotifyAuthSessionCleared = vi.mocked(authSession.notifyAuthSessionCleared) as unknown as ReturnType<typeof vi.fn>
    mockedNotifyAuthSessionCleared.mockClear()
    client.interceptors.request.clear()
    client.interceptors.response.clear()
    client.interceptors.error.clear()
  })

  afterEach(() => {
    client.interceptors.request.clear()
    client.interceptors.response.clear()
    client.interceptors.error.clear()
    client.setConfig({ baseUrl: '/api', fetch: undefined })
    vi.unstubAllGlobals()
  })

  it('runs the same unauthorized flow for SSE 401 responses', async () => {
    localStorage.setItem('token', 'stale-token')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => new Response('', {
      status: 401,
      statusText: 'Unauthorized',
    }))
    const onSseError = vi.fn()

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })

    const result = await client.sse.get({
      url: '/events',
      onSseError,
      sseMaxRetryAttempts: 1,
    })

    await drain(result.stream)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBe('Bearer stale-token')
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem('token')).toBeNull()
    expect(onSseError).toHaveBeenCalledTimes(1)
  })

  it('runs the same unauthorized flow for REST 401 responses', async () => {
    localStorage.setItem('token', 'stale-token')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => new Response('', {
      status: 401,
      statusText: 'Unauthorized',
    }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })

    await client.get({ url: '/users/me' })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('normalizes stored token whitespace before comparing 401 request tokens', async () => {
    localStorage.setItem('token', '  stale-token  ')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => new Response('', {
      status: 401,
      statusText: 'Unauthorized',
    }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })

    await client.get({ url: '/users/me' })

    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBe('Bearer stale-token')
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('does not let a stale request 401 clear a newer login token', async () => {
    localStorage.setItem('token', 'old-token')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => {
      localStorage.setItem('token', 'new-token')
      return new Response('', {
        status: 401,
        statusText: 'Unauthorized',
      })
    })

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })

    await client.get({ url: '/users/me' })

    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBe('Bearer old-token')
    expect(onUnauthorized).not.toHaveBeenCalled()
    expect(mockedNotifyAuthSessionCleared).not.toHaveBeenCalled()
    expect(localStorage.getItem('token')).toBe('new-token')
  })

  it('handles a 401 when the current session has a token but the request has no bearer token', async () => {
    localStorage.setItem('token', 'current-token')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => new Response('', {
      status: 401,
      statusText: 'Unauthorized',
    }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })
    client.interceptors.request.use((request) => {
      request.headers.delete('Authorization')
      return request
    })

    await client.get({ url: '/users/me' })

    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBeNull()
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('leaves the token alone on non-401 responses', async () => {
    localStorage.setItem('token', 'valid-token')
    const fetchMock = vi.fn(async () => new Response('{}', { status: 200 }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })

    await client.get({ url: '/users/me' })

    expect(localStorage.getItem('token')).toBe('valid-token')
    expect(mockedNotifyAuthSessionCleared).not.toHaveBeenCalled()
  })

  it('notifies only once for repeated 401 responses after the token is cleared', async () => {
    localStorage.setItem('token', 'stale-token')
    const onUnauthorized = vi.fn()
    const fetchMock = vi.fn(async () => new Response('', {
      status: 401,
      statusText: 'Unauthorized',
    }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
      onUnauthorized,
    })

    await client.get({ url: '/first' })
    await client.get({ url: '/second' })

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(mockedNotifyAuthSessionCleared).toHaveBeenCalledTimes(1)
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('does not add duplicate request interceptors across repeated setup calls', async () => {
    localStorage.setItem('token', 'valid-token')
    const fetchMock = vi.fn(async () => new Response('', { status: 204 }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })
    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })

    await client.get({ url: '/users/me' })

    expect(client.interceptors.request.fns.filter(Boolean)).toHaveLength(1)
    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBe('Bearer valid-token')
  })

  it('still injects the bearer token through request interceptors', async () => {
    localStorage.setItem('token', 'valid-token')
    const fetchMock = vi.fn(async () => new Response('', { status: 204 }))

    setupApiClient({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })

    await client.get({ url: '/users/me' })

    const request = fetchMock.mock.calls[0]?.[0] as Request
    expect(request.headers.get('Authorization')).toBe('Bearer valid-token')
  })
})
