import { describe, expect, it } from 'vitest'
import type { BotsBot } from '@memohai/sdk'
import {
  collectBotCreateProgressStream,
  reduceBotCreateProgressEvent,
  type BotCreateStreamEvent,
} from './useBotCreateStream'

describe('useBotCreateStream', () => {
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
})
