import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  UIRuntimeStateEvent,
  UIStreamEvent,
  WSClientMessage,
} from '@/composables/api/useChat'
import {
  createRuntimeSubscriptionController,
  type RuntimeSubscriptionApplyResult,
  type RuntimeSubscriptionRef,
} from './subscription'

const sessionOne = { botId: 'bot-1', sessionId: 'session-1' }

function runtimeSnapshot(
  ref: RuntimeSubscriptionRef = sessionOne,
  seq = 1,
): UIRuntimeStateEvent {
  return {
    type: 'runtime_snapshot',
    bot_id: ref.botId,
    session_id: ref.sessionId,
    epoch: `epoch-${ref.sessionId}`,
    seq,
    snapshot: {
      bot_id: ref.botId,
      session_id: ref.sessionId,
      epoch: `epoch-${ref.sessionId}`,
      seq,
      queue: [],
    },
  }
}

function commandEvent(
  type: 'command_result' | 'command_error',
  invocationId: string,
  actionId: 'runtime_subscribe' | 'runtime_unsubscribe' = 'runtime_subscribe',
): UIStreamEvent {
  return {
    type,
    invocation_id: invocationId,
    action_id: actionId,
    terminal: true,
    ...(type === 'command_error'
      ? { error: { code: 'runtime_subscription_failed', message: 'temporary failure' } }
      : {}),
  }
}

function makeController(options: {
  connected?: boolean
  apply?: (
    ref: RuntimeSubscriptionRef,
    event: Exclude<UIRuntimeStateEvent, { type: 'runtime_dropped' }>,
  ) => RuntimeSubscriptionApplyResult | Promise<RuntimeSubscriptionApplyResult>
} = {}) {
  let connected = options.connected ?? true
  let invocationSequence = 0
  const sent: Array<{ botId: string, message: WSClientMessage }> = []
  const ensureConnected = vi.fn(() => connected)
  const send = vi.fn((botId: string, message: WSClientMessage) => {
    if (!connected) return false
    sent.push({ botId, message })
    return true
  })
  const awaitCheckpoint = vi.fn()
  const release = vi.fn()
  const apply = vi.fn(options.apply ?? (() => ({ kind: 'applied' as const })))
  const onError = vi.fn()
  const controller = createRuntimeSubscriptionController({
    transport: { ensureConnected, send },
    runtime: { awaitCheckpoint, release, apply },
    createInvocationId: () => `invocation-${++invocationSequence}`,
    retryDelay: () => 10,
    onError,
  })

  return {
    controller,
    sent,
    ensureConnected,
    send,
    awaitCheckpoint,
    release,
    apply,
    onError,
    setConnected(value: boolean) {
      connected = value
    },
  }
}

function messagesOfType(
  sent: Array<{ botId: string, message: WSClientMessage }>,
  type: WSClientMessage['type'],
) {
  return sent.filter(item => item.message.type === type).map(item => item.message)
}

afterEach(() => {
  vi.useRealTimers()
})

describe('runtime subscription controller', () => {
  it('deduplicates desired sessions and unsubscribes released state once', () => {
    const { controller, sent, awaitCheckpoint, release } = makeController()

    controller.reconcile([sessionOne, { botId: ' bot-1 ', sessionId: ' session-1 ' }])
    controller.reconcile([sessionOne])

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(1)
    expect(awaitCheckpoint).toHaveBeenCalledOnce()

    controller.handleEvent('bot-1', runtimeSnapshot())
    controller.reconcile([])

    expect(messagesOfType(sent, 'runtime_unsubscribe')).toHaveLength(1)
    expect(release).toHaveBeenCalledOnce()
    expect(release).toHaveBeenCalledWith(sessionOne)
  })

  it('retains desired state while disconnected and subscribes on open', () => {
    const { controller, sent, awaitCheckpoint, setConnected } = makeController({ connected: false })

    controller.reconcile([sessionOne])
    expect(sent).toEqual([])
    expect(awaitCheckpoint).toHaveBeenCalledOnce()

    setConnected(true)
    controller.handleTransportOpen('bot-1')

    expect(messagesOfType(sent, 'runtime_subscribe')).toEqual([
      expect.objectContaining({ session_id: 'session-1' }),
    ])
    expect(awaitCheckpoint).toHaveBeenCalledOnce()
  })

  it('resubscribes every desired session after a transport reconnect', () => {
    const sessionTwo = { botId: 'bot-1', sessionId: 'session-2' }
    const { controller, sent, awaitCheckpoint } = makeController()
    controller.reconcile([sessionOne, sessionTwo])
    controller.handleEvent('bot-1', runtimeSnapshot(sessionOne))
    controller.handleEvent('bot-1', runtimeSnapshot(sessionTwo))
    sent.length = 0

    controller.handleTransportClose('bot-1')
    controller.handleTransportOpen('bot-1')

    expect(messagesOfType(sent, 'runtime_subscribe')).toEqual(expect.arrayContaining([
      expect.objectContaining({ session_id: 'session-1' }),
      expect.objectContaining({ session_id: 'session-2' }),
    ]))
    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(2)
    expect(awaitCheckpoint).toHaveBeenCalledTimes(4)
  })

  it('forces one full resubscribe after dropped events until a checkpoint arrives', () => {
    const { controller, sent, apply } = makeController()
    controller.reconcile([sessionOne])
    controller.handleEvent('bot-1', runtimeSnapshot())
    sent.length = 0
    apply.mockClear()

    const dropped: UIStreamEvent = {
      type: 'runtime_dropped',
      bot_id: 'bot-1',
      session_id: 'session-1',
      message: 'subscriber overflow',
    }
    controller.handleEvent('bot-1', dropped)
    controller.handleEvent('bot-1', dropped)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(1)
    expect(messagesOfType(sent, 'runtime_subscribe')[0]).not.toHaveProperty('after_seq')
    expect(apply).not.toHaveBeenCalled()
  })

  it('uses the same full-resubscribe path when the runtime reducer requests resync', () => {
    let reduction: RuntimeSubscriptionApplyResult = { kind: 'applied' }
    const { controller, sent } = makeController({ apply: () => reduction })
    controller.reconcile([sessionOne])
    controller.handleEvent('bot-1', runtimeSnapshot())
    sent.length = 0
    reduction = { kind: 'resync' }

    controller.handleEvent('bot-1', {
      type: 'runtime_delta',
      bot_id: 'bot-1',
      session_id: 'session-1',
      epoch: 'epoch-session-1',
      stream_id: 'stream-1',
      seq: 3,
      delta: { reset_messages: true },
    })

    expect(messagesOfType(sent, 'runtime_subscribe')).toEqual([
      expect.objectContaining({ session_id: 'session-1' }),
    ])
  })

  it('cancels retry backoff when a checkpoint is accepted before its command reply', async () => {
    vi.useFakeTimers()
    const { controller, sent } = makeController()
    controller.reconcile([sessionOne])
    const firstInvocation = messagesOfType(sent, 'runtime_subscribe')[0]!.invocation_id!

    expect(controller.handleEvent('bot-1', commandEvent('command_error', firstInvocation))).toBe(true)
    await vi.advanceTimersByTimeAsync(10)
    const secondInvocation = messagesOfType(sent, 'runtime_subscribe')[1]!.invocation_id!

    controller.handleEvent('bot-1', runtimeSnapshot())
    expect(controller.handleEvent('bot-1', commandEvent('command_error', secondInvocation))).toBe(true)
    await vi.advanceTimersByTimeAsync(100)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(2)
  })

  it('consumes a superseded invocation error without downgrading newer recovery', async () => {
    vi.useFakeTimers()
    const { controller, sent } = makeController()
    controller.reconcile([sessionOne])
    const staleInvocation = messagesOfType(sent, 'runtime_subscribe')[0]!.invocation_id!
    controller.handleEvent('bot-1', runtimeSnapshot())
    controller.handleEvent('bot-1', {
      type: 'runtime_dropped',
      bot_id: 'bot-1',
      session_id: 'session-1',
    })

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(2)
    expect(controller.handleEvent('bot-1', commandEvent('command_error', staleInvocation))).toBe(true)
    await vi.advanceTimersByTimeAsync(100)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(2)
  })

  it('stops a scheduled retry when the session is no longer desired', async () => {
    vi.useFakeTimers()
    const { controller, sent, release } = makeController()
    controller.reconcile([sessionOne])
    const invocationId = messagesOfType(sent, 'runtime_subscribe')[0]!.invocation_id!
    controller.handleEvent('bot-1', commandEvent('command_error', invocationId))

    controller.reconcile([])
    await vi.advanceTimersByTimeAsync(100)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(1)
    expect(release).toHaveBeenCalledWith(sessionOne)
  })

  it('ignores late dropped events after a session is released', () => {
    const { controller, sent } = makeController()
    controller.reconcile([sessionOne])
    controller.handleEvent('bot-1', runtimeSnapshot())
    controller.reconcile([])
    sent.length = 0

    expect(controller.handleEvent('bot-1', {
      type: 'runtime_dropped',
      bot_id: 'bot-1',
      session_id: 'session-1',
    })).toBe(true)

    expect(sent).toEqual([])
  })

  it('rejects a runtime envelope from the wrong source bot and requests a checkpoint', () => {
    const { controller, sent, apply } = makeController()
    controller.reconcile([sessionOne])
    controller.handleEvent('bot-1', runtimeSnapshot())
    sent.length = 0
    apply.mockClear()

    controller.handleEvent('bot-1', runtimeSnapshot({ botId: 'bot-2', sessionId: 'session-1' }, 2))

    expect(apply).not.toHaveBeenCalled()
    expect(messagesOfType(sent, 'runtime_subscribe')).toEqual([
      expect.objectContaining({ session_id: 'session-1' }),
    ])
  })

  it('consumes unsubscribe failures without retrying a released session', async () => {
    vi.useFakeTimers()
    const { controller, sent } = makeController()
    controller.reconcile([sessionOne])
    controller.handleEvent('bot-1', runtimeSnapshot())
    controller.remove(sessionOne)
    const unsubscribe = messagesOfType(sent, 'runtime_unsubscribe')[0]!

    expect(controller.handleEvent(
      'bot-1',
      commandEvent('command_error', unsubscribe.invocation_id!, 'runtime_unsubscribe'),
    )).toBe(true)
    await vi.advanceTimersByTimeAsync(100)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(1)
  })

  it('clears retry timers and runtime state when disposed', async () => {
    vi.useFakeTimers()
    const { controller, sent, release } = makeController()
    controller.reconcile([sessionOne])
    const invocationId = messagesOfType(sent, 'runtime_subscribe')[0]!.invocation_id!
    controller.handleEvent('bot-1', commandEvent('command_error', invocationId))

    controller.dispose()
    await vi.advanceTimersByTimeAsync(100)

    expect(messagesOfType(sent, 'runtime_subscribe')).toHaveLength(1)
    expect(release).toHaveBeenCalledWith(sessionOne)
  })

  it('leaves unrelated websocket sideband events for their owning controller', () => {
    const { controller } = makeController()
    controller.reconcile([sessionOne])

    expect(controller.handleEvent('bot-1', {
      type: 'session_created',
      stream_id: 'stream-1',
      session_id: 'session-1',
    })).toBe(false)
    expect(controller.handleEvent('bot-1', {
      type: 'command_result',
      invocation_id: 'other-command',
      action_id: 'tool_approval_response',
      terminal: true,
    })).toBe(false)
  })
})
