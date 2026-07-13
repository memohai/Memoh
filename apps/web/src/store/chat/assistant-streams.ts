import { computed, reactive, type Ref } from 'vue'
import type { ChatAssistantTurn, ChatMessage, ChatUserTurn } from './types'

export interface RuntimeReplacementState {
  kind: 'retry' | 'edit'
  optimisticUserTurn: ChatUserTurn | null
  replacedTurns: ChatMessage[]
  restoreForkAnchor: (() => void) | null
  applied: boolean
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
  abortRequested: boolean
  abortSentGeneration: string
}

interface PendingAssistantStream extends AssistantStream {
  sessionId: string
  resolve: () => void
  reject: (error: Error) => void
}

export interface StreamIdentity {
  stream_id?: string
  session_id?: string
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

type BeforeReject = (streamId: string) => void

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
  const terminalStreams = new Map<string, string>()

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
    return assistantStreamsForSession(botId, targetSessionId).length > 0
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

  function fallbackStreamId(botId: string, targetSessionId?: string | null): string {
    const bid = botId.trim() || 'unbound'
    const sid = (targetSessionId ?? '').trim()
    return sid ? `session:${bid}:${sid}:agent-stream` : `bot:${bid}:legacy-stream`
  }

  function streamIdForEvent(botId: string, event: StreamIdentity, targetSessionId?: string): string {
    const explicit = (event.stream_id ?? '').trim()
    if (explicit) return explicit
    const sid = (event.session_id ?? targetSessionId ?? '').trim()
    const activeIds = assistantStreamsForSession(botId, sid).map(stream => stream.streamId)
    return activeIds.length === 1 ? activeIds[0]! : fallbackStreamId(botId, sid)
  }

  // Promise construction registers synchronously. Callers rely on the stream
  // being discoverable before ws.send() can synchronously replay an event.
  function trackAssistantStream(input: TrackAssistantStreamInput): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const id = input.streamId.trim()
      if (!id) {
        reject(new Error('stream_id is required'))
        return
      }
      if (streams.has(id)) {
        reject(new Error(`stream_id ${id} is already active`))
        return
      }
      if (isTerminalStream(id, input.runtimeGeneration)) {
        reject(new Error(`stream_id ${id} is already terminal`))
        return
      }
      const stream: PendingAssistantStream = {
        streamId: id,
        assistantTurn: input.assistantTurn,
        botId: input.botId,
        sessionId: input.sessionId.trim(),
        composerScope: input.composerScope?.trim() || 'chat',
        viewId: input.viewId?.trim() || 'chat',
        runtimeObserved: false,
        runtimeGeneration: input.runtimeGeneration?.trim() ?? '',
        abortRequested: false,
        abortSentGeneration: '',
        resolve,
        reject,
      }
      streams.set(id, stream)
      onTracked?.(stream)
    })
  }

  function getAssistantStream(streamId: string): AssistantStream | undefined {
    return streams.get(streamId.trim())
  }

  function finishAssistantStream(streamId: string): PendingAssistantStream | undefined {
    const stream = streams.get(streamId.trim())
    if (!stream) return undefined
    rememberTerminalStream(stream.streamId, stream.runtimeGeneration)
    streams.delete(stream.streamId)
    if (!activeStreams().some(active => active.assistantTurn === stream.assistantTurn)) {
      finishAssistantTurn(stream.assistantTurn)
    }
    onFinished?.(stream)
    return stream
  }

  function rememberTerminalStream(streamId: string, generation = '') {
    const id = streamId.trim()
    if (!id) return
    terminalStreams.delete(id)
    terminalStreams.set(id, generation.trim())
    if (terminalStreams.size <= TERMINAL_STREAM_HISTORY_LIMIT) return
    const oldest = terminalStreams.keys().next().value
    if (oldest) terminalStreams.delete(oldest)
  }

  function isTerminalStream(streamId: string | undefined, generation?: string): boolean {
    const id = streamId?.trim()
    if (!id) return false
    const terminalGeneration = terminalStreams.get(id)
    if (terminalGeneration === undefined) return false
    const requestedGeneration = generation?.trim() ?? ''
    return !requestedGeneration || terminalGeneration === requestedGeneration
  }

  function terminalStreamGeneration(streamId: string | undefined): string | undefined {
    const id = streamId?.trim()
    return id ? terminalStreams.get(id) : undefined
  }

  function forgetTerminalStream(streamId: string) {
    terminalStreams.delete(streamId.trim())
  }

  function resolveAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectAssistantStream(streamId: string, error: Error) {
    const stream = streams.get(streamId.trim())
    if (stream) beforeReject?.(stream, error)
    finishAssistantStream(streamId)?.reject(error)
  }

  function discardAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectAllStreams(error: Error, beforeReject?: BeforeReject) {
    for (const stream of activeStreams()) {
      beforeReject?.(stream.streamId)
      rejectAssistantStream(stream.streamId, error)
    }
  }

  // Deferred draft streams start unbound and may be assigned exactly once by
  // session_created. A duplicate or late event cannot move them to a new session.
  function recordCreatedSession(streamId: string | undefined, targetSessionId: string): string {
    const id = streamId?.trim()
    const sid = targetSessionId.trim()
    if (!id || !sid) return ''
    const stream = streams.get(id)
    const canonicalSessionId = createdSessionsByStream.get(id) || stream?.sessionId || sid
    if (stream && !stream.sessionId) stream.sessionId = canonicalSessionId
    if (!createdSessionsByStream.has(id)) createdSessionsByStream.set(id, canonicalSessionId)
    return canonicalSessionId
  }

  function createdSessionIdForStream(streamId: string): string {
    return createdSessionsByStream.get(streamId.trim()) ?? ''
  }

  function forgetCreatedSession(streamId: string) {
    createdSessionsByStream.delete(streamId.trim())
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
    streamIdForEvent,
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
