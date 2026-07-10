import { defineStore, storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import { toast } from '@felinic/ui'
import enMessages from '@/i18n/locales/en.json'
import zhMessages from '@/i18n/locales/zh.json'
import jaMessages from '@/i18n/locales/ja.json'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  normalizedRuntimeType,
  provisionalSessionTitle,
  shouldRefreshFromMessageCreated,
  type SidebarSessionMode,
} from './chat-list.utils'
import {
  cloneRequestedSkills,
  createStreamId,
  hasUserAttachments,
  isPendingBot,
  nextId,
  normalizeRequestedSkills,
  requestedSkillRequestsForWire,
  serverMessageId,
} from './chat-list.normalize'
import { createFsChangeBeacon } from './chat/fs-beacon'
import { createCommandEventRegistry } from './chat/command-events'
import { createSessionList } from './chat/session-list'
import { acpSessionMetadata, createACPStaging } from './chat/acp-staging'
import { createTranscriptController } from './chat/transcript'
import { createAssistantStreamRegistry } from './chat/assistant-streams'
import {
  createBackgroundTaskTracker,
  normalizeBackgroundTask,
} from './chat/background-tasks'
import type {
  ACPAgentSessionInput,
  ActiveChatTarget,
  ChatAssistantTurn,
  ChatMessage,
  ChatUserTurn,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStage,
} from './chat/types'
import {
  createSession,
  deleteSession as requestDeleteSession,
  ensureACPRuntime as requestEnsureACPRuntime,
  setACPRuntimeModel as requestSetACPRuntimeModel,
  updateSessionAgent as requestUpdateSessionAgent,
  updateSessionTitle as requestUpdateSessionTitle,
  forkSessionFromMessage as requestForkSessionFromMessage,
  fetchSession,
  fetchSessions,
  type Bot,
  type BotSessionActivityEvent,
  type SessionSummary,
  type SessionMessageStreamEvent,
  type ChatAttachment,
  type CommandEventResponse,
  type ChatWebSocket,
  type UIToolApproval,
  type UIUserInput,
  type RequestedSkillSelection,
  type WSUserInputAnswer,
  type UIStreamEvent,
  executeQuickAction,
  fetchBots,
  fetchMessagesUI,
  sendLocalChannelMessage,
  streamBotSessionsActivityEvents,
  streamSessionMessageEvents,
  connectWebSocket,
  locateMessageUI,
} from '@/composables/api/useChat'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH } from '@/utils/acp'
import { getBotsByBotIdSettings } from '@memohai/sdk'
import type { AcpagentRuntimeStatus } from '@memohai/sdk'

export type {
  ACPAgentSessionInput,
  ActiveChatTarget,
  AttachmentBlock,
  AttachmentItem,
  BackgroundTask,
  ChatAssistantTurn,
  ChatMessage,
  ChatSystemTurn,
  ChatUserTurn,
  ContentBlock,
  ErrorBlock,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStage,
  TextBlock,
  ThinkingBlock,
  ToolCallBlock,
} from './chat/types'

// fs-change beacon lives in ./chat/fs-beacon; types re-exported so existing
// consumers keep importing them from the store module.
export type { FsChangeBatch, FsChangeEvent, FsToolKind } from './chat/fs-beacon'

function currentLocale() {
  const storage = globalThis.localStorage
  const locale = typeof storage?.getItem !== 'function'
    ? ''
    : storage.getItem('language')
  return locale === 'zh' || locale === 'ja' ? locale : 'en'
}

function localizedMessages() {
  const locale = currentLocale()
  return locale === 'zh' ? zhMessages : locale === 'ja' ? jaMessages : enMessages
}

function userInputConnectionLostMessage() {
  const messages = localizedMessages()
  return messages.chat.tools.userInputConnectionLost
}

function sendFailedMessage() {
  const messages = localizedMessages()
  return messages.chat.sendFailed
}

function commandErrorMessage(code: string) {
  const messages = localizedMessages()
  const errors = messages.chat.slash.errorMessages as Record<string, string>
  return errors[code] || errors.generic || 'Slash command failed.'
}

function forkFailedMessage() {
  const messages = localizedMessages()
  return messages.chat.forkFailed
}

type WebNewCommandResult =
  | { kind: 'none' }
  | { kind: 'handled' }
  | { kind: 'error'; message: string }

type WebSlashCommandResult =
  | { kind: 'none' }
  | { kind: 'handled' }
  | { kind: 'error'; message: string }

function parseWebNewCommand(text: string): { mode: 'chat' | 'discuss' | ''; agentId: string } | null {
  const input = text.trim()
  if (!input.startsWith('/new')) return null
  const parts = input.split(/\s+/)
  if (parts[0] !== '/new') return null
  const positional = parts.slice(1).filter(part => part && !part.startsWith('-'))
  const first = positional[0]?.toLowerCase() ?? ''
  const second = positional[1]?.toLowerCase() ?? ''
  if (first === 'chat' || first === 'discuss') {
    return { mode: first, agentId: second }
  }
  return { mode: '', agentId: first }
}

interface StartupSendFailure {
  id: string
  botId: string
  sessionId: string
  composerScope?: string
  error: string
  restoreInput: string
  restoreAttachments?: ChatAttachment[]
  restoreRequestedSkills?: RequestedSkillSelection[]
}

class StreamFailureError extends Error {
  stage: SendMessageStage

  constructor(message: string, stage: SendMessageStage) {
    super(message)
    this.name = 'StreamFailureError'
    this.stage = stage
  }
}

class CommandStreamError extends StreamFailureError {
  constructor(message: string) {
    super(message, 'startup')
    this.name = 'CommandStreamError'
  }
}

export const useChatStore = defineStore('chat', () => {
  const selectionStore = useChatSelectionStore()
  const { currentBotId, sessionId, draftIntent, explicitSelection: explicitSessionSelection } = storeToRefs(selectionStore)

  const fsBeacon = createFsChangeBeacon({ currentBotId, sessionId })
  const {
    fsChangedAt,
    markFsChanged,
    affectsPath,
    fsEventForPath,
    bumpFsChangedAtIfFsMutation,
    resetFsBeacon,
    clearFsForBotSwitch,
  } = fsBeacon
  const backgroundTasks = createBackgroundTaskTracker()
  const {
    rememberBackgroundTask,
    applyPendingBackgroundEventsToTool,
  } = backgroundTasks
  const transcript = createTranscriptController({
    currentBotId,
    sessionId,
    rememberBackgroundTask,
    applyPendingBackgroundEventsToTool,
    bumpFsChangedAtIfFsMutation,
    fetchMessages: fetchMessagesUI,
    locateMessage: locateMessageUI,
  })
  const {
    messages,
    loadingMessages,
    loadingOlder,
    hasMoreOlder,
    hasLoadedOlder,
    normalizeTurn,
    clearHistoryView,
    prepareForInitialization,
    markHistoryEmpty,
    replaceHistoryView,
    refreshCurrentSession,
    loadInitialMessages,
    fetchSessionWindow,
    loadOlderMessages,
    findMessageIdByExternalId,
    locateMessageByExternalId,
    isActiveSessionTarget,
    appendTurnToSession,
    appendToView,
    removeFromView,
    removeTurnFromSession,
    replaceTailFromTurn,
    restoreTailFromOptimistic,
    createOptimisticAssistantTurn,
    createOptimisticUserTurn,
    upsertAssistantUIMessage,
    hasVisibleAssistantBlocks,
    finishAssistantTurn,
    snapshotToolApprovalStates,
    restoreToolApprovalStates,
    snapshotUserInputStates,
    restoreUserInputStates,
    finalizeStreamFailure,
    latestOptimisticUserText,
    markToolApprovalDecision,
    resetUserScope: resetTranscriptUserScope,
  } = transcript
  const assistantStreams = createAssistantStreamRegistry({ currentBotId, sessionId, finishAssistantTurn })
  const {
    streaming,
    streamingSessionId,
    assistantStreamsForSession,
    isSessionStreaming,
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
  } = assistantStreams
  // In-flight tool-approval responses, keyed by response stream id. Silent
  // entries belong to a session that is already streaming: their events are
  // swallowed instead of rendered as a new assistant turn. Entries normally
  // clear on the response stream's end/error; the expiry covers streams whose
  // terminal event never arrives (e.g. a WebSocket drop mid-approval), so the
  // approval doesn't stay locked against retries until a reload.
  const APPROVAL_RESPONSE_TTL_MS = 2 * 60 * 1000
  const approvalResponseStreams = new Map<string, { approvalId: string, silent: boolean, at: number }>()
  const forkingMessages = new Set<string>()
  // Sessions-list bookkeeping + fork-anchor tracking (see ./chat/session-list).
  const sessionList = createSessionList({ currentBotId, sessionId, messages })
  const {
    sessions,
    sessionsCursor,
    hasMoreSessions,
    loadingMoreSessions,
    activeSession,
    knownSessions,
    activeChatReadOnly,
    activeChatCanFork,
    withForkAnchorFromUITurns,
    syncForkAnchorFromUITurns,
    updateForkAnchorForReplacedMessage,
    replaceSessions,
    appendSessions,
    upsertSession,
    rememberSession,
    knownSessionSummary,
    hasListedSession,
    patchSessionInList,
    updateKnownSessionTitle,
    removeSessionFromList,
    touchSessionInList,
    touchKnownSession,
    fallbackSessionAfterDelete,
    markSessionDeleted,
    clearDeletedSessionIds,
    clearRememberedSessions,
  } = sessionList
  transcript.setSnapshotHook(syncForkAnchorFromUITurns)
  transcript.setRefreshAppliedHook((targetSessionId, latestTimestamp) => {
    touchSessionInList(targetSessionId, latestTimestamp)
  })
  const loading = ref(false)
  // `loadingChats` covers the bot-level boot path (sessions list fetch), so
  // the sidebar can show its skeleton + suppress its empty-state placeholder
  // exactly while the sessions list is in flight.
  // `loadingMessages` covers the per-session transcript fetch — the sidebar
  // never reacts to it, only the chat pane uses it to keep its own empty
  // placeholders hidden while a fresh transcript is on its way.
  const loadingChats = ref(false)
  const initializing = ref(false)
  let initializeRerunRequested = false
  let initializingBotId: string | null = null
  let initializePromise: Promise<void> | null = null
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')
  const startupSendFailure = ref<StartupSendFailure | null>(null)
  // Slash-command event registry (see ./chat/command-events for scoping rules).
  const commandEventRegistry = createCommandEventRegistry({ currentBotId, sessionId })
  const {
    commandEvent,
    currentCommandScope,
    commandEventForScope,
    rememberCommandEvent,
    showCommandError,
    clearCommandEvent,
    rescopeSessionCommandEventToComposer,
    resetCommandEvents,
  } = commandEventRegistry
  // Bumps when the user sends a message, carrying the resolved session id and
  // whether that send just promoted a draft (created the session). The workspace
  // tab store watches this to pin the chat tab — a session you have sent in is no
  // longer an ephemeral "preview" tab. seq forces the watch to fire on repeats.
  const userSentInSession = ref<{ id: string, wasDraft: boolean, seq: number } | null>(null)
  let userSendSeq = 0
  // Bumps after a session delete succeeds. Consumers that own per-session UI
  // chrome must not infer deletion from the paginated session list: a valid open
  // tab can fall off the current page without being deleted.
  const deletedSession = ref<{ id: string, botId: string, seq: number, composerScope?: string } | null>(null)
  let deletedSessionSeq = 0

  let activeWs: ChatWebSocket | null = null
  let webSocketGeneration = 0
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let sessionListRefreshPromise: { botId: string; promise: Promise<void> } | null = null
  // Two independent streams replace the deleted bot-wide messages SSE:
  // - sessionMessagesStream follows the active sessionId and feeds the
  //   transcript (server pushes a small backlog + live messages for that
  //   session only, so no client-supplied cursor is needed).
  // - botSessionsActivityStream is bot-wide and lightweight: identifiers
  //   only, never message bodies, used to keep the sidebar live-sorted and
  //   to notice sessions created from external channels.
  const sessionMessagesStream = useRetryingStream()
  const botSessionsActivityStream = useRetryingStream()
  const acpRuntimeStatuses = ref<Record<string, AcpagentRuntimeStatus | undefined>>({})
  const acpRuntimePending = ref<Record<string, boolean>>({})
  const acpRuntimeRequests = new Map<string, Promise<AcpagentRuntimeStatus>>()
  let selectSessionRequestId = 0

  const hasExplicitSessionSelection = computed(() => explicitSessionSelection.value)




















  async function ensureSessionSummary(targetBotId: string, targetSessionId: string, requestId?: number): Promise<SessionSummary | null> {
    const bid = targetBotId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return null
    const known = knownSessionSummary(sid)
    if (known) return known

    try {
      const fetched = await fetchSession(bid, sid)
      if (requestId !== undefined && requestId !== selectSessionRequestId) return null
      if ((currentBotId.value ?? '').trim() !== bid || (sessionId.value ?? '').trim() !== sid) return null
      rememberSession(fetched)
      return fetched
    } catch {
      return null
    }
  }




  async function cleanupFailedDeferredSession(botId: string, targetSessionId: string, fallbackComposerScope = '') {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return

    const rescopedComposerScope = rescopeSessionCommandEventToComposer(bid, sid)
    const composerScope = rescopedComposerScope || fallbackComposerScope.trim()
    markSessionDeleted(bid, sid)
    const deletedSignal: { id: string, botId: string, seq: number, composerScope?: string } = { id: sid, botId: bid, seq: ++deletedSessionSeq }
    if (composerScope) deletedSignal.composerScope = composerScope
    deletedSession.value = deletedSignal
    clearACPRuntimeStatus(bid, sid)
    if ((currentBotId.value ?? '').trim() === bid) {
      removeSessionFromList(sid)
      if ((sessionId.value ?? '').trim() === sid) {
        sessionId.value = null
        explicitSessionSelection.value = false
        draftIntent.value = true
        sessionMessagesStream.stop()
        clearHistoryView()
      }
    }

    try {
      await requestDeleteSession(bid, sid)
    } catch {
      // Best-effort cleanup: the send failure result is the user-facing error.
    }
  }





  function acpRuntimeKey(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    return bid && sid ? `${bid}:${sid}` : ''
  }

  function setACPRuntimeStatus(botId: string, targetSessionId: string, runtime: AcpagentRuntimeStatus | undefined) {
    const key = acpRuntimeKey(botId, targetSessionId)
    if (!key) return
    if (!runtime) {
      const next = { ...acpRuntimeStatuses.value }
      delete next[key]
      acpRuntimeStatuses.value = next
      return
    }
    acpRuntimeStatuses.value = {
      ...acpRuntimeStatuses.value,
      [key]: runtime,
    }
  }

  function setACPRuntimePending(botId: string, targetSessionId: string, pending: boolean) {
    const key = acpRuntimeKey(botId, targetSessionId)
    if (!key) return
    const next = { ...acpRuntimePending.value }
    if (pending) {
      next[key] = true
    } else {
      delete next[key]
    }
    acpRuntimePending.value = next
  }

  function clearACPRuntimeStatus(botId: string, targetSessionId: string) {
    setACPRuntimeStatus(botId, targetSessionId, undefined)
    setACPRuntimePending(botId, targetSessionId, false)
    acpRuntimeRequests.delete(acpRuntimeKey(botId, targetSessionId))
  }

  // Pending-ACP session staging (see ./chat/acp-staging for the generation /
  // identity-key model). Transcript and select-session invalidation are
  // injected so the staging machine never touches store internals directly.
  const acpStaging = createACPStaging({
    currentBotId,
    sessionId,
    draftIntent,
    explicitSessionSelection,
    acpRuntimeStatuses,
    acpRuntimeKey,
    setACPRuntimeStatus,
    clearACPRuntimeStatus,
    bumpSelectSessionRequest: () => {
      selectSessionRequestId++
    },
    clearTranscriptForDraft: () => {
      sessionMessagesStream.stop()
      clearHistoryView()
    },
  })
  const {
    pendingACPSessionInput,
    pendingACPRuntimeId,
    pendingACPSessionMetadata,
    pendingACPModelId,
    pendingACPRuntimeStatus,
    pendingACPRuntimeEnsuring,
    rememberDefaultACPInput,
    cachedDefaultACPInput,
    cacheDefaultACPSession,
    stageACPSession,
    stageDefaultACPSession,
    stageNewACPSession,
    resetToEmptyComposer,
    ensurePendingACPRuntime,
    setPendingACPModel,
    clearPendingACPSession,
    detachPendingACPSession,
    pendingACPMatchesInput,
  } = acpStaging

  watch(currentBotId, (newId) => {
    if (newId) {
      void initialize()
    } else {
      resetUserScopedState()
    }
  }, { immediate: true })

  onAuthSessionCleared(() => resetUserScopedState({ clearSelection: true }))



  function refreshLoadingForSession(botId: string, targetSessionId: string) {
    if (!isActiveSessionTarget(botId, targetSessionId)) return
    loading.value = isSessionStreaming(targetSessionId)
  }


  function ensureDiscussStream(streamId: string, targetSessionId?: string) {
    const id = streamIdForEvent({ stream_id: streamId, session_id: targetSessionId }, targetSessionId)
    const existing = getAssistantStream(id)
    if (existing) return existing
    if (isTerminalStream(id)) return null
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    const bid = (currentBotId.value ?? '').trim()
    const assistantTurn = createOptimisticAssistantTurn()
    appendTurnToSession(bid, sid, assistantTurn)
    void trackAssistantStream({ streamId: id, assistantTurn, botId: bid, sessionId: sid }).catch((error: Error) => {
      finalizeStreamFailure(assistantTurn, bid, sid, error)
    })
    return getAssistantStream(id)!
  }


  function handleWSSessionCreated(event: { stream_id?: string; session_id: string }, sourceBotId = '') {
    const eventSessionId = event.session_id.trim()
    if (isTerminalStream(event.stream_id)) return
    const pending = event.stream_id ? getAssistantStream(event.stream_id) : undefined
    const bid = (pending?.botId || sourceBotId || currentBotId.value || '').trim()
    if (!bid || !eventSessionId) return
    const sid = recordCreatedSession(event.stream_id, eventSessionId) || eventSessionId
    if ((currentBotId.value ?? '').trim() !== bid) return
    if (sessionId.value && sessionId.value !== sid) return

    const now = new Date().toISOString()
    if (!knownSessionSummary(sid)) {
      upsertSession({
        id: sid,
        bot_id: bid,
        type: 'chat',
        session_mode: 'chat',
        runtime_type: 'model',
        title: provisionalSessionTitle(latestOptimisticUserText()),
        created_at: now,
        updated_at: now,
      })
    }
    sessionId.value = sid
    explicitSessionSelection.value = true
    draftIntent.value = false
    userSentInSession.value = { id: sid, wasDraft: true, seq: ++userSendSeq }
  }

  function rememberStartupSendFailure(failure: Omit<StartupSendFailure, 'id'>) {
    startupSendFailure.value = {
      ...failure,
      id: nextId(),
      restoreAttachments: failure.restoreAttachments ? [...failure.restoreAttachments] : undefined,
      restoreRequestedSkills: failure.restoreRequestedSkills ? failure.restoreRequestedSkills.map(skill => ({ ...skill })) : undefined,
    }
  }

  function clearStartupSendFailure(id?: string) {
    if (!id || startupSendFailure.value?.id === id) {
      startupSendFailure.value = null
    }
  }

  function pruneEmptyAssistantTurnIfPending(streamId: string) {
    const session = getAssistantStream(streamId)
    if (!session) return
    const turn = session.assistantTurn
    if (turn.messages.length > 0) return
    removeTurnFromSession(session.botId, session.sessionId, turn)
  }

  function purgeStaleApprovalResponses() {
    const now = Date.now()
    for (const [streamId, entry] of approvalResponseStreams) {
      if (now - entry.at < APPROVAL_RESPONSE_TTL_MS) continue
      markToolApprovalDecision(entry.approvalId, 'pending')
      approvalResponseStreams.delete(streamId)
    }
  }

  function hasPendingApprovalResponse(approvalId: string) {
    purgeStaleApprovalResponses()
    for (const entry of approvalResponseStreams.values()) {
      if (entry.approvalId === approvalId) return true
    }
    return false
  }


  // Undo the optimistic decision when the response stream fails, so the user
  // can retry instead of being stuck with buttons that vanished for nothing.
  function rollbackApprovalResponse(streamId: string) {
    const approvalId = approvalResponseStreams.get(streamId)?.approvalId
    if (approvalId) markToolApprovalDecision(approvalId, 'pending')
  }

  function handleWSStreamEvent(event: UIStreamEvent, targetSessionId?: string, sourceBotId = '') {
    if (event.type === 'session_created') {
      handleWSSessionCreated(event, sourceBotId)
      return
    }
    if (event.type === 'user_message') {
      const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
      const bid = sourceBotId || currentBotId.value || ''
      const streamId = streamIdForEvent(event, sid)
      if (isTerminalStream(streamId)) return
      appendTurnToSession(bid, sid, normalizeTurn(event.data))
      const pending = getAssistantStream(streamId)
      if (pending && !messages.includes(pending.assistantTurn)) {
        appendTurnToSession(bid || pending.botId, sid || pending.sessionId, pending.assistantTurn)
      }
      return
    }
    if (event.type === 'command_result' || event.type === 'command_error') {
      const invocationId = event.invocation_id?.trim() ?? ''
      const pending = invocationId ? getAssistantStream(invocationId) : undefined
      rememberCommandEvent(event, {
        botId: pending?.botId || sourceBotId,
        sessionId: event.session_id || pending?.sessionId || targetSessionId,
        composerScope: pending?.composerScope || event.composer_scope,
      })
      if (event.type === 'command_error' && invocationId) {
        if (pending) {
          const message = event.error?.message || 'slash command failed'
          rejectAssistantStream(invocationId, new CommandStreamError(message))
          loading.value = isSessionStreaming(sessionId.value)
        }
      }
      return
    }

    const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
    const streamId = streamIdForEvent(event, sid)
    // The server may emit end after error. It must not recreate the stream, but
    // it still triggers the final authoritative refresh below.
    if (isTerminalStream(streamId) && event.type !== 'end') return

    if (approvalResponseStreams.get(streamId)?.silent) {
      if (event.type === 'end' || event.type === 'error') {
        if (event.type === 'error') {
          rollbackApprovalResponse(streamId)
          toast.error(resolveApiErrorMessage(event, event.message || 'tool approval failed'))
        }
        approvalResponseStreams.delete(streamId)
        loading.value = isSessionStreaming(sessionId.value)
      }
      return
    }

    switch (event.type) {
      case 'start':
        ensureDiscussStream(streamId, sid)
        break
      case 'message':
        const messageStream = ensureDiscussStream(streamId, sid)
        if (messageStream) upsertAssistantUIMessage(messageStream.assistantTurn, event.data)
        break
      case 'end':
        const endedSession = getAssistantStream(streamId)
        const endedBotId = endedSession?.botId ?? currentBotId.value ?? ''
        const endedSessionId = (endedSession?.sessionId || sid || '').trim()
        approvalResponseStreams.delete(streamId)
        pruneEmptyAssistantTurnIfPending(streamId)
        resolveAssistantStream(streamId)
        loading.value = isSessionStreaming(sessionId.value)
        // Only refresh when the ended stream belongs to the active session.
        // Otherwise the REST round trip lands after the user has switched
        // away and `refreshCurrentSession` drops the result anyway.
        if (
          endedSessionId
          && !isSessionStreaming(endedSessionId)
          && endedSessionId === (sessionId.value ?? '').trim()
          && endedBotId === (currentBotId.value ?? '').trim()
        ) {
          void refreshCurrentSession(endedBotId, endedSessionId)
        } else if (endedSessionId && !isSessionStreaming(endedSessionId)) {
          // Background session: skip the REST refresh, but still bump the
          // sidebar timestamp so the ended session floats to the top of the
          // list instead of remaining ordered by its last streamed delta.
          touchSessionInList(endedSessionId, new Date().toISOString())
        }
        break
      case 'error': {
        const session = getAssistantStream(streamId) ?? ensureDiscussStream(streamId, sid)
        if (!session) break
        const message = event.message || 'stream error'
        const stage: SendMessageStage = hasVisibleAssistantBlocks(session.assistantTurn) ? 'stream' : 'startup'
        rollbackApprovalResponse(streamId)
        approvalResponseStreams.delete(streamId)
        rejectAssistantStream(streamId, new StreamFailureError(message, stage))
        loading.value = isSessionStreaming(sessionId.value)
        break
      }
    }
  }

  function stopWebSocket() {
    webSocketGeneration += 1
    if (activeWs) {
      activeWs.close()
      activeWs = null
    }
  }

  function resetUserScopedState(options: { clearSelection?: boolean } = {}) {
    stopStreams()
    abortAllAssistantStreams()
    stopWebSocket()

    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
    sessionListRefreshPromise = null

    replaceSessions([])
    clearDeletedSessionIds()
    sessionsCursor.value = null
    hasMoreSessions.value = false
    loadingMoreSessions.value = false
    bots.value = []
    sessionId.value = null
    explicitSessionSelection.value = false
    if (options.clearSelection && currentBotId.value) {
      currentBotId.value = null
    }
    resetTranscriptUserScope()
    loading.value = false
    loadingChats.value = false
    initializing.value = false
    initializeRerunRequested = false
    initializingBotId = null
    initializePromise = null
    overrideModelId.value = ''
    overrideReasoningEffort.value = ''
    startupSendFailure.value = null
    resetCommandEvents()
    resetFsBeacon()
    clearPendingACPSession()

    clearStreamHistory()
    approvalResponseStreams.clear()
    forkingMessages.clear()
    backgroundTasks.clearBackgroundTasks()
  }

  function startWebSocket(targetBotId: string) {
    const bid = targetBotId.trim()
    stopWebSocket()
    if (!bid) return
    const generation = webSocketGeneration
    activeWs = connectWebSocket(bid, (event) => {
      if (generation !== webSocketGeneration) return
      handleWSStreamEvent(event, undefined, bid)
    })
  }

  function ensureWebSocket(targetBotId: string): ChatWebSocket | null {
    const bid = targetBotId.trim()
    if (!bid) return null
    if (!activeWs) {
      startWebSocket(bid)
    }
    return activeWs
  }


  function refreshSessionsList(targetBotId: string): Promise<void> {
    const bid = targetBotId.trim()
    if (!bid) return Promise.resolve()
    if (sessionListRefreshPromise?.botId === bid) return sessionListRefreshPromise.promise

    const promise = fetchSessions(bid)
      .then((response) => {
        if ((currentBotId.value ?? '').trim() !== bid) return
        replaceSessions(response.items)
        sessionsCursor.value = response.nextCursor
        hasMoreSessions.value = response.nextCursor !== null
      })
      .catch((error) => {
        console.error('Failed to refresh sessions:', error)
      })
      .finally(() => {
        if (sessionListRefreshPromise?.promise === promise) {
          sessionListRefreshPromise = null
        }
      })

    sessionListRefreshPromise = { botId: bid, promise }
    return promise
  }

  async function loadMoreSessions(): Promise<void> {
    if (!hasMoreSessions.value || loadingMoreSessions.value) return
    const bid = (currentBotId.value ?? '').trim()
    const cursor = sessionsCursor.value
    if (!bid || !cursor) return
    loadingMoreSessions.value = true
    try {
      const response = await fetchSessions(bid, { cursor })
      if ((currentBotId.value ?? '').trim() !== bid) return
      appendSessions(response.items)
      sessionsCursor.value = response.nextCursor
      hasMoreSessions.value = response.nextCursor !== null
    } catch (error) {
      console.error('Failed to load more sessions:', error)
    } finally {
      loadingMoreSessions.value = false
    }
  }

  function scheduleRefreshCurrentSession(expectedSessionId?: string, delay = 100) {
    const sid = (sessionId.value ?? '').trim()
    if (!sid) return
    if (expectedSessionId?.trim() && expectedSessionId.trim() !== sid) return
    if (refreshTimer) return

    refreshTimer = setTimeout(() => {
      refreshTimer = null
      const sidNow = (sessionId.value ?? '').trim()
      const streamActive = isSessionStreaming(sidNow)
      if (streamActive) return
      void refreshCurrentSession()
    }, delay)
  }

  function handleSessionMessageEvent(targetBotId: string, targetSessionId: string, event: SessionMessageStreamEvent) {
    if (event.type === 'ping') return
    if (event.type === 'dropped') {
      void refreshCurrentSession(targetBotId, targetSessionId)
      return
    }

    if (event.type === 'background_task') {
      const eventSessionId = event.session_id?.trim()
      if (eventSessionId && eventSessionId !== targetSessionId) return
      const task = normalizeBackgroundTask(event, event.type)
      if (!task) return
      backgroundTasks.mergeBackgroundTaskIntoMatchingTools(rememberBackgroundTask(task), messages)
      if (eventSessionId) touchSessionInList(eventSessionId)
      return
    }

    if (event.type === 'session_title_updated') {
      const sid = event.session_id.trim()
      const title = event.title.trim()
      if (!sid || !title) return
      updateKnownSessionTitle(sid, title)
      return
    }

    // message_created. Per-session SSE delivers raw messages; the server's
    // backlog handshake (per-stream backlogIDs) ensures the client sees only
    // genuinely live events post-backlog, so we don't try to dedup against
    // already-known message ids — the comparison was unsound anyway because
    // `messages` holds aggregated UI turns whose ids live in a different
    // namespace from raw bot_history_messages.id. The downstream
    // `scheduleRefreshCurrentSession` is debounced and idempotent, so an
    // occasional redundant REST round trip is cheap.
    const raw = event.message
    if (!raw) return
    const messageSessionId = String(raw.session_id ?? '').trim()
    if (messageSessionId && messageSessionId !== targetSessionId) return
    if (messageSessionId) touchSessionInList(messageSessionId, raw.created_at)
    if (!shouldRefreshFromMessageCreated(targetBotId, sessionId.value, streamingSessionId.value, event)) return
    scheduleRefreshCurrentSession(messageSessionId)
  }

  function handleBotSessionsActivityEvent(targetBotId: string, event: BotSessionActivityEvent) {
    if (event.type === 'ping') return
    if (event.type === 'dropped') {
      void refreshSessionsList(targetBotId)
      return
    }

    if (event.type === 'session_touched') {
      const sid = event.session_id.trim()
      if (!sid) return
      const touched = touchKnownSession(sid, event.updated_at)
      if (touched.source === 'listed') return
      if (touched.source === 'remembered') {
        if (touched.visibleInRecents) void refreshSessionsList(targetBotId)
        return
      }
      // Unknown session — likely created from another channel. Reload the
      // first page so it shows up in the sidebar.
      void refreshSessionsList(targetBotId)
      return
    }

    if (event.type === 'session_title_changed') {
      const sid = event.session_id.trim()
      const title = event.title.trim()
      if (!sid || !title) return
      updateKnownSessionTitle(sid, title)
      return
    }

    // session_created — server filters to user-facing types, but emits only
    // `session_id` / `title` / `created_at` (no session type, no metadata).
    // A stub with `type: undefined` would fail every consumer that branches
    // on session.type, so reload the first page instead and let the server
    // return the full summary.
    const sid = event.session_id.trim()
    if (!sid || hasListedSession(sid)) return
    void refreshSessionsList(targetBotId)
  }

  function startSessionMessagesStream(targetBotId: string, targetSessionId: string) {
    sessionMessagesStream.stop()
    const bid = targetBotId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return

    // The chat pane reads `loadingMessages` to suppress empty-state
    // placeholders (e.g. "system session has no records") while a fresh
    // transcript is on its way. The sidebar deliberately ignores it — only
    // `loadingChats` (sessions-list boot) makes the sidebar spin.
    sessionMessagesStream.start(async (signal) => {
      try {
        await loadInitialMessages(bid, sid)
        for (const stream of assistantStreamsForSession(bid, sid)) {
          if (!messages.includes(stream.assistantTurn)) {
            appendTurnToSession(bid, sid, stream.assistantTurn)
          }
        }
      } catch (error) {
        console.error('Failed to load session messages:', error)
      }
      await streamSessionMessageEvents(bid, sid, signal, (event) => {
        handleSessionMessageEvent(bid, sid, event)
      })
    })
  }

  function startBotSessionsActivityStream(targetBotId: string) {
    botSessionsActivityStream.stop()
    const bid = targetBotId.trim()
    if (!bid) return

    botSessionsActivityStream.start(async (signal) => {
      await streamBotSessionsActivityEvents(bid, signal, (event) => {
        handleBotSessionsActivityEvent(bid, event)
      })
    })
  }

  // Closes both SSE subscriptions. The per-session stream restarts on the
  // next `sessionId` change; the bot-wide stream restarts on the next
  // `initialize()` after a bot or session-token change.
  function stopStreams() {
    sessionMessagesStream.stop()
    botSessionsActivityStream.stop()
  }

  function abort() {
    const abortError = new Error('aborted')
    abortError.name = 'AbortError'
    rejectSessionStreams(sessionId.value, abortError, (streamId) => {
      if (activeWs?.connected) activeWs.abort(streamId)
    })
    loading.value = isSessionStreaming(sessionId.value)
  }

  function abortAllAssistantStreams() {
    const abortError = new Error('aborted')
    abortError.name = 'AbortError'
    approvalResponseStreams.clear()
    rejectAllStreams(abortError, (streamId) => {
      if (activeWs?.connected) activeWs.abort(streamId)
    })
    loading.value = false
  }

  async function ensureBot(): Promise<string | null> {
    try {
      const list = await fetchBots()
      bots.value = list
      if (!list.length) {
        currentBotId.value = null
        return null
      }
      if (currentBotId.value) {
        const found = list.find(bot => bot.id === currentBotId.value)
        if (found && !isPendingBot(found)) return currentBotId.value
      }
      const ready = list.find(bot => !isPendingBot(bot))
      currentBotId.value = ready ? ready.id : list[0]!.id
      return currentBotId.value
    } catch (error) {
      console.error('Failed to fetch bots:', error)
      return currentBotId.value
    }
  }

  // Re-pull the bot list without touching the current bot/session selection.
  // The store loads bots once at init and isn't wired to the settings pages'
  // query cache, so per-bot config edited in settings (enabled agents, model,
  // name…) would otherwise stay stale in the composer until a full reload.
  // currentBot is a computed over bots, so swapping the list reactively
  // refreshes the composer's agent list and metadata in place.
  async function refreshBots(): Promise<void> {
    try {
      bots.value = await fetchBots()
    } catch (error) {
      console.error('Failed to refresh bots:', error)
    }
  }


  function sessionMetadata(session: SessionSummary | null): Record<string, unknown> {
    if (!session) return {}
    return {
      ...(session.metadata && typeof session.metadata === 'object' ? session.metadata : {}),
      ...(session.runtime_metadata && typeof session.runtime_metadata === 'object' ? session.runtime_metadata : {}),
    }
  }

  const activeChatTarget = computed<ActiveChatTarget>(() => {
    const explicitSelection = explicitSessionSelection.value
    const sid = (sessionId.value ?? '').trim()
    if (sid) {
      const session = activeSession.value
      const runtimeType = session ? normalizedRuntimeType(session) : 'unknown'
      return {
        kind: 'session',
        sessionId: sid,
        session,
        runtimeType,
        isACP: runtimeType === 'acp_agent',
        isPendingACP: false,
        metadata: sessionMetadata(session),
        explicitSelection,
      }
    }

    const metadata = pendingACPSessionMetadata.value
    if (metadata) {
      return {
        kind: 'draft-acp',
        sessionId: null,
        session: null,
        runtimeType: 'acp_agent',
        isACP: true,
        isPendingACP: true,
        metadata,
        explicitSelection,
      }
    }

    return {
      kind: 'draft-native',
      sessionId: null,
      session: null,
      runtimeType: 'model',
      isACP: false,
      isPendingACP: false,
      metadata: {},
      explicitSelection,
    }
  })























  async function createACPSession(input: ACPAgentSessionInput): Promise<{ session: SessionSummary; runtime?: AcpagentRuntimeStatus }> {
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    const metadata = acpSessionMetadata(input)
    const runtimeId = input.runtimeId?.trim() ?? ''
    const sessionMode = input.sessionMode === 'discuss' ? 'discuss' : 'chat'
    // The warm staged runtime is bound server-side inside session creation;
    // no separate adopt/bind round trip and nothing for a watcher to race.
    const created = await createSession(bid, {
      title: input.title ?? '',
      type: sessionMode,
      sessionMode,
      runtimeType: 'acp_agent',
      metadata: {},
      runtimeMetadata: metadata,
      acpRuntimeId: runtimeId || undefined,
    })
    upsertSession(created)
    sessionId.value = created.id
    explicitSessionSelection.value = true
    draftIntent.value = false
    clearHistoryView()
    if (runtimeId) {
      // The staged runtime now belongs to the session — reset local staging
      // without closing it.
      pendingACPSessionInput.value = null
      pendingACPRuntimeId.value = ''
    } else {
      clearPendingACPSession()
    }
    const runtime = input.startRuntime ? await ensureACPRuntime(created.id) : undefined
    return { session: created, runtime }
  }

  async function updateCurrentSessionAgent(input: ACPAgentSessionInput): Promise<{ session: SessionSummary; runtime?: AcpagentRuntimeStatus }> {
    if (!sessionId.value) return createACPSession(input)
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value
    if (!bid) throw new Error('Bot not selected')
    const metadata = acpSessionMetadata(input)
    const sessionMode = activeSession.value?.session_mode || (activeSession.value?.type === 'discuss' ? 'discuss' : 'chat')
    const updated = await requestUpdateSessionAgent(bid, sid, {
      type: sessionMode === 'discuss' ? 'discuss' : 'acp_agent',
      sessionMode,
      runtimeType: 'acp_agent',
      metadata,
      runtimeMetadata: metadata,
    })
    upsertSession(updated)
    explicitSessionSelection.value = true
    draftIntent.value = false
    clearPendingACPSession()
    clearACPRuntimeStatus(bid, sid)
    const runtime = input.startRuntime ? await ensureACPRuntime(sid) : undefined
    return { session: updated, runtime }
  }

  async function updateCurrentSessionToMemoh(): Promise<SessionSummary | null> {
    clearPendingACPSession()
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid) return null
    const sessionMode = activeSession.value?.session_mode || (activeSession.value?.type === 'discuss' ? 'discuss' : 'chat')
    const updated = await requestUpdateSessionAgent(bid, sid, {
      type: sessionMode === 'discuss' ? 'discuss' : 'chat',
      sessionMode,
      runtimeType: 'model',
      metadata: {},
      runtimeMetadata: {},
    })
    upsertSession(updated)
    explicitSessionSelection.value = true
    draftIntent.value = false
    clearACPRuntimeStatus(bid, sid)
    return updated
  }

  async function ensureACPRuntime(sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value ?? ''
    const sid = sessionID?.trim() || sessionId.value || ''
    if (!bid || !sid) throw new Error('ACP session is not selected')
    const key = acpRuntimeKey(bid, sid)
    const existing = acpRuntimeRequests.get(key)
    if (existing) return existing

    setACPRuntimePending(bid, sid, true)
    const request = requestEnsureACPRuntime(bid, sid)
      .then((runtime) => {
        if (acpRuntimeRequests.get(key) === request) {
          setACPRuntimeStatus(bid, sid, runtime)
        }
        return runtime
      })
      .finally(() => {
        if (acpRuntimeRequests.get(key) === request) {
          acpRuntimeRequests.delete(key)
          setACPRuntimePending(bid, sid, false)
        }
      })
    acpRuntimeRequests.set(key, request)
    return request
  }

  async function setACPRuntimeModel(modelID: string, sessionID?: string): Promise<AcpagentRuntimeStatus> {
    const bid = currentBotId.value ?? ''
    const sid = sessionID?.trim() || sessionId.value || ''
    const mid = modelID.trim()
    if (!bid || !sid || !mid) throw new Error('ACP model is not selected')
    const runtime = await requestSetACPRuntimeModel(bid, sid, mid)
    setACPRuntimeStatus(bid, sid, runtime)
    return runtime
  }

  async function ensureActiveSession(firstPrompt?: string) {
    if (sessionId.value) return
    if (pendingACPSessionInput.value) {
      const detached = detachPendingACPSession()
      if (!detached) return
      const pending = detached.input
      const pendingModelId = pending.modelId?.trim() ?? ''
      const runtimeId = detached.runtimeId
      let created: SessionSummary
      try {
        ({ session: created } = await createACPSession({ ...pending, runtimeId }))
      } catch (error) {
        // Session creation failed: restore the staged agent (and keep its
        // warm runtime) so the user can simply retry.
        if (!pendingACPSessionInput.value && !sessionId.value) {
          pendingACPSessionInput.value = pending
          pendingACPRuntimeId.value = runtimeId
        }
        throw error
      }
      const bid = currentBotId.value ?? ''
      if (bid && runtimeId) {
        clearACPRuntimeStatus(bid, runtimeId)
      }
      if (pendingModelId) {
        // Bind carries the staged model with the runtime. Only when the bind
        // fell back to a cold start does the model need re-applying.
        const runtime = await ensureACPRuntime(created.id)
        const currentModelId = runtime?.models?.current_model_id?.trim() ?? ''
        if (currentModelId !== pendingModelId) {
          await setACPRuntimeModel(pendingModelId)
        }
      }
      return
    }
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    const created = await createSession(bid)
    // Show the first prompt optimistically as the title so the sidebar/tab never
    // flashes "Untitled Session" while the server's title model runs. This is a
    // LOCAL display value only — the server creates the session untitled and
    // persists the real title via the title-generation flow. Keeping the session
    // untitled server-side preserves the "title empty ⇒ needs an LLM title"
    // invariant the backend guards on (restart-safe), and the optimistic value
    // mirrors backend fallbackSessionTitle so the SSE-confirmed title lands
    // without flicker.
    if (firstPrompt?.trim()) {
      created.title = provisionalSessionTitle(firstPrompt)
    }
    upsertSession(created)
    sessionId.value = created.id
    draftIntent.value = false
    explicitSessionSelection.value = true
    clearHistoryView()
  }

  // defaultRuntimeIsACP reports whether the bot's default chat runtime is an
  // external ACP agent. It is a lightweight precheck (runtime/agent presence
  // only); full eligibility (workspace_exec, agent enabled, managed fields) is
  // still decided by the chat-pane default-ACP initializer. Used to keep the
  // history-vs-default-ACP decision inside the init chain instead of letting
  // initialize() auto-select a history session that a late watcher then drops.
  async function defaultRuntimeIsACP(botId: string): Promise<boolean> {
    const input = await defaultACPSessionInputFromSettings(botId)
    return input !== null
  }

  async function defaultACPSettingsForAgent(botId: string, agentId: string): Promise<Partial<ACPAgentSessionInput>> {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const { data } = await (getBotsByBotIdSettings as any)({ path: { bot_id: botId }, throwOnError: true })
      const settings = data as {
        chat_runtime?: string
        chat_acp_agent_id?: string | null
        chat_acp_project_path?: string | null
        chat_acp_project_mode?: string | null
      } | undefined
      if (settings?.chat_runtime !== 'acp_agent') return {}
      if ((settings.chat_acp_agent_id ?? '').trim() !== agentId) return {}
      return {
        projectPath: settings.chat_acp_project_path?.trim() || undefined,
        projectMode: settings.chat_acp_project_mode?.trim() || undefined,
      }
    } catch {
      return {}
    }
  }

  async function defaultACPSessionInputFromSettings(botId: string): Promise<ACPAgentSessionInput | null> {
    const bid = botId.trim()
    if (!bid) return null
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const { data } = await (getBotsByBotIdSettings as any)({ path: { bot_id: bid }, throwOnError: true })
      const settings = data as {
        chat_runtime?: string
        chat_acp_agent_id?: string | null
        chat_acp_project_path?: string | null
        chat_acp_project_mode?: string | null
      } | undefined
      if (settings?.chat_runtime !== 'acp_agent') {
        rememberDefaultACPInput(bid, null)
        return null
      }
      const agentId = settings.chat_acp_agent_id?.trim() ?? ''
      if (!agentId) {
        rememberDefaultACPInput(bid, null)
        return null
      }
      const input = {
        agentId,
        projectPath: settings.chat_acp_project_path?.trim() || ACP_DEFAULT_PROJECT_PATH,
        projectMode: settings.chat_acp_project_mode?.trim() || ACP_DEFAULT_PROJECT_MODE,
      }
      rememberDefaultACPInput(bid, input)
      return input
    } catch {
      return null
    }
  }


  async function stageDefaultACPFromSettings(requestId: number) {
    const bid = (currentBotId.value ?? '').trim()
    if (!bid || sessionId.value || explicitSessionSelection.value) return
    const cached = cachedDefaultACPInput(bid)
    if (cached.loaded) {
      if (cached.input && !pendingACPMatchesInput(cached.input)) stageDefaultACPSession(cached.input)
      return
    }
    const input = await defaultACPSessionInputFromSettings(bid)
    if (!input) return
    if (requestId !== selectSessionRequestId) return
    if ((currentBotId.value ?? '').trim() !== bid || sessionId.value || explicitSessionSelection.value) return
    if (pendingACPMatchesInput(input)) return
    stageDefaultACPSession(input)
  }

  async function initialize() {
    if (initializing.value) {
      const requestedBotId = (currentBotId.value ?? '').trim() || null
      if (initializingBotId && requestedBotId !== initializingBotId) {
        initializeRerunRequested = true
      }
      if (initializePromise) await initializePromise
      return
    }

    const run = (async () => {
      initializing.value = true
      loadingChats.value = true
      try {
        do {
          initializeRerunRequested = false
          initializingBotId = (currentBotId.value ?? '').trim() || null
          // Every entry into initialize starts from a clean transcript window. We
          // reset here unconditionally so the success path that hydrates
          // `sessionId` without clearing messages can't carry a stale
          // `hasLoadedOlder = true` from a previous bot into the new bot's first
          // refresh (which would take the merge branch and duplicate optimistic
          // turns).
          prepareForInitialization()
          stopStreams()
          stopWebSocket()

          const bid = await ensureBot()
          if (!bid) {
            replaceSessions([])
            sessionsCursor.value = null
            hasMoreSessions.value = false
            sessionId.value = null
            clearPendingACPSession()
            clearHistoryView()
            continue
          }
          initializingBotId = bid

          let response: Awaited<ReturnType<typeof fetchSessions>>
          let defaultIsACP = false
          try {
            ;[response, defaultIsACP] = await Promise.all([
              fetchSessions(bid),
              defaultRuntimeIsACP(bid),
            ])
          } catch (error) {
            if ((currentBotId.value ?? '').trim() !== bid) {
              initializeRerunRequested = true
              continue
            }
            throw error
          }
          if ((currentBotId.value ?? '').trim() !== bid) {
            initializeRerunRequested = true
            continue
          }

          const visibleSessions = replaceSessions(response.items)
          sessionsCursor.value = response.nextCursor
          hasMoreSessions.value = response.nextCursor !== null

          const restoredSessionId = (sessionId.value ?? '').trim()
          const restoredExplicitSession = restoredSessionId && explicitSessionSelection.value
            ? await ensureSessionSummary(bid, restoredSessionId)
            : null
          if ((currentBotId.value ?? '').trim() !== bid) {
            initializeRerunRequested = true
            continue
          }
          const preservePendingACPStage = !!pendingACPSessionInput.value && !sessionId.value
          const preserveExplicitEmptyComposer = explicitSessionSelection.value && !sessionId.value
          const preferDefaultACP = defaultIsACP
            && !preservePendingACPStage
            && !preserveExplicitEmptyComposer
            && !explicitSessionSelection.value

          if (preservePendingACPStage) {
            sessionId.value = null
            markHistoryEmpty()
          } else if (preserveExplicitEmptyComposer) {
            sessionId.value = null
            clearHistoryView()
          } else if (preferDefaultACP) {
            sessionId.value = null
            explicitSessionSelection.value = false
            draftIntent.value = false
            clearHistoryView()
          } else if (restoredExplicitSession) {
            draftIntent.value = false
          } else if (!visibleSessions.length) {
            sessionId.value = null
            explicitSessionSelection.value = false
            clearHistoryView()
          } else {
            // Keep a VALID persisted session; otherwise, if the user intentionally
            // closed down to the draft "New Session" page, keep that on reload instead
            // of force-opening a random session; otherwise pick the most recent real
            // conversation (the server sorts by recency). Skip schedule runs — they
            // are read-only execution history, so landing on a cron run when
            // switching bots would be surprising; a schedule run is reachable from
            // the sidebar's Schedule pivot.
            // Transcript hydration is driven by startSessionMessagesStream below — no
            // eager loadMessages REST round trip from here.
            if (sessionId.value && knownSessionSummary(sessionId.value)) {
              draftIntent.value = false
            } else if (draftIntent.value) {
              sessionId.value = null
              clearHistoryView()
            } else {
              const firstConversation = visibleSessions.find(s => (s.type ?? 'chat') !== 'schedule')
              sessionId.value = (firstConversation ?? visibleSessions[0]!).id
              explicitSessionSelection.value = false
            }
          }

          startWebSocket(bid)
          startBotSessionsActivityStream(bid)
          if (sessionId.value) startSessionMessagesStream(bid, sessionId.value)
        } while (initializeRerunRequested)
      } finally {
        loadingChats.value = false
        initializing.value = false
        initializingBotId = null
        initializeRerunRequested = false
        if (initializePromise === run) {
          initializePromise = null
        }
      }
    })()
    initializePromise = run
    await run
  }

  // Switching sessions is an explicit operation: stop the active SSE, blank
  // the view, restart the SSE for the new session. We do NOT use a watcher on
  // `sessionId` — a watcher fires asynchronously and races with operations
  // that mutate `messages` between the assignment and the watcher microtask
  // (e.g. an optimistic turn appended during `sendMessage` is wiped when the
  // pending watcher finally runs `clearHistoryView()`).
  //
  // We deliberately do NOT call `abortAllAssistantStreams()` here: an
  // assistant stream that started in session A keeps running server-side
  // after the user switches to B, and finalizes against A's history when
  // the user comes back (the `appendTurnToSession` / WS handlers are
  // already gated on `sessionId.value === <stream's sessionId>`, so the
  // orphan does not bleed into B's view).
  function switchActiveSession(sid: string) {
    sessionMessagesStream.stop()
    clearHistoryView()
    const bid = (currentBotId.value ?? '').trim()
    if (!bid || !sid) return
    startSessionMessagesStream(bid, sid)
  }

  async function selectBot(targetBotId: string) {
    if (currentBotId.value === targetBotId) return
    selectSessionRequestId++
    abort()
    abortAllAssistantStreams()
    clearPendingACPSession()
    clearFsForBotSwitch()
    currentBotId.value = targetBotId
    sessionId.value = null
    clearRememberedSessions()
    explicitSessionSelection.value = false
    draftIntent.value = false
    await initialize()
  }

  async function selectSession(targetSessionId: string, options: { explicitSelection?: boolean } = {}) {
    const sid = targetSessionId.trim()
    if (!sid) return
    const sameSession = sid === sessionId.value
    const requestId = ++selectSessionRequestId
    const bid = (currentBotId.value ?? '').trim()
    clearPendingACPSession()
    sessionId.value = sid
    draftIntent.value = false
    explicitSessionSelection.value = options.explicitSelection !== false
    if (!sameSession) switchActiveSession(sid)
    // Even when `sid` is already the persisted selection, a page refresh may
    // have no summary for it yet (for example an ACP session outside the first
    // sidebar page). Hydrate before consumers branch on runtime_type.
    await ensureSessionSummary(bid, sid, requestId)
  }

  async function createNewSession(options: { explicitSelection?: boolean } = {}) {
    const bid = await ensureBot()
    if (!bid) return
    resetToEmptyComposer({
      explicitSelection: options.explicitSelection === true,
      draftIntent: true,
    })
  }

  // Switch the global view to the draft (no real session yet). Unlike
  // createNewSession this assumes the bot is already active and only resets the
  // view, so per-session chat tabs can activate their draft tab without minting a
  // session. selectSession early-returns on an empty id, so a draft needs this.
  function selectDraft(options: { explicitSelection?: boolean } = {}) {
    const explicitSelection = options.explicitSelection === true
    resetToEmptyComposer({
      clearPendingACP: false,
      explicitSelection,
      draftIntent: true,
    })
    if (!explicitSelection) {
      void stageDefaultACPFromSettings(selectSessionRequestId)
    }
  }

  async function handleWebNewCommand(text: string, attachments?: ChatAttachment[]): Promise<WebNewCommandResult> {
    const parsed = parseWebNewCommand(text)
    if (!parsed) return { kind: 'none' }
    if (attachments?.length) {
      return { kind: 'error', message: 'Attachments are not supported with /new' }
    }
    const agentId = parsed.agentId.trim()
    if (!agentId) {
      if (parsed.mode === 'discuss') {
        return { kind: 'error', message: 'Discuss ACP sessions require an agent, for example /new discuss codex' }
      }
      await createNewSession({ explicitSelection: true })
      return { kind: 'handled' }
    }
    if (agentId !== 'codex' && agentId !== 'claude-code') {
      return { kind: 'error', message: `Unknown ACP agent "${agentId}"` }
    }
    const bid = await ensureBot()
    if (!bid) return { kind: 'error', message: 'Bot not ready' }
    const defaults = await defaultACPSettingsForAgent(bid, agentId)
    stageNewACPSession({
      agentId,
      sessionMode: parsed.mode === 'discuss' ? 'discuss' : 'chat',
      ...defaults,
    })
    return { kind: 'handled' }
  }

  function isWebSlashInput(text: string): boolean {
    return text.trim().startsWith('/')
  }

  function quickActionIDForSlash(text: string): string {
    const parts = text.trim().split(/\s+/)
    const command = parts[0]?.toLowerCase() ?? ''
    const action = parts[1]?.toLowerCase() ?? ''
    if (command === '/help' && !action) return 'help'
    if (command === '/skill' && (!action || action === 'list')) return 'skill.list'
    return ''
  }

  async function handleWebSlashCommand(text: string, hasRequestedSkills = false, composerScope = 'chat'): Promise<WebSlashCommandResult> {
    if (!isWebSlashInput(text) || hasRequestedSkills) return { kind: 'none' }
    const bid = currentBotId.value ?? ''
    if (!bid) return { kind: 'error', message: 'Bot not selected' }
    const sid = sessionId.value ?? ''
    const scope = composerScope.trim() || 'chat'

    const actionID = quickActionIDForSlash(text)
    if (!actionID) return { kind: 'none' }
    const skillActivationAllowed = !activeChatTarget.value.isACP
    let event: CommandEventResponse | null
    try {
      event = await executeQuickAction(bid, actionID, {
        invocationId: createStreamId(),
        composerScope: scope,
        sessionId: sid || undefined,
        skillActivationAllowed,
      })
    } catch (error) {
      const message = resolveApiErrorMessage(error, commandErrorMessage('generic'))
      showCommandError('generic', message, {
        botId: bid,
        sessionId: sid || undefined,
        composerScope: scope,
      })
      return { kind: 'error', message }
    }

    if (!event) return { kind: 'none' }
    rememberCommandEvent(event, { botId: bid, sessionId: sid || undefined, composerScope: scope })
    if (event.type === 'command_error') {
      return { kind: 'error', message: event.error?.message || commandErrorMessage('generic') }
    }
    return { kind: 'handled' }
  }

  async function removeSession(targetSessionId: string, options: { fallbackMode?: SidebarSessionMode } = {}) {
    const delId = targetSessionId.trim()
    if (!delId) return
    const bid = currentBotId.value ?? ''
    if (!bid) throw new Error('Bot not selected')
    await requestDeleteSession(bid, delId)
    markSessionDeleted(bid, delId)
    deletedSession.value = { id: delId, botId: bid, seq: ++deletedSessionSeq }
    if ((currentBotId.value ?? '').trim() !== bid) return
    clearACPRuntimeStatus(bid, delId)
    removeSessionFromList(delId)
    if (sessionId.value !== delId) return
    const fallbackMode = options.fallbackMode ?? 'recent'
    const nextSession = fallbackSessionAfterDelete(fallbackMode)
    if (!nextSession) {
      sessionId.value = null
      explicitSessionSelection.value = false
      draftIntent.value = false
      sessionMessagesStream.stop()
      clearHistoryView()
      return
    }
    const next = nextSession.id
    sessionId.value = next
    explicitSessionSelection.value = false
    draftIntent.value = false
    switchActiveSession(next)
  }

  async function renameSession(targetSessionId: string, title: string): Promise<SessionSummary> {
    const sid = targetSessionId.trim()
    const nextTitle = title.trim()
    if (!sid) throw new Error('Session not selected')
    const bid = currentBotId.value ?? ''
    if (!bid) throw new Error('Bot not selected')
    const updated = await requestUpdateSessionTitle(bid, sid, nextTitle)
    const patch: Partial<SessionSummary> = { title: updated.title ?? nextTitle }
    if (updated.updated_at) patch.updated_at = updated.updated_at
    patchSessionInList(sid, patch)
    return updated
  }

  async function forkMessage(messageId: string, options: { title?: string } = {}): Promise<boolean> {
    const bid = (currentBotId.value ?? '').trim()
    const sid = (sessionId.value ?? '').trim()
    const mid = messageId.trim()
    if (!bid || !sid || !mid || activeChatReadOnly.value || !activeChatCanFork.value || streaming.value || loadingMessages.value) return false

    const key = `${bid}:${sid}:${mid}`
    if (forkingMessages.has(key)) return false
    forkingMessages.add(key)
    try {
      const forked = await requestForkSessionFromMessage(bid, sid, mid, { title: options.title })
      if ((currentBotId.value ?? '').trim() !== bid || (sessionId.value ?? '').trim() !== sid) {
        void refreshSessionsList(bid)
        return true
      }

      upsertSession(forked)
      rememberSession(forked)
      void refreshSessionsList(bid)

      const turns = await fetchSessionWindow(bid, forked.id)
      const anchoredForked = withForkAnchorFromUITurns(forked, turns)
      if (anchoredForked !== forked) {
        upsertSession(anchoredForked)
        rememberSession(anchoredForked)
      }
      if ((currentBotId.value ?? '').trim() !== bid || (sessionId.value ?? '').trim() !== sid) {
        return true
      }

      selectSessionRequestId++
      clearPendingACPSession()
      sessionMessagesStream.stop()
      sessionId.value = forked.id
      explicitSessionSelection.value = true
      draftIntent.value = false
      replaceHistoryView(turns, forked.id)
      startSessionMessagesStream(bid, forked.id)
      return true
    } catch (error) {
      toast.error(resolveApiErrorMessage(error, forkFailedMessage()))
      return false
    } finally {
      forkingMessages.delete(key)
    }
  }

  async function sendMessage(text: string, attachments?: ChatAttachment[], options: SendMessageOptions = {}): Promise<SendMessageResult> {
    const trimmed = text.trim()
    const requestedSkills = normalizeRequestedSkills(options.requestedSkills)
    const composerScope = options.composerScope?.trim() || 'chat'
    if (!trimmed && !attachments?.length && requestedSkills.length === 0) return { ok: false, stage: 'startup' }

    if (requestedSkills.length > 0 && isWebSlashInput(trimmed)) {
      const message = commandErrorMessage('invalid_skill_slash_syntax')
      showCommandError('invalid_skill_slash_syntax', message, currentCommandScope(composerScope))
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }

    if (isWebSlashInput(trimmed) && attachments?.length) {
      const message = commandErrorMessage('slash_attachments_unsupported')
      showCommandError('slash_attachments_unsupported', message, currentCommandScope(composerScope))
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }

    const newCommand = await handleWebNewCommand(trimmed, attachments)
    if (newCommand.kind === 'handled') return { ok: true }
    if (newCommand.kind === 'error') {
      return { ok: false, stage: 'startup', error: newCommand.message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }
    const slashCommand = await handleWebSlashCommand(trimmed, requestedSkills.length > 0, composerScope)
    if (slashCommand.kind === 'handled') return { ok: true }
    if (slashCommand.kind === 'error') {
      return { ok: false, stage: 'startup', error: slashCommand.message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }
    clearCommandEvent(currentCommandScope(composerScope))
    if (streaming.value || loadingMessages.value || !currentBotId.value) return { ok: false, stage: 'startup' }

    let assistantTurn: ChatAssistantTurn | null = null
    let userTurn: ChatUserTurn | null = null
    let sendBotId = ''
    let sendSessionId = ''
    let sendStreamId = ''
    let turnAppendStarted = false

    const wasDraft = !sessionId.value
    const serverSlashActivation = isWebSlashInput(trimmed) && quickActionIDForSlash(trimmed) === ''
    const serverSkillActivation = requestedSkills.length > 0 || serverSlashActivation
    if (serverSkillActivation && !sessionId.value && pendingACPSessionInput.value) {
      const message = commandErrorMessage('unsupported_skill_slash_context')
      showCommandError('unsupported_skill_slash_context', message, currentCommandScope(composerScope))
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills), composerScope }
    }

    loading.value = true
    const deferSessionCreation = serverSkillActivation && wasDraft
    try {
      if (!deferSessionCreation) {
        await ensureActiveSession(wasDraft ? trimmed : undefined)
      }

      const bid = currentBotId.value!
      const sid = sessionId.value ?? ''
      if (!sid && !deferSessionCreation) throw new Error('Session not selected')
      sendBotId = bid
      sendSessionId = sid
      sendStreamId = createStreamId()
      // Tell the tab store to pin (and, for a draft, repoint) this session's tab.
      if (sid) {
        userSentInSession.value = { id: sid, wasDraft, seq: ++userSendSeq }
      }

      assistantTurn = createOptimisticAssistantTurn()
      turnAppendStarted = true
      options.onBeforeTurnAppend?.()
      if (!serverSkillActivation) {
        userTurn = createOptimisticUserTurn(trimmed, attachments)
        appendToView(userTurn, assistantTurn)
      }

      const modelId = overrideModelId.value || undefined
      const effort = overrideReasoningEffort.value
      const reasoningEffort = effort || undefined

      const ws = ensureWebSocket(bid)
      if (ws) {
        if (!ws.connected) {
          throw new StreamFailureError('WebSocket is not connected', 'startup')
        }
        const completion = trackAssistantStream({
          streamId: sendStreamId,
          assistantTurn,
          botId: bid,
          sessionId: sid,
          composerScope,
        })
        ws.send({
          type: 'message',
          stream_id: sendStreamId,
          invocation_id: sendStreamId,
          composer_scope: composerScope,
          text: trimmed,
          session_id: sid || undefined,
          attachments,
          requested_skills: requestedSkills.length ? requestedSkillRequestsForWire(requestedSkills) : undefined,
          model_id: modelId,
          reasoning_effort: reasoningEffort,
        })
        await completion
        const createdSessionId = createdSessionIdForStream(sendStreamId)
        const fallbackActiveSessionId = (currentBotId.value ?? '').trim() === bid ? sessionId.value ?? '' : ''
        const refreshSessionId = sendSessionId || createdSessionId || fallbackActiveSessionId
        forgetCreatedSession(sendStreamId)
        if (refreshSessionId) await refreshCurrentSession(bid, refreshSessionId)
      } else {
        if (serverSkillActivation) throw new StreamFailureError('WebSocket is required for skill activation', 'startup')
        await sendLocalChannelMessage(bid, trimmed, attachments, { modelId, reasoningEffort })
        await refreshCurrentSession(bid, sid)
      }

      loading.value = false
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const isAbort = err.name === 'AbortError'
      const isCommandError = err instanceof CommandStreamError
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const stage: SendMessageStage = err instanceof StreamFailureError
        ? err.stage
        : (assistantTurn && hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      const createdSessionId = sendStreamId ? createdSessionIdForStream(sendStreamId) : ''
      const bid = sendBotId || currentBotId.value || ''
      const sid = sendSessionId || (deferSessionCreation ? '' : sessionId.value || '')

      if (assistantTurn) finalizeStreamFailure(assistantTurn, bid, sid, err)
      if (!isAbort && stage === 'startup' && userTurn) {
        removeTurnFromSession(bid, sid, userTurn)
      }
      if (!isAbort && stage === 'startup' && deferSessionCreation && wasDraft && createdSessionId) {
        await cleanupFailedDeferredSession(bid, createdSessionId, composerScope)
      }

      if (sendStreamId) discardAssistantStream(sendStreamId)
      if (sendStreamId) forgetCreatedSession(sendStreamId)
      loading.value = false

      if (!isAbort && stage === 'startup' && turnAppendStarted) {
        options.onTurnAppendAborted?.()
      }

      if (isAbort) return { ok: false, stage: 'stream', error: reason }
      if (stage === 'startup') {
        const currentBid = (currentBotId.value ?? '').trim()
        const currentSid = (sessionId.value ?? '').trim()
        const stillCurrent = currentBid === bid && (!sid || currentSid === sid)
        const deferredDraftStillCurrent = !(deferSessionCreation && wasDraft && currentSid)
        const commandErrorRestoredDraft = isCommandError && deferSessionCreation && wasDraft && !currentSid
        if (stillCurrent && deferredDraftStillCurrent && (!isCommandError || commandErrorRestoredDraft)) {
          rememberStartupSendFailure({ botId: bid, sessionId: sid, composerScope, error: reason, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) })
        }
        return { ok: false, stage, error: reason, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills), composerScope }
      }
      return { ok: false, stage, error: reason }
    }
  }

  function latestVisibleUserTurn(): ChatUserTurn | null {
    for (let i = messages.length - 1; i >= 0; i--) {
      const turn = messages[i]
      if (turn.role === 'user' && !turn.__optimistic) return turn
    }
    return null
  }

  function latestVisibleAssistantTurn(): ChatAssistantTurn | null {
    for (let i = messages.length - 1; i >= 0; i--) {
      const turn = messages[i]
      if (turn.role === 'assistant' && !turn.__optimistic) return turn
    }
    return null
  }

  function isLatestVisibleUserTurn(turn: ChatMessage): turn is ChatUserTurn {
    if (turn.role !== 'user') return false
    const latest = latestVisibleUserTurn()
    return Boolean(latest && serverMessageId(latest) === serverMessageId(turn))
  }

  function isLatestVisibleAssistantTurn(turn: ChatMessage): turn is ChatAssistantTurn {
    if (turn.role !== 'assistant') return false
    const latest = latestVisibleAssistantTurn()
    return Boolean(latest && serverMessageId(latest) === serverMessageId(turn))
  }

  async function retryLatestAssistant(messageId: string): Promise<SendMessageResult> {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    const targetID = messageId.trim()
    if (!bid || !sid || !targetID) return { ok: false, stage: 'startup' }
    if (streaming.value || loadingMessages.value) return { ok: false, stage: 'startup' }
    const target = messages.find(turn => serverMessageId(turn) === targetID)
    if (!target || !isLatestVisibleAssistantTurn(target)) return { ok: false, stage: 'startup' }

    const streamId = createStreamId()
    const assistantTurn = createOptimisticAssistantTurn()
    const restoreForkAnchor = updateForkAnchorForReplacedMessage(sid, target)
    const replacedTurns = replaceTailFromTurn(target, [assistantTurn])
    loading.value = true
    try {
      const ws = ensureWebSocket(bid)
      if (!ws?.connected) {
        throw new StreamFailureError('WebSocket is not connected', 'startup')
      }
      const completion = trackAssistantStream({ streamId, assistantTurn, botId: bid, sessionId: sid })
      ws.send({
        type: 'retry_message',
        stream_id: streamId,
        session_id: sid,
        message_id: targetID,
        model_id: overrideModelId.value || undefined,
        reasoning_effort: overrideReasoningEffort.value || undefined,
      })
      await completion
      await refreshCurrentSession(bid, sid)
      refreshLoadingForSession(bid, sid)
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const stage: SendMessageStage = err instanceof StreamFailureError
        ? err.stage
        : (hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      discardAssistantStream(streamId)
      if (stage === 'startup') {
        restoreForkAnchor?.()
        restoreTailFromOptimistic(bid, sid, null, assistantTurn, replacedTurns)
      } else {
        finalizeStreamFailure(assistantTurn, bid, sid, err)
      }
      refreshLoadingForSession(bid, sid)
      return { ok: false, stage, error: reason }
    }
  }

  async function editLatestUser(messageId: string, text: string): Promise<SendMessageResult> {
    const trimmed = text.trim()
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    const targetID = messageId.trim()
    if (!bid || !sid || !targetID || !trimmed) return { ok: false, stage: 'startup' }
    if (streaming.value || loadingMessages.value) return { ok: false, stage: 'startup' }
    const target = messages.find(turn => serverMessageId(turn) === targetID)
    if (!target || !isLatestVisibleUserTurn(target)) return { ok: false, stage: 'startup' }
    if (hasUserAttachments(target)) return { ok: false, stage: 'startup' }

    const streamId = createStreamId()
    const userTurn = createOptimisticUserTurn(trimmed)
    const assistantTurn = createOptimisticAssistantTurn()
    const restoreForkAnchor = updateForkAnchorForReplacedMessage(sid, target)
    const replacedTurns = replaceTailFromTurn(target, [userTurn, assistantTurn])
    loading.value = true
    try {
      const ws = ensureWebSocket(bid)
      if (!ws?.connected) {
        throw new StreamFailureError('WebSocket is not connected', 'startup')
      }
      const completion = trackAssistantStream({ streamId, assistantTurn, botId: bid, sessionId: sid })
      ws.send({
        type: 'edit_message',
        stream_id: streamId,
        session_id: sid,
        message_id: targetID,
        text: trimmed,
        model_id: overrideModelId.value || undefined,
        reasoning_effort: overrideReasoningEffort.value || undefined,
      })
      await completion
      await refreshCurrentSession(bid, sid)
      refreshLoadingForSession(bid, sid)
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const stage: SendMessageStage = err instanceof StreamFailureError
        ? err.stage
        : (hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      discardAssistantStream(streamId)
      if (stage === 'startup') {
        restoreForkAnchor?.()
        restoreTailFromOptimistic(bid, sid, userTurn, assistantTurn, replacedTurns)
      } else {
        finalizeStreamFailure(assistantTurn, bid, sid, err)
      }
      refreshLoadingForSession(bid, sid)
      return { ok: false, stage, error: reason, restoreInput: text }
    }
  }

  async function respondToolApproval(approval: UIToolApproval, decision: 'approve' | 'reject') {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    const approvalId = approval.approval_id?.trim()
    if (!bid || !sid || !approvalId) return false
    if (approval.status !== 'pending' || approval.can_approve === false) return false
    if (hasPendingApprovalResponse(approvalId)) return false
    const ws = ensureWebSocket(bid)
    if (!ws?.connected) {
      toast.error(userInputConnectionLostMessage())
      return false
    }
    const streamId = createStreamId()
    const silent = isSessionStreaming(sid)
    approvalResponseStreams.set(streamId, { approvalId, silent, at: Date.now() })
    const previousApprovalStates = snapshotToolApprovalStates(approvalId)
    let assistantTurn: ChatAssistantTurn | null = null
    if (!silent) {
      assistantTurn = createOptimisticAssistantTurn()
      appendToView(assistantTurn)
      void trackAssistantStream({ streamId, assistantTurn, botId: bid, sessionId: sid }).catch((error: Error) => {
        finalizeStreamFailure(assistantTurn, bid, sid, error)
      })
      loading.value = true
    }
    // Optimistically update the approved/rejected tool block before the
    // server snapshot arrives so the buttons disappear immediately.
    markToolApprovalDecision(approvalId, decision === 'approve' ? 'approved' : 'rejected')
    try {
      ws.send({
        type: 'tool_approval_response',
        stream_id: streamId,
        session_id: sid,
        approval_id: approvalId,
        short_id: approval.short_id,
        decision,
      })
    } catch (error) {
      restoreToolApprovalStates(previousApprovalStates)
      approvalResponseStreams.delete(streamId)
      if (!silent) {
        discardAssistantStream(streamId)
        if (assistantTurn) removeFromView(assistantTurn)
      }
      loading.value = false
      toast.error(resolveApiErrorMessage(error, 'Failed to send tool approval response.'))
      return false
    }
    return true
  }

  async function respondUserInput(
    userInput: UIUserInput,
    payload: { answers?: WSUserInputAnswer[]; canceled?: boolean; reason?: string },
  ) {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid || !userInput.user_input_id) return
    const ws = ensureWebSocket(bid)
    if (!ws?.connected) {
      toast.error(userInputConnectionLostMessage())
      return
    }
    const streamId = createStreamId()
    const previousUserInputStates = snapshotUserInputStates(userInput.user_input_id)
    const assistantTurn = createOptimisticAssistantTurn()
    appendToView(assistantTurn)
    void trackAssistantStream({ streamId, assistantTurn, botId: bid, sessionId: sid }).catch((error: Error) => {
      finalizeStreamFailure(assistantTurn, bid, sid, error)
      // While the main session stream is still active a refresh would
      // clobber its in-flight state; roll back and let its end refresh
      // bring truth.
      if (isSessionStreaming(sid)) {
        restoreUserInputStates(previousUserInputStates)
        return
      }
      void refreshCurrentSession(bid, sid).catch(() => {
        restoreUserInputStates(previousUserInputStates)
      })
    })
    loading.value = true

    const status = payload.canceled ? 'canceled' : 'submitted'
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type === 'tool' && block.userInput?.user_input_id === userInput.user_input_id) {
          block.userInput = {
            ...block.userInput,
            status,
            can_respond: false,
          }
        }
      }
    }

    try {
      ws.send({
        type: 'user_input_response',
        stream_id: streamId,
        session_id: sid,
        user_input_id: userInput.user_input_id,
        short_id: userInput.short_id,
        answers: payload.answers,
        canceled: payload.canceled === true,
        reason: payload.reason,
      })
    } catch (error) {
      restoreUserInputStates(previousUserInputStates)
      discardAssistantStream(streamId)
      removeFromView(assistantTurn)
      loading.value = false
      toast.error(resolveApiErrorMessage(error, 'Failed to send user input response.'))
    }
  }

  return {
    messages,
    streaming,
    streamingSessionId,
    sessions,
    sessionsCursor,
    hasMoreSessions,
    loadingMoreSessions,
    loadMoreSessions,
    acpRuntimeStatuses,
    acpRuntimePending,
    pendingACPSessionInput,
    pendingACPSessionMetadata,
    pendingACPModelId,
    pendingACPRuntimeId,
    pendingACPRuntimeStatus,
    pendingACPRuntimeEnsuring,
    sessionId,
    hasExplicitSessionSelection,
    currentBotId,
    bots,
    activeSession,
    activeChatTarget,
    activeChatReadOnly,
    activeChatCanFork,
    knownSessions,
    knownSessionSummary,
    isSessionStreaming,
    loading,
    loadingChats,
    loadingMessages,
    loadingOlder,
    hasMoreOlder,
    // Exposed for tests only — do not branch on this in components. The
    // leading underscore reflects the test-only contract at the call site.
    _hasLoadedOlder: hasLoadedOlder,
    overrideModelId,
    overrideReasoningEffort,
    startupSendFailure,
    commandEvent,
    commandEventForScope,
    showCommandError,
    fsChangedAt,
    markFsChanged,
    affectsPath,
    fsEventForPath,
    initialize,
    refreshBots,
    selectBot,
    selectSession,
    stageACPSession,
    stageDefaultACPSession,
    cacheDefaultACPSession,
    resetToEmptyComposer,
    ensurePendingACPRuntime,
    setPendingACPModel,
    clearPendingACPSession,
    createACPSession,
    updateCurrentSessionAgent,
    updateCurrentSessionToMemoh,
    acpRuntimeKey,
    ensureACPRuntime,
    setACPRuntimeModel,
    createNewSession,
    selectDraft,
    userSentInSession,
    deletedSession,
    removeSession,
    renameSession,
    forkMessage,
    sendMessage,
    retryLatestAssistant,
    editLatestUser,
    respondToolApproval,
    respondUserInput,
    loadOlderMessages,
    findMessageIdByExternalId,
    locateMessageByExternalId,
    clearStartupSendFailure,
    clearCommandEvent,
    abort,
  }
})
