import { ref, toRaw } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createAssistantStreamRegistry } from './assistant-streams'
import type { ChatAssistantTurn } from './types'

function assistantTurn(id: string): ChatAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages: [],
    timestamp: '2026-01-01T00:00:00.000Z',
    streaming: true,
    __optimistic: true,
  }
}

function makeRegistry(activeSessionId: string | null = 'session-a', onTracked?: Parameters<typeof createAssistantStreamRegistry>[0]['onTracked']) {
  const currentBotId = ref<string | null>('bot-1')
  const sessionId = ref<string | null>(activeSessionId)
  const finishAssistantTurn = vi.fn((turn: ChatAssistantTurn) => {
    turn.streaming = false
  })
  const registry = createAssistantStreamRegistry({ currentBotId, sessionId, finishAssistantTurn, onTracked })
  return { registry, currentBotId, sessionId, finishAssistantTurn }
}

function track(
  registry: ReturnType<typeof createAssistantStreamRegistry>,
  streamId: string,
  targetSessionId = 'session-a',
  botId = 'bot-1',
) {
  const turn = assistantTurn(`turn-${streamId}`)
  const completion = registry.trackAssistantStream({
    streamId,
    assistantTurn: turn,
    botId,
    sessionId: targetSessionId,
  })
  return { turn, completion }
}

describe('assistant stream registry', () => {
  it('notifies synchronously after a stream is registered', async () => {
    const onTracked = vi.fn()
    const { registry } = makeRegistry('session-a', onTracked)
    const entry = track(registry, 'stream-tracked')

    expect(onTracked).toHaveBeenCalledOnce()
    expect(onTracked).toHaveBeenCalledWith(expect.objectContaining({
      streamId: 'stream-tracked', botId: 'bot-1', sessionId: 'session-a',
    }))
    expect(registry.getAssistantStream('stream-tracked')).toBeDefined()

    registry.resolveAssistantStream('stream-tracked')
    await entry.completion
  })

  it('registers synchronously and resolves only after removing the active stream', async () => {
    const { registry, finishAssistantTurn } = makeRegistry()
    const { turn, completion } = track(registry, 'stream-1')

    expect(toRaw(registry.getAssistantStream('stream-1')!.assistantTurn)).toBe(turn)
    expect(registry.streaming.value).toBe(true)

    const settled = vi.fn()
    const observed = completion.then(() => settled('resolved'))
    registry.resolveAssistantStream('stream-1')
    registry.resolveAssistantStream('stream-1')
    await observed

    expect(settled).toHaveBeenCalledOnce()
    expect(finishAssistantTurn).toHaveBeenCalledOnce()
    expect(turn.streaming).toBe(false)
    expect(registry.getAssistantStream('stream-1')).toBeUndefined()
    expect(registry.streaming.value).toBe(false)
  })

  it('rejects blank and duplicate ids without replacing the original stream', async () => {
    const { registry } = makeRegistry()
    await expect(track(registry, ' ').completion).rejects.toThrow('stream_id is required')

    const original = track(registry, 'stream-1')
    const duplicate = track(registry, 'stream-1')
    await expect(duplicate.completion).rejects.toThrow('stream_id stream-1 is already active')
    expect(toRaw(registry.getAssistantStream('stream-1')!.assistantTurn)).toBe(original.turn)

    const failure = new Error('failed')
    registry.rejectAssistantStream('stream-1', failure)
    await expect(original.completion).rejects.toBe(failure)
    expect(original.turn.streaming).toBe(false)
  })

  it('discards a pre-dispatch stream as a settled terminal transition', async () => {
    const { registry } = makeRegistry()
    const entry = track(registry, 'stream-1')

    registry.discardAssistantStream('stream-1')

    await expect(entry.completion).resolves.toBeUndefined()
    expect(entry.turn.streaming).toBe(false)
    expect(registry.getAssistantStream('stream-1')).toBeUndefined()
    expect(registry.isTerminalStream('stream-1')).toBe(true)
    await expect(track(registry, 'stream-1').completion).rejects.toThrow('stream_id stream-1 is already terminal')
  })

  it('reactively prioritizes only the selected bot streaming session', async () => {
    const { registry, currentBotId, sessionId } = makeRegistry('session-b')
    const first = track(registry, 'stream-a', 'session-a')
    const second = track(registry, 'stream-b', 'session-b')
    const otherBot = track(registry, 'stream-other', 'session-b', 'bot-2')

    expect(registry.streaming.value).toBe(true)
    expect(registry.streamingSessionId.value).toBe('session-b')
    expect(registry.assistantStreamsForSession('bot-1', 'session-a').map(stream => stream.streamId)).toEqual(['stream-a'])
    expect(registry.assistantStreamsForSession('bot-2', 'session-a')).toEqual([])
    expect(registry.isSessionStreaming('bot-1', 'session-b')).toBe(true)
    expect(registry.isSessionStreaming('bot-2', 'session-b')).toBe(true)

    sessionId.value = 'session-c'
    expect(registry.streaming.value).toBe(false)
    expect(registry.streamingSessionId.value).toBe('session-a')

    currentBotId.value = 'bot-2'
    expect(registry.streaming.value).toBe(false)
    expect(registry.streamingSessionId.value).toBe('session-b')
    currentBotId.value = 'bot-1'

    registry.resolveAssistantStream('stream-a')
    await first.completion
    expect(registry.streamingSessionId.value).toBe('session-b')

    registry.resolveAssistantStream('stream-b')
    await second.completion
    expect(registry.streamingSessionId.value).toBeNull()

    registry.resolveAssistantStream('stream-other')
    await otherBot.completion
  })

  it('routes missing event ids only when the session has one unambiguous stream', async () => {
    const { registry } = makeRegistry()
    const first = track(registry, 'stream-a')

    expect(registry.streamIdForEvent('bot-1', { session_id: 'session-a' })).toBe('stream-a')
    expect(registry.streamIdForEvent('bot-1', { stream_id: 'explicit', session_id: 'session-a' })).toBe('explicit')

    const second = track(registry, 'stream-b')
    expect(registry.streamIdForEvent('bot-1', { session_id: 'session-a' })).toBe('session:bot-1:session-a:agent-stream')
    expect(registry.streamIdForEvent('bot-1', {}, '')).toBe('bot:bot-1:legacy-stream')

    registry.resolveAssistantStream('stream-a')
    registry.resolveAssistantStream('stream-b')
    await Promise.all([first.completion, second.completion])
  })

  it('keeps a shared continuation turn streaming until every stream finishes', async () => {
    const { registry } = makeRegistry()
    const turn = assistantTurn('shared-turn')
    const first = registry.trackAssistantStream({
      streamId: 'main-stream', assistantTurn: turn, botId: 'bot-1', sessionId: 'session-a',
    })
    const second = registry.trackAssistantStream({
      streamId: 'response-stream', assistantTurn: turn, botId: 'bot-1', sessionId: 'session-a',
    })

    registry.resolveAssistantStream('response-stream')
    await second
    expect(turn.streaming).toBe(true)

    registry.resolveAssistantStream('main-stream')
    await first
    expect(turn.streaming).toBe(false)
  })

  it('maps resumed stream block ids after the existing assistant turn', async () => {
    const { registry } = makeRegistry()
    const turn = assistantTurn('resumed-turn')
    turn.messages.push({
      id: 4,
      type: 'tool',
      name: 'ask_user',
      input: {},
      tool_call_id: 'call-ask',
      running: false,
      toolCallId: 'call-ask',
      toolName: 'ask_user',
      result: null,
      done: true,
    })
    const completion = registry.trackAssistantStream({
      streamId: 'response-stream',
      assistantTurn: turn,
      botId: 'bot-1',
      sessionId: 'session-a',
    })

    expect(registry.mapAssistantStreamMessage('response-stream', {
      id: 0,
      type: 'reasoning',
      content: 'Continuing',
    })).toMatchObject({ id: 5, content: 'Continuing' })
    expect(registry.mapAssistantStreamMessage('response-stream', {
      id: 0,
      type: 'reasoning',
      content: 'Continuing with more detail',
    })).toMatchObject({ id: 5, content: 'Continuing with more detail' })
    expect(registry.mapAssistantStreamMessage('response-stream', {
      id: 1,
      type: 'text',
      content: 'Done',
    })).toMatchObject({ id: 6, content: 'Done' })

    registry.resolveAssistantStream('response-stream')
    await completion
  })

  it('binds a deferred stream once and retains created-session metadata past terminal', async () => {
    const { registry, sessionId } = makeRegistry(null)
    const deferred = track(registry, 'stream-1', '')

    expect(registry.streaming.value).toBe(true)
    expect(registry.streamingSessionId.value).toBeNull()
    expect(registry.isUnboundComposerStreaming('bot-1')).toBe(true)
    expect(registry.isUnboundComposerStreaming('bot-1', 'chat')).toBe(true)
    expect(registry.isUnboundComposerStreaming('bot-2')).toBe(false)
    expect(registry.activeUnboundStreamIds('bot-1')).toEqual(['stream-1'])
    expect(registry.activeUnboundStreamIds('bot-1', 'other')).toEqual([])
    registry.recordCreatedSession('stream-1', 'session-created')
    registry.recordCreatedSession('stream-1', 'conflicting-session')
    expect(registry.getAssistantStream('stream-1')?.sessionId).toBe('session-created')
    expect(registry.createdSessionIdForStream('stream-1')).toBe('session-created')

    sessionId.value = 'session-created'
    expect(registry.streaming.value).toBe(true)
    registry.resolveAssistantStream('stream-1')
    await deferred.completion

    expect(registry.createdSessionIdForStream('stream-1')).toBe('session-created')
    registry.forgetCreatedSession('stream-1')
    expect(registry.createdSessionIdForStream('stream-1')).toBe('')
  })

  it('records created-session metadata even after the pending entry is gone', async () => {
    const { registry } = makeRegistry()
    registry.recordCreatedSession('late-stream', 'session-created')
    registry.recordCreatedSession('late-stream', 'conflicting-session')
    expect(registry.createdSessionIdForStream('late-stream')).toBe('session-created')

    const terminal = track(registry, 'terminal-stream')
    registry.resolveAssistantStream('terminal-stream')
    await terminal.completion
    expect(registry.isTerminalStream('terminal-stream')).toBe(true)
    registry.clearStreamHistory()
    expect(registry.createdSessionIdForStream('late-stream')).toBe('')
    expect(registry.isTerminalStream('terminal-stream')).toBe(false)
  })

  it('rejects the global stream snapshot in insertion order', async () => {
    const { registry } = makeRegistry()
    const first = track(registry, 'stream-a1', 'session-a')
    const second = track(registry, 'stream-b1', 'session-b')
    const third = track(registry, 'stream-a2', 'session-a')
    const completions = [first, second, third].map(entry => entry.completion.catch(error => error))
    const failure = new Error('aborted')
    const beforeReject: string[] = []

    registry.rejectAllStreams(failure, (streamId) => {
      expect(registry.getAssistantStream(streamId)).toBeDefined()
      beforeReject.push(streamId)
    })
    expect(beforeReject).toEqual(['stream-a1', 'stream-b1', 'stream-a2'])
    expect(await Promise.all(completions)).toEqual([failure, failure, failure])
  })

  it('isolates identical stream ids across sessions', async () => {
    const { registry } = makeRegistry()
    const first = track(registry, 'shared-stream', 'session-a')
    const second = track(registry, 'shared-stream', 'session-b')

    expect(registry.getAssistantStream('shared-stream')).toBeUndefined()
    expect(toRaw(registry.getAssistantStream('shared-stream', 'bot-1', 'session-a')!.assistantTurn)).toBe(first.turn)
    expect(toRaw(registry.getAssistantStream('shared-stream', 'bot-1', 'session-b')!.assistantTurn)).toBe(second.turn)

    registry.resolveAssistantStream('shared-stream', 'bot-1', 'session-a')
    await first.completion
    expect(registry.isTerminalStream('shared-stream', undefined, 'bot-1', 'session-a')).toBe(true)
    expect(registry.getAssistantStream('shared-stream', 'bot-1', 'session-b')).toBeDefined()

    registry.resolveAssistantStream('shared-stream', 'bot-1', 'session-b')
    await second.completion
  })

  it('rejects an unbound stream instead of overwriting a bound identity during rekey', async () => {
    const { registry } = makeRegistry()
    const bound = track(registry, 'shared-stream', 'session-created')
    const unbound = track(registry, 'shared-stream', '')

    expect(registry.recordCreatedSession('shared-stream', 'session-created', 'bot-1')).toBe('')
    await expect(unbound.completion).rejects.toThrow('stream_id shared-stream is already active in session session-created')
    expect(registry.getAssistantStream('shared-stream', 'bot-1', 'session-created')?.assistantTurn.id)
      .toBe(bound.turn.id)

    registry.resolveAssistantStream('shared-stream', 'bot-1', 'session-created')
    await bound.completion
  })
})
