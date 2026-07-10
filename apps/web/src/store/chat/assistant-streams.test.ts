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

function makeRegistry(activeSessionId: string | null = 'session-a') {
  const currentBotId = ref<string | null>('bot-1')
  const sessionId = ref<string | null>(activeSessionId)
  const finishAssistantTurn = vi.fn((turn: ChatAssistantTurn) => {
    turn.streaming = false
  })
  const registry = createAssistantStreamRegistry({ currentBotId, sessionId, finishAssistantTurn })
  return { registry, currentBotId, sessionId, finishAssistantTurn }
}

function track(
  registry: ReturnType<typeof createAssistantStreamRegistry>,
  streamId: string,
  targetSessionId = 'session-a',
) {
  const turn = assistantTurn(`turn-${streamId}`)
  const completion = registry.trackAssistantStream({
    streamId,
    assistantTurn: turn,
    botId: 'bot-1',
    sessionId: targetSessionId,
  })
  return { turn, completion }
}

describe('assistant stream registry', () => {
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

  it('reactively prioritizes the selected streaming session', async () => {
    const { registry, sessionId } = makeRegistry('session-b')
    const first = track(registry, 'stream-a', 'session-a')
    const second = track(registry, 'stream-b', 'session-b')

    expect(registry.streaming.value).toBe(true)
    expect(registry.streamingSessionId.value).toBe('session-b')
    expect(registry.assistantStreamsForSession('bot-1', 'session-a').map(stream => stream.streamId)).toEqual(['stream-a'])
    expect(registry.assistantStreamsForSession('bot-2', 'session-a')).toEqual([])

    sessionId.value = 'session-c'
    expect(registry.streaming.value).toBe(false)
    expect(registry.streamingSessionId.value).toBe('session-a')

    registry.resolveAssistantStream('stream-a')
    await first.completion
    expect(registry.streamingSessionId.value).toBe('session-b')

    registry.resolveAssistantStream('stream-b')
    await second.completion
    expect(registry.streamingSessionId.value).toBeNull()
  })

  it('routes missing event ids only when the session has one unambiguous stream', async () => {
    const { registry } = makeRegistry()
    const first = track(registry, 'stream-a')

    expect(registry.streamIdForEvent({ session_id: 'session-a' })).toBe('stream-a')
    expect(registry.streamIdForEvent({ stream_id: 'explicit', session_id: 'session-a' })).toBe('explicit')

    const second = track(registry, 'stream-b')
    expect(registry.streamIdForEvent({ session_id: 'session-a' })).toBe('session:session-a:agent-stream')
    expect(registry.streamIdForEvent({}, '')).toBe('legacy-stream')

    registry.resolveAssistantStream('stream-a')
    registry.resolveAssistantStream('stream-b')
    await Promise.all([first.completion, second.completion])
  })

  it('binds a deferred stream once and retains created-session metadata past terminal', async () => {
    const { registry, sessionId } = makeRegistry(null)
    const deferred = track(registry, 'stream-1', '')

    expect(registry.streaming.value).toBe(true)
    expect(registry.streamingSessionId.value).toBeNull()
    expect(registry.isUnboundComposerStreaming('bot-1')).toBe(true)
    expect(registry.isUnboundComposerStreaming('bot-1', 'chat')).toBe(true)
    expect(registry.isUnboundComposerStreaming('bot-2')).toBe(false)
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

  it('rejects session and global snapshots in insertion order', async () => {
    const { registry } = makeRegistry()
    const first = track(registry, 'stream-a1', 'session-a')
    const second = track(registry, 'stream-b1', 'session-b')
    const third = track(registry, 'stream-a2', 'session-a')
    const completions = [first, second, third].map(entry => entry.completion.catch(error => error))
    const failure = new Error('aborted')
    const beforeReject: string[] = []

    registry.rejectSessionStreams('session-a', failure, (streamId) => {
      expect(registry.getAssistantStream(streamId)).toBeDefined()
      beforeReject.push(streamId)
    })
    expect(beforeReject).toEqual(['stream-a1', 'stream-a2'])
    expect(registry.getAssistantStream('stream-b1')).toBeDefined()

    registry.rejectAllStreams(failure, (streamId) => {
      expect(registry.getAssistantStream(streamId)).toBeDefined()
      beforeReject.push(streamId)
    })
    expect(beforeReject).toEqual(['stream-a1', 'stream-a2', 'stream-b1'])
    expect(await Promise.all(completions)).toEqual([failure, failure, failure])
  })
})
