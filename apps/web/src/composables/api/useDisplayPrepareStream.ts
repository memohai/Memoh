import { client } from '@memohai/sdk/client'
import type { Options } from '@memohai/sdk'

// codesync(display-prepare-stream): keep these manual SSE payload types in sync
// with internal/handlers/display.go.
export type DisplayPrepareStreamEvent =
  | { type: 'progress'; step?: string; message?: string; percent?: number }
  | { type: 'complete'; step?: string; message?: string; percent?: number }
  | { type: 'error'; step?: string; message: string }

export type DisplayPrepareStreamData = {
  path: { bot_id: string }
  body?: never
  query?: never
  url: '/bots/{bot_id}/container/display/prepare'
}

function isDisplayPrepareStreamEvent(value: unknown): value is DisplayPrepareStreamEvent {
  if (!value || typeof value !== 'object') return false
  const event = value as Record<string, unknown>
  switch (event.type) {
    case 'progress':
    case 'complete':
      return (event.step === undefined || typeof event.step === 'string')
        && (event.message === undefined || typeof event.message === 'string')
        && (event.percent === undefined || typeof event.percent === 'number')
    case 'error':
      return typeof event.message === 'string'
        && (event.step === undefined || typeof event.step === 'string')
    default:
      return false
  }
}

function toError(error: unknown): Error {
  if (error instanceof Error) return error
  if (typeof error === 'string' && error.trim()) return new Error(error)
  return new Error('Display preparation stream failed')
}

export async function postBotsByBotIdContainerDisplayPrepareStream(
  options: Options<DisplayPrepareStreamData>,
): Promise<{ stream: AsyncGenerator<DisplayPrepareStreamEvent, void, unknown> }> {
  let streamError: unknown
  const { throwOnError: _throwOnError, ...rest } = options
  const result = await client.sse.post<DisplayPrepareStreamEvent>({
    url: '/bots/{bot_id}/container/display/prepare',
    ...rest,
    headers: {
      ...options.headers as Record<string, string>,
      Accept: 'text/event-stream',
    },
    onSseError: (error) => {
      streamError = error
    },
    responseValidator: async (data) => {
      if (!isDisplayPrepareStreamEvent(data)) {
        throw new Error('Invalid display preparation stream event')
      }
    },
    sseMaxRetryAttempts: 1,
  })

  return {
    stream: (async function* () {
      for await (const event of result.stream as AsyncGenerator<unknown, void, unknown>) {
        if (!isDisplayPrepareStreamEvent(event)) {
          throw new Error('Invalid display preparation stream event')
        }
        yield event
      }
      if (streamError) {
        throw toError(streamError)
      }
    })(),
  }
}
