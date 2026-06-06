import { describe, expect, it } from 'vitest'
import type { BotsBot } from '@memohai/sdk'
import { reduceBotCreateProgressEvent } from './useBotCreateStream'

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
})
