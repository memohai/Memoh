import { client } from '@memohai/sdk/client'
import type { Options } from '@memohai/sdk'
import type { BotsBot, HandlersCreateContainerResponse, PostBotsData } from '@memohai/sdk'
import type { ContainerCreateLayerStatus } from './useContainerStream'
import {
  fetchSSEProblem,
  isSSEErrorEvent,
  localizeSSEErrorEvent,
  normalizeSSEFailure,
  type SSEErrorEvent,
} from './sse-error'

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
  | { type: 'complete'; container: HandlersCreateContainerResponse }
  | { type: 'ready'; bot: BotsBot }
  | SSEErrorEvent

export type BotCreateStreamResult = {
  stream: AsyncGenerator<BotCreateStreamEvent, void, unknown>
}

export type BotCreateProgress = {
  phase: 'preserving' | 'pulling' | 'creating' | 'restoring' | 'complete' | 'error'
  layers?: ContainerCreateLayerStatus[]
  image?: string
  error?: string
}

export type BotCreateProgressState = {
  bot?: BotsBot
  progress?: BotCreateProgress
  setupError?: string
  errorCode?: string
}

export function botCreateProgressPercent(progress: BotCreateProgress | null | undefined): number {
  const layers = progress?.layers
  if (!layers || layers.length === 0) return 0
  let totalOffset = 0
  let totalSize = 0
  for (const layer of layers) {
    totalOffset += layer.offset
    totalSize += layer.total
  }
  return totalSize > 0 ? Math.round((totalOffset / totalSize) * 100) : 0
}

export function reduceBotCreateProgressEvent(
  state: BotCreateProgressState,
  event: BotCreateStreamEvent,
): BotCreateProgressState {
  switch (event.type) {
    case 'bot_created':
      return { ...state, bot: event.bot }
    case 'pulling':
      return { ...state, progress: { phase: 'pulling', image: event.image } }
    case 'pull_progress':
      return {
        ...state,
        progress: {
          phase: 'pulling',
          image: state.progress?.image,
          layers: event.layers,
        },
      }
    case 'pull_skipped':
    case 'pull_delegated':
      return {
        ...state,
        progress: event.image === 'local'
          ? { phase: 'creating' }
          : { phase: 'pulling', image: event.image },
      }
    case 'creating':
      return { ...state, progress: { phase: 'creating' } }
    case 'restoring':
      return { ...state, progress: { phase: 'restoring' } }
    case 'complete':
      return { ...state, progress: { phase: 'complete' } }
    case 'ready':
      return { ...state, bot: event.bot }
    case 'error':
      return {
        ...state,
        setupError: event.message,
        errorCode: event.code,
        progress: { phase: 'error', error: event.message },
      }
  }
}

export type CollectBotCreateProgressStreamOptions = {
  initialState?: BotCreateProgressState
  onState?: (state: BotCreateProgressState) => void
  onEvent?: (event: BotCreateStreamEvent) => void
}

export async function collectBotCreateProgressStream(
  stream: AsyncGenerator<BotCreateStreamEvent, void, unknown>,
  options: CollectBotCreateProgressStreamOptions = {},
): Promise<BotCreateProgressState> {
  let state = options.initialState ?? {}
  let ready = false

  const update = (next: BotCreateProgressState) => {
    state = next
    options.onState?.(state)
  }

  try {
    for await (const event of stream) {
      options.onEvent?.(event)
      if (event.type === 'ready') {
        ready = true
      }
      update(reduceBotCreateProgressEvent(state, event))
    }
  } catch (error) {
    if (!state.bot) {
      throw error
    }
    if (ready) {
      return state
    }
    const message = toError(error).message
    update({
      ...state,
      setupError: message,
      progress: { phase: 'error', error: message },
    })
  }

  return state
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
    case 'complete':
      return !!event.container && typeof event.container === 'object'
    case 'error':
      return isSSEErrorEvent(event)
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
    fetch: fetchSSEProblem,
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
        streamError = undefined
        yield event.type === 'error'
          ? localizeSSEErrorEvent(event)
          : event
      }

      if (streamError) {
        throw normalizeSSEFailure(streamError, 'Bot create stream failed')
      }
    })(),
  }
}
