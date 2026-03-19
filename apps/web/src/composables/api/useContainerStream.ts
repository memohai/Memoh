import { client } from '@memoh/sdk/client'
import type { Options } from '@memoh/sdk'
import type {
  HandlersCreateContainerResponse,
  PostBotsByBotIdContainerData,
} from '@memoh/sdk'

export type ContainerCreateLayerStatus = {
  ref: string
  offset: number
  total: number
}

// codesync(container-create-stream): keep these manual SSE payload types in sync
// with internal/handlers/containerd.go.
export type ContainerCreateStreamEvent =
  | { type: 'pulling'; image: string }
  | { type: 'pull_progress'; layers: ContainerCreateLayerStatus[] }
  | { type: 'creating' }
  | { type: 'restoring' }
  | { type: 'complete'; container: HandlersCreateContainerResponse }
  | { type: 'error'; message: string }

export type ContainerCreateStreamResult = {
  stream: AsyncGenerator<ContainerCreateStreamEvent, void, unknown>
}

function isLayerStatus(value: unknown): value is ContainerCreateLayerStatus {
  return !!value
    && typeof value === 'object'
    && typeof (value as { ref?: unknown }).ref === 'string'
    && typeof (value as { offset?: unknown }).offset === 'number'
    && typeof (value as { total?: unknown }).total === 'number'
}

function isContainerCreateStreamEvent(value: unknown): value is ContainerCreateStreamEvent {
  if (!value || typeof value !== 'object') return false

  const event = value as Record<string, unknown>
  switch (event.type) {
    case 'pulling':
      return typeof event.image === 'string'
    case 'pull_progress':
      return Array.isArray(event.layers) && event.layers.every(isLayerStatus)
    case 'creating':
    case 'restoring':
      return true
    case 'complete':
      return !!event.container && typeof event.container === 'object'
    case 'error':
      return typeof event.message === 'string'
    default:
      return false
  }
}

function toError(error: unknown): Error {
  if (error instanceof Error) return error
  if (typeof error === 'string' && error.trim()) return new Error(error)
  return new Error('Container create stream failed')
}

export async function postBotsByBotIdContainerStream(
  options: Options<PostBotsByBotIdContainerData>,
): Promise<ContainerCreateStreamResult> {
  let streamError: unknown

  const { throwOnError: _throwOnError, ...rest } = options
  const result = await client.sse.post<ContainerCreateStreamEvent>({
    url: '/bots/{bot_id}/container',
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
      if (!isContainerCreateStreamEvent(data)) {
        throw new Error('Invalid container create stream event')
      }
    },
    sseMaxRetryAttempts: 1,
  })

  return {
    stream: (async function* () {
      for await (const event of result.stream as AsyncGenerator<unknown, void, unknown>) {
        if (!isContainerCreateStreamEvent(event)) {
          throw new Error('Invalid container create stream event')
        }
        yield event
      }

      if (streamError) {
        throw toError(streamError)
      }
    })(),
  }
}
