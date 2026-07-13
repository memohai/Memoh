import { afterEach, describe, expect, it, vi } from 'vitest'
import { client } from '@memohai/sdk/client'
import {
  fetchSSEProblem,
  isSSEErrorEvent,
  localizeSSEErrorEvent,
  normalizeSSEFailure,
} from './sse-error'

describe('SSE error boundary', () => {
  afterEach(() => {
    client.setConfig({ fetch: undefined })
    vi.restoreAllMocks()
  })

  it('decodes a rejected SSE request as a structured HTTP problem', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      code: 'bot.name_taken',
      args: { field: 'name' },
      detail: 'This name is already taken.',
    }), {
      status: 409,
      headers: { 'Content-Type': 'application/problem+json' },
    }))
    client.setConfig({ fetch: fetchMock as unknown as typeof fetch })

    await expect(fetchSSEProblem('http://example.test/bots')).rejects.toMatchObject({
      code: 'bot.name_taken',
      status: 409,
    })
  })

  it('validates and localizes the shared stream error shape', () => {
    const event = {
      type: 'error' as const,
      code: 'workspace.unreachable',
      args: {},
      detail: 'The workspace could not be reached.',
      message: 'The workspace could not be reached.',
      request_id: 'req-sse-1',
    }

    expect(isSSEErrorEvent(event)).toBe(true)
    expect(localizeSSEErrorEvent(event).message).toBe('The workspace could not be reached.')
  })

  it('preserves a structured failure instead of flattening it into Error.message', () => {
    const problem = { code: 'workspace.unreachable', args: {}, status: 503 }
    expect(normalizeSSEFailure(problem, 'fallback')).toBe(problem)
  })
})
