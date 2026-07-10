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
  sessionId: Ref<string | null>
}

type BeforeReject = (streamId: string) => void

export function createAssistantStreamRegistry({ sessionId }: AssistantStreamRegistryDeps) {
  const streams = reactive(new Map<string, PendingAssistantStream>())
  const createdSessionsByStream = new Map<string, string>()

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

  function isSessionStreaming(targetSessionId?: string | null): boolean {
    return activeStreamIdsForSession(targetSessionId).length > 0
  }

  const streamingSessionId = computed(() => {
    const activeSid = (sessionId.value ?? '').trim()
    const activeSessionIds = activeStreams().map(stream => stream.sessionId)
    if (activeSid && activeSessionIds.includes(activeSid)) return activeSid
    return activeSessionIds[0] ?? null
  })

  const streaming = computed(() => isSessionStreaming(sessionId.value))

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
    stream.assistantTurn.streaming = false
    streams.delete(stream.streamId)
    return stream
  }

  function resolveAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectAssistantStream(streamId: string, error: Error) {
    finishAssistantStream(streamId)?.reject(error)
  }

  function forgetAssistantStream(streamId: string) {
    streams.delete(streamId.trim())
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
  function recordCreatedSession(streamId: string | undefined, targetSessionId: string) {
    const id = streamId?.trim()
    const sid = targetSessionId.trim()
    if (!id || !sid) return
    const stream = streams.get(id)
    if (stream && !stream.sessionId) stream.sessionId = sid
    createdSessionsByStream.set(id, sid)
  }

  function createdSessionIdForStream(streamId: string): string {
    return createdSessionsByStream.get(streamId.trim()) ?? ''
  }

  function forgetCreatedSession(streamId: string) {
    createdSessionsByStream.delete(streamId.trim())
  }

  function clearCreatedSessions() {
    createdSessionsByStream.clear()
  }

  return {
    streaming,
    streamingSessionId,
    activeStreamIdsForSession,
    isSessionStreaming,
    streamIdForEvent,
    trackAssistantStream,
    getAssistantStream,
    resolveAssistantStream,
    rejectAssistantStream,
    forgetAssistantStream,
    rejectSessionStreams,
    rejectAllStreams,
    recordCreatedSession,
    createdSessionIdForStream,
    forgetCreatedSession,
    clearCreatedSessions,
  }
}
