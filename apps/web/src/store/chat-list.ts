import { defineStore, storeToRefs } from 'pinia'
import { computed, onScopeDispose, reactive, ref, watch } from 'vue'
import { toast } from '@felinic/ui'
import enMessages from '@/i18n/locales/en.json'
import zhMessages from '@/i18n/locales/zh.json'
import jaMessages from '@/i18n/locales/ja.json'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { parseMemohError, resolveApiErrorMessage } from '@/utils/api-error'
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
  serverMessageId,
} from './chat-list.normalize'
import { createFsChangeBeacon } from './chat/fs-beacon'
import { createCommandEventRegistry } from './chat/command-events'
import { createSessionList } from './chat/session-list'
import {
  acpSessionMetadata,
  createACPStaging,
  type DetachedACPSession,
} from './chat/acp-staging'
import {
  createChatViewRegistry,
  type ChatViewEntry,
} from './chat/view-registry'
import { createAssistantStreamRegistry, type AssistantStream } from './chat/assistant-streams'
import { createChatRealtimeController } from './chat/realtime'
import { createACPRuntimeRegistry } from './chat/acp-runtime-registry'
import { createSessionListRefresh } from './chat/session-list-refresh'
import {
  createRuntimeSessionStore,
  type RuntimeSessionQueueHandle,
} from './chat/sync/runtime-session'
import { createPersistedHistoryReconciler } from './chat/sync/history'
import {
  createSynchronizedTranscript,
  disposeSynchronizedTranscript,
  projectRuntimeMessage,
  projectRuntimeMessages,
  promoteSynchronizedTranscript,
  readSynchronizedTranscript,
  type SynchronizedTranscript,
  type SynchronizedTranscriptView,
} from './chat/sync/projection'
import {
  findSettledAssistant,
  settleRuntimeTerminal,
  type RuntimeTerminalStatus,
} from './chat/sync/settle'
import {
  createChatIntents,
  type IntentPlaceholderRequest,
  type PendingChatIntent,
  type RestoreComposerDraftRequest,
  type ChatIntentWireMessage,
} from './chat/sync/intents'
import {
  createRuntimeSubscriptionController,
  type RuntimeSubscriptionRef,
} from './chat/sync/subscription'
import {
  createApprovalResponseTracker,
  type ApprovalResponse,
  type ApprovalResponseOutcome,
} from './chat/approval-responses'
import {
  createBackgroundTaskTracker,
  normalizeBackgroundTask,
} from './chat/background-tasks'
import type {
  ACPAgentSessionInput,
  ActiveChatTarget,
  ChatAssistantTurn,
  ChatMessage,
  ChatRunState,
  ChatUserTurn,
  ChatWorkspaceTargetSelectionSource,
  ChatWorkspaceTargetSnapshot,
  ChatViewTarget,
  SendMessageOptions,
  SendMessageResult,
  SendMessageStage,
} from './chat/types'
import {
  createSession,
  deleteSession as requestDeleteSession,
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
  type UIMessage,
  type UIRowIdentity,
  type UIRuntimeStateEvent,
  type UIRuntimeDeltaEvent,
  type UIToolApproval,
  type UIUserInput,
  type RequestedSkillSelection,
  type WSUserInputAnswer,
  type UIStreamEvent,
  executeQuickAction,
  fetchBots,
  fetchMessagesUI,
  locateMessageUI,
} from '@/composables/api/useChat'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH } from '@/utils/acp'
import { isGuiToolName } from '@/utils/gui-tools'
import { getBotsByBotIdSettings } from '@memohai/sdk'
import {
  isSessionRuntimeActiveStatus as isRuntimeActiveStatus,
  isSessionRuntimeTerminalStatus as isRuntimeTerminalStatus,
} from '@memohai/sdk/session-runtime'
import type {
  ConversationUiMessage,
  ConversationUiTurn,
  SessionruntimeRunOperationView,
  SessionruntimeSnapshot,
} from '@memohai/sdk'

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
  ChatWorkspaceTargetSnapshot,
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
export type { ChatViewEntry, ChatViewTarget } from './chat/view-registry'

const RUNTIME_ADMISSION_TIMEOUT_MS = 30_000
const RUNTIME_ABORT_WATCHDOG_MS = 30_000

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

function responseAbortedMessage() {
  const messages = localizedMessages()
  return messages.chat.responseAborted
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

interface PreparedRuntimeReplacement {
  kind: 'retry' | 'edit'
  target: ChatMessage
  optimisticUserTurn: ChatUserTurn | null
}

type PendingAssistantStream = AssistantStream
type StoreChatViewEntry = ChatViewEntry<SynchronizedTranscript>
type PublicStoreChatViewEntry = Omit<StoreChatViewEntry, 'transcript'> & {
  readonly transcript: SynchronizedTranscriptView
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

interface ComposerDraftRestore extends RestoreComposerDraftRequest {
  seq: number
}

interface GuiToolUseRequest {
  botId: string
  sessionId: string
  toolCallId: string
  toolName: string
  seq: number
}

class StreamFailureError extends Error {
  stage: SendMessageStage
  feedback?: unknown

  constructor(message: string, stage: SendMessageStage, feedback?: unknown) {
    super(message)
    this.name = 'StreamFailureError'
    this.stage = stage
    this.feedback = feedback
  }
}

class RuntimeAbortError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AbortError'
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
  const acpRuntimeRegistry = createACPRuntimeRegistry({ currentBotId, sessionId })
  const {
    acpRuntimeStatuses,
    acpRuntimePending,
    acpRuntimeKey,
    clearACPRuntimeStatus,
    ensureACPRuntime,
    setACPRuntimeModel,
    setACPRuntimeReasoning,
    resetACPRuntimeRegistry,
  } = acpRuntimeRegistry

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
    mergeBackgroundTaskIntoMatchingTools,
  } = backgroundTasks
  const guiToolUseRequested = ref<GuiToolUseRequest | null>(null)
  const seenGuiToolCalls = new Set<string>()
  let guiToolUseRequestSeq = 0
  const focusedChatViewId = ref('chat')
  let transcriptSnapshotHook: (view: StoreChatViewEntry, targetSessionId: string | undefined, turns: import('@/composables/api/useChat.types').UITurn[]) => void = () => {}
  let transcriptRefreshAppliedHook: (view: StoreChatViewEntry, targetSessionId: string, latestTimestamp?: string) => void = () => {}
  let sessionStreamingProbe: (botId: string, targetSessionId: string) => boolean = () => false
  let stopEvictedSessionStream: (botId: string, targetSessionId: string) => void = () => {}
  let discardEvictedDraftStage: (view: StoreChatViewEntry) => void = () => {}
  const chatViews = createChatViewRegistry({
    createTranscript: (target, hooks) => createSynchronizedTranscript(target, {
      rememberBackgroundTask,
      applyPendingBackgroundEventsToTool,
      mergeBackgroundTaskIntoMatchingTools,
      bumpFsChangedAtIfFsMutation,
      fetchMessages: fetchMessagesUI,
      locateMessage: locateMessageUI,
    }, hooks),
    onPromoteTranscript: promoteSynchronizedTranscript,
    onDisposeTranscript: disposeSynchronizedTranscript,
    isSessionStreaming: (botId, targetSessionId) => sessionStreamingProbe(botId, targetSessionId),
    onSnapshot: (view, targetSessionId, turns) => transcriptSnapshotHook(view, targetSessionId, turns),
    onRefreshApplied: (view, targetSessionId, latestTimestamp) => {
      transcriptRefreshAppliedHook(view, targetSessionId, latestTimestamp)
    },
    onEvict: (view) => {
      if (view.kind === 'session' && view.sessionId) {
        stopEvictedSessionStream(view.botId, view.sessionId)
      } else if (view.kind === 'draft') {
        discardEvictedDraftStage(view)
      }
    },
  })
  function normalizedChatViewTarget(target?: Partial<ChatViewTarget>): ChatViewTarget {
    const botId = (target?.botId ?? currentBotId.value ?? '').trim() || '__unbound__'
    const targetSessionId = target && 'sessionId' in target
      ? target.sessionId?.trim() || null
      : sessionId.value?.trim() || null
    const viewId = target?.viewId?.trim() || focusedChatViewId.value.trim() || 'chat'
    return { botId, sessionId: targetSessionId, viewId }
  }

  function isFocusedChatTarget(target: ChatViewTarget): boolean {
    const resolved = normalizedChatViewTarget(target)
    if (
      resolved.botId !== (currentBotId.value ?? '').trim()
      || resolved.viewId !== focusedChatViewId.value
    ) return false
    const selectedSessionId = (sessionId.value ?? '').trim()
    return resolved.sessionId
      ? selectedSessionId === resolved.sessionId
      : !selectedSessionId
  }

  const draftSessionCreations = reactive(new Set<string>())

  function draftSessionCreationKey(target: ChatViewTarget): string {
    const resolved = normalizedChatViewTarget(target)
    return `${resolved.botId}\u0000${resolved.viewId}`
  }

  function isChatViewCreatingSession(target: ChatViewTarget): boolean {
    const resolved = normalizedChatViewTarget(target)
    return !resolved.sessionId && draftSessionCreations.has(draftSessionCreationKey(resolved))
  }

  function chatView(target?: Partial<ChatViewTarget>): StoreChatViewEntry {
    // KeepAlive panes cache this lookup. Eviction changes the registry map
    // without changing their target props, so the revision is the invalidation
    // edge that makes a reactivated pane bind the newly-created transcript.
    void chatViews.revision.value
    return chatViews.getOrCreate(normalizedChatViewTarget(target))
  }

  const publicChatViewEntries = new WeakMap<StoreChatViewEntry, PublicStoreChatViewEntry>()
  function publicChatViewEntry(entry: StoreChatViewEntry): PublicStoreChatViewEntry {
    const existing = publicChatViewEntries.get(entry)
    if (existing) return existing
    const transcript = readSynchronizedTranscript(entry.transcript)
    const view = new Proxy(entry, {
      get(source, property, receiver) {
        if (property === 'transcript') return transcript
        return Reflect.get(source, property, receiver)
      },
      set() {
        return false
      },
    }) as PublicStoreChatViewEntry
    publicChatViewEntries.set(entry, view)
    return view
  }

  function publicChatView(target?: Partial<ChatViewTarget>): PublicStoreChatViewEntry {
    return publicChatViewEntry(chatView(target))
  }

  function workspaceTargetSelectionFor(target?: ChatViewTarget) {
    const view = chatView(target)
    return {
      targetId: view.workspaceTargetId.value,
      snapshot: view.workspaceTargetSnapshot.value,
      source: view.workspaceTargetSelectionSource.value,
    }
  }

  function setWorkspaceTargetSelection(
    target: ChatViewTarget,
    targetId: string,
    snapshot: ChatWorkspaceTargetSnapshot | null = null,
    source: ChatWorkspaceTargetSelectionSource = 'user',
  ) {
    const id = targetId.trim()
    if (!id) return
    const view = chatView(target)
    view.workspaceTargetId.value = id
    view.workspaceTargetSnapshot.value = snapshot ? { ...snapshot, target_id: id } : null
    view.workspaceTargetSelectionSource.value = source
  }

  function initializeWorkspaceTargetSelection(
    target: ChatViewTarget,
    targetId: string,
    snapshot: ChatWorkspaceTargetSnapshot | null,
    source: Extract<ChatWorkspaceTargetSelectionSource, 'default' | 'session'>,
  ) {
    const id = targetId.trim()
    if (!id) return
    const view = chatView(target)
    const currentSource = view.workspaceTargetSelectionSource.value
    if (currentSource === 'user') return
    if (source === 'default' && currentSource !== 'unset') return
    setWorkspaceTargetSelection(target, id, snapshot, source)
  }

  function resetWorkspaceTargetSelection(target: ChatViewTarget) {
    const view = chatView(target)
    view.workspaceTargetId.value = ''
    view.workspaceTargetSnapshot.value = null
    view.workspaceTargetSelectionSource.value = 'unset'
  }

  function sessionTranscript(botId: string, targetSessionId: string) {
    return chatViews.getOrCreate({
      botId: botId.trim(),
      sessionId: targetSessionId.trim(),
      viewId: focusedChatViewId.value,
    }).transcript
  }

  function activeTranscript() {
    return chatView().transcript
  }

  function transcriptForTarget(target?: Partial<ChatViewTarget>) {
    return chatView(target).transcript
  }

  function transcriptForTurn(turn: ChatMessage) {
    return chatViews.entries().find(view => view.transcript.messages.includes(turn))?.transcript
      ?? null
  }

  const messages = computed(() => readSynchronizedTranscript(activeTranscript()).messages)
  const loadingMessages = computed(() => activeTranscript().loadingMessages.value)
  const loadingOlder = computed(() => activeTranscript().loadingOlder.value)
  const hasMoreOlder = computed(() => activeTranscript().hasMoreOlder.value)
  const hasLoadedOlder = computed({
    get: () => activeTranscript().hasLoadedOlder.value,
    set: value => { activeTranscript().hasLoadedOlder.value = value },
  })

  const normalizeTurn = (...args: Parameters<SynchronizedTranscript['normalizeTurn']>) => activeTranscript().normalizeTurn(...args)
  const clearHistoryView = (...args: Parameters<SynchronizedTranscript['clearHistoryView']>) => activeTranscript().clearHistoryView(...args)
  const prepareForInitialization = () => activeTranscript().prepareForInitialization()
  const markHistoryEmpty = () => activeTranscript().markHistoryEmpty()
  const fetchSessionWindow = (botId: string, targetSessionId: string) => sessionTranscript(botId, targetSessionId).fetchSessionWindow(botId, targetSessionId)
  const loadOlderMessages = (target?: ChatViewTarget) => transcriptForTarget(target).loadOlderMessages()
  const findMessageIdByExternalId = (externalMessageId: string, target?: ChatViewTarget) => transcriptForTarget(target).findMessageIdByExternalId(externalMessageId)
  const locateMessageByExternalId = (externalMessageId: string, target?: ChatViewTarget) => transcriptForTarget(target).locateMessageByExternalId(externalMessageId)
  function isActiveSessionTarget(botId: string, targetSessionId: string) {
    return currentBotId.value === botId.trim() && sessionId.value === targetSessionId.trim()
  }
  const appendTurnToSession = (botId: string, targetSessionId: string, turn: ChatMessage) => sessionTranscript(botId, targetSessionId).appendTurnToSession(botId, targetSessionId, turn)
  const reattachTurnToSession = (botId: string, targetSessionId: string, turn: ChatMessage) => sessionTranscript(botId, targetSessionId).reattachTurnToSession(botId, targetSessionId, turn)
  const removeTurnFromSession = (botId: string, targetSessionId: string, turn: ChatMessage) => {
    const transcript = targetSessionId.trim()
      ? sessionTranscript(botId, targetSessionId)
      : transcriptForTurn(turn)
    transcript?.removeTurnFromSession(botId, targetSessionId, turn)
  }
  const restoreTailFromOptimistic = (botId: string, targetSessionId: string, optimisticUserTurn: ChatUserTurn | null, assistantTurn: ChatAssistantTurn, replacedTurns: ChatMessage[]) => sessionTranscript(botId, targetSessionId).restoreTailFromOptimistic(botId, targetSessionId, optimisticUserTurn, assistantTurn, replacedTurns)
  const upsertAssistantUIMessage = (...args: Parameters<SynchronizedTranscript['upsertAssistantUIMessage']>) => transcriptForTurn(args[0])?.upsertAssistantUIMessage(...args)
  const replaceAssistantUIMessageSnapshot = (...args: Parameters<SynchronizedTranscript['replaceAssistantUIMessageSnapshot']>) => transcriptForTurn(args[0])?.replaceAssistantUIMessageSnapshot(...args)
  const hasVisibleAssistantBlocks = (turn: ChatAssistantTurn) => transcriptForTurn(turn)?.hasVisibleAssistantBlocks(turn) ?? false
  const finishAssistantTurn = (turn: ChatAssistantTurn) => { transcriptForTurn(turn)?.finishAssistantTurn(turn) }
  const appendAssistantError = (...args: Parameters<SynchronizedTranscript['appendAssistantError']>) => transcriptForTurn(args[0])?.appendAssistantError(...args)
  const finalizeStreamFailure = (...args: Parameters<SynchronizedTranscript['finalizeStreamFailure']>) => transcriptForTurn(args[0])?.finalizeStreamFailure(...args)
  const assistantTurnForRuntimeError = (targetSessionId: string, identity: Parameters<SynchronizedTranscript['assistantTurnForRuntimeError']>[1]) => sessionTranscript(currentBotId.value ?? '', targetSessionId).assistantTurnForRuntimeError(targetSessionId, identity)
  const hasTurn = (turn: ChatMessage) => chatViews.entries().some(view => view.transcript.hasTurn(turn))
  const markToolApprovalDecision = (approvalId: string, status: 'approved' | 'rejected' | 'pending') => activeTranscript().markToolApprovalDecision(approvalId, status)
  const resetTranscriptUserScope = () => chatViews.resetAll()
  const runtimeAdmissionTimers = new Map<string, ReturnType<typeof setTimeout>>()
  const runtimeAbortWatchdogTimers = new Map<string, ReturnType<typeof setTimeout>>()
  const assistantStreams = createAssistantStreamRegistry({
    currentBotId,
    sessionId,
    finishAssistantTurn,
    beforeReject: (stream) => {
      if (stream.runtimeReplacement && !stream.runtimeReplacement.historyCommitted && !stream.runtimeObserved && !hasVisibleAssistantBlocks(stream.assistantTurn)) {
        restoreRuntimeReplacement(stream)
      }
    },
    onTracked: (stream) => {
      if (stream.sessionId) subscribeRuntime(stream.botId, stream.sessionId)
    },
    onFinished: (stream) => {
      clearRuntimeAdmissionTimeout(stream.streamId, stream.botId, stream.sessionId)
      clearRuntimeAbortWatchdog(stream.streamId, stream.botId, stream.sessionId)
      reconcileRuntimeSubscriptions(stream.botId)
    },
  })
  const {
    streaming,
    streamingSessionId,
    activeStreams: allAssistantStreams,
    assistantStreamsForSession,
    activeUnboundStreamIds,
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
  } = assistantStreams
  sessionStreamingProbe = isSessionStreaming

  function isChatViewStreaming(target: ChatViewTarget, composerScope?: string): boolean {
    const resolved = normalizedChatViewTarget(target)
    return resolved.sessionId
      ? isSessionStreaming(resolved.botId, resolved.sessionId)
      : isUnboundComposerStreaming(
          resolved.botId,
          composerScope?.trim() || `${resolved.botId}:${resolved.viewId}`,
        )
  }
  const runtimeAssistantGenerations = new WeakMap<ChatAssistantTurn, string>()
  const runtimeUserGenerations = new WeakMap<ChatUserTurn, string>()
  const runtimeOperationAdmissions = new Map<string, boolean>()
  const approvalResponses = createApprovalResponseTracker({
    rollbackApproval: approvalId => markToolApprovalDecision(approvalId, 'pending'),
    onExpired: handleExpiredApprovalResponse,
  })
  const {
    hasPendingApprovalResponse,
    beginApprovalResponse,
    getApprovalResponse,
    settleApprovalResponse,
    pendingApprovalResponses,
    markPendingApprovalResponsesUncertain,
    markApprovalResponseReplaySent,
    markApprovalResponseReplayFailed,
    pendingApprovalResponsesForSession,
    isTerminalApprovalResponse,
    resolveApprovalResponseIdentity,
    resetApprovalResponses,
  } = approvalResponses
  type UserInputStateSnapshot = ReturnType<SynchronizedTranscript['snapshotUserInputStates']>[number]
  function restoreUserInputStates(states: UserInputStateSnapshot[]) {
    for (const view of chatViews.entries()) {
      const owned = states.filter(state => view.transcript.messages.some(message =>
        message.role === 'assistant' && message.messages.includes(state.block),
      ))
      if (owned.length > 0) view.transcript.restoreUserInputStates(owned)
    }
  }
  interface PendingUserInputResponse {
    streamId: string
    userInputId: string
    botId: string
    sessionId: string
    shortId?: number
    answers?: WSUserInputAnswer[]
    canceled: boolean
    reason?: string
    awaitingResync: boolean
    replaySent: boolean
    replayFailed: boolean
  }
  const userInputResponseStreams = new Map<string, UserInputStateSnapshot[]>()
  const pendingUserInputResponses = new Map<string, PendingUserInputResponse>()
  const terminalUserInputResponseIds = new Set<string>()
  const terminalUserInputResponseLimit = 512

  function sidebandResponseKey(botId: string, targetSessionId: string, id: string) {
    return `${botId.trim()}\u0000${targetSessionId.trim()}\u0000${id.trim()}`
  }

  function rememberTerminalUserInputResponse(botId: string, targetSessionId: string, streamId: string) {
    const key = sidebandResponseKey(botId, targetSessionId, streamId)
    terminalUserInputResponseIds.add(key)
    if (terminalUserInputResponseIds.size <= terminalUserInputResponseLimit) return
    const oldest = terminalUserInputResponseIds.values().next().value
    if (oldest) terminalUserInputResponseIds.delete(oldest)
  }

  function isTerminalUserInputResponse(streamId: string, botId?: string, targetSessionId?: string) {
    const id = streamId.trim()
    if (!id) return false
    if (botId !== undefined && targetSessionId !== undefined) {
      return terminalUserInputResponseIds.has(sidebandResponseKey(botId, targetSessionId, id))
    }
    const bid = botId?.trim()
    const sid = targetSessionId?.trim()
    const matches = [...terminalUserInputResponseIds].filter((key) => {
      const [candidateBotId, candidateSessionId, candidateStreamId] = key.split('\u0000')
      return candidateStreamId === id
        && (bid === undefined || candidateBotId === bid)
        && (sid === undefined || candidateSessionId === sid)
    })
    return matches.length === 1
  }

  function resolveUserInputResponseIdentity(streamId: string, botId?: string, targetSessionId?: string) {
    const id = streamId.trim()
    if (!id) return undefined
    const bid = botId?.trim()
    const sid = targetSessionId?.trim()
    const keys = new Set<string>()
    for (const pending of pendingUserInputResponses.values()) {
      if (
        pending.streamId === id
        && (bid === undefined || pending.botId === bid)
        && (sid === undefined || pending.sessionId === sid)
      ) keys.add(sidebandResponseKey(pending.botId, pending.sessionId, pending.streamId))
    }
    for (const key of terminalUserInputResponseIds) {
      const [candidateBotId, candidateSessionId, candidateStreamId] = key.split('\u0000')
      if (
        candidateStreamId === id
        && (bid === undefined || candidateBotId === bid)
        && (sid === undefined || candidateSessionId === sid)
      ) keys.add(key)
    }
    if (keys.size !== 1) return undefined
    const [resolvedBotId, resolvedSessionId] = [...keys][0]!.split('\u0000')
    return { botId: resolvedBotId!, sessionId: resolvedSessionId! }
  }
  const runtimeSessions = createRuntimeSessionStore()
  const composerDraftRestores = ref<Record<string, ComposerDraftRestore[]>>({})
  let composerDraftRestoreSeq = 0
  let realtimeWebSocketBotId = ''

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
  transcriptSnapshotHook = (_view, targetSessionId, turns) => {
    syncForkAnchorFromUITurns(targetSessionId, turns)
  }
  transcriptRefreshAppliedHook = (view, targetSessionId, latestTimestamp) => {
    touchSessionInList(targetSessionId, latestTimestamp)
    reconcileApprovalResponsesFromTranscript(view.botId, targetSessionId)
    const runtimeState = runtimeSessions.get(acpRuntimeKey(view.botId, targetSessionId))
    if (runtimeState?.snapshot) {
      projectRuntimeSnapshot(runtimeState.snapshot, view.botId, targetSessionId, runtimeState.seq, true, false, true)
    }
  }
  const sessionListRefresh = createSessionListRefresh({
    currentBotId,
    fetchSessions,
    applySnapshot: (response) => {
      replaceSessions(response.items)
      sessionsCursor.value = response.nextCursor
      hasMoreSessions.value = response.nextCursor !== null
    },
  })
  const refreshSessionsList = sessionListRefresh.refresh
  const resetSessionListRefresh = sessionListRefresh.reset
  const persistedHistory = createPersistedHistoryReconciler({
    fetchLatest: ({ botId, sessionId }) => fetchMessagesUI(botId, sessionId, { limit: 30 }),
    apply: ({ botId, sessionId }, turns) => {
      return runtimeSessions.enqueue(acpRuntimeKey(botId, sessionId), () => {
        const view = chatViews.getSession(botId, sessionId)
        if (!view) return
        const overlapId = turns.find((turn) => {
          const existing = view.transcript.findTurnByServerId(turn.id)
          return existing?.syncState?.persistence === 'persisted'
        })?.id ?? ''
        if (!view.transcript.replacePersistedSuffix(turns, overlapId, sessionId)) {
          // A latest page without overlap cannot be safely joined to the local
          // transcript: resetting avoids presenting a continuous history with
          // an invisible gap after an SSE subscriber overflow.
          view.transcript.replacePersistedWindow(turns, sessionId)
        }
        transcriptRefreshAppliedHook(
          view,
          sessionId,
          view.transcript.messages.at(-1)?.timestamp,
        )
      })
    },
    onError: (ref, error) => {
      console.error(`Failed to reconcile persisted history ${ref.botId}/${ref.sessionId}:`, error)
    },
  })
  function requestPersistedHistory(botId: string, targetSessionId: string) {
    if (!chatViews.getSession(botId, targetSessionId)) return
    void persistedHistory.reconcile({ botId, sessionId: targetSessionId }).catch(() => {})
  }
  const realtime = createChatRealtimeController({
    onWebSocketEvent: (botId, event) => handleWSStreamEvent(event, undefined, botId),
    onWebSocketOpen: handleWebSocketOpen,
    onWebSocketClose: handleWebSocketClose,
    prepareSessionMessages,
    onSessionMessageEvent: handleSessionMessageEvent,
    onBotSessionsActivityEvent: handleBotSessionsActivityEvent,
  })
  const {
    startWebSocket: startRealtimeWebSocket,
    stopWebSocket: stopRealtimeWebSocket,
    ensureWebSocketConnected,
    sendWebSocketMessage,
    abortWebSocketStream: sendAbortWebSocketStream,
    startSessionMessagesStream,
    stopSessionMessagesStream,
    startBotSessionsActivityStream,
    stopStreams,
  } = realtime
  const runtimeSubscriptionController = createRuntimeSubscriptionController({
    transport: {
      ensureConnected: ensureWebSocketConnected,
      send: sendWebSocketMessage,
    },
    runtime: {
      awaitCheckpoint: ({ botId, sessionId }) => {
        void runtimeSessions.enqueue(acpRuntimeKey(botId, sessionId), runtimeSession => {
          runtimeSession.awaitCheckpoint()
        })
      },
      release: ({ botId, sessionId }) => {
        runtimeSessions.remove(acpRuntimeKey(botId, sessionId))
      },
      apply: applyRuntimeSubscriptionEvent,
    },
    createInvocationId: createStreamId,
    onError: (ref, error) => {
      console.error(`Runtime subscription failed for ${ref.botId}/${ref.sessionId}:`, error)
    },
  })
  interface IntentStartContext {
    appendUserTurn: boolean
    onBeforeTurnAppend?: () => void
    assistantTurn: ChatAssistantTurn | null
    userTurn: ChatUserTurn | null
  }
  let intentStartContext: IntentStartContext | null = null
  const intentCompletions = new Map<string, Promise<void>>()

  function intentCompletionKey(botId: string, streamId: string) {
    return `${botId.trim()}\u0000${streamId.trim()}`
  }

  function intentTranscript(intent: Pick<PendingChatIntent, 'botId' | 'sessionId' | 'viewId'>) {
    return transcriptForTarget({
      botId: intent.botId,
      sessionId: intent.sessionId || null,
      viewId: intent.viewId,
    })
  }

  function removeIntentPlaceholder(intent: PendingChatIntent) {
    const stream = getAssistantStream(intent.streamId, intent.botId, intent.sessionId)
      ?? getAssistantStream(intent.streamId, intent.botId, '')
    const transcript = intentTranscript(intent)
    if (stream?.runtimeReplacement) restoreRuntimeReplacement(stream)
    const userTurn = intent.userTurnKey
      ? transcript.messages.find(turn => turn.id === intent.userTurnKey && turn.role === 'user')
      : undefined
    const assistantTurn = stream?.assistantTurn
      ?? transcript.messages.find((turn): turn is ChatAssistantTurn =>
        turn.id === intent.assistantTurnKey && turn.role === 'assistant')
    if (userTurn) transcript.removeTurnFromSession(intent.botId, intent.sessionId, userTurn)
    if (assistantTurn) transcript.removeTurnFromSession(intent.botId, intent.sessionId, assistantTurn)
  }

  function composerDraftRestoreKey(request: Pick<RestoreComposerDraftRequest, 'botId' | 'sessionId' | 'viewId' | 'composerScope'>) {
    return [request.botId, request.viewId, request.sessionId, request.composerScope]
      .map(value => value.trim())
      .join('\u0000')
  }

  function publishComposerDraftRestore(request: RestoreComposerDraftRequest) {
    const restore: ComposerDraftRestore = {
      ...request,
      attachments: request.attachments.map(attachment => ({ ...attachment })),
      requestedSkills: request.requestedSkills.map(skill => ({ ...skill })),
      seq: ++composerDraftRestoreSeq,
    }
    const key = composerDraftRestoreKey(restore)
    const queue = composerDraftRestores.value[key] ?? []
    composerDraftRestores.value = {
      ...composerDraftRestores.value,
      [key]: [...queue, restore],
    }
  }

  function composerDraftRestoreFor(target: ChatViewTarget, composerScope: string): ComposerDraftRestore | null {
    const botId = target.botId.trim()
    const viewId = target.viewId.trim()
    const sessionId = target.sessionId?.trim() ?? ''
    const scope = composerScope.trim()
    const exact = composerDraftRestores.value[composerDraftRestoreKey({ botId, viewId, sessionId, composerScope: scope })]
    if (exact?.length) return exact[0] ?? null
    if (!sessionId) return null
    const promotedDraft = composerDraftRestores.value[composerDraftRestoreKey({ botId, viewId, sessionId: '', composerScope: scope })]
    return promotedDraft?.[0] ?? null
  }

  const chatIntents = createChatIntents({
    createStreamId,
    dispatch: (botId: string, message: ChatIntentWireMessage) => {
      if (!ensureWebSocketConnected(botId)) return false
      return sendWebSocketMessage(botId, message)
    },
    createPlaceholder: (request: IntentPlaceholderRequest) => {
      const transcript = transcriptForTarget({
        botId: request.botId,
        sessionId: request.sessionId || null,
        viewId: request.viewId,
      })
      const assistantTurn = transcript.createOptimisticAssistantTurn()
      let userTurn: ChatUserTurn | null = null
      intentStartContext?.onBeforeTurnAppend?.()
      if (request.kind === 'send' && (intentStartContext?.appendUserTurn ?? true)) {
        userTurn = transcript.createOptimisticUserTurn(request.text, request.attachments, request.streamId)
        transcript.appendToView(userTurn, assistantTurn)
      }
      if (request.kind !== 'send') beginRuntimeOperationAdmission(request.streamId, request.botId, request.sessionId)
      const completion = trackAssistantStream({
        streamId: request.streamId,
        assistantTurn,
        botId: request.botId,
        sessionId: request.sessionId,
        composerScope: request.composerScope,
        viewId: request.viewId,
      })
      intentCompletions.set(intentCompletionKey(request.botId, request.streamId), completion)
      if (intentStartContext) {
        intentStartContext.assistantTurn = assistantTurn
        intentStartContext.userTurn = userTurn
      }
      return {
        userTurnKey: userTurn?.id,
        assistantTurnKey: assistantTurn.id,
      }
    },
    rollbackPlaceholder: (intent) => {
      removeIntentPlaceholder(intent)
      finishRuntimeOperationAdmission(intent.streamId, intent.botId, intent.sessionId)
      discardAssistantStream(intent.streamId, intent.botId, intent.sessionId)
      intentCompletions.delete(intentCompletionKey(intent.botId, intent.streamId))
    },
    removeVanished: removeIntentPlaceholder,
    restoreReplacement: (intent) => {
      const stream = getAssistantStream(intent.streamId, intent.botId, intent.sessionId)
      if (stream) restoreRuntimeReplacement(stream)
    },
    restoreComposerDraft: publishComposerDraftRestore,
    onClaim: (intent, handle) => {
      const stream = getAssistantStream(intent.streamId, intent.botId, intent.sessionId)
      if (stream) markRuntimeStreamObserved(stream, handle.generation)
    },
  })
  stopEvictedSessionStream = (botId, targetSessionId) => {
    persistedHistory.cancel({ botId, sessionId: targetSessionId })
    runtimeSubscriptionController.remove({ botId, sessionId: targetSessionId })
    runtimeSessions.remove(acpRuntimeKey(botId, targetSessionId))
    stopSessionMessagesStream(botId, targetSessionId)
  }

  function releaseHiddenSessionView(view: StoreChatViewEntry | null) {
    if (!view || view.kind !== 'session' || !view.sessionId) return
    if (view.visiblePanelIds.size > 0) return
    // Visibility owns the live subscription; the registry independently owns
    // cached transcript retention for streaming and pending interactions.
    stopSessionMessagesStream(view.botId, view.sessionId)
    chatViews.prune()
    reconcileRuntimeSubscriptions(view.botId)
  }

  function bindChatView(panelId: string, target: ChatViewTarget, visible = true): PublicStoreChatViewEntry {
    const change = chatViews.bindPanel(panelId, normalizedChatViewTarget(target), visible)
    if (panelId.trim() === focusedChatViewId.value && change.view.kind === 'draft') {
      activateDraftACPStage({
        botId: change.view.botId,
        sessionId: null,
        viewId: change.view.viewId,
      })
    }
    releaseHiddenSessionView(change.deactivatedSession)
    if (change.activatedSession?.sessionId) {
      startSessionMessagesStream(change.activatedSession.botId, change.activatedSession.sessionId)
      reconcileRuntimeSubscriptions(change.activatedSession.botId)
    }
    if (visible && change.view.kind === 'session' && change.view.sessionId) {
      void ensureVisibleSessionSummary(change.view.botId, change.view.sessionId)
    }
    return publicChatViewEntry(change.view)
  }

  function setChatViewVisible(panelId: string, visible: boolean) {
    const change = chatViews.setPanelVisible(panelId, visible)
    if (!change) return
    releaseHiddenSessionView(change.deactivatedSession)
    if (change.activatedSession?.sessionId) {
      startSessionMessagesStream(change.activatedSession.botId, change.activatedSession.sessionId)
      reconcileRuntimeSubscriptions(change.activatedSession.botId)
    }
    if (visible && change.view.kind === 'session' && change.view.sessionId) {
      void ensureVisibleSessionSummary(change.view.botId, change.view.sessionId)
    }
  }

  function unbindChatView(panelId: string) {
    releaseHiddenSessionView(chatViews.unbindPanel(panelId))
  }

  function focusChatView(viewId: string) {
    const id = viewId.trim()
    if (!id || id === focusedChatViewId.value) return
    saveLiveDraftACPStage()
    focusedChatViewId.value = id
    const view = chatViews.getPanel(id)
    if (view?.kind === 'draft') {
      activateDraftACPStage({ botId: view.botId, sessionId: null, viewId: view.viewId })
    }
  }

  function promoteDraftChatView(target: ChatViewTarget, targetSessionId: string): StoreChatViewEntry {
    invalidateDraftViewCommand(target)
    const promoted = chatViews.promoteDraft(target.botId, target.viewId, targetSessionId)
    if (promoted.visiblePanelIds.size > 0 && promoted.sessionId) {
      startSessionMessagesStream(promoted.botId, promoted.sessionId)
      reconcileRuntimeSubscriptions(promoted.botId)
    }
    return promoted
  }

  function runtimeGenerationForStream(botId: string | undefined, targetSessionId: string | undefined, streamId: string): string {
    const bid = botId?.trim() ?? ''
    const sid = targetSessionId?.trim() ?? ''
    const id = streamId.trim()
    if (!bid || !sid || !id) return ''
    const run = runtimeSessions.get(acpRuntimeKey(bid, sid))?.snapshot?.current_run_view
    if ((run?.stream_id ?? '').trim() !== id) return ''
    return (run?.generation ?? '').trim()
  }

  function abortWebSocketStream(streamId: string, botId?: string, targetSessionId?: string): boolean {
    const id = streamId.trim()
    const requestedBotId = botId?.trim() ?? ''
    const requestedSessionId = targetSessionId?.trim() ?? ''
    const stream = requestedBotId && requestedSessionId
      ? getAssistantStream(id, requestedBotId, requestedSessionId)
      : getAssistantStream(id)
    const bid = botId?.trim() || stream?.botId || ''
    const sid = targetSessionId?.trim() || stream?.sessionId || ''
    const intent = chatIntents.get(bid, sid, id)
    const generation = runtimeGenerationForStream(bid, sid, id)
      || stream?.runtimeGeneration?.trim()
      || intent?.generation?.trim()
      || ''
    if (!generation && stream?.abortSent) return true
    if (generation && stream?.abortSentGeneration === generation) return true
    if (!generation && intent) {
      const sent = chatIntents.cancelPendingSend(bid, sid, id)
      if (sent && stream) {
        stream.abortRequested = true
        stream.abortSent = true
        armRuntimeAbortWatchdog(stream)
      }
      return sent
    }
    if (!generation && stream?.runtimeObserved) return false
    const sent = generation
      ? chatIntents.abortRun({ botId: bid, sessionId: sid, streamId: id, generation })
      : sendAbortWebSocketStream(id, bid, sid, '')
    if (sent && stream) {
      stream.abortRequested = true
      stream.abortSent = true
      stream.abortSentGeneration = generation
      armRuntimeAbortWatchdog(stream)
    }
    return sent
  }
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
  let initializeToken: symbol | null = null
  let userScopeGeneration = 0
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')
  const startupSendFailures = ref<Record<string, StartupSendFailure>>({})
  function startupSendFailureKey(botId: string, targetSessionId: string, composerScope = '') {
    const bid = botId.trim()
    const scope = composerScope.trim()
    if (scope) return `composer:${bid}:${scope}`
    return `session:${bid}:${targetSessionId.trim()}`
  }
  function startupSendFailureFor(target: ChatViewTarget, composerScope = ''): StartupSendFailure | null {
    const resolved = normalizedChatViewTarget(target)
    const scopedKey = startupSendFailureKey(resolved.botId, resolved.sessionId ?? '', composerScope)
    const scoped = startupSendFailures.value[scopedKey]
    if (scoped) return scoped
    if (resolved.sessionId) {
      return startupSendFailures.value[startupSendFailureKey(resolved.botId, resolved.sessionId)] ?? null
    }
    return null
  }
  const startupSendFailure = computed(() => startupSendFailureFor(
    normalizedChatViewTarget(),
    focusedChatViewId.value === 'chat'
      ? 'chat'
      : `${(currentBotId.value ?? '').trim()}:${focusedChatViewId.value}`,
  ))
  // Slash-command event registry (see ./chat/command-events for scoping rules).
  const commandEventRegistry = createCommandEventRegistry({ currentBotId, sessionId })
  const {
    commandEvent,
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
  const userSentInSession = ref<{
    id: string
    botId: string
    viewId: string
    wasDraft: boolean
    seq: number
  } | null>(null)
  let userSendSeq = 0
  const draftViewRequested = ref<{
    botId: string
    viewId: string
    expectedSessionId: string | null
    explicitSelection: boolean
    input: ACPAgentSessionInput | null
    activate: boolean
    seq: number
  } | null>(null)
  let draftViewRequestSeq = 0
  const draftViewCommandVersions = new Map<string, number>()
  let draftViewCommandSequence = 0
  const forkedSessionRequested = ref<{
    botId: string
    viewId: string
    expectedSessionId: string
    sessionId: string
    title: string
    explicitSelection: true
    activate: boolean
    seq: number
  } | null>(null)
  let forkedSessionRequestSeq = 0
  // Bumps after a session delete succeeds. Consumers that own per-session UI
  // chrome must not infer deletion from the paginated session list: a valid open
  // tab can fall off the current page without being deleted.
  const deletedSession = ref<{ id: string, botId: string, seq: number, composerScope?: string } | null>(null)
  let deletedSessionSeq = 0

  let selectSessionRequestId = 0
  const visibleSessionSummaryRequests = new Map<string, Promise<SessionSummary | null>>()

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

  function ensureVisibleSessionSummary(targetBotId: string, targetSessionId: string): Promise<SessionSummary | null> {
    const bid = targetBotId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return Promise.resolve(null)
    const known = knownSessionSummary(sid)
    if (known) return Promise.resolve(known)
    const key = `${bid}\u0000${sid}`
    const pending = visibleSessionSummaryRequests.get(key)
    if (pending) return pending
    const generation = userScopeGeneration
    const request = (async () => {
      try {
        const fetched = await fetchSession(bid, sid)
        if (generation !== userScopeGeneration || (currentBotId.value ?? '').trim() !== bid) return null
        rememberSession(fetched)
        return fetched
      } catch {
        return null
      } finally {
        if (visibleSessionSummaryRequests.get(key) === request) {
          visibleSessionSummaryRequests.delete(key)
        }
      }
    })()
    visibleSessionSummaryRequests.set(key, request)
    return request
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
    stopSessionMessagesStream(bid, sid)
    chatViews.removeSession(bid, sid)
    if ((currentBotId.value ?? '').trim() === bid) {
      removeSessionFromList(sid)
      if ((sessionId.value ?? '').trim() === sid) {
        sessionId.value = null
        explicitSessionSelection.value = false
        draftIntent.value = true
        clearHistoryView()
      }
    }

    try {
      await requestDeleteSession(bid, sid)
    } catch {
      // Best-effort cleanup: the send failure result is the user-facing error.
    }
  }





  // Pending-ACP session staging (see ./chat/acp-staging for the generation /
  // identity-key model). Transcript and select-session invalidation are
  // injected so the staging machine never touches store internals directly.
  const acpStaging = createACPStaging({
    currentBotId,
    sessionId,
    draftIntent,
    explicitSessionSelection,
    runtimeRegistry: acpRuntimeRegistry,
    bumpSelectSessionRequest: () => {
      selectSessionRequestId++
    },
    clearTranscriptForDraft: () => {
      const bid = (currentBotId.value ?? '').trim()
      const sid = (sessionId.value ?? '').trim()
      if (bid && sid) stopSessionMessagesStream(bid, sid)
      clearHistoryView()
      reconcileRuntimeSubscriptions()
    },
  })
  const {
    pendingACPSessionInput,
    pendingACPRuntimeId,
    pendingACPSessionMetadata,
    pendingACPRuntimeStatus,
    pendingACPRuntimeEnsuring,
    rememberDefaultACPInput,
    cachedDefaultACPInput,
    cacheDefaultACPSession,
    stageACPSession: stageFocusedACPSession,
    stageDefaultACPSession: stageFocusedDefaultACPSession,
    stageNewACPSession: stageFocusedNewACPSession,
    resetToEmptyComposer: resetFocusedEmptyComposer,
    ensurePendingACPRuntime: ensureFocusedPendingACPRuntime,
    setPendingACPModel: setFocusedPendingACPModel,
    setPendingACPReasoning: setFocusedPendingACPReasoning,
    clearPendingACPSession,
    detachPendingACPSession,
    restorePendingACPSession,
    releasePendingACPSession,
    discardDetachedACPSession,
    pendingACPMatchesInput: focusedPendingACPMatchesInput,
  } = acpStaging

  interface DraftACPStage extends DetachedACPSession {
    viewId: string
  }

  const draftACPStages = ref<Record<string, DraftACPStage>>({})
  let liveDraftACP: { botId: string, viewId: string } | null = null

  function draftACPStageKey(botId: string, viewId: string) {
    return `${botId.trim()}\u0000${viewId.trim()}`
  }

  function sameDraftACPStage(left: { botId: string, viewId: string } | null, right: ChatViewTarget) {
    return !!left
      && left.botId === right.botId.trim()
      && left.viewId === right.viewId.trim()
      && !right.sessionId
  }

  function rememberDraftACPStage(target: Pick<ChatViewTarget, 'botId' | 'viewId'>, detached: DetachedACPSession) {
    const key = draftACPStageKey(target.botId, target.viewId)
    draftACPStages.value = {
      ...draftACPStages.value,
      [key]: {
        botId: detached.botId.trim() || target.botId.trim(),
        viewId: target.viewId.trim(),
        input: { ...detached.input },
        runtimeId: detached.runtimeId.trim(),
      },
    }
  }

  function syncLiveDraftACPStage() {
    if (!liveDraftACP || !pendingACPSessionInput.value) return
    rememberDraftACPStage(liveDraftACP, {
      botId: liveDraftACP.botId,
      input: pendingACPSessionInput.value,
      runtimeId: pendingACPRuntimeId.value,
    })
  }

  function saveLiveDraftACPStage() {
    if (!liveDraftACP) return
    const owner = liveDraftACP
    const detached = detachPendingACPSession()
    if (detached) rememberDraftACPStage(owner, detached)
    liveDraftACP = null
  }

  function activateDraftACPStage(target: ChatViewTarget) {
    const resolved = normalizedChatViewTarget(target)
    if (resolved.sessionId || !resolved.botId || !resolved.viewId) return
    if (sameDraftACPStage(liveDraftACP, resolved)) return
    saveLiveDraftACPStage()
    liveDraftACP = { botId: resolved.botId, viewId: resolved.viewId }
    const saved = draftACPStages.value[draftACPStageKey(resolved.botId, resolved.viewId)]
    if (saved) {
      restorePendingACPSession(saved.input, saved.runtimeId, saved.botId)
    } else {
      releasePendingACPSession()
    }
  }

  function forgetDraftACPStage(target: ChatViewTarget) {
    const resolved = normalizedChatViewTarget(target)
    const key = draftACPStageKey(resolved.botId, resolved.viewId)
    if (sameDraftACPStage(liveDraftACP, resolved)) {
      releasePendingACPSession()
      liveDraftACP = null
    }
    if (!(key in draftACPStages.value)) return
    const { [key]: _removed, ...rest } = draftACPStages.value
    draftACPStages.value = rest
  }

  function discardDraftACPStage(target: ChatViewTarget) {
    const resolved = normalizedChatViewTarget(target)
    const key = draftACPStageKey(resolved.botId, resolved.viewId)
    if (sameDraftACPStage(liveDraftACP, resolved)) {
      clearPendingACPSession()
      liveDraftACP = null
    } else {
      const saved = draftACPStages.value[key]
      if (saved) discardDetachedACPSession(saved)
    }
    if (!(key in draftACPStages.value)) return
    const { [key]: _removed, ...rest } = draftACPStages.value
    draftACPStages.value = rest
  }

  discardEvictedDraftStage = (view) => {
    draftViewCommandVersions.delete(draftSessionCreationKey({
      botId: view.botId,
      sessionId: null,
      viewId: view.viewId,
    }))
    discardDraftACPStage({ botId: view.botId, sessionId: null, viewId: view.viewId })
  }

  function pendingACPStateFor(target: ChatViewTarget) {
    const resolved = normalizedChatViewTarget(target)
    if (resolved.sessionId) return null
    const live = sameDraftACPStage(liveDraftACP, resolved)
    const saved = live && pendingACPSessionInput.value
      ? {
          botId: liveDraftACP!.botId,
          viewId: liveDraftACP!.viewId,
          input: pendingACPSessionInput.value,
          runtimeId: pendingACPRuntimeId.value,
        }
      : draftACPStages.value[draftACPStageKey(resolved.botId, resolved.viewId)]
    if (!saved) return null
    const runtimeKey = acpRuntimeKey(saved.botId, saved.runtimeId)
    return {
      input: { ...saved.input },
      metadata: acpSessionMetadata(saved.input),
      runtimeId: saved.runtimeId,
      runtimeStatus: runtimeKey ? acpRuntimeStatuses.value[runtimeKey] : undefined,
      ensuring: live ? pendingACPRuntimeEnsuring.value : false,
    }
  }

  function targetDraftForACP(target?: ChatViewTarget): ChatViewTarget {
    const resolved = normalizedChatViewTarget(target)
    return { ...resolved, sessionId: null }
  }

  function stageACPSession(
    input: ACPAgentSessionInput,
    options: { explicitSelection?: boolean } = {},
    target?: ChatViewTarget,
  ) {
    const draft = targetDraftForACP(target)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    stageFocusedACPSession(input, options)
    syncLiveDraftACPStage()
  }

  function stageDefaultACPSession(input: ACPAgentSessionInput, target?: ChatViewTarget) {
    const draft = targetDraftForACP(target)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    stageFocusedDefaultACPSession(input)
    syncLiveDraftACPStage()
  }

  function stageNewACPSession(input: ACPAgentSessionInput, target?: ChatViewTarget) {
    const draft = targetDraftForACP(target)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    stageFocusedNewACPSession(input)
    syncLiveDraftACPStage()
  }

  function resetToEmptyComposer(
    options: { clearPendingACP?: boolean, explicitSelection?: boolean, draftIntent?: boolean } = {},
    target?: ChatViewTarget,
  ) {
    const draft = targetDraftForACP(target)
    resetWorkspaceTargetSelection(draft)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    resetFocusedEmptyComposer(options)
    if (options.clearPendingACP !== false) forgetDraftACPStage(draft)
  }

  async function ensurePendingACPRuntime(target?: ChatViewTarget) {
    const draft = targetDraftForACP(target)
    activateDraftACPStage(draft)
    try {
      return await ensureFocusedPendingACPRuntime()
    } finally {
      syncLiveDraftACPStage()
    }
  }

  async function setPendingACPModel(modelId: string, target?: ChatViewTarget) {
    const draft = targetDraftForACP(target)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    try {
      return await setFocusedPendingACPModel(modelId)
    } finally {
      syncLiveDraftACPStage()
    }
  }

  async function setPendingACPReasoning(effort: string, target?: ChatViewTarget) {
    const draft = targetDraftForACP(target)
    invalidateDraftViewCommand(draft)
    activateDraftACPStage(draft)
    try {
      return await setFocusedPendingACPReasoning(effort)
    } finally {
      syncLiveDraftACPStage()
    }
  }

  function pendingACPMatchesInput(input: ACPAgentSessionInput, target?: ChatViewTarget) {
    if (!target) return focusedPendingACPMatchesInput(input)
    const state = pendingACPStateFor(target)
    if (!state) return false
    const metadata = acpSessionMetadata(input)
    return state.metadata.acp_agent_id === metadata.acp_agent_id
      && state.metadata.project_path === metadata.project_path
      && state.metadata.acp_project_mode === metadata.acp_project_mode
  }

  watch(currentBotId, (newId) => {
    if (newId) {
      void initialize()
    } else {
      resetUserScopedState()
    }
  }, { immediate: true })

  const stopAuthSessionCleared = onAuthSessionCleared(() => resetUserScopedState({ clearSelection: true }))
  onScopeDispose(() => {
    stopAuthSessionCleared()
    stopStreams()
    stopWebSocket()
    clearAllRuntimeAdmissionTimeouts()
    clearAllRuntimeAbortWatchdogs()
    resetSessionListRefresh()
    persistedHistory.reset()
    runtimeSubscriptionController.dispose()
    runtimeSessions.clear()
    chatIntents.clear()
    for (const stream of allAssistantStreams()) {
      discardAssistantStream(stream.streamId, stream.botId, stream.sessionId)
    }
    chatViews.resetAll()
  })


  function runtimeStatusError(status: string, message: string, assistantTurn?: ChatAssistantTurn, runtimeObserved = false): Error {
    if (status === 'aborted') {
      return new RuntimeAbortError(responseAbortedMessage())
    }
    const stage: SendMessageStage = runtimeObserved || (assistantTurn && hasVisibleAssistantBlocks(assistantTurn)) ? 'stream' : 'startup'
    return new StreamFailureError(message || 'runtime interrupted', stage)
  }

  function runtimeInactiveError(assistantTurn: ChatAssistantTurn): Error {
    const stage: SendMessageStage = hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup'
    return new StreamFailureError('runtime state is no longer active', stage)
  }

  function runtimeAdmissionKey(streamId: string, botId: string, targetSessionId: string) {
    return `${botId.trim()}\u0000${targetSessionId.trim()}\u0000${streamId.trim()}`
  }

  function beginRuntimeOperationAdmission(streamId: string, botId: string, targetSessionId: string) {
    const key = runtimeAdmissionKey(streamId, botId, targetSessionId)
    runtimeOperationAdmissions.set(key, false)
  }

  function finishRuntimeOperationAdmission(streamId: string, botId: string, targetSessionId: string): boolean {
    const key = runtimeAdmissionKey(streamId, botId, targetSessionId)
    const observed = runtimeOperationAdmissions.get(key) ?? false
    runtimeOperationAdmissions.delete(key)
    return observed
  }

  function clearRuntimeAdmissionTimeout(streamId: string, botId: string, targetSessionId: string) {
    const id = streamId.trim()
    const key = runtimeAdmissionKey(id, botId, targetSessionId)
    const timer = runtimeAdmissionTimers.get(key)
    if (timer) clearTimeout(timer)
    runtimeAdmissionTimers.delete(key)
  }

  function clearAllRuntimeAdmissionTimeouts() {
    for (const timer of runtimeAdmissionTimers.values()) clearTimeout(timer)
    runtimeAdmissionTimers.clear()
    runtimeOperationAdmissions.clear()
  }

  function clearRuntimeAbortWatchdog(streamId: string, botId: string, targetSessionId: string) {
    const key = runtimeAdmissionKey(streamId, botId, targetSessionId)
    const timer = runtimeAbortWatchdogTimers.get(key)
    if (timer) clearTimeout(timer)
    runtimeAbortWatchdogTimers.delete(key)
  }

  function clearAllRuntimeAbortWatchdogs() {
    for (const timer of runtimeAbortWatchdogTimers.values()) clearTimeout(timer)
    runtimeAbortWatchdogTimers.clear()
  }

  function armRuntimeAbortWatchdog(stream: PendingAssistantStream) {
    const { streamId, botId, sessionId: targetSessionId } = stream
    if (!targetSessionId) return
    const key = runtimeAdmissionKey(streamId, botId, targetSessionId)
    clearRuntimeAbortWatchdog(streamId, botId, targetSessionId)
    runtimeAbortWatchdogTimers.set(key, setTimeout(() => {
      runtimeAbortWatchdogTimers.delete(key)
      const pending = getAssistantStream(streamId, botId, targetSessionId)
      if (!pending?.abortRequested) return
      pending.abortSent = false
      pending.abortSentGeneration = ''
      requestRuntimeResync(botId, targetSessionId)
    }, RUNTIME_ABORT_WATCHDOG_MS))
  }

  function armRuntimeAdmissionTimeout(streamId: string, botId: string, targetSessionId: string) {
    const id = streamId.trim()
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const stream = getAssistantStream(id, bid, sid)
    if (!stream || stream.runtimeObserved) return
    const key = runtimeAdmissionKey(id, bid, sid)
    clearRuntimeAdmissionTimeout(id, bid, sid)
    runtimeAdmissionTimers.set(key, setTimeout(() => {
      runtimeAdmissionTimers.delete(key)
      const pending = getAssistantStream(id, bid, sid)
      if (!pending || pending.runtimeObserved) return
      pruneEmptyAssistantTurnIfPending(id, bid, sid)
      rejectAssistantStream(id, new StreamFailureError('runtime command was not acknowledged', 'startup'), bid, sid)
      refreshLoadingForSession(pending.botId, pending.sessionId)
    }, RUNTIME_ADMISSION_TIMEOUT_MS))
  }

  function markRuntimeStreamObserved(stream: PendingAssistantStream, runtimeGeneration = '') {
    stream.runtimeObserved = true
    const operationKey = runtimeAdmissionKey(stream.streamId, stream.botId, stream.sessionId)
    if (runtimeOperationAdmissions.has(operationKey)) runtimeOperationAdmissions.set(operationKey, true)
    clearRuntimeAdmissionTimeout(stream.streamId, stream.botId, stream.sessionId)
    const generation = runtimeGeneration.trim()
    if (generation) stream.runtimeGeneration = generation
  }

  function rejectRuntimeStreamsForSession(botId: string, targetSessionId: string): boolean {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return false
    let rejected = false
    for (const stream of assistantStreamsForSession(bid, sid)) {
      if (!stream.runtimeObserved) continue
      settleApprovalResponse(stream.streamId, 'failed', stream.botId, stream.sessionId)
      pruneEmptyAssistantTurnIfPending(stream.streamId, stream.botId, stream.sessionId)
      rejectAssistantStream(stream.streamId, runtimeInactiveError(stream.assistantTurn), stream.botId, stream.sessionId)
      rejected = true
    }
    return rejected
  }

  function rejectSupersededRuntimeStreams(botId: string, targetSessionId: string, currentStreamId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const activeStreamId = currentStreamId.trim()
    if (!bid || !sid || !activeStreamId) return
    for (const stream of assistantStreamsForSession(bid, sid)) {
      if (stream.streamId === activeStreamId) continue
      if (!stream.runtimeObserved) continue
      settleApprovalResponse(stream.streamId, 'failed', stream.botId, stream.sessionId)
      pruneEmptyAssistantTurnIfPending(stream.streamId, stream.botId, stream.sessionId)
      rejectAssistantStream(stream.streamId, runtimeInactiveError(stream.assistantTurn), stream.botId, stream.sessionId)
    }
  }


  function refreshLoadingForSession(botId: string, targetSessionId: string) {
    if (!isActiveSessionTarget(botId, targetSessionId)) return
    loading.value = isSessionStreaming(botId, targetSessionId)
  }

  function isActiveSessionStreaming() {
    return isSessionStreaming(currentBotId.value, sessionId.value)
  }


  function runtimeProjectionGenerationMatches(
    projectionGeneration: string | undefined,
    runtimeGeneration: string,
  ): boolean {
    const expected = runtimeGeneration.trim()
    const actual = projectionGeneration?.trim() ?? ''
    return !expected || expected === actual
  }

  function runtimeAssistantErrorIdentityFor(streamId: string, runtimeGeneration = '', botId?: string, targetSessionId?: string) {
    const id = streamId.trim()
    const generation = runtimeGeneration.trim()
      || getAssistantStream(id, botId, targetSessionId)?.runtimeGeneration.trim()
      || terminalStreamGeneration(id, botId, targetSessionId)?.trim()
      || ''
    return id && generation ? { streamId: id, generation } : undefined
  }

  function hasSameRuntimeRequestStableIdentity(existing: ChatUserTurn, canonical: ChatUserTurn): boolean {
    if (canonical.serverId) {
      if (existing.serverId === canonical.serverId || existing.id === canonical.serverId) return true
      if (existing.serverId) return false
    }
    return Number.isSafeInteger(existing.turnPosition)
      && Number.isSafeInteger(existing.turnMessageSeq)
      && existing.turnPosition === canonical.turnPosition
      && existing.turnMessageSeq === canonical.turnMessageSeq
  }

  function runtimeUserTurnMatches(
    existing: ChatUserTurn,
    canonical: ChatUserTurn,
    runtimeGeneration: string,
  ): boolean {
    if (hasSameRuntimeRequestStableIdentity(existing, canonical)) return true
    const projectionGeneration = existing.syncState?.generation?.trim()
      || runtimeUserGenerations.get(existing)?.trim()
      || ''
    if (projectionGeneration) return runtimeProjectionGenerationMatches(projectionGeneration, runtimeGeneration)
      && (
        existing.id === canonical.id
        || Boolean(canonical.externalMessageId && existing.externalMessageId === canonical.externalMessageId)
      )
    if (runtimeGeneration.trim() && !existing.__optimistic) return false
    return existing.id === canonical.id
      || Boolean(existing.__optimistic && canonical.externalMessageId && existing.externalMessageId === canonical.externalMessageId)
  }

  function runtimeAssistantTurnMatches(existing: ChatAssistantTurn, runtimeGeneration: string): boolean {
    const projectionGeneration = existing.syncState?.generation?.trim()
      || runtimeAssistantGenerations.get(existing)?.trim()
      || ''
    if (projectionGeneration) return runtimeProjectionGenerationMatches(projectionGeneration, runtimeGeneration)
    return existing.__optimistic === true
  }

  function assistantTurnForRuntimeStream(
    botId: string,
    targetSessionId: string,
    streamId: string,
    runtimeGeneration: string,
    requestUserTurn?: ConversationUiTurn,
  ): ChatAssistantTurn | null {
    const id = streamId.trim()
    if (!id) return null
    const generation = runtimeGeneration.trim()
    const generationID = generation ? `runtime-${id}-${generation}` : ''
    const transcriptMessages = sessionTranscript(botId, targetSessionId).messages
    const runtimeTurn = transcriptMessages.find((turn): turn is ChatAssistantTurn =>
      turn.role === 'assistant'
      && (turn.id === `runtime-${id}` || Boolean(generationID && turn.id === generationID))
      && runtimeAssistantTurnMatches(turn, generation),
    )
    if (runtimeTurn) return runtimeTurn
    const canonicalRequest = normalizeRuntimeUserTurn(requestUserTurn, id, generation, botId, targetSessionId)
    const requestIndex = transcriptMessages.findIndex(turn => turn.role === 'user'
      && (canonicalRequest
        ? runtimeUserTurnMatches(turn, canonicalRequest, generation)
        : turn.__optimistic === true && turn.externalMessageId === id))
    if (requestIndex < 0) return null
    for (let index = requestIndex + 1; index < transcriptMessages.length; index += 1) {
      const turn = transcriptMessages[index]
      if (!turn || turn.role === 'user') break
      if (turn.role === 'assistant' && runtimeAssistantTurnMatches(turn, generation)) return turn
    }
    return null
  }

  function isReusedRuntimeGeneration(streamId: string, runtimeGeneration: string, botId: string, targetSessionId: string): boolean {
    const id = streamId.trim()
    const generation = runtimeGeneration.trim()
    const terminalGeneration = terminalStreamGeneration(id, botId, targetSessionId)?.trim() ?? ''
    return Boolean(id && generation && terminalGeneration && terminalGeneration !== generation)
  }

  function ensureRuntimeStream(
    streamId: string,
    botId: string,
    targetSessionId: string,
    append = true,
    runtimeGeneration = '',
    requestUserTurn?: ConversationUiTurn,
  ): PendingAssistantStream | null {
    const id = streamId.trim()
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!id || !bid || !sid) return null
    const existing = getAssistantStream(id, bid, sid)
    if (existing) return existing

    const reusedGeneration = isReusedRuntimeGeneration(id, runtimeGeneration, bid, sid)
    const approvalId = getApprovalResponse(id, bid, sid)?.approvalId ?? ''
    const userInputId = userInputResponseStreams.get(sidebandResponseKey(bid, sid, id))?.[0]?.userInput.user_input_id ?? ''
    const transcript = sessionTranscript(bid, sid)
    const reusableTurn = reusedGeneration
      ? null
      : (approvalId ? transcript.assistantTurnForApproval(approvalId) : null)
        ?? (userInputId ? transcript.assistantTurnForUserInput(userInputId) : null)
        ?? assistantTurnForRuntimeStream(bid, sid, id, runtimeGeneration, requestUserTurn)
    const baseAssistantTurnID = `runtime-${id}`
    const assistantTurnID = reusedGeneration && transcript.messages.some(turn => turn.id === baseAssistantTurnID)
      ? `${baseAssistantTurnID}-${runtimeGeneration}`
      : baseAssistantTurnID
    const assistantTurn = reusableTurn
      ?? transcript.createOptimisticAssistantTurn(assistantTurnID)
    if (runtimeGeneration.trim()) runtimeAssistantGenerations.set(assistantTurn, runtimeGeneration.trim())
    if (append && !hasTurn(assistantTurn)) appendTurnToSession(bid, sid, assistantTurn)
    void trackAssistantStream({ streamId: id, assistantTurn, botId: bid, sessionId: sid, runtimeGeneration }).catch((error: Error) => {
      if (assistantTurn.messages.some(block => block.type === 'error')) return
      finalizeStreamFailure(assistantTurn, bid, sid, error, runtimeAssistantErrorIdentityFor(id, runtimeGeneration, bid, sid))
    })
    return getAssistantStream(id, bid, sid) ?? null
  }

  function clearUserInputResponseStream(streamId: string, botId: string, targetSessionId: string) {
    const id = streamId.trim()
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const removedResponseState = userInputResponseStreams.delete(sidebandResponseKey(bid, sid, id))
    let removedPendingResponse = false
    for (const [key, pending] of pendingUserInputResponses) {
      if (pending.streamId === id && pending.botId === bid && pending.sessionId === sid) {
        pendingUserInputResponses.delete(key)
        removedPendingResponse = true
      }
    }
    if ((removedResponseState || removedPendingResponse) && id && bid && sid) {
      rememberTerminalUserInputResponse(bid, sid, id)
    }
  }

  function reconcileDisconnectedUserInputResponses(
    snapshot: SessionruntimeSnapshot,
    botId: string,
    targetSessionId: string,
  ) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const runtimeUserInputs = (snapshot.current_run_view?.messages ?? [])
      .map(message => message.user_input)
    for (const pending of pendingUserInputResponses.values()) {
      if (!pending.awaitingResync || pending.botId !== bid || pending.sessionId !== sid) continue
      const runtimeUserInput = runtimeUserInputs.find(userInput => (userInput?.user_input_id ?? '').trim() === pending.userInputId)
      const status = (runtimeUserInput?.status ?? '').trim().toLowerCase()
      if (status && status !== 'pending') {
        clearUserInputResponseStream(pending.streamId, pending.botId, pending.sessionId)
      } else if (status === 'pending') {
        resendPendingUserInputResponse(pending)
      } else {
        requestPersistedHistory(bid, sid)
      }
    }
  }

  function resendPendingUserInputResponse(pending: PendingUserInputResponse) {
    if (!pending.awaitingResync || pending.replaySent) return
    pending.replaySent = true
    sessionTranscript(pending.botId, pending.sessionId)
      .markUserInputDecision(pending.userInputId, pending.canceled ? 'canceled' : 'submitted')
    try {
      if (sendWebSocketMessage(pending.botId, {
        type: 'user_input_response',
        stream_id: pending.streamId,
        session_id: pending.sessionId,
        user_input_id: pending.userInputId,
        short_id: pending.shortId,
        answers: pending.answers,
        canceled: pending.canceled,
        reason: pending.reason,
      })) return
    } catch {
      // Transcript reconciliation below decides whether this response can retry.
    }
    pending.replayFailed = true
    pending.replaySent = false
  }

  function reconcileDisconnectedApprovalResponses(
    snapshot: SessionruntimeSnapshot,
    botId: string,
    targetSessionId: string,
  ) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const runtimeMessages = snapshot.current_run_view?.messages ?? []
    for (const pending of pendingApprovalResponses()) {
      if (!pending.awaitingResync || pending.botId !== bid || pending.sessionId !== sid) continue
      const runtimeApproval = runtimeMessages
        .map(message => message.approval)
        .find(approval => (approval?.approval_id ?? '').trim() === pending.approvalId)
      const status = (runtimeApproval?.status ?? '').trim().toLowerCase()
      if (status && status !== 'pending') {
        settleApprovalResponse(pending.streamId, 'succeeded', pending.botId, pending.sessionId)
      } else if (status === 'pending') {
        resendPendingApprovalResponse(pending)
      } else {
        requestPersistedHistory(bid, sid)
      }
    }
  }

  function reconcileApprovalResponsesFromTranscript(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    const approvals = sessionTranscript(bid, sid).messages
      .filter(message => message.role === 'assistant')
      .flatMap(message => message.messages)
      .map(message => message.type === 'tool' ? message.approval : undefined)
    for (const pending of pendingApprovalResponses()) {
      if (!pending.awaitingResync || pending.botId !== bid || pending.sessionId !== sid) continue
      const approval = approvals.find(item => (item?.approval_id ?? '').trim() === pending.approvalId)
      const status = (approval?.status ?? '').trim().toLowerCase()
      if (status && status !== 'pending') {
        settleApprovalResponse(pending.streamId, 'succeeded', pending.botId, pending.sessionId)
      } else if (status === 'pending' && pending.replayFailed) {
        settleApprovalResponse(pending.streamId, 'failed', pending.botId, pending.sessionId)
      }
    }
  }

  function normalizeRuntimeUserTurn(
    turn: ConversationUiTurn | undefined,
    streamId: string,
    runtimeGeneration = '',
    botId = '',
    targetSessionId = '',
  ): ChatUserTurn | null {
    if (!turn || (turn.role ?? '').trim().toLowerCase() !== 'user') return null
    const normalized = normalizeTurn({
      role: 'user',
      text: turn.text ?? '',
      attachments: (turn.attachments ?? []).map(attachment => ({
        id: attachment.id,
        type: attachment.type ?? '',
        base64: attachment.base64,
        name: attachment.name,
        mime: attachment.mime,
        url: attachment.url,
        path: attachment.path,
        content_hash: attachment.content_hash,
        storage_key: attachment.storage_key,
        size: attachment.size,
        metadata: attachment.metadata,
      })),
      timestamp: turn.timestamp ?? new Date().toISOString(),
      platform: turn.platform,
      sender_display_name: turn.sender_display_name,
      sender_avatar_url: turn.sender_avatar_url,
      sender_user_id: turn.sender_user_id,
      external_message_id: turn.external_message_id?.trim() || streamId,
      reply: turn.reply,
      forward: turn.forward,
      user_message_kind: turn.user_message_kind,
      skill_activation: turn.skill_activation,
      id: turn.id,
      turn_position: turn.turn_position,
      turn_message_seq: turn.turn_message_seq,
    })
    if (!normalized || normalized.role !== 'user') return null
    normalized.serverId = turn.id?.trim() || undefined
    normalized.id = isReusedRuntimeGeneration(streamId, runtimeGeneration, botId, targetSessionId)
      ? `runtime-${streamId}-${runtimeGeneration}-user`
      : `runtime-${streamId}-user`
    if (runtimeGeneration.trim()) runtimeUserGenerations.set(normalized, runtimeGeneration.trim())
    return normalized
  }

  function applyRuntimeRequestUserTurn(
    stream: PendingAssistantStream,
    turn: ConversationUiTurn | undefined,
    status = 'running',
  ) {
    const canonical = normalizeRuntimeUserTurn(
      turn,
      stream.streamId,
      stream.runtimeGeneration,
      stream.botId,
      stream.sessionId,
    )
    if (!canonical) return

    const transcript = sessionTranscript(stream.botId, stream.sessionId)
    transcript.setMessageSyncState(canonical, {
      run: runtimeStatusToChatState(status),
      presence: 'live',
      persistence: 'unknown',
      streamId: stream.streamId,
      generation: stream.runtimeGeneration,
    })
    const transcriptMessages = transcript.messages
    const existing = transcriptMessages.find((message): message is ChatUserTurn => message.role === 'user'
      && runtimeUserTurnMatches(message, canonical, stream.runtimeGeneration))
    if (existing) {
      transcript.adoptRuntimeUserTurn(existing, canonical)
      if (stream.runtimeGeneration) runtimeUserGenerations.set(existing, stream.runtimeGeneration)
      return
    }

    const assistantIndex = transcriptMessages.indexOf(stream.assistantTurn)
    if (assistantIndex >= 0) {
      transcript.insertTurnAt(assistantIndex, canonical)
      return
    }
    appendTurnToSession(stream.botId, stream.sessionId, canonical)
  }

  function prepareRuntimeReplacement(
    botId: string,
    targetSessionId: string,
    operation: SessionruntimeRunOperationView,
    streamId: string,
    runtimeGeneration = '',
    status = 'running',
  ): PreparedRuntimeReplacement | null {
    const kind = (operation.kind ?? '').trim().toLowerCase()
    if (kind !== 'retry' && kind !== 'edit') return null
    const replaceFrom = (operation.replace_from_message_id ?? '').trim()
    if (!replaceFrom) return null
    const target = sessionTranscript(botId, targetSessionId).findTurnByServerId(replaceFrom)
    if (!target || (kind === 'retry' && target.role !== 'assistant') || (kind === 'edit' && target.role !== 'user')) {
      return null
    }
    const optimisticUserTurn = kind === 'edit'
      ? normalizeRuntimeUserTurn(operation.replacement_user_turn, streamId, runtimeGeneration, botId, targetSessionId)
      : null
    if (kind === 'edit' && !optimisticUserTurn) return null
    if (optimisticUserTurn) {
      sessionTranscript(botId, targetSessionId).setMessageSyncState(optimisticUserTurn, {
        run: runtimeStatusToChatState(status),
        presence: 'live',
        persistence: 'unknown',
        streamId,
        generation: runtimeGeneration,
      })
    }
    return { kind, target, optimisticUserTurn }
  }

  function applyRuntimeReplacement(stream: PendingAssistantStream, prepared: PreparedRuntimeReplacement) {
    const existing = stream.runtimeReplacement
    if (existing?.applied) return

    const transcript = sessionTranscript(stream.botId, stream.sessionId)
    const restoreForkAnchor = updateForkAnchorForReplacedMessage(stream.sessionId, prepared.target, transcript.messages)
    const replacedTurns = transcript.replaceTailFromTurn(
      prepared.target,
      prepared.optimisticUserTurn
        ? [prepared.optimisticUserTurn, stream.assistantTurn]
        : [stream.assistantTurn],
    )
    if (existing) {
      existing.kind = prepared.kind
      existing.optimisticUserTurn = prepared.optimisticUserTurn
      existing.replacedTurns = replacedTurns
      existing.restoreForkAnchor = restoreForkAnchor
      existing.applied = true
      return
    }
    stream.runtimeReplacement = {
      kind: prepared.kind,
      optimisticUserTurn: prepared.optimisticUserTurn,
      replacedTurns,
      restoreForkAnchor,
      applied: true,
      historyCommitted: false,
    }
  }

  function restoreRuntimeReplacement(stream: PendingAssistantStream): boolean {
    const replacement = stream.runtimeReplacement
    if (!replacement?.applied || replacement.historyCommitted) return false
    replacement.restoreForkAnchor?.()
    restoreTailFromOptimistic(
      stream.botId,
      stream.sessionId,
      replacement.optimisticUserTurn,
      stream.assistantTurn,
      replacement.replacedTurns,
    )
    replacement.applied = false
    return true
  }

  const committedEmptyRuntimeReplacements = new WeakSet<PendingAssistantStream>()
  function commitEmptyRuntimeReplacement(stream: PendingAssistantStream): boolean {
    if (committedEmptyRuntimeReplacements.has(stream)) return true
    if (!stream.runtimeReplacement?.applied) return false
    stream.runtimeReplacement = undefined
    committedEmptyRuntimeReplacements.add(stream)
    sessionTranscript(stream.botId, stream.sessionId).removeTurnFromSession(
      stream.botId,
      stream.sessionId,
      stream.assistantTurn,
    )
    return true
  }

  const terminalSettlePromises = new Map<string, Promise<void>>()
  const TERMINAL_SETTLE_RETRY_DELAYS_MS = [50, 200] as const

  function runtimeTerminalSettleStatus(status: string): RuntimeTerminalStatus {
    if (status === 'completed' || status === 'aborted') return status
    return 'errored'
  }

  function runtimeStatusToChatState(status: string): ChatRunState {
    switch (status.trim().toLowerCase()) {
      case 'admitting': return 'admitting'
      case 'aborting': return 'aborting'
      case 'completed': return 'completed'
      case 'aborted': return 'aborted'
      case 'errored': return 'errored'
      case 'lost':
      case 'interrupted': return 'lost'
      default: return 'running'
    }
  }

  function runtimeTerminalTurnId(rowLedger: UIRowIdentity[], runtimeMessages: UIMessage[]): string {
    const turnIds = new Set<string>()
    const collect = (row: UIRowIdentity) => {
      const turnId = row.turn_id?.trim() ?? ''
      if (turnId) turnIds.add(turnId)
    }
    rowLedger.forEach(collect)
    runtimeMessages.forEach(message => message.row_identities?.forEach(collect))
    return turnIds.size === 1 ? [...turnIds][0]! : ''
  }

  function terminalSettleKey(botId: string, targetSessionId: string, streamId: string, generation: string) {
    return `${botId.trim()}\u0000${targetSessionId.trim()}\u0000${streamId.trim()}\u0000${generation.trim()}`
  }

  function settleProjectedRuntime(
    botId: string,
    streamId: string,
    targetSessionId: string,
    status: string,
    error: string,
    runtimeMessages: UIMessage[],
    runtimeRowLedger: UIRowIdentity[],
    generation: string,
    runtimeRequest?: ConversationUiTurn,
  ): Promise<void> {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const id = streamId.trim()
    const runGeneration = generation.trim()
    const key = terminalSettleKey(bid, sid, id, runGeneration)
    const settleUserScopeGeneration = userScopeGeneration
    persistedHistory.cancel({ botId: bid, sessionId: sid })
    const current = terminalSettlePromises.get(key)
    if (current) return current

    const promise = (async () => {
      const settleStatus = runtimeTerminalSettleStatus(status)
      const transcript = sessionTranscript(bid, sid)
      const terminalTurnId = runtimeTerminalTurnId(runtimeRowLedger, runtimeMessages)
      const candidateIntent = chatIntents.get(bid, sid, id)
      const intent = candidateIntent && (!runGeneration || candidateIntent.generation === runGeneration)
        ? candidateIntent
        : null
      const candidateStream = getAssistantStream(id, bid, sid)
      const streamBeforeSettle = candidateStream && (!runGeneration || candidateStream.runtimeGeneration === runGeneration)
        ? candidateStream
        : undefined
      const runtimeKey = acpRuntimeKey(bid, sid)
      const settleScopeIsCurrent = () => settleUserScopeGeneration === userScopeGeneration
      const enqueueSettleMutation = (mutation: () => void) => {
        if (!settleScopeIsCurrent()) return
        return runtimeSessions.enqueue(runtimeKey, (runtimeSession) => {
          if (settleScopeIsCurrent() && runtimeSession.isCurrent()) mutation()
        })
      }
      const removeSettledIntentPlaceholder = () => {
        if (streamBeforeSettle?.runtimeReplacement) restoreRuntimeReplacement(streamBeforeSettle)
        const userTurn = intent?.userTurnKey
          ? transcript.messages.find(turn => turn.id === intent.userTurnKey && turn.role === 'user')
          : undefined
        const assistantTurn = streamBeforeSettle?.assistantTurn
          ?? (intent
            ? transcript.messages.find((turn): turn is ChatAssistantTurn =>
                turn.id === intent.assistantTurnKey && turn.role === 'assistant')
            : undefined)
        if (userTurn) transcript.removeTurnFromSession(bid, sid, userTurn)
        if (assistantTurn) transcript.removeTurnFromSession(bid, sid, assistantTurn)
      }
      if (streamBeforeSettle) {
        if (status === 'completed') {
          resolveAssistantStream(id, bid, sid)
        } else {
          rejectAssistantStream(
            id,
            runtimeStatusError(status, error || status, streamBeforeSettle.assistantTurn, true),
            bid,
            sid,
          )
        }
        refreshLoadingForSession(bid, sid)
      }
      if (status === 'completed' && runtimeMessages.length === 0 && streamBeforeSettle) {
        await enqueueSettleMutation(() => {
          commitEmptyRuntimeReplacement(streamBeforeSettle)
        })
      }
      const settleOnce = () => settleRuntimeTerminal({ status: settleStatus, runtimeMessages, rowLedger: runtimeRowLedger }, {
          fetchHistory: () => fetchMessagesUI(bid, sid, terminalTurnId
            ? { turnId: terminalTurnId }
            : { limit: 30 }),
          replacePersisted: (match) => {
            return enqueueSettleMutation(() => transcript.mergePersistedTurn(match.history, sid))
          },
          removeOptimistic: () => {
            return enqueueSettleMutation(() => {
              if (settleStatus === 'completed' && streamBeforeSettle && commitEmptyRuntimeReplacement(streamBeforeSettle)) return
              if (streamBeforeSettle?.runtimeReplacement && restoreRuntimeReplacement(streamBeforeSettle)) return
              if (intent) {
                removeSettledIntentPlaceholder()
                return
              }
              if (!streamBeforeSettle) return
              const request = transcript.messages.find(turn =>
                turn.role === 'user'
                && turn.externalMessageId === id
                && turn.syncState?.persistence !== 'persisted')
              if (request) transcript.removeTurnFromSession(bid, sid, request)
              transcript.removeTurnFromSession(bid, sid, streamBeforeSettle.assistantTurn)
            })
          },
          restoreDraft: () => {
            return enqueueSettleMutation(() => {
              if (!intent || intent.kind !== 'send') return
              publishComposerDraftRestore({
                botId: intent.botId,
                sessionId: intent.sessionId,
                viewId: intent.viewId,
                composerScope: intent.composerScope,
                text: intent.text,
                attachments: intent.attachments,
                requestedSkills: intent.requestedSkills,
              })
            })
          },
          keepPartial: () => {},
        })
      let result: Awaited<ReturnType<typeof settleRuntimeTerminal>> | undefined
      let settleError: unknown
      for (let attempt = 0; attempt <= TERMINAL_SETTLE_RETRY_DELAYS_MS.length; attempt += 1) {
        try {
          result = await settleOnce()
          break
        } catch (error) {
          settleError = error
          if (!settleScopeIsCurrent()) return
          const delay = TERMINAL_SETTLE_RETRY_DELAYS_MS[attempt]
          if (delay === undefined) break
          await new Promise(resolve => setTimeout(resolve, delay))
          if (!settleScopeIsCurrent()) return
        }
      }
      if (!result) {
        console.error(`Failed to settle runtime ${bid}/${sid}/${id}:`, settleError)
        result = { status: settleStatus, persistence: 'unknown', action: 'keep_partial' }
      }

      if (!settleScopeIsCurrent()) return

      const stableIds = result.persistence === 'persisted'
        ? result.match.ledger.map(row => row.stableId)
        : []
      await enqueueSettleMutation(() => {
        chatIntents.terminal({
          botId: bid,
          sessionId: sid,
          streamId: id,
          generation: runGeneration,
          status: status === 'interrupted' || status === 'lost' ? status : settleStatus,
          persistence: result.persistence,
          stableIds,
        }, { effectsApplied: true })

        const requestStableId = (runtimeRequest?.id ?? '').trim()
        const request = transcript.messages.find((turn): turn is ChatUserTurn => {
          if (turn.role !== 'user') return false
          if (requestStableId && (turn.serverId === requestStableId || turn.id === requestStableId)) return true
          if (
            Number.isSafeInteger(runtimeRequest?.turn_position)
            && Number.isSafeInteger(runtimeRequest?.turn_message_seq)
            && turn.turnPosition === runtimeRequest?.turn_position
            && turn.turnMessageSeq === runtimeRequest?.turn_message_seq
          ) return true
          return turn.syncState?.streamId === id
            && (!runGeneration || turn.syncState.generation === runGeneration)
        })
        if (request) {
          transcript.setMessageSyncState(request, {
            run: runtimeStatusToChatState(status),
            presence: result.persistence === 'persisted' ? 'settled' : 'live',
            persistence: result.persistence,
            streamId: id,
            generation: runGeneration,
          })
        }
        if (streamBeforeSettle && result.persistence === 'persisted') {
          const persisted = findSettledAssistant(transcript.messages, runtimeMessages)
          if (persisted) {
            streamBeforeSettle.assistantTurn = persisted
            streamBeforeSettle.runtimeReplacement = undefined
            transcript.setMessageSyncState(persisted, {
              run: runtimeStatusToChatState(status),
              presence: 'settled',
              persistence: 'persisted',
              streamId: id,
              generation: runGeneration,
            })
          }
        } else if (streamBeforeSettle && transcript.messages.includes(streamBeforeSettle.assistantTurn)) {
          transcript.setMessageSyncState(streamBeforeSettle.assistantTurn, {
            run: runtimeStatusToChatState(status),
            presence: 'live',
            persistence: result.persistence,
            streamId: id,
            generation: runGeneration,
          })
        }
        releaseHiddenSessionView(chatViews.getSession(bid, sid) ?? null)
      })
    })().finally(() => {
      terminalSettlePromises.delete(key)
    })
    terminalSettlePromises.set(key, promise)
    return promise
  }

  function applyRuntimeSnapshot(
    runtimeSession: RuntimeSessionQueueHandle,
    snapshot: SessionruntimeSnapshot,
    botId: string,
    targetSessionId: string,
    eventSeq?: number,
    eventEpoch?: string,
    allowHistoryReplay = true,
  ) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return { kind: 'ignored' as const }
    if ((snapshot.bot_id ?? '').trim() !== bid || (snapshot.session_id ?? '').trim() !== sid) {
      return { kind: 'resync' as const }
    }
    const reduced = runtimeSession.applySnapshot(
      snapshot,
      eventSeq,
      eventEpoch,
    )
    if (!reduced) return { kind: 'ignored' as const }
    const { previous: previousState, reduction } = reduced
    if (reduction.kind === 'resync') {
      return { kind: 'resync' as const }
    }
    if (reduction.kind !== 'applied' || !reduction.state.snapshot) return { kind: 'ignored' as const }
    reconcileDisconnectedApprovalResponses(reduction.state.snapshot, bid, sid)
    handleRuntimeGenerationChange(
      previousState.snapshot?.current_run_view,
      reduction.state.snapshot.current_run_view,
      bid,
      sid,
    )
    projectRuntimeSnapshot(reduction.state.snapshot, bid, sid, reduction.state.seq, true, allowHistoryReplay)
    reconcileDisconnectedUserInputResponses(reduction.state.snapshot, bid, sid)
    return { kind: 'applied' as const }
  }

  function handleRuntimeGenerationChange(
    previousRun: SessionruntimeSnapshot['current_run_view'],
    nextRun: SessionruntimeSnapshot['current_run_view'],
    botId: string,
    targetSessionId: string,
  ) {
    const previousStreamId = (previousRun?.stream_id ?? '').trim()
    const nextStreamId = (nextRun?.stream_id ?? '').trim()
    const previousGeneration = (previousRun?.generation ?? '').trim()
    const nextGeneration = (nextRun?.generation ?? '').trim()
    if (!previousStreamId || previousStreamId !== nextStreamId || !previousGeneration || !nextGeneration || previousGeneration === nextGeneration) return

    const stream = getAssistantStream(nextStreamId, botId, targetSessionId)
    if (stream && stream.runtimeGeneration !== nextGeneration) {
      settleApprovalResponse(nextStreamId, 'failed', botId, targetSessionId)
      pruneEmptyAssistantTurnIfPending(nextStreamId, botId, targetSessionId)
      discardAssistantStream(nextStreamId, botId, targetSessionId)
    }
    refreshLoadingForSession(botId, targetSessionId)
  }

  function projectRuntimeSnapshot(
    snapshot: SessionruntimeSnapshot,
    bid: string,
    sid: string,
    acceptedSeq: number | undefined,
    authoritativeSnapshot: boolean,
    allowHistoryReplay = true,
    replayTerminalProjection = false,
  ) {
    const run = snapshot.current_run_view
    const streamId = (run?.stream_id ?? '').trim()
    const runGeneration = (run?.generation ?? '').trim()
    const transcriptMessages = sessionTranscript(bid, sid).messages
    if (!run || !streamId) {
      rejectRuntimeStreamsForSession(bid, sid)
      loading.value = isSessionStreaming(currentBotId.value, sessionId.value)
      return
    }

    if (authoritativeSnapshot) {
      rejectSupersededRuntimeStreams(bid, sid, streamId)
    }

    const status = (run.status ?? '').trim().toLowerCase()
    if (runGeneration) {
      chatIntents.claimRun({ botId: bid, sessionId: sid, streamId, generation: runGeneration })
    }
    const runtimeMessages = projectRuntimeMessages(run.messages ?? [])
    for (const message of run.messages ?? []) {
      notifyRuntimeGuiToolUse(message, bid, sid, streamId)
    }
    const terminalFailure = isRuntimeTerminalStatus(status) && status !== 'completed'
    const terminalErrorMessage = terminalFailure
      ? runtimeStatusError(status, run.error ?? status).message
      : ''
    if (replayTerminalProjection && terminalFailure) {
      const errorIdentity = runtimeAssistantErrorIdentityFor(streamId, runGeneration, bid, sid)
      const replayedErrorTurn = errorIdentity
        ? assistantTurnForRuntimeError(sid, errorIdentity)
        : null
      if (replayedErrorTurn) {
        if (runtimeMessages.length > 0) {
          sessionTranscript(bid, sid).clearAssistantMessages(replayedErrorTurn)
          for (const message of runtimeMessages) upsertAssistantUIMessage(replayedErrorTurn, message)
          appendAssistantError(replayedErrorTurn, sid, terminalErrorMessage, false, errorIdentity)
        }
        sessionTranscript(bid, sid).setAssistantStreaming(replayedErrorTurn, false)
        loading.value = isSessionStreaming(bid, sid)
        return
      }
      forgetTerminalStream(streamId, bid, sid)
    }
    const operation = run.operation
    let stream: PendingAssistantStream | null | undefined = getAssistantStream(streamId, bid, sid)
    if (!stream && isRuntimeTerminalStatus(status) && findSettledAssistant(transcriptMessages, runtimeMessages)) {
      void settleProjectedRuntime(
        bid,
        streamId,
        sid,
        status,
        run.error ?? status,
        runtimeMessages,
        run.row_ledger ?? [],
        runGeneration,
        run.request_user_turn ?? operation?.replacement_user_turn,
      )
      loading.value = isSessionStreaming(bid, sid)
      return
    }
    if (stream) {
      markRuntimeStreamObserved(stream, run.generation ?? '')
      if (stream.runtimeGeneration) runtimeAssistantGenerations.set(stream.assistantTurn, stream.runtimeGeneration)
    }
    if (stream?.abortRequested && isRuntimeActiveStatus(status)) {
      abortWebSocketStream(streamId, bid, sid)
    }
    if (stream && !transcriptMessages.includes(stream.assistantTurn) && stream.runtimeReplacement?.applied) {
      stream.runtimeReplacement = undefined
    }
    let preparedReplacement: PreparedRuntimeReplacement | null = null
    if (operation && !stream?.runtimeReplacement?.applied) {
      preparedReplacement = prepareRuntimeReplacement(bid, sid, operation, streamId, runGeneration, status)
      if (!preparedReplacement) {
        if (isRuntimeTerminalStatus(status)) {
          void settleProjectedRuntime(bid, streamId, sid, status, run.error ?? status, runtimeMessages, run.row_ledger ?? [], runGeneration, run.request_user_turn ?? operation.replacement_user_turn)
          loading.value = isSessionStreaming(bid, sid)
          return
        }
        if (!stream && isRuntimeActiveStatus(status) && !isTerminalStream(streamId, runGeneration, bid, sid)) {
          stream = ensureRuntimeStream(streamId, bid, sid, false, runGeneration, run.request_user_turn)
        }
        const key = acpRuntimeKey(bid, sid)
        if (allowHistoryReplay) {
          if (!sessionTranscript(bid, sid).loadingMessages.value) {
            void persistedHistory.reconcile({ botId: bid, sessionId: sid }).catch((error) => {
              console.error('Failed to hydrate runtime replacement target:', error)
              if (acceptedSeq === undefined || runtimeSessions.get(key)?.seq !== acceptedSeq) return
              const hydrationError = new StreamFailureError('Failed to hydrate runtime replacement history', 'startup')
              if (
                stream
                && getAssistantStream(streamId, bid, sid) === stream
                && (!runGeneration || stream.runtimeGeneration === runGeneration)
              ) {
                rejectAssistantStream(streamId, hydrationError, bid, sid)
              }
              loading.value = isSessionStreaming(currentBotId.value, sessionId.value)
            })
          }
        } else if (stream) {
          if (status === 'completed') {
            resolveAssistantStream(streamId, bid, sid)
          } else {
            rejectAssistantStream(streamId, runtimeInactiveError(stream.assistantTurn), bid, sid)
          }
        }
        loading.value = allowHistoryReplay
          ? isRuntimeActiveStatus(status) || isSessionStreaming(bid, sid)
          : isSessionStreaming(bid, sid)
        return
      }
      if (!stream && isTerminalStream(streamId, runGeneration, bid, sid)) {
        if (status === 'completed' || runtimeMessages.length === 0) {
          loading.value = isSessionStreaming(bid, sid)
          return
        }
        stream = ensureRuntimeStream(streamId, bid, sid, false, runGeneration, run.request_user_turn)
      }
    }
    const replacementAwaitingCommit = Boolean(operation && run.history_committed !== true)
    if (!stream && !replacementAwaitingCommit && !isTerminalStream(streamId, runGeneration, bid, sid) && (isRuntimeActiveStatus(status) || runtimeMessages.length > 0 || terminalFailure || preparedReplacement)) {
      stream = ensureRuntimeStream(streamId, bid, sid, !operation, runGeneration, run.request_user_turn)
      if (stream) {
        markRuntimeStreamObserved(stream, run.generation ?? '')
        if (stream.runtimeGeneration) runtimeAssistantGenerations.set(stream.assistantTurn, stream.runtimeGeneration)
      }
    }
    if (stream && preparedReplacement && run.history_committed === true) {
      applyRuntimeReplacement(stream, preparedReplacement)
    }
    if (stream?.runtimeReplacement && run.history_committed === true) {
      stream.runtimeReplacement.historyCommitted = true
    }

    if (stream) {
      applyRuntimeRequestUserTurn(stream, run.request_user_turn, status)
      sessionTranscript(bid, sid).setMessageSyncState(stream.assistantTurn, {
        run: runtimeStatusToChatState(status),
        presence: 'live',
        persistence: 'unknown',
        streamId,
        generation: runGeneration,
      })
      reattachRuntimeAssistantTurn(stream)
      const persistedAssistantRow = runtimeMessages.find(message => message.stable_id?.trim())
      if (persistedAssistantRow) {
        sessionTranscript(bid, sid).setAssistantRowIdentity(stream.assistantTurn, {
          stableId: persistedAssistantRow.stable_id,
          turnPosition: persistedAssistantRow.turn_position,
          turnMessageSeq: persistedAssistantRow.turn_message_seq,
        })
      }
      if (authoritativeSnapshot) {
        stream.runtimeMessageIds = replaceAssistantUIMessageSnapshot(
          stream.assistantTurn,
          runtimeMessages,
          stream.runtimeMessageIds,
        ) ?? new Set<number>()
      } else {
        for (const message of runtimeMessages) {
          const messageId = upsertAssistantUIMessage(stream.assistantTurn, message)
          if (messageId !== undefined) stream.runtimeMessageIds.add(messageId)
        }
      }
      sessionTranscript(bid, sid).setAssistantStreaming(stream.assistantTurn, isRuntimeActiveStatus(status))
    }

    if (isRuntimeActiveStatus(status)) {
      loading.value = true
      return
    }

    if (isRuntimeTerminalStatus(status)) {
      settleApprovalResponse(streamId, status === 'completed' ? 'succeeded' : 'failed', bid, sid)
      if (stream && terminalFailure) {
        projectRuntimeTerminalError(
          stream,
          bid,
          sid,
          terminalErrorMessage,
          status !== 'aborted' || Boolean(chatIntents.get(bid, sid, streamId)),
        )
      }
      void settleProjectedRuntime(bid, streamId, sid, status, run.error ?? status, runtimeMessages, run.row_ledger ?? [], runGeneration, run.request_user_turn ?? operation?.replacement_user_turn)
      loading.value = isSessionStreaming(currentBotId.value, sessionId.value)
    }
  }

  function reattachRuntimeAssistantTurn(stream: PendingAssistantStream) {
    const transcriptMessages = sessionTranscript(stream.botId, stream.sessionId).messages
    if (transcriptMessages.includes(stream.assistantTurn)) return
    const hydratedIndex = transcriptMessages.findIndex(message => message.id === stream.assistantTurn.id)
    if (hydratedIndex >= 0) {
      sessionTranscript(stream.botId, stream.sessionId).replaceTurnAt(hydratedIndex, stream.assistantTurn)
      return
    }
    const requestIndex = runtimeRequestIndexForStream(stream)
    if (requestIndex >= 0) {
      sessionTranscript(stream.botId, stream.sessionId).insertTurnAt(requestIndex + 1, stream.assistantTurn)
      return
    }
    appendTurnToSession(stream.botId, stream.sessionId, stream.assistantTurn)
  }

  function runtimeRequestIndexForStream(stream: PendingAssistantStream): number {
    const transcriptMessages = sessionTranscript(stream.botId, stream.sessionId).messages
    for (let index = transcriptMessages.length - 1; index >= 0; index -= 1) {
      const turn = transcriptMessages[index]
      if (turn?.role !== 'user' || turn.externalMessageId !== stream.streamId) continue
      const generation = runtimeUserGenerations.get(turn)?.trim() ?? ''
      if (stream.runtimeGeneration && generation === stream.runtimeGeneration) return index
      if (!stream.runtimeGeneration && turn.__optimistic) return index
    }
    return -1
  }

  function projectRuntimeTerminalError(
    stream: PendingAssistantStream,
    botId: string,
    targetSessionId: string,
    message: string,
    restoreEmptyReplacement = false,
  ) {
    const transcriptMessages = sessionTranscript(botId, targetSessionId).messages
    const restoredEmptyReplacement = restoreEmptyReplacement
      && !hasVisibleAssistantBlocks(stream.assistantTurn)
      && restoreRuntimeReplacement(stream)
    let target = transcriptMessages.includes(stream.assistantTurn) ? stream.assistantTurn : null
    const restoredReplacement = restoredEmptyReplacement
      || Boolean(stream.runtimeReplacement && !stream.runtimeReplacement.applied)
    if (!target && stream.runtimeReplacement?.applied) {
      for (let index = transcriptMessages.length - 1; index >= 0; index -= 1) {
        const turn = transcriptMessages[index]
        if (turn?.role === 'assistant') {
          target = turn
          break
        }
      }
    }
    if (!target) {
      target = stream.assistantTurn
      const requestIndex = runtimeRequestIndexForStream(stream)
      if (requestIndex >= 0) {
        sessionTranscript(botId, targetSessionId).insertTurnAt(requestIndex + 1, target)
      } else {
        appendTurnToSession(botId, targetSessionId, target)
      }
    }
    sessionTranscript(botId, targetSessionId).setAssistantStreaming(target, false)
    if (!target.messages.some(block => block.type === 'error' && block.content === message)) {
      appendAssistantError(
        target,
        targetSessionId,
        message,
        restoredReplacement,
        runtimeAssistantErrorIdentityFor(stream.streamId, stream.runtimeGeneration, stream.botId, stream.sessionId),
      )
    }
  }


  function handleWSSessionCreated(event: { stream_id?: string; session_id: string }, sourceBotId = '') {
    const eventSessionId = event.session_id.trim()
    const candidateBotId = (sourceBotId || currentBotId.value || '').trim()
    if (isTerminalStream(event.stream_id, undefined, candidateBotId, '') || isTerminalApprovalResponse(event.stream_id, candidateBotId, '')) return
    const pending = event.stream_id ? getAssistantStream(event.stream_id, candidateBotId, '') : undefined
    const bid = (pending?.botId || candidateBotId).trim()
    if (!bid || !eventSessionId) return
    const sid = recordCreatedSession(event.stream_id, eventSessionId, bid)
    if (!sid) return
    if (event.stream_id) chatIntents.rebindSession(bid, event.stream_id, sid)
    if (pending) {
      clearRuntimeAdmissionTimeout(pending.streamId, bid, '')
      armRuntimeAdmissionTimeout(pending.streamId, bid, sid)
    }
    const viewId = pending?.viewId?.trim() || focusedChatViewId.value
    const promoted = promoteDraftChatView({ botId: bid, sessionId: null, viewId }, sid)
    subscribeRuntime(bid, sid)
    reconcileRuntimeSubscriptions(bid)
    if (pending?.abortRequested) abortWebSocketStream(pending.streamId, bid, sid)
    if ((currentBotId.value ?? '').trim() !== bid) return

    const now = new Date().toISOString()
    if (!knownSessionSummary(sid)) {
      upsertSession({
        id: sid,
        bot_id: bid,
        type: 'chat',
        session_mode: 'chat',
        runtime_type: 'model',
        title: provisionalSessionTitle(promoted.transcript.latestOptimisticUserText()),
        created_at: now,
        updated_at: now,
      })
    }
    userSentInSession.value = {
      id: sid,
      botId: bid,
      viewId,
      wasDraft: true,
      seq: ++userSendSeq,
    }
    if (focusedChatViewId.value !== viewId) return
    if (sessionId.value && sessionId.value !== sid) return
    sessionId.value = sid
    explicitSessionSelection.value = true
    draftIntent.value = false
  }

  function rememberStartupSendFailure(failure: Omit<StartupSendFailure, 'id'>) {
    const stored: StartupSendFailure = {
      ...failure,
      id: nextId(),
      restoreAttachments: failure.restoreAttachments ? [...failure.restoreAttachments] : undefined,
      restoreRequestedSkills: failure.restoreRequestedSkills ? failure.restoreRequestedSkills.map(skill => ({ ...skill })) : undefined,
    }
    const key = startupSendFailureKey(failure.botId, failure.sessionId, failure.composerScope)
    startupSendFailures.value = { ...startupSendFailures.value, [key]: stored }
  }

  function clearStartupSendFailure(id?: string) {
    if (!id) {
      startupSendFailures.value = {}
      return
    }
    const next = { ...startupSendFailures.value }
    for (const [key, failure] of Object.entries(next)) {
      if (failure.id === id) delete next[key]
    }
    startupSendFailures.value = next
  }

  function clearComposerDraftRestore(seq: number) {
    const next = { ...composerDraftRestores.value }
    for (const [key, queue] of Object.entries(next)) {
      const remaining = queue.filter(restore => restore.seq !== seq)
      if (remaining.length === queue.length) continue
      if (remaining.length > 0) next[key] = remaining
      else delete next[key]
      composerDraftRestores.value = next
      return
    }
  }

  function pruneEmptyAssistantTurnIfPending(streamId: string, botId?: string, targetSessionId?: string) {
    const session = getAssistantStream(streamId, botId, targetSessionId)
    if (!session) return
    const turn = session.assistantTurn
    if (turn.messages.length > 0) return
    if (restoreRuntimeReplacement(session)) return
    removeTurnFromSession(session.botId, session.sessionId, turn)
  }

  function handleExpiredApprovalResponse(response: ApprovalResponse) {
    abortWebSocketStream(response.streamId, response.botId, response.sessionId)
    const stream = getAssistantStream(response.streamId, response.botId, response.sessionId)
    if (stream) {
      const turn = stream.assistantTurn
      discardAssistantStream(response.streamId, response.botId, response.sessionId)
      if (turn.messages.length === 0) {
        removeTurnFromSession(response.botId, response.sessionId, turn)
      }
    }
    refreshLoadingForSession(response.botId, response.sessionId)
  }

  function applyRuntimeMessageAppend(
    snapshot: SessionruntimeSnapshot,
    stream: PendingAssistantStream | undefined,
    append: NonNullable<NonNullable<UIRuntimeDeltaEvent['delta']>['message_appends']>[number],
  ) {
    const run = snapshot.current_run_view
    if (!run) return
    const runtimeMessages = run.messages ?? []
    const runtimeMessage = runtimeMessages.find(message => message.id === append.id && message.type === append.type)
    if (!runtimeMessage) return
    if (!stream) return

    if (
      (append.type === 'text' || append.type === 'reasoning')
      && sessionTranscript(stream.botId, stream.sessionId)
        .appendAssistantContent(stream.assistantTurn, append.id, append.type, append.content)
    ) {
      stream.runtimeMessageIds.add(append.id)
      return
    }
    const normalized = projectRuntimeMessage(runtimeMessage, runtimeMessages.indexOf(runtimeMessage))
    const incremental = normalized && (normalized.type === 'text' || normalized.type === 'reasoning')
      ? { ...normalized, content: append.content }
      : normalized
    if (incremental) {
      const messageId = upsertAssistantUIMessage(stream.assistantTurn, incremental)
      if (messageId !== undefined) stream.runtimeMessageIds.add(messageId)
    }
  }

  function applyRuntimeProgressAppend(
    snapshot: SessionruntimeSnapshot,
    stream: PendingAssistantStream | undefined,
    append: NonNullable<NonNullable<UIRuntimeDeltaEvent['delta']>['progress_appends']>[number],
  ) {
    const run = snapshot.current_run_view
    if (!run) return
    const runtimeMessages = run.messages ?? []
    const runtimeMessage = runtimeMessages.find(message => message.id === append.id)
    if (!runtimeMessage) return
    if (!stream) return

    const appended = sessionTranscript(stream.botId, stream.sessionId).appendToolProgress(
      stream.assistantTurn,
      append.id,
      append.progress,
      { present: 'input' in append, value: 'input' in append ? append.input : undefined },
    )
    if (!appended) {
      const incrementalMessage = {
        ...runtimeMessage,
        progress: [append.progress],
        ...('input' in append ? { input: append.input } : {}),
      }
      const normalized = projectRuntimeMessage(incrementalMessage, runtimeMessages.indexOf(runtimeMessage))
      if (normalized) {
        const messageId = upsertAssistantUIMessage(stream.assistantTurn, normalized)
        if (messageId !== undefined) stream.runtimeMessageIds.add(messageId)
      }
      return
    }
    stream.runtimeMessageIds.add(append.id)
  }

  function applyRuntimeMessageUpsert(
    snapshot: SessionruntimeSnapshot,
    stream: PendingAssistantStream | undefined,
    incoming: ConversationUiMessage,
  ) {
    const run = snapshot.current_run_view
    if (!run) return
    const runtimeMessages = run.messages ?? []
    const index = runtimeMessages.findIndex(message => message.id === incoming.id)
    const runtimeMessage = index >= 0 ? runtimeMessages[index]! : incoming
    notifyRuntimeGuiToolUse(runtimeMessage, snapshot.bot_id ?? '', snapshot.session_id ?? '', run.stream_id ?? '')
    if (!stream) return
    const normalized = projectRuntimeMessage(runtimeMessage, index >= 0 ? index : runtimeMessages.length - 1)
    if (normalized) {
      const messageId = upsertAssistantUIMessage(stream.assistantTurn, normalized)
      if (messageId !== undefined) stream.runtimeMessageIds.add(messageId)
    }
  }

  function notifyRuntimeGuiToolUse(
    message: ConversationUiMessage,
    botId: string,
    targetSessionId: string,
    streamId: string,
  ) {
    if (message.type !== 'tool' || message.running !== true || !isGuiToolName(message.name ?? '')) return
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    const toolCallId = message.tool_call_id?.trim() ?? ''
    const toolName = message.name?.trim() ?? ''
    const dedupeKey = `${bid}:${sid}:${toolCallId || `${streamId.trim()}:${message.id}:${toolName}`}`
    if (seenGuiToolCalls.has(dedupeKey)) return
    seenGuiToolCalls.add(dedupeKey)
    guiToolUseRequested.value = {
      botId: bid,
      sessionId: sid,
      toolCallId,
      toolName,
      seq: ++guiToolUseRequestSeq,
    }
  }

  function applyRuntimeDelta(
    runtimeSession: RuntimeSessionQueueHandle,
    event: UIRuntimeDeltaEvent,
    botId: string,
    targetSessionId: string,
  ) {
    const bid = (event.bot_id ?? botId).trim()
    const sid = (event.session_id ?? targetSessionId).trim()
    const delta = event.delta
    if (!bid || !sid || !delta) return { kind: 'ignored' as const }

    const reduced = runtimeSession.applyDelta(event, bid, sid)
    if (!reduced) return { kind: 'ignored' as const }
    const { previous: previousState, reduction } = reduced
    if (reduction.kind === 'ignored') return { kind: 'ignored' as const }
    if (reduction.kind !== 'resync' && reduction.state.snapshot) {
      handleRuntimeGenerationChange(
        previousState.snapshot?.current_run_view,
        reduction.state.snapshot.current_run_view,
        bid,
        sid,
      )
    }
    if (reduction.kind === 'resync' || !reduction.state.snapshot) {
      return { kind: 'resync' as const }
    }
    const snapshot = reduction.state.snapshot
    if (delta.current_run_view) {
      projectRuntimeSnapshot(snapshot, bid, sid, reduction.state.seq, true)
      return { kind: 'applied' as const }
    }

    const run = snapshot.current_run_view
    if (!run) return { kind: 'applied' as const }
    const streamId = (run.stream_id ?? '').trim()
    const stream = streamId ? getAssistantStream(streamId, bid, sid) : undefined
    if (stream) {
      markRuntimeStreamObserved(stream, run.generation ?? '')
    }

    if (delta.reset_messages) {
      if (stream) {
        replaceAssistantUIMessageSnapshot(stream.assistantTurn, [], stream.runtimeMessageIds)
        stream.runtimeMessageIds = new Set<number>()
      }
    }
    for (const append of delta.message_appends ?? []) {
      applyRuntimeMessageAppend(snapshot, stream, append)
    }
    for (const append of delta.progress_appends ?? []) {
      applyRuntimeProgressAppend(snapshot, stream, append)
    }
    for (const message of delta.message_upserts ?? []) {
      applyRuntimeMessageUpsert(snapshot, stream, message)
    }

    const patch = delta.run
    if (patch) {
      projectRuntimeSnapshot(snapshot, bid, sid, reduction.state.seq, false)
      return { kind: 'applied' as const }
    }

    if (stream) {
      sessionTranscript(bid, sid).setAssistantStreaming(
        stream.assistantTurn,
        isRuntimeActiveStatus(run.status),
      )
    }
    refreshLoadingForSession(bid, sid)
    return { kind: 'applied' as const }
  }

  function applyRuntimeSubscriptionEvent(
    ref: RuntimeSubscriptionRef,
    event: Exclude<UIRuntimeStateEvent, { type: 'runtime_dropped' }>,
  ) {
    const snapshot = 'snapshot' in event ? event.snapshot ?? null : null
    const declaredBotId = (event.bot_id ?? '').trim()
    const declaredSessionId = (event.session_id ?? '').trim()
    if (!declaredBotId || !declaredSessionId) return { kind: 'resync' as const }
    if (declaredBotId !== ref.botId || declaredSessionId !== ref.sessionId) return { kind: 'resync' as const }
    const payloadBotId = (snapshot?.bot_id ?? '').trim()
    const payloadSessionId = (snapshot?.session_id ?? '').trim()
    const payloadStreamId = (snapshot?.current_run_view?.stream_id ?? '').trim()
    const envelopeStreamId = (event.stream_id ?? '').trim()
    if (
      (payloadBotId && ref.botId !== payloadBotId)
      || (payloadSessionId && ref.sessionId !== payloadSessionId)
      || (envelopeStreamId && payloadStreamId && envelopeStreamId !== payloadStreamId)
      || (snapshot && envelopeStreamId && !snapshot.current_run_view)
    ) {
      return { kind: 'resync' as const }
    }

    const key = acpRuntimeKey(ref.botId, ref.sessionId)
    const queued = runtimeSessions.enqueue(key, (runtimeSession) => {
      if (event.type === 'runtime_delta') {
        return applyRuntimeDelta(runtimeSession, event, ref.botId, ref.sessionId)
      }
      if (!snapshot) return { kind: 'ignored' as const }
      return applyRuntimeSnapshot(
        runtimeSession,
        snapshot,
        ref.botId,
        ref.sessionId,
        event.seq,
        event.epoch,
      )
    })
    if (queued && typeof (queued as Promise<unknown>).then === 'function') {
      return (queued as Promise<ReturnType<typeof applyRuntimeSnapshot> | undefined>)
        .then(result => result ?? { kind: 'ignored' as const })
    }
    return queued ?? { kind: 'ignored' as const }
  }

  function handleWSStreamEvent(event: UIStreamEvent, targetSessionId?: string, sourceBotId = '') {
    if (runtimeSubscriptionController.handleEvent(sourceBotId, event)) return

    if (event.type === 'session_created') {
      handleWSSessionCreated(event, sourceBotId)
      return
    }
    if (event.type === 'user_message') {
      const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
      const bid = sourceBotId || currentBotId.value || ''
      const streamId = (event.stream_id ?? '').trim()
      if (!streamId) return
      if (isTerminalStream(streamId, undefined, bid, sid) || isTerminalApprovalResponse(streamId, bid, sid)) return
      appendTurnToSession(bid, sid, normalizeTurn(event.data))
      const pending = getAssistantStream(streamId, bid, sid)
      if (pending && !hasTurn(pending.assistantTurn)) {
        appendTurnToSession(bid || pending.botId, sid || pending.sessionId, pending.assistantTurn)
      }
      return
    }
    if (event.type === 'command_result' || event.type === 'command_error') {
      const invocationId = event.invocation_id?.trim() ?? ''
      let commandBotId = (sourceBotId || currentBotId.value || '').trim()
      let commandSessionId = (event.session_id ?? targetSessionId ?? '').trim()
      const approvalAction = event.action_id === 'tool_approval_response'
      const userInputAction = event.action_id === 'user_input_response'
      const sideband = approvalAction || userInputAction
      const sidebandIdentity = invocationId && approvalAction
        ? resolveApprovalResponseIdentity(invocationId, commandBotId || undefined, commandSessionId || undefined)
        : invocationId && userInputAction
          ? resolveUserInputResponseIdentity(invocationId, commandBotId || undefined, commandSessionId || undefined)
          : undefined
      if (sideband && invocationId && !sidebandIdentity) return
      commandBotId ||= sidebandIdentity?.botId || ''
      commandSessionId ||= sidebandIdentity?.sessionId || ''
      const pending = invocationId
        ? getAssistantStream(invocationId, commandBotId || undefined, commandSessionId || undefined)
        : undefined
      if (sideband && invocationId) {
        if (
          event.action_id === 'tool_approval_response'
          && isTerminalApprovalResponse(
            invocationId,
            commandBotId || undefined,
            commandSessionId || undefined,
          )
        ) return
        if (
          event.action_id === 'user_input_response'
          && isTerminalUserInputResponse(
            invocationId,
            commandBotId || undefined,
            commandSessionId || undefined,
          )
        ) return
        if (event.type === 'command_error') {
          const uncertainApproval = getApprovalResponse(invocationId, commandBotId, commandSessionId)
          if (uncertainApproval?.awaitingResync && markApprovalResponseReplayFailed(invocationId, commandBotId, commandSessionId)) {
            requestRuntimeResync(uncertainApproval.botId, uncertainApproval.sessionId)
            requestPersistedHistory(uncertainApproval.botId, uncertainApproval.sessionId)
            return
          }
          settleApprovalResponse(invocationId, 'failed', commandBotId, commandSessionId)
          const userInputState = userInputResponseStreams.get(sidebandResponseKey(commandBotId, commandSessionId, invocationId))
          if (userInputState) restoreUserInputStates(userInputState)
          toast.error(event.error?.message || 'runtime response failed')
        } else {
          settleApprovalResponse(invocationId, 'succeeded', commandBotId, commandSessionId)
        }
        clearUserInputResponseStream(invocationId, commandBotId, commandSessionId)
        if (pending) {
          pruneEmptyAssistantTurnIfPending(invocationId, commandBotId, commandSessionId)
          if (event.type === 'command_error') {
            rejectAssistantStream(invocationId, new CommandStreamError(event.error?.message || 'runtime response failed'), commandBotId, commandSessionId)
          } else {
            resolveAssistantStream(invocationId, commandBotId, commandSessionId)
          }
        }
        loading.value = isSessionStreaming(currentBotId.value, sessionId.value)
        return
      }
      rememberCommandEvent(event, {
        botId: pending?.botId || sourceBotId,
        sessionId: event.session_id || pending?.sessionId || targetSessionId,
        composerScope: pending?.composerScope || event.composer_scope,
      })
      if (event.type === 'command_error' && invocationId) {
        if (pending) {
          const message = event.error?.message || 'slash command failed'
          rejectAssistantStream(invocationId, new CommandStreamError(message), commandBotId, commandSessionId)
          loading.value = isActiveSessionStreaming()
        }
      }
      return
    }

    if (event.type !== 'error') return
    const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
    const bid = sourceBotId || currentBotId.value || ''
    const streamId = (event.stream_id ?? '').trim()
    if (!streamId) return
    if (
      isTerminalStream(streamId, undefined, bid, sid)
      || isTerminalApprovalResponse(streamId, bid, sid)
      || isTerminalUserInputResponse(streamId, bid, sid)
    ) return
    const approvalResponse = getApprovalResponse(streamId, bid, sid)
    const userInputResponse = userInputResponseStreams.get(sidebandResponseKey(bid, sid, streamId))
    if (approvalResponse?.silent || userInputResponse) {
      settleApprovalResponse(streamId, 'failed', bid, sid)
      if (userInputResponse) restoreUserInputStates(userInputResponse)
      toast.error(resolveApiErrorMessage(event, event.message || 'runtime response failed'))
      clearUserInputResponseStream(streamId, bid, sid)
      loading.value = isActiveSessionStreaming()
      return
    }
    const stream = getAssistantStream(streamId, bid, sid)
    if (stream?.runtimeObserved && stream.abortRequested) {
      const generation = runtimeGenerationForStream(bid, sid, streamId)
      chatIntents.rejectAbort({ botId: bid, sessionId: sid, streamId, generation })
      stream.abortRequested = false
      stream.abortSent = false
      stream.abortSentGeneration = ''
      toast.error(resolveApiErrorMessage(event, event.message || 'runtime abort failed'))
      return
    }
    if (!stream || stream.runtimeObserved) return
    const message = event.message || 'stream error'
    settleApprovalResponse(streamId, 'failed', bid, sid)
    const userInputState = userInputResponseStreams.get(sidebandResponseKey(bid, sid, streamId))
    if (userInputState) restoreUserInputStates(userInputState)
    clearUserInputResponseStream(streamId, bid, sid)
    rejectAssistantStream(streamId, new StreamFailureError(message, 'startup'), stream.botId, stream.sessionId)
    loading.value = isActiveSessionStreaming()
    releaseHiddenSessionView(chatViews.getSession(stream.botId, stream.sessionId) ?? null)
  }

  function resetUserScopedState(options: { clearSelection?: boolean } = {}) {
    userScopeGeneration += 1
    stopStreams()
    abortAllAssistantStreams()
    // Stream rejection schedules send/retry/edit continuations. Clear their
    // ownership synchronously so those continuations cannot restore an old
    // user's intent after the transcript and selection scope are reset.
    chatIntents.clear()
    intentCompletions.clear()
    clearAllRuntimeAdmissionTimeouts()
    clearAllRuntimeAbortWatchdogs()
    stopWebSocket()
    clearPendingACPSession()

    resetSessionListRefresh()
    persistedHistory.reset()

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
    draftACPStages.value = {}
    liveDraftACP = null
    loading.value = false
    loadingChats.value = false
    initializing.value = false
    initializeRerunRequested = false
    initializingBotId = null
    initializePromise = null
    initializeToken = null
    overrideModelId.value = ''
    overrideReasoningEffort.value = ''
    startupSendFailures.value = {}
    composerDraftRestores.value = {}
    draftViewRequested.value = null
    forkedSessionRequested.value = null
    draftViewCommandVersions.clear()
    visibleSessionSummaryRequests.clear()
    draftSessionCreations.clear()
    resetCommandEvents()
    resetFsBeacon()
    resetACPRuntimeRegistry()

    clearStreamHistory()
    resetApprovalResponses()
    forkingMessages.clear()
    backgroundTasks.clearBackgroundTasks()
    seenGuiToolCalls.clear()
    guiToolUseRequested.value = null
    userInputResponseStreams.clear()
    pendingUserInputResponses.clear()
    terminalUserInputResponseIds.clear()
    resetRuntimeTransportState()
  }

  function resetRuntimeTransportState() {
    runtimeSubscriptionController.reset()
    runtimeSessions.clear()
  }

  function handleWebSocketOpen(botId: string) {
    const bid = botId.trim()
    realtimeWebSocketBotId = bid
    runtimeSubscriptionController.handleTransportOpen(bid)
    reconcileRuntimeSubscriptions(botId)
  }

  function markRuntimeTransportClosed(botId: string) {
    const bid = botId.trim()
    if (!bid) return
    chatIntents.handleTransportClose(bid)
    for (const stream of allAssistantStreams()) {
      if (stream.botId !== bid) continue
      stream.abortSent = false
      stream.abortSentGeneration = ''
    }
    markPendingUserInputResponsesUncertain(botId)
    markPendingApprovalResponsesUncertain(botId)
    runtimeSubscriptionController.handleTransportClose(bid)
  }

  function handleWebSocketClose(botId: string) {
    markRuntimeTransportClosed(botId)
  }

  function markPendingUserInputResponsesUncertain(botId: string) {
    const bid = botId.trim()
    for (const pending of pendingUserInputResponses.values()) {
      if (pending.botId === bid) {
        pending.awaitingResync = true
        pending.replaySent = false
        pending.replayFailed = false
      }
    }
  }

  function resendPendingApprovalResponse(pending: ApprovalResponse) {
    if (!pending.awaitingResync || pending.replaySent || !pending.decision) return
    if (!markApprovalResponseReplaySent(pending.streamId, pending.botId, pending.sessionId)) return
    try {
      if (sendWebSocketMessage(pending.botId, {
        type: 'tool_approval_response',
        stream_id: pending.streamId,
        session_id: pending.sessionId,
        approval_id: pending.approvalId,
        short_id: pending.shortId,
        decision: pending.decision,
      })) return
    } catch {
      // The next transport generation retries the same response.
    }
    markPendingApprovalResponsesUncertain(pending.botId)
  }

  function startWebSocket(botId: string) {
    markRuntimeTransportClosed(realtimeWebSocketBotId)
    realtimeWebSocketBotId = botId.trim()
    try {
      startRealtimeWebSocket(botId)
    } catch (error) {
      realtimeWebSocketBotId = ''
      throw error
    }
  }

  function stopWebSocket() {
    markRuntimeTransportClosed(realtimeWebSocketBotId)
    stopRealtimeWebSocket()
    realtimeWebSocketBotId = ''
  }

  function requestRuntimeResync(botId: string, targetSessionId: string) {
    if (!runtimeSessionDesired(botId, targetSessionId)) {
      runtimeSubscriptionController.remove({ botId, sessionId: targetSessionId })
      return
    }
    runtimeSubscriptionController.requestCheckpoint({ botId, sessionId: targetSessionId })
  }

  function runtimeSessionDesired(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return false
    if ((currentBotId.value ?? '').trim() === bid && (sessionId.value ?? '').trim() === sid) return true
    if (chatViews.entries().some(view => view.kind === 'session'
      && view.botId === bid
      && view.sessionId === sid
      && view.visiblePanelIds.size > 0)) return true
    return assistantStreamsForSession(bid, sid).length > 0
  }

  function unsubscribeRuntime(botId: string, targetSessionId: string) {
    runtimeSubscriptionController.remove({ botId, sessionId: targetSessionId })
  }

  function desiredRuntimeSubscriptions(targetBotId?: string): RuntimeSubscriptionRef[] {
    const bid = (targetBotId ?? currentBotId.value ?? '').trim()
    if (!bid) return []
    const desiredSessionIDs = new Set<string>()
    const activeSessionID = (sessionId.value ?? '').trim()
    if ((currentBotId.value ?? '').trim() === bid && activeSessionID) desiredSessionIDs.add(activeSessionID)
    for (const view of chatViews.entries()) {
      if (view.kind === 'session' && view.botId === bid && view.sessionId && view.visiblePanelIds.size > 0) {
        desiredSessionIDs.add(view.sessionId)
      }
    }
    for (const stream of allAssistantStreams()) {
      if (stream.botId === bid && stream.sessionId) desiredSessionIDs.add(stream.sessionId)
    }
    return [...desiredSessionIDs].map(sessionId => ({ botId: bid, sessionId }))
  }

  function reconcileRuntimeSubscriptions(targetBotId?: string) {
    runtimeSubscriptionController.reconcile(desiredRuntimeSubscriptions(targetBotId))
  }

  function subscribeRuntime(targetBotId?: string, targetSessionId?: string) {
    const bid = (targetBotId ?? currentBotId.value ?? '').trim()
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!bid || !sid) return
    const desired = desiredRuntimeSubscriptions(bid)
    if (!desired.some(ref => ref.sessionId === sid)) desired.push({ botId: bid, sessionId: sid })
    runtimeSubscriptionController.reconcile(desired)
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

  function handleSessionMessageEvent(targetBotId: string, targetSessionId: string, event: SessionMessageStreamEvent) {
    if (event.type === 'ping') return
    if (event.type === 'dropped') {
      requestPersistedHistory(targetBotId, targetSessionId)
      return
    }

    if (event.type === 'background_task') {
      const eventSessionId = event.session_id?.trim()
      if (eventSessionId && eventSessionId !== targetSessionId) return
      const task = normalizeBackgroundTask(event, event.type)
      if (!task) return
      const view = chatViews.getSession(targetBotId, targetSessionId)
      if (view) {
        view.transcript.applyBackgroundTask(task)
      }
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

    // Raw SSE rows need the UI aggregation performed by the history endpoint.
    // The sync reconciler coalesces notifications and exact-merges the latest
    // persisted window; runtime-owned turns are settled by their terminal edge.
    const raw = event.message
    if (!raw) return
    const eventBotId = String(event.bot_id ?? '').trim()
    const messageBotId = String(raw.bot_id ?? '').trim()
    if ((eventBotId && eventBotId !== targetBotId) || (messageBotId && messageBotId !== targetBotId)) return
    const messageSessionId = String(raw.session_id ?? '').trim()
    if (messageSessionId && messageSessionId !== targetSessionId) return
    if (messageSessionId) touchSessionInList(messageSessionId, raw.created_at)
    const sid = messageSessionId || targetSessionId
    if (!shouldRefreshFromMessageCreated(
      targetBotId,
      sid,
      isSessionStreaming(targetBotId, sid) ? sid : null,
      event,
    )) return
    void persistedHistory.reconcile({ botId: targetBotId, sessionId: sid })
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

  async function prepareSessionMessages(targetBotId: string, targetSessionId: string) {
    const bid = targetBotId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    const runtimeKey = acpRuntimeKey(bid, sid)
    const view = chatViews.getOrCreate({ botId: bid, sessionId: sid, viewId: focusedChatViewId.value })
    const hydration = view.transcript.beginInitialMessagesLoad(bid, sid)
    if (!hydration) return

    // The chat pane reads `loadingMessages` to suppress empty-state
    // placeholders (e.g. "system session has no records") while a fresh
    // transcript is on its way. The sidebar deliberately ignores it — only
    // `loadingChats` (sessions-list boot) makes the sidebar spin.
    subscribeRuntime(bid, sid)
    reconcileRuntimeSubscriptions(bid)
    try {
      const turns = await hydration.turns
      await runtimeSessions.enqueue(runtimeKey, () => {
        if (chatViews.getSession(bid, sid)?.transcript !== view.transcript) return
        if (view.transcript.applyInitialMessagesLoad(hydration, turns)) view.initialized = true
      })
    } finally {
      try {
        await runtimeSessions.enqueue(runtimeKey, () => {
          const runtimeState = runtimeSessions.get(runtimeKey)
          const runtimeRun = runtimeState?.snapshot?.current_run_view
          for (const stream of assistantStreamsForSession(bid, sid)) {
            const runtimeGeneration = (runtimeRun?.generation ?? '').trim()
            if (
              runtimeRun?.stream_id?.trim() === stream.streamId
              && (!stream.runtimeGeneration || !runtimeGeneration || stream.runtimeGeneration === runtimeGeneration)
            ) {
              applyRuntimeRequestUserTurn(
                stream,
                runtimeRun.request_user_turn,
                runtimeRun.status ?? 'running',
              )
            }
            reattachTurnToSession(bid, sid, stream.assistantTurn)
          }
          if (runtimeState?.snapshot) {
            projectRuntimeSnapshot(runtimeState.snapshot, bid, sid, runtimeState.seq, true, false, true)
          }
        })
      } finally {
        view.transcript.finishInitialMessagesLoad(hydration)
      }
    }
  }

  function abort(target?: ChatViewTarget) {
    const resolved = normalizedChatViewTarget(target)
    const approvalStreamIds = abortApprovalResponses(
      pendingApprovalResponsesForSession(resolved.botId, resolved.sessionId ?? ''),
      'failed',
    )
    const streamIds = resolved.sessionId
      ? assistantStreamsForSession(resolved.botId, resolved.sessionId).map(stream => stream.streamId)
      : activeUnboundStreamIds(
          resolved.botId,
          target ? `${resolved.botId}:${resolved.viewId}` : undefined,
        )
    for (const streamId of streamIds) {
      if (!approvalStreamIds.has(sidebandResponseKey(resolved.botId, resolved.sessionId ?? '', streamId))) {
        const stream = getAssistantStream(streamId, resolved.botId, resolved.sessionId ?? '')
        if (stream) stream.abortRequested = true
        if (stream && !stream.sessionId) continue
        if (!abortWebSocketStream(streamId, stream?.botId, stream?.sessionId)) {
          if (stream) {
            stream.abortRequested = false
            stream.abortSent = false
            stream.abortSentGeneration = ''
          }
          toast.error('WebSocket is not connected')
        }
      }
    }
    loading.value = isActiveSessionStreaming()
    chatViews.prune()
  }

  function abortApprovalResponses(responses: ApprovalResponse[], outcome: ApprovalResponseOutcome): Set<string> {
    const streamIds = new Set<string>()
    for (const response of responses) {
      streamIds.add(sidebandResponseKey(response.botId, response.sessionId, response.streamId))
      settleApprovalResponse(response.streamId, outcome, response.botId, response.sessionId)
    }
    return streamIds
  }

  function abortAllAssistantStreams() {
    const abortError = new Error('aborted')
    abortError.name = 'AbortError'
    const approvalStreamIds = abortApprovalResponses(pendingApprovalResponses(), 'canceled')
    rejectAllStreams(abortError, (streamId, botId, targetSessionId) => {
      if (!approvalStreamIds.has(sidebandResponseKey(botId, targetSessionId, streamId))) {
        abortWebSocketStream(streamId, botId, targetSessionId)
      }
    })
    loading.value = false
  }

  async function ensureBot(): Promise<string | null> {
    const generation = userScopeGeneration
    try {
      const list = await fetchBots()
      if (generation !== userScopeGeneration) return null
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
      if (generation !== userScopeGeneration) return null
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
    const generation = userScopeGeneration
    try {
      const list = await fetchBots()
      if (generation !== userScopeGeneration) return
      bots.value = list
    } catch (error) {
      if (generation !== userScopeGeneration) return
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

  function chatTargetFor(target?: ChatViewTarget): ActiveChatTarget {
    const resolved = normalizedChatViewTarget(target)
    const focused = resolved.viewId === focusedChatViewId.value
      && resolved.botId === (currentBotId.value ?? '').trim()
    const explicitSelection = focused ? explicitSessionSelection.value : false
    const sid = (resolved.sessionId ?? '').trim()
    if (sid) {
      const session = knownSessionSummary(sid)
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

    const pendingState = pendingACPStateFor(resolved)
    if (pendingState) {
      return {
        kind: 'draft-acp',
        sessionId: null,
        session: null,
        runtimeType: 'acp_agent',
        isACP: true,
        isPendingACP: true,
        metadata: pendingState.metadata,
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
  }

  const activeChatTarget = computed<ActiveChatTarget>(() => chatTargetFor())

  function chatReadOnlyFor(target: ChatViewTarget): boolean {
    const resolved = normalizedChatViewTarget(target)
    const session = chatTargetFor(resolved).session
    if (!session) return Boolean(resolved.sessionId)
    const type = session.type ?? 'chat'
    if (type === 'heartbeat' || type === 'schedule' || type === 'subagent') return true
    const channelType = (session.channel_type ?? '').trim().toLowerCase()
    return Boolean(channelType && channelType !== 'local')
  }

  function chatCanForkFor(target: ChatViewTarget): boolean {
    return chatTargetFor(target).session?.type === 'chat'
  }























  async function createACPSessionRecord(
    botId: string,
    input: ACPAgentSessionInput,
  ): Promise<SessionSummary> {
    const bid = botId.trim()
    if (!bid) throw new Error('Bot not ready')
    const metadata = acpSessionMetadata(input)
    const runtimeId = input.runtimeId?.trim() ?? ''
    const sessionMode = input.sessionMode === 'discuss' ? 'discuss' : 'chat'
    // The warm staged runtime is bound server-side inside session creation;
    // no separate adopt/bind round trip and nothing for a watcher to race.
    return createSession(bid, {
      title: input.title ?? '',
      type: sessionMode,
      sessionMode,
      runtimeType: 'acp_agent',
      metadata: {},
      runtimeMetadata: metadata,
      acpRuntimeId: runtimeId || undefined,
    })
  }

  async function createACPSessionForTarget(
    input: ACPAgentSessionInput,
    target: ChatViewTarget,
  ): Promise<{ session: SessionSummary }> {
    const draft = targetDraftForACP(target)
    const generation = userScopeGeneration
    const stagedBeforeCreate = pendingACPStateFor(draft)
    const runtimeId = input.runtimeId?.trim() ?? ''
    const created = await createACPSessionRecord(draft.botId, input)
    const assertCurrentScope = () => {
      if (
        generation === userScopeGeneration
        && (currentBotId.value ?? '').trim() === draft.botId
      ) return
      const error = new Error('Chat scope changed during ACP Session creation')
      error.name = 'AbortError'
      throw error
    }

    try {
      assertCurrentScope()
    } catch (error) {
      await rollbackFailedACPSessionCreation(created, draft, stagedBeforeCreate?.input ?? input, runtimeId, generation)
      throw error
    }

    upsertSession(created)
    rememberSession(created)
    if (runtimeId) clearACPRuntimeStatus(draft.botId, runtimeId)
    promoteDraftChatView(draft, created.id)
    if (stagedBeforeCreate) {
      if (runtimeId && stagedBeforeCreate.runtimeId === runtimeId) {
        forgetDraftACPStage(draft)
      } else {
        discardDraftACPStage(draft)
      }
    }
    if (isFocusedChatTarget(draft)) {
      sessionId.value = created.id
      explicitSessionSelection.value = true
      draftIntent.value = false
    }
    return { session: created }
  }

  async function rollbackFailedACPSessionCreation(
    created: SessionSummary,
    draft: ChatViewTarget,
    stagedInput: ACPAgentSessionInput,
    stagedRuntimeId: string,
    generation: number,
  ) {
    if (generation !== userScopeGeneration) return

    markSessionDeleted(draft.botId, created.id)
    stopSessionMessagesStream(draft.botId, created.id)
    chatViews.removeSession(draft.botId, created.id)
    clearACPRuntimeStatus(draft.botId, created.id)
    if (stagedRuntimeId) clearACPRuntimeStatus(draft.botId, stagedRuntimeId)
    if ((currentBotId.value ?? '').trim() === draft.botId) {
      removeSessionFromList(created.id)
    }

    if (sameDraftACPStage(liveDraftACP, draft)) {
      releasePendingACPSession()
      liveDraftACP = null
    }
    rememberDraftACPStage(draft, {
      botId: draft.botId,
      input: normalizedACPInput({ ...stagedInput, runtimeId: undefined }),
      runtimeId: '',
    })
    if (isFocusedChatTarget(draft)) activateDraftACPStage(draft)

    try {
      await requestDeleteSession(draft.botId, created.id)
    } catch {
      // The tombstone keeps a failed cleanup out of this client until auth reset.
    }
  }

  async function createACPSession(input: ACPAgentSessionInput): Promise<{ session: SessionSummary }> {
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    return createACPSessionForTarget(input, {
      botId: bid,
      sessionId: null,
      viewId: focusedChatViewId.value,
    })
  }

  async function updateCurrentSessionAgent(
    input: ACPAgentSessionInput,
    target?: ChatViewTarget,
  ): Promise<{ session: SessionSummary }> {
    const resolved = normalizedChatViewTarget(target)
    if (!resolved.sessionId) return createACPSessionForTarget(input, resolved)
    const bid = resolved.botId
    const sid = resolved.sessionId
    if (!bid) throw new Error('Bot not selected')
    const metadata = acpSessionMetadata(input)
    const targetSession = knownSessionSummary(sid)
    const sessionMode = targetSession?.session_mode || (targetSession?.type === 'discuss' ? 'discuss' : 'chat')
    const generation = userScopeGeneration
    const updated = await requestUpdateSessionAgent(bid, sid, {
      type: sessionMode === 'discuss' ? 'discuss' : 'acp_agent',
      sessionMode,
      runtimeType: 'acp_agent',
      metadata,
      runtimeMetadata: metadata,
    })
    if (generation !== userScopeGeneration || (currentBotId.value ?? '').trim() !== bid) return { session: updated }
    upsertSession(updated)
    if (isFocusedChatTarget(resolved)) {
      explicitSessionSelection.value = true
      draftIntent.value = false
    }
    clearACPRuntimeStatus(bid, sid)
    return { session: updated }
  }

  async function updateCurrentSessionToMemoh(target?: ChatViewTarget): Promise<SessionSummary | null> {
    const resolved = normalizedChatViewTarget(target)
    const bid = resolved.botId
    const sid = resolved.sessionId ?? ''
    if (!bid || !sid) return null
    const targetSession = knownSessionSummary(sid)
    const sessionMode = targetSession?.session_mode || (targetSession?.type === 'discuss' ? 'discuss' : 'chat')
    const generation = userScopeGeneration
    const updated = await requestUpdateSessionAgent(bid, sid, {
      type: sessionMode === 'discuss' ? 'discuss' : 'chat',
      sessionMode,
      runtimeType: 'model',
      metadata: {},
      runtimeMetadata: {},
    })
    if (generation !== userScopeGeneration || (currentBotId.value ?? '').trim() !== bid) return null
    upsertSession(updated)
    if (isFocusedChatTarget(resolved)) {
      explicitSessionSelection.value = true
      draftIntent.value = false
    }
    clearACPRuntimeStatus(bid, sid)
    return updated
  }

  async function ensureChatViewSession(target: ChatViewTarget, firstPrompt?: string): Promise<ChatViewTarget> {
    if (target.sessionId) return target
    const creationKey = draftSessionCreationKey(target)
    if (draftSessionCreations.has(creationKey)) {
      throw new StreamFailureError('Session creation is already in progress', 'startup')
    }
    draftSessionCreations.add(creationKey)
    try {
      const pendingACP = pendingACPStateFor(target)
      if (pendingACP) {
        const { session: created } = await createACPSessionForTarget({
          ...pendingACP.input,
          runtimeId: pendingACP.runtimeId,
        }, target)
        if (firstPrompt?.trim() && !created.title?.trim()) {
          created.title = provisionalSessionTitle(firstPrompt)
          upsertSession(created)
          rememberSession(created)
        }
        return { ...target, sessionId: created.id }
      }

      const generation = userScopeGeneration
      const created = await createSession(target.botId)
      if (
        generation !== userScopeGeneration
        || (currentBotId.value ?? '').trim() !== target.botId
      ) {
        const error = new Error('Chat scope changed during Session creation')
        error.name = 'AbortError'
        throw error
      }
      if (firstPrompt?.trim()) created.title = provisionalSessionTitle(firstPrompt)
      upsertSession(created)
      rememberSession(created)
      promoteDraftChatView(target, created.id)
      if (isFocusedChatTarget(target)) {
        sessionId.value = created.id
        explicitSessionSelection.value = true
        draftIntent.value = false
      }
      return { ...target, sessionId: created.id }
    } finally {
      draftSessionCreations.delete(creationKey)
    }
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

    const generation = userScopeGeneration
    const runToken = Symbol('chat-initialize')
    initializeToken = runToken
    const run = (async () => {
      initializing.value = true
      loadingChats.value = true
      try {
        do {
          if (generation !== userScopeGeneration) return
          initializeRerunRequested = false
          initializingBotId = (currentBotId.value ?? '').trim() || null
          // Every entry into initialize starts from a clean transcript window. We
          // reset here unconditionally so the success path that hydrates
          // `sessionId` without clearing messages can't carry a stale
          // `hasLoadedOlder = true` from a previous bot into the new bot's first
          // refresh (which would take the merge branch and duplicate optimistic
          // turns).
          prepareForInitialization()
          resetSessionListRefresh()
          persistedHistory.reset()
          stopStreams()
          stopWebSocket()

          const bid = await ensureBot()
          if (generation !== userScopeGeneration) return
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
            if (generation !== userScopeGeneration) return
            if ((currentBotId.value ?? '').trim() !== bid) {
              initializeRerunRequested = true
              continue
            }
            throw error
          }
          if (generation !== userScopeGeneration) return
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
          if (generation !== userScopeGeneration) return
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
        if (generation === userScopeGeneration) {
          loadingChats.value = false
          initializing.value = false
          initializingBotId = null
          initializeRerunRequested = false
        }
        if (initializeToken === runToken) {
          initializePromise = null
          initializeToken = null
        }
      }
    })()
    initializePromise = run
    await run
  }

  // Selection changes focus only. Visible panel bindings own Session SSE
  // lifetimes, and keyed transcript views retain their own cached history.
  //
  // We deliberately do NOT call `abortAllAssistantStreams()` here: an
  // assistant stream that started in session A keeps running server-side
  // after the user switches to B, and finalizes against A's history when
  // the user comes back (the `appendTurnToSession` / WS handlers are
  // already gated on `sessionId.value === <stream's sessionId>`, so the
  // orphan does not bleed into B's view).
  function switchActiveSession(sid: string, previousSessionId = '') {
    const bid = (currentBotId.value ?? '').trim()
    loading.value = isSessionStreaming(bid, sid)
    if (!bid || !sid) return
    const previous = previousSessionId.trim()
    if (previous && previous !== sid) {
      releaseHiddenSessionView(chatViews.getSession(bid, previous) ?? null)
    }
    subscribeRuntime(bid, sid)
    reconcileRuntimeSubscriptions(bid)
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
    const previousSessionId = (sessionId.value ?? '').trim()
    const sameSession = sid === previousSessionId
    const requestId = ++selectSessionRequestId
    const bid = (currentBotId.value ?? '').trim()
    clearPendingACPSession()
    sessionId.value = sid
    draftIntent.value = false
    explicitSessionSelection.value = options.explicitSelection !== false
    if (!sameSession) switchActiveSession(sid, previousSessionId)
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

  function normalizedACPInput(input: ACPAgentSessionInput): ACPAgentSessionInput {
    const metadata = acpSessionMetadata(input)
    return {
      ...input,
      agentId: String(metadata.acp_agent_id ?? ''),
      projectPath: String(metadata.project_path ?? ''),
      projectMode: String(metadata.acp_project_mode ?? ''),
    }
  }

  function applyDraftViewRequest(
    request: NonNullable<typeof draftViewRequested.value>,
    mirrorGlobalSelection: boolean,
  ) {
    const target: ChatViewTarget = {
      botId: request.botId,
      sessionId: null,
      viewId: request.viewId,
    }
    if (mirrorGlobalSelection) {
      if (request.input) stageNewACPSession(request.input, target)
      else resetToEmptyComposer({ explicitSelection: request.explicitSelection, draftIntent: true }, target)
      return
    }

    const draft = chatView(target)
    draft.transcript.clearHistoryView()
    discardDraftACPStage(target)
    if (request.input) {
      rememberDraftACPStage(target, {
        botId: request.botId,
        input: normalizedACPInput(request.input),
        runtimeId: '',
      })
    }
  }

  function requestDraftView(target: ChatViewTarget, input: ACPAgentSessionInput | null, activate = isFocusedChatTarget(target)) {
    const resolved = normalizedChatViewTarget(target)
    const request = {
      botId: resolved.botId,
      viewId: resolved.viewId,
      expectedSessionId: resolved.sessionId,
      explicitSelection: true,
      input: input ? normalizedACPInput(input) : null,
      activate,
      seq: ++draftViewRequestSeq,
    }
    draftViewRequested.value = request
  }

  function invalidateDraftViewCommand(target: ChatViewTarget) {
    const key = draftSessionCreationKey(target)
    draftViewCommandVersions.delete(key)
  }

  function beginDraftViewCommand(target: ChatViewTarget) {
    const key = draftSessionCreationKey(target)
    const version = ++draftViewCommandSequence
    draftViewCommandVersions.set(key, version)
    return {
      isCurrent: () => draftViewCommandVersions.get(key) === version,
      finish: () => {
        if (draftViewCommandVersions.get(key) === version) {
          draftViewCommandVersions.delete(key)
        }
      },
    }
  }

  async function handleWebNewCommand(
    text: string,
    attachments: ChatAttachment[] | undefined,
    target: ChatViewTarget,
  ): Promise<WebNewCommandResult> {
    const parsed = parseWebNewCommand(text)
    if (!parsed) return { kind: 'none' }
    const generation = userScopeGeneration
    const activate = isFocusedChatTarget(target)
    if (attachments?.length) {
      return { kind: 'error', message: 'Attachments are not supported with /new' }
    }
    const agentId = parsed.agentId.trim()
    if (!agentId) {
      if (parsed.mode === 'discuss') {
        return { kind: 'error', message: 'Discuss ACP sessions require an agent, for example /new discuss codex' }
      }
      const command = beginDraftViewCommand(target)
      if (command.isCurrent()) requestDraftView(target, null, activate)
      command.finish()
      return { kind: 'handled' }
    }
    if (agentId !== 'codex' && agentId !== 'claude-code') {
      return { kind: 'error', message: `Unknown ACP agent "${agentId}"` }
    }
    const command = beginDraftViewCommand(target)
    try {
      const targetBotId = target.botId === '__unbound__' ? '' : target.botId
      const bid = targetBotId || await ensureBot()
      if (!bid) return { kind: 'error', message: 'Bot not ready' }
      const defaults = await defaultACPSettingsForAgent(bid, agentId)
      if (
        generation !== userScopeGeneration
        || (currentBotId.value ?? '').trim() !== bid
        || !command.isCurrent()
      ) {
        return { kind: 'handled' }
      }
      requestDraftView(target, {
        agentId,
        sessionMode: parsed.mode === 'discuss' ? 'discuss' : 'chat',
        ...defaults,
      }, activate)
      return { kind: 'handled' }
    } finally {
      command.finish()
    }
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

  async function handleWebSlashCommand(
    text: string,
    hasRequestedSkills = false,
    composerScope = 'chat',
    target?: ChatViewTarget,
  ): Promise<WebSlashCommandResult> {
    if (!isWebSlashInput(text) || hasRequestedSkills) return { kind: 'none' }
    const resolved = normalizedChatViewTarget(target)
    const bid = resolved.botId
    if (!bid) return { kind: 'error', message: 'Bot not selected' }
    const sid = resolved.sessionId ?? ''
    const scope = composerScope.trim() || 'chat'
    const commandScope = {
      botId: bid,
      sessionId: sid || undefined,
      composerScope: scope,
    }

    const actionID = quickActionIDForSlash(text)
    if (!actionID) return { kind: 'none' }
    const skillActivationAllowed = !chatTargetFor(resolved).isACP
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
      showCommandError('generic', message, commandScope)
      return { kind: 'error', message }
    }

    if (!event) return { kind: 'none' }
    rememberCommandEvent(event, commandScope)
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
    chatIntents.discardTarget({ botId: bid, sessionId: delId })
    const deletedError = new Error('aborted')
    deletedError.name = 'AbortError'
    for (const stream of assistantStreamsForSession(bid, delId)) {
      rejectAssistantStream(stream.streamId, deletedError, stream.botId, stream.sessionId)
    }
    for (const pending of [...pendingUserInputResponses.values()]) {
      if (pending.botId === bid && pending.sessionId === delId) {
        clearUserInputResponseStream(pending.streamId, bid, delId)
      }
    }
    abortApprovalResponses(pendingApprovalResponsesForSession(bid, delId), 'canceled')
    markSessionDeleted(bid, delId)
    deletedSession.value = { id: delId, botId: bid, seq: ++deletedSessionSeq }
    stopSessionMessagesStream(bid, delId)
    chatViews.removeSession(bid, delId)
    if ((currentBotId.value ?? '').trim() !== bid) return
    clearACPRuntimeStatus(bid, delId)
    unsubscribeRuntime(bid, delId)
    removeSessionFromList(delId)
    if (sessionId.value !== delId) return
    const fallbackMode = options.fallbackMode ?? 'recent'
    const nextSession = fallbackSessionAfterDelete(fallbackMode)
    if (!nextSession) {
      sessionId.value = null
      explicitSessionSelection.value = false
      draftIntent.value = false
      clearHistoryView()
      return
    }
    const next = nextSession.id
    sessionId.value = next
    explicitSessionSelection.value = false
    draftIntent.value = false
    switchActiveSession(next, delId)
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

  async function forkMessage(messageId: string, options: { title?: string, target?: ChatViewTarget } = {}): Promise<boolean> {
    const target = normalizedChatViewTarget(options.target)
    const bid = target.botId
    const sid = target.sessionId ?? ''
    const mid = messageId.trim()
    const view = chatView(target)
    const generation = userScopeGeneration
    const activate = isFocusedChatTarget(target)
    const source = view.transcript.findTurnByServerId(mid)
    if (
      !bid || !sid || !mid
      || (source && source.role !== 'assistant')
      || source?.__optimistic === true
      || (source?.role === 'assistant' && source.__ephemeral === true)
      || chatReadOnlyFor(target)
      || !chatCanForkFor(target)
      || isChatViewStreaming(target)
      || view.transcript.loadingMessages.value
    ) return false

    const canonicalMessageId = source ? serverMessageId(source) : mid
    const key = `${bid}:${sid}:${canonicalMessageId}`
    if (forkingMessages.has(key)) return false
    forkingMessages.add(key)
    try {
      const forked = await requestForkSessionFromMessage(bid, sid, canonicalMessageId, { title: options.title })
      if (generation !== userScopeGeneration || (currentBotId.value ?? '').trim() !== bid) return true

      upsertSession(forked)
      rememberSession(forked)
      void refreshSessionsList(bid)

      const turns = await fetchSessionWindow(bid, forked.id)
      if (generation !== userScopeGeneration || (currentBotId.value ?? '').trim() !== bid) return true
      const anchoredForked = withForkAnchorFromUITurns(forked, turns)
      if (anchoredForked !== forked) {
        upsertSession(anchoredForked)
        rememberSession(anchoredForked)
      }
      sessionTranscript(bid, forked.id).replaceHistoryView(turns, forked.id)
      forkedSessionRequested.value = {
        botId: bid,
        viewId: target.viewId,
        expectedSessionId: sid,
        sessionId: forked.id,
        title: (anchoredForked.title ?? options.title ?? '').trim(),
        explicitSelection: true,
        activate,
        seq: ++forkedSessionRequestSeq,
      }
      chatViews.prune()
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
    let viewTarget = normalizedChatViewTarget(options.target)
    const composerScope = options.composerScope?.trim()
      || (options.target ? `${viewTarget.botId}:${viewTarget.viewId}` : 'chat')
    const commandScope = {
      botId: viewTarget.botId,
      sessionId: viewTarget.sessionId ?? undefined,
      composerScope,
    }
    if (!trimmed && !attachments?.length && requestedSkills.length === 0) return { ok: false, stage: 'startup' }

    if (requestedSkills.length > 0 && isWebSlashInput(trimmed)) {
      const message = commandErrorMessage('invalid_skill_slash_syntax')
      showCommandError('invalid_skill_slash_syntax', message, commandScope)
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }

    if (isWebSlashInput(trimmed) && attachments?.length) {
      const message = commandErrorMessage('slash_attachments_unsupported')
      showCommandError('slash_attachments_unsupported', message, commandScope)
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }

    const newCommand = await handleWebNewCommand(trimmed, attachments, viewTarget)
    if (newCommand.kind === 'handled') return { ok: true }
    if (newCommand.kind === 'error') {
      return { ok: false, stage: 'startup', error: newCommand.message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }
    const slashCommand = await handleWebSlashCommand(trimmed, requestedSkills.length > 0, composerScope, viewTarget)
    if (slashCommand.kind === 'handled') return { ok: true }
    if (slashCommand.kind === 'error') {
      return { ok: false, stage: 'startup', error: slashCommand.message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) }
    }
    if (viewTarget.sessionId && chatReadOnlyFor(viewTarget)) {
      return { ok: false, stage: 'startup' }
    }
    clearCommandEvent(commandScope)
    const initialView = chatView(viewTarget)
    if (
      isChatViewStreaming(viewTarget, composerScope)
      || isChatViewCreatingSession(viewTarget)
      || initialView.transcript.loadingMessages.value
      || !viewTarget.botId
    ) return { ok: false, stage: 'startup' }

    let assistantTurn: ChatAssistantTurn | null = null
    let userTurn: ChatUserTurn | null = null
    let sendBotId = ''
    let sendSessionId = ''
    let sendStreamId = ''
    let turnAppendStarted = false

    const wasDraft = !viewTarget.sessionId
    const serverSlashActivation = isWebSlashInput(trimmed) && quickActionIDForSlash(trimmed) === ''
    const serverSkillActivation = requestedSkills.length > 0 || serverSlashActivation
    if (
      serverSkillActivation
      && wasDraft
      && pendingACPStateFor(viewTarget)
    ) {
      const message = commandErrorMessage('unsupported_skill_slash_context')
      showCommandError('unsupported_skill_slash_context', message, commandScope)
      return { ok: false, stage: 'startup', error: message, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills), composerScope }
    }

    loading.value = true
    const deferSessionCreation = serverSkillActivation && wasDraft
    try {
      if (!deferSessionCreation) {
        viewTarget = await ensureChatViewSession(viewTarget, wasDraft ? trimmed : undefined)
      }

      const bid = viewTarget.botId
      const sid = viewTarget.sessionId ?? ''
      if (!sid && !deferSessionCreation) throw new Error('Session not selected')
      sendBotId = bid
      sendSessionId = sid
      // Tell the tab store to pin (and, for a draft, repoint) this session's tab.
      if (sid) {
        userSentInSession.value = {
          id: sid,
          botId: bid,
          viewId: viewTarget.viewId,
          wasDraft,
          seq: ++userSendSeq,
        }
      }

      const modelId = options.modelId?.trim() || overrideModelId.value || undefined
      const effort = options.reasoningEffort?.trim() || overrideReasoningEffort.value
      const reasoningEffort = effort || undefined
      const startContext: IntentStartContext = {
        appendUserTurn: !serverSkillActivation,
        onBeforeTurnAppend: options.onBeforeTurnAppend,
        assistantTurn: null,
        userTurn: null,
      }
      intentStartContext = startContext
      const started = chatIntents.send({
        botId: bid,
        sessionId: sid,
        viewId: viewTarget.viewId,
        composerScope,
        text: trimmed,
        attachments,
        requestedSkills,
        modelId,
        reasoningEffort,
        workspaceTargetId: options.workspaceTargetId,
      })
      intentStartContext = null
      assistantTurn = startContext.assistantTurn
      userTurn = startContext.userTurn
      turnAppendStarted = assistantTurn !== null
      if (!started.ok) {
        loading.value = false
        if (turnAppendStarted) options.onTurnAppendAborted?.()
        const reason = resolveApiErrorMessage(started.error, started.error.message || sendFailedMessage())
        return {
          ok: false,
          stage: 'startup',
          error: reason,
          restoreInput: text,
          restoreAttachments: attachments,
          restoreRequestedSkills: cloneRequestedSkills(requestedSkills),
          composerScope,
        }
      }
      sendStreamId = started.intent.streamId
      const completion = intentCompletions.get(intentCompletionKey(bid, sendStreamId))
      if (!completion) throw new StreamFailureError('Chat intent was not registered', 'startup')
      armRuntimeAdmissionTimeout(sendStreamId, bid, sid)
      await completion
      intentCompletions.delete(intentCompletionKey(bid, sendStreamId))
      forgetCreatedSession(sendStreamId, bid)

      loading.value = false
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const isAbort = err.name === 'AbortError'
      const isCommandError = err instanceof CommandStreamError
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const errorCode = parseMemohError(error)?.code
      const stage: SendMessageStage = err instanceof StreamFailureError
        ? err.stage
        : (assistantTurn && hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      const bid = sendBotId || viewTarget.botId || currentBotId.value || ''
      const createdSessionId = sendStreamId ? createdSessionIdForStream(sendStreamId, bid) : ''
      const sid = sendSessionId || createdSessionId
      if (sendStreamId) intentCompletions.delete(intentCompletionKey(bid, sendStreamId))
      const failedPendingIntent = sendStreamId
        ? chatIntents.failPending(bid, sid, sendStreamId)
        : null

      if (!failedPendingIntent && assistantTurn && hasTurn(assistantTurn)) finalizeStreamFailure(
        assistantTurn,
        bid,
        sid,
        err,
        runtimeAssistantErrorIdentityFor(sendStreamId, '', bid, sid),
      )
      if (!failedPendingIntent && !isAbort && stage === 'startup' && userTurn) {
        removeTurnFromSession(bid, sid, userTurn)
      }
      if (!isAbort && stage === 'startup' && deferSessionCreation && wasDraft && createdSessionId) {
        await cleanupFailedDeferredSession(bid, createdSessionId, composerScope)
      }

      if (!failedPendingIntent && sendStreamId) discardAssistantStream(sendStreamId, bid, sid)
      if (sendStreamId) forgetCreatedSession(sendStreamId, bid)
      loading.value = false

      if (!isAbort && stage === 'startup' && turnAppendStarted) {
        options.onTurnAppendAborted?.()
      }

      if (isAbort) return { ok: false, stage: 'stream', error: reason, errorCode }
      if (stage === 'startup') {
        const currentBid = (currentBotId.value ?? '').trim()
        const currentSid = (sessionId.value ?? '').trim()
        const restoredOriginalDraft = deferSessionCreation
          && wasDraft
          && !currentSid
          && focusedChatViewId.value === viewTarget.viewId
        const stillCurrent = currentBid === bid
          && (!sid || currentSid === sid || restoredOriginalDraft)
        const deferredDraftStillCurrent = !(deferSessionCreation && wasDraft && currentSid)
        const commandErrorRestoredDraft = isCommandError && deferSessionCreation && wasDraft && !currentSid
        if (stillCurrent && deferredDraftStillCurrent && (!isCommandError || commandErrorRestoredDraft)) {
          rememberStartupSendFailure({ botId: bid, sessionId: sid, composerScope, error: reason, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills) })
        }
        return { ok: false, stage, error: reason, errorCode, restoreInput: text, restoreAttachments: attachments, restoreRequestedSkills: cloneRequestedSkills(requestedSkills), composerScope }
      }
      return { ok: false, stage, error: reason, errorCode }
    }
  }

  async function retryLatestAssistant(
    messageId: string,
    options: { target?: ChatViewTarget, modelId?: string, reasoningEffort?: string, workspaceTargetId?: string } = {},
  ): Promise<SendMessageResult> {
    const viewTarget = normalizedChatViewTarget(options.target)
    const bid = viewTarget.botId
    const sid = viewTarget.sessionId ?? ''
    const transcript = transcriptForTarget(viewTarget)
    const targetID = messageId.trim()
    if (!bid || !sid || !targetID || chatReadOnlyFor(viewTarget)) return { ok: false, stage: 'startup' }
    if (isChatViewStreaming(viewTarget) || transcript.loadingMessages.value) return { ok: false, stage: 'startup' }
    const target = transcript.findTurnByServerId(targetID)
    if (!target || !transcript.isLatestVisibleAssistantTurn(target)) return { ok: false, stage: 'startup' }

    const startContext: IntentStartContext = {
      appendUserTurn: false,
      assistantTurn: null,
      userTurn: null,
    }
    intentStartContext = startContext
    const started = chatIntents.retry({
      botId: bid,
      sessionId: sid,
      viewId: viewTarget.viewId,
      messageId: targetID,
      modelId: options.modelId?.trim() || overrideModelId.value,
      reasoningEffort: options.reasoningEffort?.trim() || overrideReasoningEffort.value,
      workspaceTargetId: options.workspaceTargetId,
    })
    intentStartContext = null
    if (!started.ok || !startContext.assistantTurn) {
      return { ok: false, stage: 'startup', error: started.ok ? 'Chat intent was not registered' : started.error.message }
    }
    const streamId = started.intent.streamId
    const assistantTurn = startContext.assistantTurn
    loading.value = true
    try {
      const completion = intentCompletions.get(intentCompletionKey(bid, streamId))
      if (!completion) throw new StreamFailureError('Chat intent was not registered', 'startup')
      armRuntimeAdmissionTimeout(streamId, bid, sid)
      await completion
      intentCompletions.delete(intentCompletionKey(bid, streamId))
      finishRuntimeOperationAdmission(streamId, bid, sid)
      refreshLoadingForSession(bid, sid)
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const errorCode = parseMemohError(error)?.code
      intentCompletions.delete(intentCompletionKey(bid, streamId))
      const failedPendingIntent = chatIntents.failPending(bid, sid, streamId)
      const runtimeOwned = finishRuntimeOperationAdmission(streamId, bid, sid)
      const stage: SendMessageStage = runtimeOwned
        ? 'stream'
        : err instanceof StreamFailureError
        ? err.stage
        : (hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      if (!failedPendingIntent) discardAssistantStream(streamId, bid, sid)
      if (!failedPendingIntent && !runtimeOwned) {
        if (stage === 'startup') {
          removeTurnFromSession(bid, sid, assistantTurn)
        } else {
          finalizeStreamFailure(assistantTurn, bid, sid, err, runtimeAssistantErrorIdentityFor(streamId, '', bid, sid))
        }
      }
      refreshLoadingForSession(bid, sid)
      return { ok: false, stage, error: reason, errorCode }
    }
  }

  async function editLatestUser(
    messageId: string,
    text: string,
    options: { target?: ChatViewTarget, modelId?: string, reasoningEffort?: string, workspaceTargetId?: string } = {},
  ): Promise<SendMessageResult> {
    const trimmed = text.trim()
    const viewTarget = normalizedChatViewTarget(options.target)
    const bid = viewTarget.botId
    const sid = viewTarget.sessionId ?? ''
    const transcript = transcriptForTarget(viewTarget)
    const targetID = messageId.trim()
    if (!bid || !sid || !targetID || !trimmed || chatReadOnlyFor(viewTarget)) return { ok: false, stage: 'startup' }
    if (isChatViewStreaming(viewTarget) || transcript.loadingMessages.value) return { ok: false, stage: 'startup' }
    const target = transcript.findTurnByServerId(targetID)
    if (!target || !transcript.isLatestVisibleUserTurn(target)) return { ok: false, stage: 'startup' }
    if (hasUserAttachments(target)) return { ok: false, stage: 'startup' }

    const startContext: IntentStartContext = {
      appendUserTurn: false,
      assistantTurn: null,
      userTurn: null,
    }
    intentStartContext = startContext
    const started = chatIntents.edit({
      botId: bid,
      sessionId: sid,
      viewId: viewTarget.viewId,
      messageId: targetID,
      text: trimmed,
      modelId: options.modelId?.trim() || overrideModelId.value,
      reasoningEffort: options.reasoningEffort?.trim() || overrideReasoningEffort.value,
      workspaceTargetId: options.workspaceTargetId,
    })
    intentStartContext = null
    if (!started.ok || !startContext.assistantTurn) {
      return { ok: false, stage: 'startup', error: started.ok ? 'Chat intent was not registered' : started.error.message, restoreInput: text }
    }
    const streamId = started.intent.streamId
    const assistantTurn = startContext.assistantTurn
    loading.value = true
    try {
      const completion = intentCompletions.get(intentCompletionKey(bid, streamId))
      if (!completion) throw new StreamFailureError('Chat intent was not registered', 'startup')
      armRuntimeAdmissionTimeout(streamId, bid, sid)
      await completion
      intentCompletions.delete(intentCompletionKey(bid, streamId))
      finishRuntimeOperationAdmission(streamId, bid, sid)
      refreshLoadingForSession(bid, sid)
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const reason = resolveApiErrorMessage(error, err.message || sendFailedMessage())
      const errorCode = parseMemohError(error)?.code
      intentCompletions.delete(intentCompletionKey(bid, streamId))
      const failedPendingIntent = chatIntents.failPending(bid, sid, streamId)
      const runtimeOwned = finishRuntimeOperationAdmission(streamId, bid, sid)
      const stage: SendMessageStage = runtimeOwned
        ? 'stream'
        : err instanceof StreamFailureError
        ? err.stage
        : (hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      if (!failedPendingIntent) discardAssistantStream(streamId, bid, sid)
      if (!failedPendingIntent && !runtimeOwned) {
        if (stage === 'startup') {
          removeTurnFromSession(bid, sid, assistantTurn)
        } else {
          finalizeStreamFailure(assistantTurn, bid, sid, err, runtimeAssistantErrorIdentityFor(streamId, '', bid, sid))
        }
      }
      refreshLoadingForSession(bid, sid)
      return { ok: false, stage, error: reason, errorCode, restoreInput: text }
    }
  }

  async function respondToolApproval(
    approval: UIToolApproval,
    decision: 'approve' | 'reject',
    target?: ChatViewTarget,
  ) {
    const viewTarget = normalizedChatViewTarget(target)
    const bid = viewTarget.botId
    const sid = viewTarget.sessionId ?? ''
    const transcript = transcriptForTarget(viewTarget)
    const approvalId = approval.approval_id?.trim()
    if (!bid || !sid || !approvalId) return false
    if (approval.status !== 'pending' || approval.can_approve === false) return false
    if (hasPendingApprovalResponse(approvalId, bid, sid)) return false
    if (!ensureWebSocketConnected(bid)) {
      toast.error(userInputConnectionLostMessage())
      return false
    }
    const streamId = createStreamId()
    const previousApprovalStates = transcript.snapshotToolApprovalStates(approvalId)
    if (!beginApprovalResponse({
      streamId,
      approvalId,
      botId: bid,
      sessionId: sid,
      silent: true,
      decision,
      shortId: approval.short_id,
      rollback: () => transcript.restoreToolApprovalStates(previousApprovalStates),
    })) return false
    // Optimistically update the approved/rejected tool block before the
    // server snapshot arrives so the buttons disappear immediately.
    transcript.markToolApprovalDecision(approvalId, decision === 'approve' ? 'approved' : 'rejected')
    try {
      if (!sendWebSocketMessage(bid, {
        type: 'tool_approval_response',
        stream_id: streamId,
        session_id: sid,
        approval_id: approvalId,
        short_id: approval.short_id,
        decision,
      })) throw new Error('WebSocket is not connected')
    } catch (error) {
      transcript.restoreToolApprovalStates(previousApprovalStates)
      settleApprovalResponse(streamId, 'canceled', bid, sid)
      refreshLoadingForSession(bid, sid)
      toast.error(resolveApiErrorMessage(error, 'Failed to send tool approval response.'))
      return false
    }
    return true
  }

  async function respondUserInput(
    userInput: UIUserInput,
    payload: { answers?: WSUserInputAnswer[]; canceled?: boolean; reason?: string },
    target?: ChatViewTarget,
  ) {
    const viewTarget = normalizedChatViewTarget(target)
    const bid = viewTarget.botId
    const sid = viewTarget.sessionId ?? ''
    const transcript = transcriptForTarget(viewTarget)
    const userInputId = userInput.user_input_id?.trim() ?? ''
    if (!bid || !sid || !userInputId) return false
    if (userInput.status !== 'pending' || userInput.can_respond === false) return false
    const pendingKey = sidebandResponseKey(bid, sid, userInputId)
    if (pendingUserInputResponses.has(pendingKey)) return false
    if (!ensureWebSocketConnected(bid)) {
      toast.error(userInputConnectionLostMessage())
      return false
    }
    const streamId = createStreamId()
    const previousUserInputStates = transcript.snapshotUserInputStates(userInputId)
    userInputResponseStreams.set(sidebandResponseKey(bid, sid, streamId), previousUserInputStates)
    pendingUserInputResponses.set(pendingKey, {
      streamId,
      userInputId,
      botId: bid,
      sessionId: sid,
      shortId: userInput.short_id,
      answers: payload.answers ? structuredClone(payload.answers) : undefined,
      canceled: payload.canceled === true,
      reason: payload.reason,
      awaitingResync: false,
      replaySent: false,
      replayFailed: false,
    })
    const status = payload.canceled ? 'canceled' : 'submitted'
    transcript.markUserInputDecision(userInputId, status)

    try {
      if (!sendWebSocketMessage(bid, {
        type: 'user_input_response',
        stream_id: streamId,
        session_id: sid,
        user_input_id: userInputId,
        short_id: userInput.short_id,
        answers: payload.answers,
        canceled: payload.canceled === true,
        reason: payload.reason,
      })) throw new Error('WebSocket is not connected')
    } catch (error) {
      transcript.restoreUserInputStates(previousUserInputStates)
      clearUserInputResponseStream(streamId, bid, sid)
      refreshLoadingForSession(bid, sid)
      toast.error(resolveApiErrorMessage(error, 'Failed to send user input response.'))
      return false
    }
    return true
  }

  return {
    messages,
    chatView: publicChatView,
    bindChatView,
    setChatViewVisible,
    unbindChatView,
    focusChatView,
    chatTargetFor,
    workspaceTargetSelectionFor,
    setWorkspaceTargetSelection,
    initializeWorkspaceTargetSelection,
    resetWorkspaceTargetSelection,
    chatReadOnlyFor,
    chatCanForkFor,
    isChatViewStreaming,
    isChatViewCreatingSession,
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
    pendingACPRuntimeId,
    pendingACPRuntimeStatus,
    pendingACPRuntimeEnsuring,
    pendingACPStateFor,
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
    startupSendFailureFor,
    composerDraftRestoreFor,
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
    setPendingACPReasoning,
    clearPendingACPSession,
    createACPSession,
    updateCurrentSessionAgent,
    updateCurrentSessionToMemoh,
    acpRuntimeKey,
    ensureACPRuntime,
    setACPRuntimeModel,
    setACPRuntimeReasoning,
    createNewSession,
    selectDraft,
    userSentInSession,
    draftViewRequested,
    applyDraftViewRequest,
    forkedSessionRequested,
    guiToolUseRequested,
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
    clearComposerDraftRestore,
    clearCommandEvent,
    abort,
  }
})
