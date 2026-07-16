import { client } from '@memohai/sdk/client'
import type { Options } from '@memohai/sdk'
import {
  fetchSSEProblem,
  isSSEErrorEvent,
  localizeSSEErrorEvent,
  normalizeSSEFailure,
  type SSEErrorEvent,
} from './sse-error'

// codesync(display-prepare-stream): keep these manual SSE payload types in sync
// with internal/handlers/display.go.
export type DisplayPrepareStreamEvent =
  | { type: 'progress'; step?: string; message?: string; percent?: number }
  | { type: 'complete'; step?: string; message?: string; percent?: number }
  | (SSEErrorEvent & { step?: string })

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
      return isSSEErrorEvent(event)
        && (event.step === undefined || typeof event.step === 'string')
    default:
      return false
  }
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
    fetch: fetchSSEProblem,
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
        yield event.type === 'error'
          ? localizeSSEErrorEvent(event)
          : event
      }
      if (streamError) {
        throw normalizeSSEFailure(streamError, 'Display preparation stream failed')
      }
    })(),
  }
}
