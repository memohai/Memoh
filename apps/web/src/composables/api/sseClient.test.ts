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

  it('threads transformed errors through SSE error interceptors', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 500, statusText: 'Internal Server Error' }))
    const onSseError = vi.fn()
    const transformed = new Error('transformed')
    const firstInterceptor = vi.fn(() => transformed)
    const secondInterceptor = vi.fn(() => undefined)

    client.setConfig({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })
    client.interceptors.error.use(error => firstInterceptor(error))
    client.interceptors.error.use(error => secondInterceptor(error))

    const result = await client.sse.get({
      url: '/events',
      onSseError,
      sseMaxRetryAttempts: 1,
    })

    await drain(result.stream)

    expect(secondInterceptor).toHaveBeenCalledWith(transformed)
    expect(onSseError).toHaveBeenCalledWith(transformed)
  })

  it('still reports the original SSE error when an error interceptor throws', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 401, statusText: 'Unauthorized' }))
    const onSseError = vi.fn()

    client.setConfig({
      baseUrl: 'http://example.test',
      fetch: fetchMock as unknown as typeof fetch,
    })
    client.interceptors.error.use(() => {
      throw new Error('interceptor failed')
    })

    const result = await client.sse.get({
      url: '/events',
      onSseError,
      sseMaxRetryAttempts: 1,
    })

    await expect(drain(result.stream)).resolves.toBeUndefined()
    expect(onSseError).toHaveBeenCalledTimes(1)
    expect(onSseError.mock.calls[0]?.[0]).toBeInstanceOf(Error)
    expect((onSseError.mock.calls[0]?.[0] as Error).message).toBe('SSE failed: 401 Unauthorized')
  })
})
