import { computed, reactive, type Ref } from 'vue'
import type { ChatAssistantTurn, ChatMessage, ChatUserTurn } from './types'

export interface RuntimeReplacementState {
  kind: 'retry' | 'edit'
  optimisticUserTurn: ChatUserTurn | null
  replacedTurns: ChatMessage[]
  restoreForkAnchor: (() => void) | null
  applied: boolean
  historyCommitted: boolean
}

export interface AssistantStream {
  readonly streamId: string
  assistantTurn: ChatAssistantTurn
  readonly botId: string
  sessionId: string
  readonly composerScope: string
  readonly viewId: string
  runtimeReplacement?: RuntimeReplacementState
  runtimeObserved: boolean
  runtimeGeneration: string
  runtimeMessageIds: Set<number>
  abortRequested: boolean
  abortSent: boolean
  abortSentGeneration: string
}

interface PendingAssistantStream extends AssistantStream {
  sessionId: string
  resolve: () => void
  reject: (error: Error) => void
}

export interface TrackAssistantStreamInput {
  streamId: string
  assistantTurn: ChatAssistantTurn
  botId: string
  sessionId: string
  composerScope?: string
  viewId?: string
  runtimeGeneration?: string
}

interface AssistantStreamRegistryDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  finishAssistantTurn: (turn: ChatAssistantTurn) => void
  beforeReject?: (stream: AssistantStream, error: Error) => void
  onTracked?: (stream: AssistantStream) => void
  onFinished?: (stream: AssistantStream) => void
}

type BeforeReject = (streamId: string, botId: string, sessionId: string) => void

const TERMINAL_STREAM_HISTORY_LIMIT = 512

export function createAssistantStreamRegistry({
  currentBotId,
  sessionId,
  finishAssistantTurn,
  beforeReject,
  onTracked,
  onFinished,
}: AssistantStreamRegistryDeps) {
  const streams = reactive(new Map<string, PendingAssistantStream>())
  const createdSessionsByStream = new Map<string, string>()
  const terminalStreams = new Map<string, { streamId: string, botId: string, sessionId: string, generation: string }>()

  function scopedStreamKey(botId: string, targetSessionId: string, streamId: string) {
    return `${botId.trim()}\u0000${targetSessionId.trim()}\u0000${streamId.trim()}`
  }

  function createdStreamKey(botId: string, streamId: string) {
    return `${botId.trim()}\u0000${streamId.trim()}`
  }

  function findAssistantStream(streamId: string, botId?: string, targetSessionId?: string): PendingAssistantStream | undefined {
    const id = streamId.trim()
    if (!id) return undefined
    if (botId !== undefined && targetSessionId !== undefined) {
      return streams.get(scopedStreamKey(botId, targetSessionId, id))
    }
    const matches = activeStreams().filter(stream => stream.streamId === id
      && (botId === undefined || stream.botId === botId.trim())
      && (targetSessionId === undefined || stream.sessionId === targetSessionId.trim()))
    return matches.length === 1 ? matches[0] : undefined
  }

  function activeStreams(): PendingAssistantStream[] {
    return [...streams.values()]
  }

  function activeUnboundStreamIds(botId: string | null | undefined, composerScope?: string): string[] {
    const bid = (botId ?? '').trim()
    const scope = composerScope?.trim()
    if (!bid) return []
    return activeStreams()
      .filter(stream => stream.botId === bid
        && !stream.sessionId
        && (!scope || stream.composerScope === scope))
      .map(stream => stream.streamId)
  }

  function assistantStreamsForSession(
    botId: string | null | undefined,
    targetSessionId: string | null | undefined,
  ): AssistantStream[] {
    const bid = (botId ?? '').trim()
    const sid = (targetSessionId ?? '').trim()
    if (!bid || !sid) return []
    return activeStreams().filter(stream => stream.botId === bid && stream.sessionId === sid)
  }

  function isSessionStreaming(
    botId: string | null | undefined,
    targetSessionId: string | null | undefined,
  ): boolean {
    return assistantStreamsForSession(botId, targetSessionId)
      .some(stream => stream.assistantTurn.streaming)
  }

  function isUnboundComposerStreaming(botId: string | null | undefined, composerScope?: string): boolean {
    return activeUnboundStreamIds(botId, composerScope).length > 0
  }

  const streamingSessionId = computed(() => {
    const bid = (currentBotId.value ?? '').trim()
    const activeSid = (sessionId.value ?? '').trim()
    const activeSessionIds = activeStreams()
      .filter(stream => stream.botId === bid)
      .map(stream => stream.sessionId)
      .filter(Boolean)
    if (activeSid && activeSessionIds.includes(activeSid)) return activeSid
    return activeSessionIds[0] ?? null
  })

  const streaming = computed(() => {
    const bid = (currentBotId.value ?? '').trim()
    const activeSid = (sessionId.value ?? '').trim()
    return activeSid
      ? isSessionStreaming(bid, activeSid)
      : isUnboundComposerStreaming(bid)
  })

  // Promise construction registers synchronously. Callers rely on the stream
  // being discoverable before ws.send() can synchronously replay an event.
  function trackAssistantStream(input: TrackAssistantStreamInput): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const id = input.streamId.trim()
      if (!id) {
        reject(new Error('stream_id is required'))
        return
      }
      const botId = input.botId.trim()
      const targetSessionId = input.sessionId.trim()
      const key = scopedStreamKey(botId, targetSessionId, id)
      if (streams.has(key)) {
        reject(new Error(`stream_id ${id} is already active`))
        return
      }
      if (isTerminalStream(id, input.runtimeGeneration, botId, targetSessionId)) {
        reject(new Error(`stream_id ${id} is already terminal`))
        return
      }
      const stream: PendingAssistantStream = {
        streamId: id,
        assistantTurn: input.assistantTurn,
        botId,
        sessionId: targetSessionId,
        composerScope: input.composerScope?.trim() || 'chat',
        viewId: input.viewId?.trim() || 'chat',
        runtimeObserved: false,
        runtimeGeneration: input.runtimeGeneration?.trim() ?? '',
        runtimeMessageIds: new Set<number>(),
        abortRequested: false,
        abortSent: false,
        abortSentGeneration: '',
        resolve,
        reject,
      }
      streams.set(key, stream)
      onTracked?.(stream)
    })
  }

  function getAssistantStream(streamId: string, botId?: string, targetSessionId?: string): AssistantStream | undefined {
    return findAssistantStream(streamId, botId, targetSessionId)
  }

  function finishAssistantStream(streamId: string, botId?: string, targetSessionId?: string): PendingAssistantStream | undefined {
    const stream = findAssistantStream(streamId, botId, targetSessionId)
    if (!stream) return undefined
    rememberTerminalStream(stream)
    streams.delete(scopedStreamKey(stream.botId, stream.sessionId, stream.streamId))
    if (!activeStreams().some(active => active.assistantTurn === stream.assistantTurn)) {
      finishAssistantTurn(stream.assistantTurn)
    }
    onFinished?.(stream)
    return stream
  }

  function rememberTerminalStream(stream: PendingAssistantStream) {
    const key = scopedStreamKey(stream.botId, stream.sessionId, stream.streamId)
    terminalStreams.delete(key)
    terminalStreams.set(key, {
      streamId: stream.streamId,
      botId: stream.botId,
      sessionId: stream.sessionId,
      generation: stream.runtimeGeneration.trim(),
    })
    if (terminalStreams.size <= TERMINAL_STREAM_HISTORY_LIMIT) return
    const oldest = terminalStreams.keys().next().value
    if (oldest) terminalStreams.delete(oldest)
  }

  function terminalStream(streamId: string | undefined, botId?: string, targetSessionId?: string) {
    const id = streamId?.trim()
    if (!id) return undefined
    if (botId !== undefined && targetSessionId !== undefined) {
      return terminalStreams.get(scopedStreamKey(botId, targetSessionId, id))
    }
    const matches = [...terminalStreams.values()].filter(stream => stream.streamId === id
      && (botId === undefined || stream.botId === botId.trim())
      && (targetSessionId === undefined || stream.sessionId === targetSessionId.trim()))
    return matches.length === 1 ? matches[0] : undefined
  }

  function isTerminalStream(streamId: string | undefined, generation?: string, botId?: string, targetSessionId?: string): boolean {
    const terminal = terminalStream(streamId, botId, targetSessionId)
    if (!terminal) return false
    const requestedGeneration = generation?.trim() ?? ''
    return !requestedGeneration || terminal.generation === requestedGeneration
  }

  function terminalStreamGeneration(streamId: string | undefined, botId?: string, targetSessionId?: string): string | undefined {
    return terminalStream(streamId, botId, targetSessionId)?.generation
  }

  function forgetTerminalStream(streamId: string, botId?: string, targetSessionId?: string) {
    const terminal = terminalStream(streamId, botId, targetSessionId)
    if (terminal) terminalStreams.delete(scopedStreamKey(terminal.botId, terminal.sessionId, terminal.streamId))
  }

  function resolveAssistantStream(streamId: string, botId?: string, targetSessionId?: string) {
    finishAssistantStream(streamId, botId, targetSessionId)?.resolve()
  }

  function rejectAssistantStream(streamId: string, error: Error, botId?: string, targetSessionId?: string) {
    const stream = findAssistantStream(streamId, botId, targetSessionId)
    if (stream) beforeReject?.(stream, error)
    finishAssistantStream(streamId, botId, targetSessionId)?.reject(error)
  }

  function discardAssistantStream(streamId: string, botId?: string, targetSessionId?: string) {
    finishAssistantStream(streamId, botId, targetSessionId)?.resolve()
  }

  function rejectAllStreams(error: Error, beforeReject?: BeforeReject) {
    for (const stream of activeStreams()) {
      beforeReject?.(stream.streamId, stream.botId, stream.sessionId)
      rejectAssistantStream(stream.streamId, error, stream.botId, stream.sessionId)
    }
  }

  // Deferred draft streams start unbound and may be assigned exactly once by
  // session_created. A duplicate or late event cannot move them to a new session.
  function recordCreatedSession(streamId: string | undefined, targetSessionId: string, botId = ''): string {
    const id = streamId?.trim()
    const sid = targetSessionId.trim()
    if (!id || !sid) return ''
    const bid = botId.trim()
    const existingCreatedSession = createdSessionIdForStream(id, bid)
    if (existingCreatedSession) return existingCreatedSession
    const stream = findAssistantStream(id, bid || undefined, '')
    const createdKey = createdStreamKey(bid || stream?.botId || '', id)
    const canonicalSessionId = createdSessionsByStream.get(createdKey) || stream?.sessionId || sid
    if (stream && !stream.sessionId) {
      const targetKey = scopedStreamKey(stream.botId, canonicalSessionId, id)
      const target = streams.get(targetKey)
      if (target && target !== stream) {
        rejectAssistantStream(id, new Error(`stream_id ${id} is already active in session ${canonicalSessionId}`), stream.botId, '')
        return ''
      }
      streams.delete(scopedStreamKey(stream.botId, '', id))
      stream.sessionId = canonicalSessionId
      streams.set(targetKey, stream)
    }
    if (!createdSessionsByStream.has(createdKey)) createdSessionsByStream.set(createdKey, canonicalSessionId)
    return canonicalSessionId
  }

  function createdSessionIdForStream(streamId: string, botId = ''): string {
    const id = streamId.trim()
    const bid = botId.trim()
    if (bid) return createdSessionsByStream.get(createdStreamKey(bid, id)) ?? ''
    const matches = [...createdSessionsByStream.entries()].filter(([key]) => key.endsWith(`\u0000${id}`))
    return matches.length === 1 ? matches[0]![1] : ''
  }

  function forgetCreatedSession(streamId: string, botId = '') {
    const id = streamId.trim()
    const bid = botId.trim()
    if (bid) {
      createdSessionsByStream.delete(createdStreamKey(bid, id))
      return
    }
    for (const key of createdSessionsByStream.keys()) {
      if (key.endsWith(`\u0000${id}`)) createdSessionsByStream.delete(key)
    }
  }

  function clearStreamHistory() {
    createdSessionsByStream.clear()
    terminalStreams.clear()
  }

  return {
    streaming,
    streamingSessionId,
    activeStreams,
    activeUnboundStreamIds,
    assistantStreamsForSession,
    isSessionStreaming,
    isUnboundComposerStreaming,
    trackAssistantStream,
    getAssistantStream,
    resolveAssistantStream,
    rejectAssistantStream,
    discardAssistantStream,
    isTerminalStream,
    terminalStreamGeneration,
    forgetTerminalStream,
    rejectAllStreams,
    recordCreatedSession,
    createdSessionIdForStream,
    forgetCreatedSession,
    clearStreamHistory,
  }
}
