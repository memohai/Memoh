import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { client } from '@memohai/sdk/client'

async function drain<T>(stream: AsyncGenerator<T, unknown, unknown>) {
  for await (const _event of stream) {
    // drain
  }
}

describe('SDK SSE client', () => {
  beforeEach(() => {
    client.interceptors.error.clear()
    client.interceptors.request.clear()
    client.interceptors.response.clear()
  })

  afterEach(() => {
    client.interceptors.error.clear()
    client.interceptors.request.clear()
    client.interceptors.response.clear()
  })

  it('runs response and error interceptors before reporting failed SSE responses', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 401, statusText: 'Unauthorized' }))
    const onSseError = vi.fn()
    const responseInterceptor = vi.fn((response: Response) => response)
    const errorInterceptor = vi.fn((error: unknown) => error)

    client.setConfig({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })
    client.interceptors.response.use((response) => responseInterceptor(response))
    client.interceptors.error.use((error) => errorInterceptor(error))

    const result = await client.sse.get({
      url: '/events',
      onSseError,
      sseMaxRetryAttempts: 1,
    })

    await drain(result.stream)

    expect(responseInterceptor).toHaveBeenCalledTimes(1)
    expect(responseInterceptor.mock.calls[0]?.[0].status).toBe(401)
    expect(errorInterceptor).toHaveBeenCalledTimes(1)
    expect(onSseError).toHaveBeenCalledTimes(1)
    expect(onSseError.mock.calls[0]?.[0]).toBeInstanceOf(Error)
    expect((onSseError.mock.calls[0]?.[0] as Error).message).toBe('SSE failed: 401 Unauthorized')
  })
})
