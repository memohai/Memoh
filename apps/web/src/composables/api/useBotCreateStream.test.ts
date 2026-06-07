import { afterEach, describe, expect, it, vi } from 'vitest'
import type { BotsBot } from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import {
  collectBotCreateProgressStream,
  postBotsStream,
  reduceBotCreateProgressEvent,
  type BotCreateStreamEvent,
} from './useBotCreateStream'

describe('useBotCreateStream', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps the created bot when container setup reports an error', () => {
    const bot: BotsBot = { id: 'bot-1', name: 'stream-bot' }

    const created = reduceBotCreateProgressEvent({}, {
      type: 'bot_created',
      bot,
    })
    const failed = reduceBotCreateProgressEvent(created, {
      type: 'error',
      message: 'container setup failed',
    })

    expect(failed.bot).toEqual(bot)
    expect(failed.setupError).toBe('container setup failed')
    expect(failed.progress).toEqual({
      phase: 'error',
      error: 'container setup failed',
    })
  })

  it('keeps the created bot when the stream fails after bot_created', async () => {
    const bot: BotsBot = { id: 'bot-1', name: 'stream-bot' }

    async function* stream(): AsyncGenerator<BotCreateStreamEvent, void, unknown> {
      yield { type: 'bot_created', bot }
      throw new Error('connection reset')
    }

    const result = await collectBotCreateProgressStream(stream())

    expect(result.bot).toEqual(bot)
    expect(result.setupError).toBe('connection reset')
    expect(result.progress).toEqual({
      phase: 'error',
      error: 'connection reset',
    })
  })

  it('calls onEvent for each event in order', async () => {
    const bot: BotsBot = { id: 'bot-1', name: 'stream-bot' }

    async function* stream(): AsyncGenerator<BotCreateStreamEvent, void, unknown> {
      yield { type: 'bot_created', bot }
      yield { type: 'creating' }
      yield {
        type: 'complete',
        container: {
          container_id: 'workspace-bot-1',
          workspace_backend: 'container',
          runtime_backend: 'io.containerd.kata.v2',
          started: true,
        },
      }
      yield { type: 'ready', bot }
    }

    const seen: string[] = []
    await collectBotCreateProgressStream(stream(), {
      onEvent: event => seen.push(event.type),
    })

    expect(seen).toEqual(['bot_created', 'creating', 'complete', 'ready'])
  })

  it('ignores stream failures after ready', async () => {
    const bot: BotsBot = { id: 'bot-1', name: 'stream-bot' }

    async function* stream(): AsyncGenerator<BotCreateStreamEvent, void, unknown> {
      yield { type: 'bot_created', bot }
      yield { type: 'ready', bot }
      throw new Error('late connection reset')
    }

    const result = await collectBotCreateProgressStream(stream())

    expect(result.bot).toEqual(bot)
    expect(result.setupError).toBeUndefined()
    expect(result.progress).toBeUndefined()
  })

  it('does not throw a stale SSE error after a later successful event', async () => {
    const bot: BotsBot = { id: 'bot-1', name: 'stream-bot' }

    vi.spyOn(client.sse, 'post').mockImplementation(async (options: Parameters<typeof client.sse.post>[0]) => {
      options.onSseError?.(new Error('transient reset'))
      return {
        stream: (async function* (): AsyncGenerator<BotCreateStreamEvent, void, unknown> {
          yield { type: 'ready', bot }
        })(),
      }
    })

    const result = await postBotsStream({
      body: { name: 'stream-bot', display_name: 'Stream Bot' },
    })
    const events: BotCreateStreamEvent[] = []
    for await (const event of result.stream) {
      events.push(event)
    }

    expect(events).toEqual([{ type: 'ready', bot }])
  })
})
