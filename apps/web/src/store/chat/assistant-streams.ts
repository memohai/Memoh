import { computed, reactive, type Ref } from 'vue'
import type { ChatAssistantTurn } from './types'

export interface AssistantStream {
  readonly streamId: string
  readonly assistantTurn: ChatAssistantTurn
  readonly botId: string
  readonly sessionId: string
  readonly composerScope: string
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
}

interface AssistantStreamRegistryDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  finishAssistantTurn: (turn: ChatAssistantTurn) => void
}

type BeforeReject = (streamId: string) => void

const TERMINAL_STREAM_HISTORY_LIMIT = 512

export function createAssistantStreamRegistry({ currentBotId, sessionId, finishAssistantTurn }: AssistantStreamRegistryDeps) {
  const streams = reactive(new Map<string, PendingAssistantStream>())
  const createdSessionsByStream = new Map<string, string>()
  const terminalStreamIds = new Set<string>()

  function activeStreams(): PendingAssistantStream[] {
    return [...streams.values()]
  }

  function activeStreamIdsForSession(targetSessionId?: string | null): string[] {
    const sid = (targetSessionId ?? '').trim()
    if (!sid) return []
    return activeStreams()
      .filter(stream => stream.sessionId === sid)
      .map(stream => stream.streamId)
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

  function assistantStreamsForSession(botId: string, targetSessionId: string): AssistantStream[] {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return []
    return activeStreams().filter(stream => stream.botId === bid && stream.sessionId === sid)
  }

  function isSessionStreaming(targetSessionId?: string | null): boolean {
    return activeStreamIdsForSession(targetSessionId).length > 0
  }

  function isUnboundComposerStreaming(botId: string | null | undefined, composerScope?: string): boolean {
    return activeUnboundStreamIds(botId, composerScope).length > 0
  }

  const streamingSessionId = computed(() => {
    const activeSid = (sessionId.value ?? '').trim()
    const activeSessionIds = activeStreams().map(stream => stream.sessionId).filter(Boolean)
    if (activeSid && activeSessionIds.includes(activeSid)) return activeSid
    return activeSessionIds[0] ?? null
  })

  const streaming = computed(() => {
    const activeSid = (sessionId.value ?? '').trim()
    return activeSid
      ? isSessionStreaming(activeSid)
      : isUnboundComposerStreaming(currentBotId.value)
  })

  function fallbackStreamId(targetSessionId?: string | null): string {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    return sid ? `session:${sid}:agent-stream` : 'legacy-stream'
  }

  function streamIdForEvent(event: StreamIdentity, targetSessionId?: string): string {
    const explicit = (event.stream_id ?? '').trim()
    if (explicit) return explicit
    const sid = (event.session_id ?? targetSessionId ?? '').trim()
    const activeIds = activeStreamIdsForSession(sid)
    return activeIds.length === 1 ? activeIds[0]! : fallbackStreamId(sid)
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
      if (terminalStreamIds.has(id)) {
        reject(new Error(`stream_id ${id} is already terminal`))
        return
      }
      streams.set(id, {
        streamId: id,
        assistantTurn: input.assistantTurn,
        botId: input.botId,
        sessionId: input.sessionId.trim(),
        composerScope: input.composerScope?.trim() || 'chat',
        resolve,
        reject,
      })
    })
  }

  function getAssistantStream(streamId: string): AssistantStream | undefined {
    return streams.get(streamId.trim())
  }

  function finishAssistantStream(streamId: string): PendingAssistantStream | undefined {
    const stream = streams.get(streamId.trim())
    if (!stream) return undefined
    rememberTerminalStream(stream.streamId)
    streams.delete(stream.streamId)
    if (!activeStreams().some(active => active.assistantTurn === stream.assistantTurn)) {
      finishAssistantTurn(stream.assistantTurn)
    }
    return stream
  }

  function rememberTerminalStream(streamId: string) {
    const id = streamId.trim()
    if (!id) return
    terminalStreamIds.add(id)
    if (terminalStreamIds.size <= TERMINAL_STREAM_HISTORY_LIMIT) return
    const oldest = terminalStreamIds.values().next().value
    if (oldest) terminalStreamIds.delete(oldest)
  }

  function isTerminalStream(streamId: string | undefined): boolean {
    const id = streamId?.trim()
    return Boolean(id && terminalStreamIds.has(id))
  }

  function resolveAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectAssistantStream(streamId: string, error: Error) {
    finishAssistantStream(streamId)?.reject(error)
  }

  function discardAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectSessionStreams(targetSessionId: string | null | undefined, error: Error, beforeReject?: BeforeReject) {
    for (const streamId of activeStreamIdsForSession(targetSessionId)) {
      beforeReject?.(streamId)
      rejectAssistantStream(streamId, error)
    }
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
    terminalStreamIds.clear()
  }

  return {
    streaming,
    streamingSessionId,
    activeStreamIdsForSession,
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
    rejectSessionStreams,
    rejectAllStreams,
    recordCreatedSession,
    createdSessionIdForStream,
    forgetCreatedSession,
    clearStreamHistory,
  }
}
