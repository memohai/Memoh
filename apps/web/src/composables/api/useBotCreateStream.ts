import { client } from '@memohai/sdk/client'
import type { Options } from '@memohai/sdk'
import type { BotsBot, PostBotsData } from '@memohai/sdk'
import type { ContainerCreateLayerStatus } from './useContainerStream'

// codesync(bot-create-stream): keep these manual SSE payload types in sync
// with internal/handlers/users.go.
export type BotCreateStreamEvent =
  | { type: 'bot_created'; bot: BotsBot }
  | { type: 'pulling'; image: string }
  | { type: 'pull_progress'; layers: ContainerCreateLayerStatus[] }
  | { type: 'pull_skipped'; image: string; message?: string }
  | { type: 'pull_delegated'; image: string; message?: string }
  | { type: 'creating' }
  | { type: 'restoring' }
  | { type: 'ready'; bot: BotsBot }
  | { type: 'error'; message: string }

export type BotCreateStreamResult = {
  stream: AsyncGenerator<BotCreateStreamEvent, void, unknown>
}

function isLayerStatus(value: unknown): value is ContainerCreateLayerStatus {
  return !!value
    && typeof value === 'object'
    && typeof (value as { ref?: unknown }).ref === 'string'
    && typeof (value as { offset?: unknown }).offset === 'number'
    && typeof (value as { total?: unknown }).total === 'number'
}

function isBot(value: unknown): value is BotsBot {
  return !!value && typeof value === 'object'
}

function isBotCreateStreamEvent(value: unknown): value is BotCreateStreamEvent {
  if (!value || typeof value !== 'object') return false

  const event = value as Record<string, unknown>
  switch (event.type) {
    case 'bot_created':
    case 'ready':
      return isBot(event.bot)
    case 'pulling':
      return typeof event.image === 'string'
    case 'pull_progress':
      return Array.isArray(event.layers) && event.layers.every(isLayerStatus)
    case 'pull_skipped':
    case 'pull_delegated':
      return typeof event.image === 'string'
        && (event.message === undefined || typeof event.message === 'string')
    case 'creating':
    case 'restoring':
      return true
    case 'error':
      return typeof event.message === 'string'
    default:
      return false
  }
}

function toError(error: unknown): Error {
  if (error instanceof Error) return error
  if (typeof error === 'string' && error.trim()) return new Error(error)
  return new Error('Bot create stream failed')
}

export async function postBotsStream(
  options: Options<PostBotsData>,
): Promise<BotCreateStreamResult> {
  let streamError: unknown

  const { throwOnError: _throwOnError, ...rest } = options
  const result = await client.sse.post<BotCreateStreamEvent>({
    url: '/bots',
    ...rest,
    headers: {
      ...options.headers as Record<string, string>,
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    onSseError: (error) => {
      streamError = error
    },
    responseValidator: async (data) => {
      if (!isBotCreateStreamEvent(data)) {
        throw new Error('Invalid bot create stream event')
      }
    },
    sseMaxRetryAttempts: 1,
  })

  return {
    stream: (async function* () {
      for await (const event of result.stream as AsyncGenerator<unknown, void, unknown>) {
        if (!isBotCreateStreamEvent(event)) {
          throw new Error('Invalid bot create stream event')
        }
        yield event
      }

      if (streamError) {
        throw toError(streamError)
      }
    })(),
  }
}
