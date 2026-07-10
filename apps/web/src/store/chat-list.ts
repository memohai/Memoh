import { defineStore, storeToRefs } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { toast } from '@felinic/ui'
import enMessages from '@/i18n/locales/en.json'
import zhMessages from '@/i18n/locales/zh.json'
import jaMessages from '@/i18n/locales/ja.json'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  isSessionVisibleInSidebarMode,
  normalizedRuntimeType,
  provisionalSessionTitle,
  shouldRefreshFromMessageCreated,
  sortByRecency,
  upsertById,
  type SidebarSessionMode,
} from './chat-list.utils'
import {
  asRecord,
  cloneRequestedSkills,
  cloneToolApprovalState,
  cloneUserInputState,
  createStreamId,
  hasUserAttachments,
  isOptimisticTurn,
  isPendingBot,
  isSameLogicalTurn,
  mergeApprovalState,
  nextId,
  normalizeAttachment,
  normalizeForwardRef,
  normalizeReplyRef,
  normalizeRequestedSkills,
  normalizeTimestamp,
  pickRawString,
  pickString,
  requestedSkillRequestsForWire,
  resolveIsSelf,
  serverMessageId,
  skillActivationTextFromRaw,
  sortChatMessages,
  taskIdFromToolBlock,
} from './chat-list.normalize'
import { createFsChangeBeacon } from './chat/fs-beacon'
import {
  createSession,
  deleteSession as requestDeleteSession,
  ensureACPRuntime as requestEnsureACPRuntime,
  createACPRuntime as requestCreateACPRuntime,
  setACPRuntimeModel as requestSetACPRuntimeModel,
  setACPRuntimeModelByID as requestSetACPRuntimeModelByID,
  closeACPRuntime as requestCloseACPRuntime,
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
  type UIAttachment,
  type UIAttachmentsMessage,
  type UIErrorMessage,
  type UIBackgroundTask,
  type UIMessage,
  type UIReasoningMessage,
  type UIReplyRef,
  type UIForwardRef,
  type UISkillActivation,
  type UISystemTurn,
  type UITextMessage,
  type UIToolApproval,
  type UIToolMessage,
  type UIUserInput,
  type RequestedSkillSelection,
  type WSUserInputAnswer,
  type UITurn,
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

export type TextBlock = UITextMessage
export type ThinkingBlock = UIReasoningMessage
export type AttachmentItem = UIAttachment
export type AttachmentBlock = UIAttachmentsMessage
export type ErrorBlock = UIErrorMessage

export interface ToolCallBlock extends UIToolMessage {
  toolCallId: string
  toolName: string
  result: unknown | null
  done: boolean
  approval?: UIToolApproval
  userInput?: UIUserInput
  backgroundTask?: BackgroundTask
}

export type ContentBlock = TextBlock | ThinkingBlock | ToolCallBlock | AttachmentBlock | ErrorBlock

// fs-change beacon lives in ./chat/fs-beacon; types re-exported so existing
// consumers keep importing them from the store module.
export type { FsChangeBatch, FsChangeEvent, FsToolKind } from './chat/fs-beacon'

export type ActiveChatTarget =
  | {
      kind: 'session'
      sessionId: string
      session: SessionSummary | null
      runtimeType: string
      isACP: boolean
      isPendingACP: false
      metadata: Record<string, unknown>
      explicitSelection: boolean
    }
  | {
      kind: 'draft-acp'
      sessionId: null
      session: null
      runtimeType: 'acp_agent'
      isACP: true
      isPendingACP: true
      metadata: Record<string, unknown>
      explicitSelection: boolean
    }
  | {
      kind: 'draft-native'
      sessionId: null
      session: null
      runtimeType: 'model'
      isACP: false
      isPendingACP: false
      metadata: Record<string, unknown>
      explicitSelection: boolean
    }

export interface ChatUserTurn {
  id: string
  serverId?: string
  role: 'user'
  text: string
  userMessageKind?: string
  skillActivation?: UISkillActivation
  attachments: AttachmentItem[]
  reply?: UIReplyRef
  forward?: UIForwardRef
  timestamp: string
  platform?: string
  senderDisplayName?: string
  senderAvatarUrl?: string
  senderUserId?: string
  externalMessageId?: string
  streaming: boolean
  isSelf: boolean
  // Set by createOptimisticUserTurn / createOptimisticAssistantTurn and
  // cleared as soon as the server twin replaces the optimistic row in
  // mergeMessages. mergeMessages keys off this flag to decide which side of
  // a (optimistic, server) pair to drop, so any new code path that creates a
  // client-only turn before the server acknowledges it MUST set this.
  __optimistic?: boolean
}

export interface ChatAssistantTurn {
  id: string
  serverId?: string
  role: 'assistant'
  messages: ContentBlock[]
  timestamp: string
  platform?: string
  externalMessageId?: string
  streaming: boolean
  // See ChatUserTurn.__optimistic.
  __optimistic?: boolean
}

interface UserInputStateSnapshot {
  block: ToolCallBlock
  userInput: UIUserInput
}

interface ToolApprovalStateSnapshot {
  block: ToolCallBlock
  approval: UIToolApproval
}

export interface BackgroundTask {
  taskId: string
  status: string
  event?: string
  botId?: string
  sessionId?: string
  command?: string
  agentId?: string
  agentSessionId?: string
  outputFile?: string
  outputTail?: string
  stream?: string
  chunk?: string
  exitCode?: number
  duration?: string
  stalled?: boolean
}

export interface ChatSystemTurn {
  id: string
  serverId?: string
  role: 'system'
  kind: 'background_task'
  backgroundTask: BackgroundTask
  timestamp: string
  platform?: string
  streaming: boolean
}

export type ChatMessage = ChatUserTurn | ChatAssistantTurn | ChatSystemTurn

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

interface PendingAssistantStream {
  streamId: string
  assistantTurn: ChatAssistantTurn
  botId: string
  sessionId: string
  composerScope: string
  done: boolean
  resolve: () => void
  reject: (err: Error) => void
}

interface StreamIdentity {
  stream_id?: string
  session_id?: string
}

export type SendMessageStage = 'startup' | 'stream'

export interface SendMessageResult {
  ok: boolean
  stage?: SendMessageStage
  error?: string
  restoreInput?: string
  restoreAttachments?: ChatAttachment[]
  restoreRequestedSkills?: RequestedSkillSelection[]
  composerScope?: string
}

export interface SendMessageOptions {
  requestedSkills?: RequestedSkillSelection[]
  composerScope?: string
  /** Called immediately before a real chat turn is appended or dispatched. */
  onBeforeTurnAppend?: () => void
  /** Called when that turn is rolled back after a startup-stage failure. */
  onTurnAppendAborted?: () => void
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

export interface ACPAgentSessionInput {
  agentId: string
  sessionMode?: 'chat' | 'discuss'
  projectPath?: string
  projectMode?: string
  modelId?: string
  title?: string
  startRuntime?: boolean
  /** Warm pre-session runtime to bind to the created session. */
  runtimeId?: string
}

interface PendingACPStageSnapshot {
  botId: string
  generation: number
  identityKey: string
  runtimeId: string
  modelId: string
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

type StoreCommandEvent = CommandEventResponse & {
  bot_id?: string
}

interface CommandEventScope {
  botId?: string
  sessionId?: string
  composerScope?: string
}

interface EphemeralAssistantError {
  content: string
  timestamp: string
  userText?: string
}

export const useChatStore = defineStore('chat', () => {
  const selectionStore = useChatSelectionStore()
  const { currentBotId, sessionId, draftIntent, explicitSelection: explicitSessionSelection } = storeToRefs(selectionStore)

  const messages = reactive<ChatMessage[]>([])
  const pendingAssistantStreams = reactive(new Map<string, PendingAssistantStream>())
  // In-flight tool-approval responses, keyed by response stream id. Silent
  // entries belong to a session that is already streaming: their events are
  // swallowed instead of rendered as a new assistant turn. Entries normally
  // clear on the response stream's end/error; the expiry covers streams whose
  // terminal event never arrives (e.g. a WebSocket drop mid-approval), so the
  // approval doesn't stay locked against retries until a reload.
  const APPROVAL_RESPONSE_TTL_MS = 2 * 60 * 1000
  const approvalResponseStreams = new Map<string, { approvalId: string, silent: boolean, at: number }>()
  const forkingMessages = new Set<string>()
  const pendingStreams = () => [...pendingAssistantStreams.values()].filter(stream => !stream.done)
  const streamingSessionId = computed(() => {
    const activeSid = (sessionId.value ?? '').trim()
    const activeSessionIds = pendingStreams().map(stream => stream.sessionId)
    if (activeSid && activeSessionIds.includes(activeSid)) return activeSid
    return activeSessionIds[0] ?? null
  })
  const streaming = computed(() => isSessionStreaming(sessionId.value))
  const sessions = ref<SessionSummary[]>([])
  const loading = ref(false)
  // `loadingChats` covers the bot-level boot path (sessions list fetch), so
  // the sidebar can show its skeleton + suppress its empty-state placeholder
  // exactly while the sessions list is in flight.
  // `loadingMessages` covers the per-session transcript fetch — the sidebar
  // never reacts to it, only the chat pane uses it to keep its own empty
  // placeholders hidden while a fresh transcript is on its way.
  const loadingChats = ref(false)
  const loadingMessages = ref(false)
  const loadingOlder = ref(false)
  const hasMoreOlder = ref(true)
  // Tracks whether the user has scrolled back and loaded at least one page of
  // older history for the current session. Used by `refreshCurrentSession` to
  // decide between merge (preserve scrolled-back history) and replace
  // (consolidate optimistic turns against the server view). Replaces a
  // timestamp-based heuristic that misfired under client/server clock skew —
  // on a fresh session's first send the optimistic user turn could carry a
  // timestamp slightly newer than the server-persisted one, which made the
  // heuristic merge instead of replace and left two user turns visible.
  const hasLoadedOlder = ref(false)
  const initializing = ref(false)
  let initializeRerunRequested = false
  let initializingBotId: string | null = null
  let initializePromise: Promise<void> | null = null
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')
  const startupSendFailure = ref<StartupSendFailure | null>(null)
  const commandEvents = ref<Record<string, StoreCommandEvent>>({})
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

  // fs-change beacon (see ./chat/fs-beacon for the debounce/dedupe invariants).
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

  let activeWs: ChatWebSocket | null = null
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refreshPromise: { key: string; promise: Promise<void> } | null = null
  let sessionListRefreshPromise: { botId: string; promise: Promise<void> } | null = null
  const latestBackgroundTasks = new Map<string, BackgroundTask>()
  const ephemeralAssistantErrors = new Map<string, EphemeralAssistantError[]>()
  const wsCreatedSessionsByStream = new Map<string, string>()
  // Two independent streams replace the deleted bot-wide messages SSE:
  // - sessionMessagesStream follows the active sessionId and feeds the
  //   transcript (server pushes a small backlog + live messages for that
  //   session only, so no client-supplied cursor is needed).
  // - botSessionsActivityStream is bot-wide and lightweight: identifiers
  //   only, never message bodies, used to keep the sidebar live-sorted and
  //   to notice sessions created from external channels.
  const sessionMessagesStream = useRetryingStream()
  const botSessionsActivityStream = useRetryingStream()
  // O(1) lookup keeps event handlers off the list scan that previously
  // blocked the UI on bots with thousands of heartbeat sessions.
  const sessionById = new Map<string, SessionSummary>()
  const sessionLookupRevision = ref(0)
  const rememberedSessions = ref<Record<string, SessionSummary>>({})
  const deletedSessionIdsByBot = new Map<string, Set<string>>()
  const sessionsCursor = ref<string | null>(null)
  const hasMoreSessions = ref(false)
  const loadingMoreSessions = ref(false)
  const acpRuntimeStatuses = ref<Record<string, AcpagentRuntimeStatus | undefined>>({})
  const acpRuntimePending = ref<Record<string, boolean>>({})
  const acpRuntimeRequests = new Map<string, Promise<AcpagentRuntimeStatus>>()
  const pendingACPSessionInput = ref<ACPAgentSessionInput | null>(null)
  const defaultACPInputsByBot = new Map<string, ACPAgentSessionInput | null>()
  // Server-generated ID of the staged runtime; the client never invents
  // runtime identifiers.
  const pendingACPRuntimeId = ref('')
  const pendingACPCreating = ref(false)
  let pendingACPCreateRequest: Promise<AcpagentRuntimeStatus | undefined> | null = null
  let pendingACPCreateKey = ''
  let pendingACPGeneration = 0
  let selectSessionRequestId = 0

  const activeSession = computed(() => knownSessionSummary(sessionId.value ?? ''))
  const hasExplicitSessionSelection = computed(() => explicitSessionSelection.value)
  const knownSessions = computed<SessionSummary[]>(() => {
    const byId = new Map<string, SessionSummary>()
    for (const session of sessions.value) byId.set(session.id, session)
    for (const session of Object.values(rememberedSessions.value)) {
      if (!byId.has(session.id)) byId.set(session.id, session)
    }
    return [...byId.values()]
  })

  function markSessionLookupChanged() {
    sessionLookupRevision.value++
  }

  function trackSessionLookupRevision() {
    return sessionLookupRevision.value
  }

  function forkSourceMetadata(session: SessionSummary | null | undefined): Record<string, unknown> | null {
    const metadata = session?.metadata
    if (!metadata || typeof metadata !== 'object') return null
    const raw = (metadata as Record<string, unknown>).forked_from
    return raw && typeof raw === 'object' ? raw as Record<string, unknown> : null
  }

  function forkSourceAnchor(session: SessionSummary | null | undefined): string {
    return String(forkSourceMetadata(session)?.fork_message_id ?? '').trim()
  }

  function forkAnchorFromUITurns(turns: UITurn[], session?: SessionSummary | null): string {
    const cutoff = Date.parse(session?.created_at ?? '')
    const hasCutoff = Number.isFinite(cutoff)
    for (let i = turns.length - 1; i >= 0; i--) {
      const turn = turns[i]
      if (turn.role !== 'assistant') continue
      const id = String(turn.id ?? '').trim()
      if (!id) continue
      if (!hasCutoff) return id
      const timestamp = Date.parse(turn.timestamp)
      if (Number.isFinite(timestamp) && timestamp <= cutoff) return id
    }
    return ''
  }

  function withForkAnchorFromUITurns(session: SessionSummary, turns: UITurn[]): SessionSummary {
    const anchor = forkAnchorFromUITurns(turns, session)
    if (!anchor) return session
    const fork = forkSourceMetadata(session)
    if (!fork) return session
    if (forkSourceAnchor(session) === anchor) return session

    const metadata = session.metadata && typeof session.metadata === 'object' ? session.metadata : {}
    return {
      ...session,
      metadata: {
        ...metadata,
        forked_from: {
          ...fork,
          fork_message_id: anchor,
        },
      },
    }
  }

  function syncForkAnchorFromUITurns(targetSessionId: string | undefined, turns: UITurn[]) {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!sid || turns.length === 0) return
    const known = knownSessionSummary(sid)
    if (!known || !forkSourceMetadata(known)) return
    const anchored = withForkAnchorFromUITurns(known, turns)
    if (anchored === known) return
    upsertSession(anchored)
    rememberSession(anchored)
  }

  function forkMessageCandidates(message: ChatMessage): string[] {
    const out = [
      message.serverId,
      message.id,
      message.role === 'system' ? undefined : message.externalMessageId,
    ]
      .map(value => value?.trim() ?? '')
      .filter(Boolean)
    return Array.from(new Set(out))
  }

  function messageMatchesForkAnchor(message: ChatMessage, anchor: string): boolean {
    const target = anchor.trim()
    return Boolean(target) && forkMessageCandidates(message).includes(target)
  }

  function latestInheritedAssistantBefore(index: number, session?: SessionSummary | null): string {
    const cutoff = Date.parse(session?.created_at ?? '')
    const hasCutoff = Number.isFinite(cutoff)
    for (let i = index - 1; i >= 0; i--) {
      const message = messages[i]
      if (message?.role !== 'assistant') continue
      if (hasCutoff) {
        const timestamp = Date.parse(message.timestamp)
        if (!Number.isFinite(timestamp) || timestamp > cutoff) continue
      }
      return serverMessageId(message) || message.id
    }
    return ''
  }

  function updateForkAnchorForReplacedMessage(targetSessionId: string, target: ChatMessage): (() => void) | null {
    const sid = targetSessionId.trim()
    if (!sid) return null
    const known = knownSessionSummary(sid)
    const fork = forkSourceMetadata(known)
    const currentAnchor = String(fork?.fork_message_id ?? '').trim()
    if (!known || !fork || !currentAnchor) return null

    const targetIndex = messages.indexOf(target)
    if (targetIndex < 0) return null
    const replacedTailContainsAnchor = messages
      .slice(targetIndex)
      .some(message => messageMatchesForkAnchor(message, currentAnchor))
    if (!replacedTailContainsAnchor) return null
    const nextAnchor = latestInheritedAssistantBefore(targetIndex, known)
    if (nextAnchor === currentAnchor) return null

    const metadata = known.metadata && typeof known.metadata === 'object' ? known.metadata : {}
    const nextFork = { ...fork }
    if (nextAnchor) {
      nextFork.fork_message_id = nextAnchor
    } else {
      delete nextFork.fork_message_id
    }
    const next = {
      ...known,
      metadata: {
        ...metadata,
        forked_from: nextFork,
      },
    }
    replaceKnownSessionSummary(next)
    rememberSession(next)
    return () => {
      replaceKnownSessionSummary(known)
      rememberSession(known)
    }
  }

  function preserveSessionSummary(incoming: SessionSummary, known?: SessionSummary | null): SessionSummary {
    let next = incoming
    if (known && !(next.title ?? '').trim() && (known.title ?? '').trim()) {
      next = { ...next, title: known.title }
    }

    const knownFork = forkSourceMetadata(known)
    if (!knownFork || !forkSourceAnchor(known) || forkSourceAnchor(next)) {
      return next
    }

    const incomingFork = forkSourceMetadata(next)
    const metadata = next.metadata && typeof next.metadata === 'object' ? next.metadata : {}
    return {
      ...next,
      metadata: {
        ...metadata,
        forked_from: {
          ...knownFork,
          ...(incomingFork ?? {}),
          fork_message_id: knownFork.fork_message_id,
        },
      },
    }
  }

  function replaceSessions(items: SessionSummary[]): SessionSummary[] {
    const currentDeleted = deletedSessionIdsByBot.get((currentBotId.value ?? '').trim())
    // A racing list refresh can fetch a session before the backend's
    // title-generation flow has persisted a title, while the client already
    // holds one — the optimistic provisional title set in ensureActiveSession,
    // which is local-only and never sent to the server. (Server-published
    // titles don't have this problem: applyFallbackTitle and the LLM path
    // both UpdateTitle before publishing, so the DB is current by the time any
    // client sees the SSE.) An empty title in the snapshot means "server
    // hasn't set one yet," not "title cleared," so preserve our non-empty title
    // instead of letting the refresh erase it (which split the sidebar from
    // the sticky tab title).
    const merged = items
      .filter(s => !currentDeleted?.has(s.id))
      .map(s => preserveSessionSummary(s, sessionById.get(s.id) ?? rememberedSessions.value[s.id]))
    sessions.value = merged
    sessionById.clear()
    for (const s of merged) sessionById.set(s.id, s)
    markSessionLookupChanged()
    return merged
  }

  function appendSessions(items: SessionSummary[]) {
    if (items.length === 0) return
    const currentDeleted = deletedSessionIdsByBot.get((currentBotId.value ?? '').trim())
    const fresh = items
      .filter(s => !sessionById.has(s.id) && !currentDeleted?.has(s.id))
      .map(s => preserveSessionSummary(s, rememberedSessions.value[s.id]))
    if (fresh.length === 0) return
    sessions.value = [...sessions.value, ...fresh]
    for (const s of fresh) sessionById.set(s.id, s)
    markSessionLookupChanged()
  }

  function upsertSession(updated: SessionSummary) {
    const currentDeleted = deletedSessionIdsByBot.get((currentBotId.value ?? '').trim())
    if (currentDeleted?.has(updated.id)) return
    const existing = sessionById.get(updated.id)
    const remembered = rememberedSessions.value[updated.id]
    const next = preserveSessionSummary(updated, existing ?? remembered)
    if (existing) {
      const rest = sessions.value.filter(session => session.id !== updated.id)
      sessions.value = [next, ...rest]
    } else {
      sessions.value = [next, ...sessions.value]
    }
    sessionById.set(next.id, next)
    markSessionLookupChanged()
    if (remembered) rememberSession(next)
  }

  function replaceKnownSessionSummary(updated: SessionSummary) {
    const currentDeleted = deletedSessionIdsByBot.get((currentBotId.value ?? '').trim())
    if (currentDeleted?.has(updated.id)) return
    if (sessionById.has(updated.id)) {
      sessions.value = sessions.value.map(session => (session.id === updated.id ? updated : session))
    }
    sessionById.set(updated.id, updated)
    markSessionLookupChanged()
  }

  function rememberSession(updated: SessionSummary) {
    rememberedSessions.value = {
      ...rememberedSessions.value,
      [updated.id]: updated,
    }
    markSessionLookupChanged()
  }

  function forgetRememberedSession(id: string) {
    if (!rememberedSessions.value[id]) return
    const next = { ...rememberedSessions.value }
    delete next[id]
    rememberedSessions.value = next
    markSessionLookupChanged()
  }

  function knownSessionSummary(targetSessionId: string): SessionSummary | null {
    const sid = targetSessionId.trim()
    if (!sid) return null
    // `sessionById` is intentionally a non-reactive O(1) lookup. Tie computed
    // consumers such as activeChatTarget to a tiny revision so an early null
    // read is invalidated when the session list/hydration later fills the map.
    trackSessionLookupRevision()
    return sessionById.get(sid) ?? rememberedSessions.value[sid] ?? null
  }

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

  function isRecentsSession(session: SessionSummary): boolean {
    const type = (session.type ?? 'chat').trim()
    return type === 'chat' || type === 'discuss' || type === 'acp_agent'
  }

  // patchSessionInList applies a partial update to one session in BOTH the
  // reactive `sessions` array (reassigned so the sidebar virtualizer and any
  // `sessions`-derived computed re-run) and the `sessionById` lookup map. SSE
  // title/touch handlers must route through this: mutating the map's stored
  // object in place (`target.title = ...`) writes the raw object but never
  // triggers `sessions.value`, so the UI stays stale until a full REST refresh
  // (the Cmd+R symptom).
  function patchSessionInList(id: string, patch: Partial<SessionSummary>) {
    const existing = sessionById.get(id)
    if (!existing) return
    const currentDeleted = deletedSessionIdsByBot.get((existing.bot_id ?? currentBotId.value ?? '').trim())
    if (currentDeleted?.has(id)) return
    const next = { ...existing, ...patch }
    sessionById.set(id, next)
    sessions.value = sessions.value.map(session => (session.id === id ? next : session))
    markSessionLookupChanged()
  }

  function removeSessionFromList(id: string) {
    if (!sessionById.has(id) && !rememberedSessions.value[id]) return
    sessions.value = sessions.value.filter(session => session.id !== id)
    sessionById.delete(id)
    markSessionLookupChanged()
    forgetRememberedSession(id)
  }

  async function cleanupFailedDeferredSession(botId: string, targetSessionId: string, fallbackComposerScope = '') {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return

    const sessionCommandKey = commandEventKey({ botId: bid, sessionId: sid })
    const sessionCommandEvent = commandEvents.value[sessionCommandKey]
    const composerScope = sessionCommandEvent?.composer_scope?.trim() || fallbackComposerScope.trim()
    if (sessionCommandEvent) {
      const scoped = { ...sessionCommandEvent }
      delete scoped.session_id
      const next = { ...commandEvents.value }
      delete next[sessionCommandKey]
      next[commandEventKey({ botId: bid, composerScope: scoped.composer_scope || 'chat' })] = scoped
      commandEvents.value = next
    }
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
        replaceMessages([])
        hasMoreOlder.value = false
        hasLoadedOlder.value = false
      }
    }

    try {
      await requestDeleteSession(bid, sid)
    } catch {
      // Best-effort cleanup: the send failure result is the user-facing error.
    }
  }

  function fallbackSessionAfterDelete(mode: SidebarSessionMode): SessionSummary | null {
    const visibleSessions = sessions.value.filter(session => isSessionVisibleInSidebarMode(session, mode))
    return sortByRecency(visibleSessions)[0] ?? null
  }

  function markSessionDeleted(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    const deletedIds = deletedSessionIdsByBot.get(bid) ?? new Set<string>()
    deletedIds.add(sid)
    deletedSessionIdsByBot.set(bid, deletedIds)
  }

  const activeChatReadOnly = computed(() => {
    const session = activeSession.value
    if (!session) return false
    const type = session.type ?? 'chat'
    if (type === 'heartbeat' || type === 'schedule' || type === 'subagent') return true
    const ct = (session.channel_type ?? '').trim().toLowerCase()
    if (ct && ct !== 'local') return true
    return false
  })

  const activeChatCanFork = computed(() => activeSession.value?.type === 'chat')

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

  watch(currentBotId, (newId) => {
    if (newId) {
      void initialize()
    } else {
      resetUserScopedState()
    }
  }, { immediate: true })

  onAuthSessionCleared(() => resetUserScopedState({ clearSelection: true }))


  function normalizeBackgroundStatus(status?: string, event?: string): string {
    const token = (status || event || '').trim().toLowerCase()
    switch (token) {
      case 'background_started':
      case 'auto_backgrounded':
      case 'started':
      case 'output':
      case 'running':
        return 'running'
      case 'queued':
      case 'queue':
        return 'queued'
      case 'complete':
      case 'completed':
      case 'success':
      case 'succeeded':
        return 'completed'
      case 'error':
      case 'failed':
      case 'failure':
        return 'failed'
      case 'stalled':
        return 'stalled'
      case 'killed':
      case 'cancelled':
      case 'canceled':
        return 'killed'
      case 'unknown':
        return 'unknown'
      default:
        return ''
    }
  }

  function isBackgroundTaskActive(task?: BackgroundTask): boolean {
    const status = normalizeBackgroundStatus(task?.status, task?.event)
    return status === 'running' || status === 'queued' || status === 'stalled'
  }

  function normalizeBackgroundTask(task?: UIBackgroundTask, eventType?: string): BackgroundTask | null {
    if (!task) return null
    const outer = task as Record<string, unknown>
    const nested = asRecord(outer.task)
    const record = Object.keys(nested).length > 0 ? nested : outer
    const taskId = pickString(record, 'task_id', 'taskId')
    if (!taskId) return null
    const event = pickString(record, 'event') || pickString(outer, 'event') || (eventType ?? '').trim()
    const status = normalizeBackgroundStatus(pickString(record, 'status'), event) || 'running'
    const exitCode = record.exit_code ?? record.exitCode
    return {
      taskId,
      status,
      event: event || undefined,
      botId: pickString(record, 'bot_id', 'botId') || pickString(outer, 'bot_id', 'botId') || undefined,
      sessionId: pickString(record, 'session_id', 'sessionId') || pickString(outer, 'session_id', 'sessionId') || undefined,
      command: pickString(record, 'command') || undefined,
      agentId: pickString(record, 'agent_id', 'agentId') || undefined,
      agentSessionId: pickString(record, 'agent_session_id', 'agentSessionId') || undefined,
      outputFile: pickString(record, 'output_file', 'outputFile') || undefined,
      outputTail: pickRawString(record, 'output_tail', 'outputTail', 'tail') || undefined,
      stream: pickString(record, 'stream') || undefined,
      chunk: pickRawString(record, 'chunk') || undefined,
      exitCode: typeof exitCode === 'number' ? exitCode : undefined,
      duration: pickString(record, 'duration') || undefined,
      stalled: record.stalled === true || status === 'stalled',
    }
  }

  function mergeBackgroundTask(existing: BackgroundTask | undefined, incoming: BackgroundTask): BackgroundTask {
    const merged: BackgroundTask = existing ? { ...existing } : { taskId: incoming.taskId, status: incoming.status }
    const writable = merged as unknown as Record<string, unknown>
    for (const [key, value] of Object.entries(incoming)) {
      if (value === undefined || value === '') continue
      writable[key] = value
    }
    if (!incoming.outputTail && incoming.chunk) {
      merged.outputTail = `${existing?.outputTail ?? ''}${incoming.chunk}`.slice(-4096)
    }
    merged.status = normalizeBackgroundStatus(merged.status, merged.event) || merged.status || 'running'
    merged.stalled = merged.stalled === true || merged.status === 'stalled'
    return merged
  }

  function rememberBackgroundTask(task: BackgroundTask): BackgroundTask {
    const latest = mergeBackgroundTask(latestBackgroundTasks.get(task.taskId), task)
    latestBackgroundTasks.set(task.taskId, latest)
    return latest
  }

  function mergeBackgroundTaskIntoToolBlock(block: ToolCallBlock, task: BackgroundTask) {
    const merged = mergeBackgroundTask(block.backgroundTask, task)
    block.backgroundTask = merged
    block.done = !isBackgroundTaskActive(merged)
    block.running = !block.done
    block.background_task = {
      task_id: merged.taskId,
      status: merged.status,
      event: merged.event,
      bot_id: merged.botId,
      session_id: merged.sessionId,
      command: merged.command,
      output_file: merged.outputFile,
      output_tail: merged.outputTail,
      stream: merged.stream,
      chunk: merged.chunk,
      exit_code: merged.exitCode,
      duration: merged.duration,
      stalled: merged.stalled,
    }
  }

  function applyPendingBackgroundEventsToTool(block: ToolCallBlock) {
    const taskId = taskIdFromToolBlock(block)
    if (!taskId) return
    const latest = latestBackgroundTasks.get(taskId)
    if (latest) {
      mergeBackgroundTaskIntoToolBlock(block, latest)
    }
  }

  function normalizeUIMessage(msg: UIMessage): ContentBlock {
    switch (msg.type) {
      case 'tool': {
        const backgroundTask = normalizeBackgroundTask(msg.background_task)
        const block: ToolCallBlock = {
          ...msg,
          toolCallId: msg.tool_call_id,
          toolName: msg.name,
          result: msg.output ?? null,
          running: backgroundTask ? isBackgroundTaskActive(backgroundTask) : msg.running,
          done: backgroundTask ? !isBackgroundTaskActive(backgroundTask) : !msg.running,
          approval: msg.approval,
          userInput: msg.user_input,
          backgroundTask: backgroundTask ?? undefined,
          progress: msg.progress ? [...msg.progress] : undefined,
        }
        applyPendingBackgroundEventsToTool(block)
        return block
      }
      case 'attachments':
        return {
          ...msg,
          attachments: msg.attachments.map(normalizeAttachment),
        }
      case 'error':
        return { ...msg }
      default:
        return { ...msg }
    }
  }

  function normalizeTurn(turn: UITurn): ChatMessage {
    if (turn.role === 'user') {
      const userMessageKind = (turn.user_message_kind ?? '').trim()
        || (turn.skill_activation ? 'skill_activation' : undefined)
      const text = turn.skill_activation
        ? skillActivationTextFromRaw(turn.text ?? '', turn.skill_activation)
        : turn.text ?? ''
      return {
        id: String(turn.id ?? nextId()),
        role: 'user',
        text,
        userMessageKind,
        skillActivation: turn.skill_activation,
        attachments: (turn.attachments ?? []).map(normalizeAttachment),
        reply: normalizeReplyRef(turn.reply),
        forward: normalizeForwardRef(turn.forward),
        timestamp: normalizeTimestamp(turn.timestamp),
        platform: (turn.platform ?? '').trim() || undefined,
        senderDisplayName: (turn.sender_display_name ?? '').trim() || undefined,
        senderAvatarUrl: (turn.sender_avatar_url ?? '').trim() || undefined,
        senderUserId: (turn.sender_user_id ?? '').trim() || undefined,
        externalMessageId: (turn.external_message_id ?? '').trim() || undefined,
        streaming: false,
        isSelf: resolveIsSelf(turn),
      }
    }

    if (turn.role === 'system') {
      const task = normalizeBackgroundTask((turn as UISystemTurn).background_task) ?? {
        taskId: String(turn.id ?? nextId()),
        status: 'completed',
      }
      const latest = rememberBackgroundTask(task)
      return {
        id: String(turn.id ?? `system-${latest.taskId}`),
        role: 'system',
        kind: 'background_task',
        backgroundTask: latest,
        timestamp: normalizeTimestamp(turn.timestamp),
        platform: (turn.platform ?? '').trim() || undefined,
        streaming: false,
      }
    }

    return {
      id: String(turn.id ?? nextId()),
      role: 'assistant',
      messages: (turn.messages ?? []).map(normalizeUIMessage),
      timestamp: normalizeTimestamp(turn.timestamp),
      platform: (turn.platform ?? '').trim() || undefined,
      externalMessageId: (turn.external_message_id ?? '').trim() || undefined,
      streaming: false,
    }
  }

  function reconcileBackgroundTasksInMessages(items: ChatMessage[]) {
    const toolsByTaskId = new Map<string, ToolCallBlock>()
    for (const item of items) {
      if (item.role === 'assistant') {
        for (const block of item.messages) {
          if (block.type !== 'tool') continue
          const taskId = taskIdFromToolBlock(block)
          if (taskId) toolsByTaskId.set(taskId, block)
        }
        continue
      }
      if (item.role === 'system' && item.kind === 'background_task') {
        const target = toolsByTaskId.get(item.backgroundTask.taskId)
        if (target) mergeBackgroundTaskIntoToolBlock(target, item.backgroundTask)
      }
    }
  }

  function mergeBackgroundTaskIntoMatchingTools(task: BackgroundTask) {
    for (const item of messages) {
      if (item.role !== 'assistant') continue
      for (const block of item.messages) {
        if (block.type !== 'tool') continue
        if (taskIdFromToolBlock(block) === task.taskId) {
          mergeBackgroundTaskIntoToolBlock(block, task)
        }
      }
    }
  }

  function ephemeralErrorId(sessionID: string, error: EphemeralAssistantError): string {
    let hash = 0
    const input = `${error.timestamp}:${error.content}`
    for (let i = 0; i < input.length; i += 1) {
      hash = ((hash << 5) - hash + input.charCodeAt(i)) | 0
    }
    return `ephemeral-error-${sessionID}-${Math.abs(hash).toString(36)}`
  }

  function hasAssistantError(items: ChatMessage[], text: string): boolean {
    return items.some(item =>
      item.role === 'assistant'
      && item.messages.some(block => block.type === 'error' && block.content === text),
    )
  }

  function findAssistantTurnForEphemeralError(items: ChatMessage[], timestamp: string): ChatAssistantTurn | null {
    const errorTime = Date.parse(timestamp)
    let target: ChatAssistantTurn | null = null

    for (const item of items) {
      const itemTime = Date.parse(item.timestamp)
      if (!Number.isNaN(errorTime) && !Number.isNaN(itemTime) && itemTime > errorTime) {
        break
      }
      if (item.role === 'user') {
        target = null
        continue
      }
      if (item.role === 'assistant') {
        target = item
      }
    }

    return target
  }

  function findUserTurnBeforeAssistant(assistantTurn: ChatAssistantTurn): ChatUserTurn | null {
    const index = messages.indexOf(assistantTurn)
    if (index < 0) return null
    for (let i = index - 1; i >= 0; i -= 1) {
      const item = messages[i]
      if (item?.role === 'user') return item
    }
    return null
  }

  function findAnchorUserIndex(items: ChatMessage[], error: EphemeralAssistantError): number {
    const targetText = (error.userText ?? '').trim()
    let fallback = -1
    for (let i = items.length - 1; i >= 0; i -= 1) {
      const item = items[i]
      if (item?.role !== 'user') continue
      if (fallback < 0) fallback = i
      if (targetText && item.text.trim() === targetText) return i
    }
    return fallback
  }

  function findAssistantAfterAnchor(items: ChatMessage[], anchorIndex: number): ChatAssistantTurn | null {
    let target: ChatAssistantTurn | null = null
    for (let i = anchorIndex + 1; i < items.length; i += 1) {
      const item = items[i]
      if (!item) continue
      if (item.role === 'user') break
      if (item.role === 'assistant') target = item
    }
    return target
  }

  function timestampAfter(value?: string): string | null {
    const ts = Date.parse(value ?? '')
    if (Number.isNaN(ts)) return null
    return new Date(ts + 1).toISOString()
  }

  function createEphemeralErrorTurn(sessionID: string, error: EphemeralAssistantError, timestamp = error.timestamp): ChatAssistantTurn {
    return {
      id: ephemeralErrorId(sessionID, error),
      role: 'assistant',
      messages: [{
        id: 0,
        type: 'error',
        content: error.content,
      }],
      timestamp,
      streaming: false,
    }
  }

  function appendEphemeralErrors(items: ChatMessage[], targetSessionId?: string) {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!sid) return
    const errors = ephemeralAssistantErrors.get(sid)
    if (!errors?.length) return
    for (const error of errors) {
      const text = error.content.trim()
      if (!text) continue
      if (hasAssistantError(items, text)) continue

      const anchorIndex = findAnchorUserIndex(items, error)
      const assistantTurn = anchorIndex >= 0
        ? findAssistantAfterAnchor(items, anchorIndex)
        : findAssistantTurnForEphemeralError(items, error.timestamp)
      if (assistantTurn) {
        assistantTurn.messages.push({
          id: nextAssistantMessageId(assistantTurn),
          type: 'error',
          content: text,
        })
      } else {
        const insertAt = anchorIndex >= 0 ? anchorIndex + 1 : items.length
        const displayTimestamp = timestampAfter(items[anchorIndex]?.timestamp) ?? error.timestamp
        items.splice(insertAt, 0, createEphemeralErrorTurn(sid, { ...error, content: text }, displayTimestamp))
      }
    }
  }

  function normalizeTurns(items: UITurn[], targetSessionId?: string): ChatMessage[] {
    const normalized = items.map(normalizeTurn)
    reconcileBackgroundTasksInMessages(normalized)
    appendEphemeralErrors(normalized, targetSessionId)
    return normalized
  }

  // Active-session-only view. There is no per-session message cache: switching
  // sessions clears `messages` and re-fetches the new session's transcript via
  // `refreshCurrentSession`. The previous identity-preserving reconciler held
  // ChatMessage references across sessions in the cache and let
  // mergeTurnInPlace mutate them, which corrupted the view when navigating
  // between sessions. Per-session SSE delivers live updates without ever
  // touching another session's data, so cross-session caching has no purpose.

  // Carry the current view's RENDER identity onto freshly fetched turns.
  //
  // Message ids are v-for keys in chat-pane (turn containers and message
  // rows) and the scroll pin's reserve key. A refresh (stream end, SSE
  // `dropped`, retry) re-derives every turn from server rows, whose ids
  // differ from the optimistic client ids — splicing them in as-is re-keys
  // the conversation tail, which remounts the just-streamed reply (markdown
  // + code highlighting re-render = visible flash) and detaches the pinned
  // viewport's reserve. Instead, incoming turns ADOPT the id already on
  // screen; the authoritative server id stays reachable via `serverId`
  // (server-facing calls already resolve `serverId ?? id`).
  //
  // Matching, in order:
  //   1. serverId — a turn that adopted once keeps its identity across every
  //      later refresh, not just the first.
  //   2. optimistic user turns — same logical turn (externalMessageId, or
  //      text + timestamp window; see isSameLogicalTurn).
  //   3. the optimistic assistant turn directly after a matched user turn
  //      adopts from the incoming assistant directly after its match
  //      (isSameLogicalTurn deliberately refuses to guess on assistant
  //      content, but adjacency to an anchored user turn is unambiguous).
  //
  // Unlike the removed cross-session reconciler (see the note above
  // replaceMessages) this never retains or mutates EXISTING objects — it
  // only stamps ids onto the incoming ones, within the active session view.
  function adoptRenderIdentity(incoming: ChatMessage[]) {
    if (messages.length === 0 || incoming.length === 0) return
    const adopted = new Set<ChatMessage>()
    const adopt = (twin: ChatMessage, existing: ChatMessage) => {
      adopted.add(twin)
      if (twin.id === existing.id) return
      twin.serverId = twin.serverId ?? twin.id
      twin.id = existing.id
    }
    const byServerId = new Map<string, ChatMessage>()
    for (const existing of messages) {
      if (existing.serverId) byServerId.set(existing.serverId, existing)
    }
    for (const twin of incoming) {
      const prior = byServerId.get(twin.serverId ?? twin.id)
      if (prior) adopt(twin, prior)
    }
    for (let i = 0; i < messages.length; i += 1) {
      const existing = messages[i]
      if (!existing || existing.role !== 'user' || !isOptimisticTurn(existing)) continue
      const twinIndex = incoming.findIndex(turn => !adopted.has(turn) && isSameLogicalTurn(existing, turn))
      if (twinIndex === -1) continue
      adopt(incoming[twinIndex]!, existing)
      const existingNext = messages[i + 1]
      const incomingNext = incoming[twinIndex + 1]
      if (
        existingNext?.role === 'assistant' && isOptimisticTurn(existingNext)
        && incomingNext?.role === 'assistant' && !adopted.has(incomingNext)
      ) {
        adopt(incomingNext, existingNext)
      }
    }
  }

  function replaceMessages(items: UITurn[], targetSessionId?: string) {
    syncForkAnchorFromUITurns(targetSessionId, items)
    const next = normalizeTurns(items, targetSessionId)
    adoptRenderIdentity(next)
    messages.splice(0, messages.length, ...next)
  }

  // Used by locateMessageByExternalId to merge a server-supplied message window
  // into the current view.
  //
  // While the user is scrolled-back (hasLoadedOlder), an SSE-triggered refresh
  // can arrive with a server-side row for a turn the user just sent. The
  // optimistic turn in `messages` carries a client-generated id and the server
  // turn carries a different server id, so a pure id-keyed dedup leaves both
  // visible until the next session switch. Match optimistic turns to their
  // server counterparts first — by externalMessageId when present, otherwise
  // by (role, content, timestamp within 5s) — and replace the optimistic turn
  // with the server one in place.
  function mergeMessages(items: UITurn[], targetSessionId?: string) {
    const incoming = normalizeTurns(items, targetSessionId)
    adoptRenderIdentity(incoming)
    const matched = new Set<string>()
    for (let i = 0; i < messages.length; i += 1) {
      const optimistic = messages[i]
      if (!optimistic || !isOptimisticTurn(optimistic)) continue
      const replacement = incoming.find(turn => !matched.has(turn.id) && isSameLogicalTurn(optimistic, turn))
      if (replacement) {
        messages[i] = replacement
        matched.add(replacement.id)
      }
    }
    const merged = new Map<string, ChatMessage>()
    for (const item of messages) merged.set(item.id, item)
    for (const item of incoming) merged.set(item.id, item)
    const sorted = sortChatMessages([...merged.values()])
    messages.splice(0, messages.length, ...sorted)
  }

  function fallbackStreamId(targetSessionId?: string | null): string {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    return sid ? `session:${sid}:agent-stream` : 'legacy-stream'
  }

  function activeStreamIdsForSession(targetSessionId?: string | null): string[] {
    const sid = (targetSessionId ?? '').trim()
    if (!sid) return []
    return pendingStreams()
      .filter(stream => stream.sessionId === sid)
      .map(stream => stream.streamId)
  }

  function isSessionStreaming(targetSessionId?: string | null): boolean {
    return activeStreamIdsForSession(targetSessionId).length > 0
  }

  function streamIdForEvent(event: StreamIdentity, targetSessionId?: string): string {
    const explicit = (event.stream_id ?? '').trim()
    if (explicit) return explicit
    const sid = (event.session_id ?? targetSessionId ?? '').trim()
    const activeIds = activeStreamIdsForSession(sid)
    return activeIds.length === 1 ? activeIds[0]! : fallbackStreamId(sid)
  }

  function trackAssistantStream(streamId: string, assistantTurn: ChatAssistantTurn, botId: string, targetSessionId: string, composerScope = 'chat'): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const id = streamId.trim()
      if (!id) {
        reject(new Error('stream_id is required'))
        return
      }
      if (pendingAssistantStreams.has(id)) {
        reject(new Error(`stream_id ${id} is already active`))
        return
      }
      pendingAssistantStreams.set(id, {
        streamId: id,
        assistantTurn,
        botId,
        sessionId: targetSessionId.trim(),
        composerScope: composerScope.trim() || 'chat',
        done: false,
        resolve,
        reject,
      })
    })
  }

  function getAssistantStream(streamId: string): PendingAssistantStream | undefined {
    return pendingAssistantStreams.get(streamId.trim())
  }

  function finishAssistantStream(streamId: string): PendingAssistantStream | undefined {
    const stream = getAssistantStream(streamId)
    if (!stream || stream.done) return undefined
    stream.assistantTurn.streaming = false
    stream.done = true
    pendingAssistantStreams.delete(stream.streamId)
    return stream
  }

  function resolveAssistantStream(streamId: string) {
    finishAssistantStream(streamId)?.resolve()
  }

  function rejectAssistantStream(streamId: string, err: Error) {
    finishAssistantStream(streamId)?.reject(err)
  }

  function forgetAssistantStream(streamId: string) {
    pendingAssistantStreams.delete(streamId.trim())
  }

  // Append/remove operate only on the active session's `messages` array.
  // Optimistic turns belonging to a now-stale session (the user switched away
  // before the assistant stream finished) are silently dropped from the view;
  // the server keeps recording the conversation and the next REST refresh on
  // that session will surface the response.

  function appendTurnToSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    if (currentBotId.value === bid && sessionId.value === sid) {
      messages.push(turn)
    }
  }

  function isActiveSessionTarget(botId: string, targetSessionId: string): boolean {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    return Boolean(bid && sid && currentBotId.value === bid && sessionId.value === sid)
  }

  function refreshLoadingForSession(botId: string, targetSessionId: string) {
    if (!isActiveSessionTarget(botId, targetSessionId)) return
    loading.value = isSessionStreaming(targetSessionId)
  }

  function removeTurnFromSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    if (botId.trim() && targetSessionId.trim() && !isActiveSessionTarget(botId, targetSessionId)) return
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
  }

  function findMessageIndexForReplacement(turn: ChatMessage): number {
    const referenceIndex = messages.indexOf(turn)
    if (referenceIndex >= 0) return referenceIndex
    const id = serverMessageId(turn)
    if (!id) return -1
    return messages.findIndex(message => serverMessageId(message) === id)
  }

  function replaceTailFromTurn(turn: ChatMessage, replacements: ChatMessage[]): ChatMessage[] {
    const idx = findMessageIndexForReplacement(turn)
    if (idx < 0) {
      messages.push(...replacements)
      return []
    }
    const replaced = messages.slice(idx)
    messages.splice(idx, messages.length - idx, ...replacements)
    return replaced
  }

  function restoreTailFromOptimistic(
    botId: string,
    targetSessionId: string,
    optimisticUserTurn: ChatUserTurn | null,
    assistantTurn: ChatAssistantTurn,
    replacedTurns: ChatMessage[],
  ) {
    if (!isActiveSessionTarget(botId, targetSessionId)) return
    const anchor = optimisticUserTurn ?? assistantTurn
    const idx = findMessageIndexForReplacement(anchor)
    if (idx >= 0) {
      const deleteCount = optimisticUserTurn ? 2 : 1
      messages.splice(idx, deleteCount, ...replacedTurns)
      return
    }
    if (optimisticUserTurn) removeTurnFromSession(botId, targetSessionId, optimisticUserTurn)
    removeTurnFromSession(botId, targetSessionId, assistantTurn)
    if (replacedTurns.length > 0) messages.push(...replacedTurns)
  }

  function createOptimisticAssistantTurn(): ChatAssistantTurn {
    return {
      id: nextId(),
      role: 'assistant',
      messages: [],
      timestamp: new Date().toISOString(),
      streaming: true,
      __optimistic: true,
    }
  }

  function createOptimisticUserTurn(text: string, attachments?: ChatAttachment[]): ChatUserTurn {
    return {
      id: nextId(),
      role: 'user',
      text,
      attachments: (attachments ?? []).map((attachment) => ({
        type: attachment.type,
        base64: attachment.base64,
        name: attachment.name ?? '',
        mime: attachment.mime ?? '',
      })),
      timestamp: new Date().toISOString(),
      streaming: false,
      isSelf: true,
      __optimistic: true,
    }
  }

  function ensureDiscussStream(streamId: string, targetSessionId?: string): PendingAssistantStream {
    const id = streamIdForEvent({ stream_id: streamId, session_id: targetSessionId }, targetSessionId)
    const existing = getAssistantStream(id)
    if (existing && !existing.done) {
      return existing
    }
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    const bid = (currentBotId.value ?? '').trim()
    const assistantTurn = createOptimisticAssistantTurn()
    appendTurnToSession(bid, sid, assistantTurn)
    void trackAssistantStream(id, assistantTurn, bid, sid).catch((error: Error) => {
      finalizeStreamFailure(assistantTurn, bid, sid, error)
    })
    return getAssistantStream(id)!
  }

  // Approval and user-input snapshots are partial messages: the ?? / || guards
  // keep them from wiping fields the stream already filled in. The block keeps
  // its id (and reactive identity) — only content fields move.
  //
  // We use ?? / || here for partial-overlay semantics — preserving an
  // already-populated field when the incoming partial omits it. This is
  // distinct from fabricating defaults for unvalidated input, which the
  // project conventions in CLAUDE.md prohibit; here both sides are typed
  // server payloads and "absent" means "no update," not "missing data."
  function mergeToolCallBlock(existing: ToolCallBlock, incoming: ToolCallBlock) {
    Object.assign(existing, incoming, {
      id: existing.id,
      name: incoming.name || existing.name,
      toolName: incoming.toolName || existing.toolName,
      input: incoming.input ?? existing.input,
      result: incoming.result ?? existing.result,
      output: incoming.output ?? existing.output,
      approval: mergeApprovalState(existing.approval, incoming.approval),
      userInput: incoming.userInput ?? existing.userInput,
      user_input: incoming.user_input ?? existing.user_input,
      backgroundTask: incoming.backgroundTask ?? existing.backgroundTask,
      background_task: incoming.background_task ?? existing.background_task,
      progress: incoming.progress ?? existing.progress,
    })
  }

  function upsertAssistantUIMessage(turn: ChatAssistantTurn, message: UIMessage) {
    const normalized = normalizeUIMessage(message)
    if (normalized.type === 'tool' && normalized.toolCallId) {
      const existing = turn.messages.find((block): block is ToolCallBlock => block.type === 'tool' && block.toolCallId === normalized.toolCallId)
      if (existing) {
        mergeToolCallBlock(existing, normalized)
        bumpFsChangedAtIfFsMutation(message)
        return
      }
    }
    turn.messages = upsertById(turn.messages, normalized)
    bumpFsChangedAtIfFsMutation(message)
  }

  function nextAssistantMessageId(turn: ChatAssistantTurn): number {
    return turn.messages.reduce((maxId, message) => Math.max(maxId, message.id), -1) + 1
  }

  function hasVisibleAssistantBlocks(turn: ChatAssistantTurn): boolean {
    return turn.messages.some(block => block.type !== 'error')
  }

  function snapshotToolApprovalStates(approvalId: string): ToolApprovalStateSnapshot[] {
    const id = approvalId.trim()
    if (!id) return []
    const snapshots: ToolApprovalStateSnapshot[] = []
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type === 'tool' && block.approval?.approval_id === id) {
          snapshots.push({
            block,
            approval: cloneToolApprovalState(block.approval),
          })
        }
      }
    }
    return snapshots
  }

  function restoreToolApprovalStates(snapshots: ToolApprovalStateSnapshot[]) {
    for (const snapshot of snapshots) {
      if (snapshot.block.approval?.approval_id !== snapshot.approval.approval_id) continue
      snapshot.block.approval = cloneToolApprovalState(snapshot.approval)
    }
  }

  function snapshotUserInputStates(userInputId: string): UserInputStateSnapshot[] {
    const id = userInputId.trim()
    if (!id) return []
    const snapshots: UserInputStateSnapshot[] = []
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type === 'tool' && block.userInput?.user_input_id === id) {
          snapshots.push({
            block,
            userInput: cloneUserInputState(block.userInput),
          })
        }
      }
    }
    return snapshots
  }

  function restoreUserInputStates(snapshots: UserInputStateSnapshot[]) {
    for (const snapshot of snapshots) {
      if (snapshot.block.userInput?.user_input_id !== snapshot.userInput.user_input_id) continue
      snapshot.block.userInput = cloneUserInputState(snapshot.userInput)
    }
  }

  function rememberAssistantError(errorMessage: string, botId: string, targetSessionId: string, assistantTurn: ChatAssistantTurn) {
    const sid = targetSessionId.trim()
    const text = errorMessage.trim()
    if (!sid || !text) return
    const current = ephemeralAssistantErrors.get(sid) ?? []
    if (current.some(item => item.content === text)) return
    const anchorUser = findUserTurnBeforeAssistant(assistantTurn)
    ephemeralAssistantErrors.set(sid, [...current, {
      content: text,
      timestamp: new Date().toISOString(),
      userText: anchorUser?.text.trim() || undefined,
    }].slice(-5))
  }

  function appendAssistantError(assistantTurn: ChatAssistantTurn, botId: string, targetSessionId: string, errorMessage: string) {
    const text = errorMessage.trim()
    if (!text) return

    rememberAssistantError(text, botId, targetSessionId, assistantTurn)
    assistantTurn.messages.push({
      id: nextAssistantMessageId(assistantTurn),
      type: 'error',
      content: text,
    })
  }

  function finalizeStreamFailure(assistantTurn: ChatAssistantTurn, botId: string, targetSessionId: string, error: Error) {
    if (!hasVisibleAssistantBlocks(assistantTurn)) {
      removeTurnFromSession(botId, targetSessionId, assistantTurn)
      return
    }
    if (error.name === 'AbortError') return
    if (assistantTurn.messages.some(block => block.type === 'error')) return
    appendAssistantError(assistantTurn, botId, targetSessionId, error.message)
  }

  function latestOptimisticUserText(): string {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      const message = messages[i]
      if (message?.role === 'user') return message.text.trim()
    }
    return ''
  }

  function handleWSSessionCreated(event: { stream_id?: string; session_id: string }, sourceBotId = '') {
    const sid = event.session_id.trim()
    const pending = event.stream_id ? getAssistantStream(event.stream_id) : undefined
    const bid = (pending?.botId || sourceBotId || currentBotId.value || '').trim()
    if (!bid || !sid) return
    const streamId = event.stream_id?.trim()
    if (streamId) wsCreatedSessionsByStream.set(streamId, sid)
    bindAssistantStreamSession(event.stream_id, sid)
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

  function currentCommandScope(composerScope = 'chat'): CommandEventScope {
    return {
      botId: currentBotId.value ?? undefined,
      sessionId: sessionId.value ?? undefined,
      composerScope,
    }
  }

  function commandEventKey(scope: CommandEventScope) {
    const bid = (scope.botId ?? '').trim()
    const sid = (scope.sessionId ?? '').trim()
    if (bid && sid) return `session:${bid}:${sid}`
    return `composer:${bid}:${(scope.composerScope ?? 'chat').trim() || 'chat'}`
  }

  function commandEventForScope(scope: CommandEventScope = currentCommandScope()): StoreCommandEvent | null {
    return commandEvents.value[commandEventKey(scope)] ?? null
  }

  const commandEvent = computed(() => commandEventForScope())

  function rememberCommandEvent(event: CommandEventResponse | null, scope: CommandEventScope = {}) {
    if (!event) {
      clearCommandEvent(scope)
      return
    }
    const scoped: StoreCommandEvent = {
      ...event,
      composer_scope: event.composer_scope || scope.composerScope || 'chat',
    }
    const bid = (scope.botId ?? '').trim()
    if (bid) scoped.bot_id = bid
    if (!scoped.session_id) {
      const sid = (scope.sessionId ?? '').trim()
      if (sid) scoped.session_id = sid
    }
    const key = commandEventKey({
      botId: scoped.bot_id || scope.botId,
      sessionId: scoped.session_id,
      composerScope: scoped.composer_scope,
    })
    commandEvents.value = {
      ...commandEvents.value,
      [key]: scoped,
    }
  }

  function showCommandError(code: string, message: string, scope: CommandEventScope = currentCommandScope()) {
    rememberCommandEvent({
      type: 'command_error',
      invocation_id: createStreamId(),
      composer_scope: scope.composerScope || 'chat',
      terminal: true,
      error: { code, message },
    }, scope)
  }

  function clearCommandEvent(scope: CommandEventScope = currentCommandScope()) {
    const key = commandEventKey(scope)
    if (!commandEvents.value[key]) return
    const next = { ...commandEvents.value }
    delete next[key]
    commandEvents.value = next
  }

  function bindAssistantStreamSession(streamId: string | undefined, targetSessionId: string) {
    const id = streamId?.trim()
    const sid = targetSessionId.trim()
    if (!id || !sid) return
    const stream = pendingAssistantStreams.get(id)
    if (stream && !stream.sessionId) {
      stream.sessionId = sid
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

  function markToolApprovalDecision(approvalId: string, status: 'approved' | 'rejected' | 'pending') {
    const id = approvalId.trim()
    if (!id) return
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type !== 'tool' || block.approval?.approval_id !== id) continue
        block.approval = {
          ...block.approval,
          status,
          can_approve: status === 'pending',
        }
      }
    }
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
        upsertAssistantUIMessage(ensureDiscussStream(streamId, sid).assistantTurn, event.data)
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
    refreshPromise = null
    sessionListRefreshPromise = null

    replaceSessions([])
    deletedSessionIdsByBot.clear()
    sessionsCursor.value = null
    hasMoreSessions.value = false
    loadingMoreSessions.value = false
    bots.value = []
    sessionId.value = null
    explicitSessionSelection.value = false
    if (options.clearSelection && currentBotId.value) {
      currentBotId.value = null
    }
    replaceMessages([])
    hasMoreOlder.value = true
    hasLoadedOlder.value = false
    loading.value = false
    loadingChats.value = false
    loadingOlder.value = false
    initializing.value = false
    initializeRerunRequested = false
    initializingBotId = null
    initializePromise = null
    overrideModelId.value = ''
    overrideReasoningEffort.value = ''
    startupSendFailure.value = null
    commandEvents.value = {}
    resetFsBeacon()
    clearPendingACPSession()

    pendingAssistantStreams.clear()
    approvalResponseStreams.clear()
    wsCreatedSessionsByStream.clear()
    forkingMessages.clear()
    latestBackgroundTasks.clear()
    ephemeralAssistantErrors.clear()
  }

  function startWebSocket(targetBotId: string) {
    const bid = targetBotId.trim()
    stopWebSocket()
    if (!bid) return
    activeWs = connectWebSocket(bid, event => handleWSStreamEvent(event, undefined, bid))
  }

  function ensureWebSocket(targetBotId: string): ChatWebSocket | null {
    const bid = targetBotId.trim()
    if (!bid) return null
    if (!activeWs) {
      startWebSocket(bid)
    }
    return activeWs
  }

  async function refreshCurrentSession(targetBotId?: string, targetSessionId?: string) {
    const bid = (targetBotId ?? currentBotId.value ?? '').trim()
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!bid || !sid) return
    const key = `${bid}:${sid}`

    if (refreshPromise) {
      if (refreshPromise.key === key) {
        await refreshPromise.promise
        return
      }
      await refreshPromise.promise
    }

    const promise = (async () => {
      const turns = await fetchMessagesUI(bid, sid, { limit: PAGE_SIZE })
      // The user may have switched away while the request was in flight. Drop
      // the result silently — the new session has its own load underway.
      if (currentBotId.value !== bid || sessionId.value !== sid) return
      // Pick replace vs merge by whether the user has scrolled back to load
      // older history. When older pages are present we MUST preserve them
      // (otherwise an SSE-triggered refresh wipes the prepended history).
      // Otherwise replace, so optimistic in-flight turns get consolidated
      // against the server's authoritative view on stream end. The signal
      // is a flag set by `loadOlderMessages` rather than a timestamp
      // comparison, because client/server clock skew on a fresh session's
      // first send could otherwise flip the decision and duplicate the user
      // turn.
      if (hasLoadedOlder.value) {
        mergeMessages(turns, sid)
      } else {
        replaceMessages(turns, sid)
        // We cannot infer end-of-history from `turns.length < PAGE_SIZE`: the
        // server pages by raw `bot_history_messages` rows but returns merged
        // UI turns (multi-row user/assistant groups collapsed into one). A 30-
        // row page collapses to ~28 turns even when the session has thousands
        // more rows behind it, so trusting that count truncates real history.
        // Leave `hasMoreOlder` at the optimistic default and let the first
        // scroll-to-top call `loadOlderMessages`, whose authoritative
        // empty-server-response handling settles the flag correctly.
        hasMoreOlder.value = true
      }
      const latest = messages[messages.length - 1]?.timestamp
      touchSessionInList(sid, latest)
    })().finally(() => {
      if (refreshPromise?.promise === promise) {
        refreshPromise = null
      }
    })
    refreshPromise = { key, promise }

    await promise
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
      mergeBackgroundTaskIntoMatchingTools(rememberBackgroundTask(task))
      if (eventSessionId) touchSessionInList(eventSessionId)
      return
    }

    if (event.type === 'session_title_updated') {
      const sid = event.session_id.trim()
      const title = event.title.trim()
      if (!sid || !title) return
      patchSessionInList(sid, { title })
      const remembered = rememberedSessions.value[sid]
      if (remembered) rememberSession({ ...remembered, title })
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
      const target = sessionById.get(sid)
      if (target) {
        if (event.updated_at && (!target.updated_at || event.updated_at > target.updated_at)) {
          patchSessionInList(sid, { updated_at: event.updated_at })
        }
        return
      }
      const remembered = rememberedSessions.value[sid]
      if (remembered) {
        if (event.updated_at && (!remembered.updated_at || event.updated_at > remembered.updated_at)) {
          rememberSession({ ...remembered, updated_at: event.updated_at })
        }
        if (isRecentsSession(remembered)) void refreshSessionsList(targetBotId)
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
      patchSessionInList(sid, { title })
      const remembered = rememberedSessions.value[sid]
      if (remembered) rememberSession({ ...remembered, title })
      return
    }

    // session_created — server filters to user-facing types, but emits only
    // `session_id` / `title` / `created_at` (no session type, no metadata).
    // A stub with `type: undefined` would fail every consumer that branches
    // on session.type, so reload the first page instead and let the server
    // return the full summary.
    const sid = event.session_id.trim()
    if (!sid || sessionById.has(sid)) return
    void refreshSessionsList(targetBotId)
  }

  // Bumped on every `startSessionMessagesStream` call. Late-resolving
  // refreshes from a previous session must NOT clear `loadingMessages` if
  // the user has already switched to another session — the newer start
  // owns the flag now.
  let loadingMessagesVersion = 0

  function startSessionMessagesStream(targetBotId: string, targetSessionId: string) {
    sessionMessagesStream.stop()
    const bid = targetBotId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return

    // The chat pane reads `loadingMessages` to suppress empty-state
    // placeholders (e.g. "system session has no records") while a fresh
    // transcript is on its way. The sidebar deliberately ignores it — only
    // `loadingChats` (sessions-list boot) makes the sidebar spin.
    loadingMessages.value = true
    const myVersion = ++loadingMessagesVersion
    sessionMessagesStream.start(async (signal) => {
      try {
        await refreshCurrentSession(bid, sid)
      } catch (error) {
        console.error('Failed to load session messages:', error)
      } finally {
        if (myVersion === loadingMessagesVersion) loadingMessages.value = false
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
    const activeIds = activeStreamIdsForSession(sessionId.value)
    const abortError = new Error('aborted')
    abortError.name = 'AbortError'
    for (const streamId of activeIds) {
      if (activeWs?.connected) activeWs.abort(streamId)
      rejectAssistantStream(streamId, abortError)
    }
    loading.value = isSessionStreaming(sessionId.value)
  }

  function abortAllAssistantStreams() {
    const abortError = new Error('aborted')
    abortError.name = 'AbortError'
    approvalResponseStreams.clear()
    for (const stream of pendingStreams()) {
      if (activeWs?.connected) activeWs.abort(stream.streamId)
      rejectAssistantStream(stream.streamId, abortError)
    }
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

  const PAGE_SIZE = 30

  async function loadOlderMessages(): Promise<number> {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid || loadingOlder.value || !hasMoreOlder.value) return 0
    const first = messages[0]
    const firstId = serverMessageId(first)
    if (!firstId) return 0

    loadingOlder.value = true
    try {
      // Page through history with cursor advancement. When merged-turn de-dup
      // collapses an entire page to zero net-new entries, we must keep
      // advancing the message-id cursor (using the earliest returned UI turn,
      // not our local list, otherwise the cursor never moves and we'd
      // terminate prematurely).
      const MAX_DEDUP_HOPS = 4
      let cursor = firstId
      for (let hop = 0; hop < MAX_DEDUP_HOPS; hop++) {
        const turns = await fetchMessagesUI(bid, sid, {
          limit: PAGE_SIZE,
          beforeMessageId: cursor,
        })

        if (turns.length === 0) {
          hasMoreOlder.value = false
          return 0
        }

        const existingIds = new Set(messages.map(message => message.id))
        const normalized = normalizeTurns(turns)
        const older = normalized.filter(turn => !existingIds.has(turn.id))

        if (older.length > 0) {
          messages.unshift(...older)
          hasLoadedOlder.value = true
          // Don't infer end-of-history from `turns.length < PAGE_SIZE`: the
          // server pages by raw DB rows (bot_history_messages.created_at) but
          // we receive merged UI turns (multi-row user/assistant groups
          // collapsed into one), so a "short" UI page is the common case, not
          // a terminal signal. Only an empty server response (handled at the
          // top of the loop) is authoritative.
          return older.length
        }

        // All returned turns were already present locally. Advance the cursor
        // past the earliest returned turn and try again on the next hop.
        const earliestTurn = normalized[0]
        const earliest = earliestTurn ? serverMessageId(earliestTurn) : ''
        if (!earliest || earliest === cursor) {
          // Pagination cursor cannot advance; bail out to avoid a request loop.
          hasMoreOlder.value = false
          return 0
        }
        cursor = earliest
      }
      // Exhausted hop budget without finding net-new turns; treat as end of
      // history rather than spinning indefinitely.
      hasMoreOlder.value = false
      return 0
    } catch (error) {
      console.error('Failed to load older messages:', error)
      return 0
    } finally {
      loadingOlder.value = false
    }
  }

  function findMessageIdByExternalId(externalMessageId: string): string | null {
    const target = externalMessageId.trim()
    if (!target) return null
    const found = messages.find(message =>
      (message.role === 'user' || message.role === 'assistant')
      && message.externalMessageId === target,
    )
    return found?.id ?? null
  }

  async function locateMessageByExternalId(externalMessageId: string): Promise<string | null> {
    const localID = findMessageIdByExternalId(externalMessageId)
    if (localID) return localID

    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    const target = externalMessageId.trim()
    if (!bid || !sid || !target) return null

    try {
      const result = await locateMessageUI(bid, sid, target, PAGE_SIZE, PAGE_SIZE)
      if (!result.items.length) return null
      mergeMessages(result.items, sid)
      hasMoreOlder.value = true
      // locateMessage merges an older slice into the view; treat this as
      // "the user has loaded older content" so future refreshes preserve it.
      hasLoadedOlder.value = true
      return result.target_id?.trim() || findMessageIdByExternalId(target)
    } catch (error) {
      console.error('Failed to locate message:', error)
      return null
    }
  }

  function touchSessionInList(targetSessionId: string, timestamp?: string) {
    const target = sessionById.get(targetSessionId)
    if (!target) return
    if (timestamp && (!target.updated_at || timestamp > target.updated_at)) {
      patchSessionInList(targetSessionId, { updated_at: timestamp })
    }
  }

  function acpSessionMetadata(input: ACPAgentSessionInput): Record<string, unknown> {
    const agentId = input.agentId.trim()
    const projectMode = input.projectMode?.trim() || ACP_DEFAULT_PROJECT_MODE
    const projectPath = input.projectPath?.trim() || ACP_DEFAULT_PROJECT_PATH
    return {
      acp_agent_id: agentId,
      project_path: projectPath,
      acp_project_mode: projectMode,
    }
  }

  function sessionMetadata(session: SessionSummary | null): Record<string, unknown> {
    if (!session) return {}
    return {
      ...(session.metadata && typeof session.metadata === 'object' ? session.metadata : {}),
      ...(session.runtime_metadata && typeof session.runtime_metadata === 'object' ? session.runtime_metadata : {}),
    }
  }

  const pendingACPSessionMetadata = computed<Record<string, unknown> | null>(() =>
    pendingACPSessionInput.value ? acpSessionMetadata(pendingACPSessionInput.value) : null,
  )
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
  const pendingACPModelId = computed(() => pendingACPSessionInput.value?.modelId?.trim() ?? '')
  const pendingACPRuntimeStatus = computed(() => {
    const bid = currentBotId.value ?? ''
    const rid = pendingACPRuntimeId.value
    const key = acpRuntimeKey(bid, rid)
    return key ? acpRuntimeStatuses.value[key] : undefined
  })
  const pendingACPRuntimeEnsuring = computed(() => pendingACPCreating.value)

  function cloneACPInput(input: ACPAgentSessionInput): ACPAgentSessionInput {
    return { ...input }
  }

  function rememberDefaultACPInput(botId: string, input: ACPAgentSessionInput | null) {
    const bid = botId.trim()
    if (!bid) return
    defaultACPInputsByBot.set(bid, input ? cloneACPInput(input) : null)
  }

  function cachedDefaultACPInput(botId: string): { loaded: boolean, input: ACPAgentSessionInput | null } {
    const bid = botId.trim()
    if (!defaultACPInputsByBot.has(bid)) return { loaded: false, input: null }
    const input = defaultACPInputsByBot.get(bid) ?? null
    return { loaded: true, input: input ? cloneACPInput(input) : null }
  }

  function cacheDefaultACPSession(input: ACPAgentSessionInput | null) {
    rememberDefaultACPInput(currentBotId.value ?? '', input)
  }

  function pendingACPIdentityKey(botId: string, input: ACPAgentSessionInput): string {
    return [botId, input.sessionMode ?? 'chat', input.agentId, input.projectPath ?? '', input.projectMode ?? ''].join('\u0000')
  }

  function pendingACPStagingKey(snapshot: Pick<PendingACPStageSnapshot, 'identityKey' | 'generation'>): string {
    return `${snapshot.generation}\u0000${snapshot.identityKey}`
  }

  function nextPendingACPGeneration() {
    pendingACPGeneration += 1
  }

  function clearPendingACPCreateTracking() {
    pendingACPCreateRequest = null
    pendingACPCreateKey = ''
    pendingACPCreating.value = false
  }

  function closeStagedRuntime(botId: string, runtimeId: string) {
    const bid = botId.trim()
    const rid = runtimeId.trim()
    if (!bid || !rid) return
    void requestCloseACPRuntime(bid, rid).catch(() => {})
    clearACPRuntimeStatus(bid, rid)
  }

  function capturePendingACPStage(): PendingACPStageSnapshot | null {
    const botId = currentBotId.value ?? ''
    const pending = pendingACPSessionInput.value
    if (!botId || !pending) return null
    return {
      botId,
      generation: pendingACPGeneration,
      identityKey: pendingACPIdentityKey(botId, pending),
      runtimeId: pendingACPRuntimeId.value,
      modelId: pending.modelId?.trim() ?? '',
    }
  }

  function isPendingACPStageCurrent(snapshot: PendingACPStageSnapshot, modelId?: string): boolean {
    const current = capturePendingACPStage()
    if (!current) return false
    return current.botId === snapshot.botId
      && current.generation === snapshot.generation
      && current.identityKey === snapshot.identityKey
      && (modelId === undefined || current.modelId === modelId)
  }

  function stageACPSession(input: ACPAgentSessionInput, options: { explicitSelection?: boolean } = {}) {
    const metadata = acpSessionMetadata(input)
    const existing = pendingACPSessionInput.value
    const samePendingAgent = Boolean(existing
      && existing.agentId === metadata.acp_agent_id
      && (existing.sessionMode || 'chat') === (input.sessionMode || 'chat')
      && (existing.projectPath || ACP_DEFAULT_PROJECT_PATH) === metadata.project_path
      && (existing.projectMode || ACP_DEFAULT_PROJECT_MODE) === metadata.acp_project_mode)
    if (!samePendingAgent) {
      nextPendingACPGeneration()
      clearPendingACPCreateTracking()
    }
    pendingACPSessionInput.value = {
      ...input,
      agentId: String(metadata.acp_agent_id ?? ''),
      projectPath: String(metadata.project_path ?? ''),
      projectMode: String(metadata.acp_project_mode ?? ''),
      modelId: input.modelId?.trim() || (samePendingAgent ? existing?.modelId : '') || '',
    }
    if (!samePendingAgent && pendingACPRuntimeId.value) {
      const bid = currentBotId.value ?? ''
      const runtimeId = pendingACPRuntimeId.value
      pendingACPRuntimeId.value = ''
      closeStagedRuntime(bid, runtimeId)
    }
    explicitSessionSelection.value = options.explicitSelection !== false
  }

  function stageDefaultACPSession(input: ACPAgentSessionInput) {
    rememberDefaultACPInput(currentBotId.value ?? '', input)
    selectSessionRequestId++
    explicitSessionSelection.value = false
    draftIntent.value = false
    sessionId.value = null
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
    stageACPSession(input, { explicitSelection: false })
  }

  function stageNewACPSession(input: ACPAgentSessionInput) {
    selectSessionRequestId++
    clearPendingACPSession()
    sessionId.value = null
    draftIntent.value = true
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
    stageACPSession(input, { explicitSelection: true })
  }

  function resetToEmptyComposer(options: { clearPendingACP?: boolean; explicitSelection?: boolean; draftIntent?: boolean } = {}) {
    selectSessionRequestId++
    if (options.clearPendingACP !== false) {
      clearPendingACPSession()
    }
    sessionId.value = null
    explicitSessionSelection.value = options.explicitSelection === true
    draftIntent.value = options.draftIntent ?? options.explicitSelection === true
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  async function ensurePendingACPRuntime(): Promise<AcpagentRuntimeStatus | undefined> {
    const snapshot = capturePendingACPStage()
    const pending = pendingACPSessionInput.value
    if (!snapshot || !pending) return undefined
    if (snapshot.runtimeId) {
      const key = acpRuntimeKey(snapshot.botId, snapshot.runtimeId)
      return acpRuntimeStatuses.value[key]
    }
    const stagingKey = pendingACPStagingKey(snapshot)
    if (pendingACPCreateRequest && pendingACPCreateKey === stagingKey) return pendingACPCreateRequest

    pendingACPCreating.value = true
    const request = requestCreateACPRuntime(snapshot.botId, {
      agentId: pending.agentId,
      projectPath: pending.projectPath,
    })
      .then((runtime) => {
        const rid = runtime?.runtime_id?.trim() ?? ''
        const current = capturePendingACPStage()
        const stillStaged = !!current
          && pendingACPStagingKey(current) === stagingKey
          && !current.runtimeId
        if (stillStaged && rid) {
          pendingACPRuntimeId.value = rid
          setACPRuntimeStatus(snapshot.botId, rid, runtime)
        } else if (rid) {
          // Staging changed while the runtime was starting: discard it.
          closeStagedRuntime(snapshot.botId, rid)
        }
        return runtime
      })
      .catch((error) => {
        if (!isPendingACPStageCurrent(snapshot)) return undefined
        throw error
      })
      .finally(() => {
        if (pendingACPCreateRequest === request) {
          clearPendingACPCreateTracking()
        }
      })
    pendingACPCreateRequest = request
    pendingACPCreateKey = stagingKey
    return request
  }

  async function setPendingACPModel(modelId: string) {
    if (!pendingACPSessionInput.value) return
    const mid = modelId.trim()
    const previousModelId = pendingACPSessionInput.value.modelId?.trim() ?? ''
    if (mid === previousModelId) return

    pendingACPSessionInput.value = {
      ...pendingACPSessionInput.value,
      modelId: mid,
    }

    const initialSnapshot = capturePendingACPStage()
    if (!initialSnapshot) return

    try {
      const runtimeId = await pendingACPModelRuntime(initialSnapshot, mid)
      if (!runtimeId) return
      await setPendingACPModelOnRuntime(initialSnapshot, runtimeId, mid)
    } catch (error) {
      if (!isPendingACPStageCurrent(initialSnapshot, mid)) return
      if (pendingACPSessionInput.value?.modelId?.trim() === mid) {
        pendingACPSessionInput.value = {
          ...pendingACPSessionInput.value,
          modelId: previousModelId,
        }
      }
      throw error
    }
  }

  async function pendingACPModelRuntime(snapshot: PendingACPStageSnapshot, modelId: string): Promise<string> {
    const current = capturePendingACPStage()
    if (!current || !isPendingACPStageCurrent(snapshot, modelId)) return ''
    if (current.runtimeId || !modelId) return current.runtimeId
    await ensurePendingACPRuntime()
    if (!isPendingACPStageCurrent(snapshot, modelId)) return ''
    return capturePendingACPStage()?.runtimeId ?? ''
  }

  async function setPendingACPModelOnRuntime(snapshot: PendingACPStageSnapshot, runtimeId: string, modelId: string) {
    try {
      const runtime = await requestSetACPRuntimeModelByID(snapshot.botId, runtimeId, modelId)
      if (!isPendingACPStageCurrent(snapshot, modelId)) return
      setACPRuntimeStatus(snapshot.botId, runtimeId, runtime)
    } catch (error) {
      if (!isPendingACPStageCurrent(snapshot, modelId)) return
      if (!isRuntimeNotFoundError(error)) throw error
      if (pendingACPRuntimeId.value !== runtimeId) return

      clearACPRuntimeStatus(snapshot.botId, runtimeId)
      pendingACPRuntimeId.value = ''

      const freshId = await pendingACPModelRuntime(snapshot, modelId)
      if (!freshId) return
      const runtime = await requestSetACPRuntimeModelByID(snapshot.botId, freshId, modelId)
      if (!isPendingACPStageCurrent(snapshot, modelId)) return
      setACPRuntimeStatus(snapshot.botId, freshId, runtime)
    }
  }

  // The runtime endpoints fail closed with this fixed message when the
  // referenced runtime is gone (idle-reaped or never existed).
  function isRuntimeNotFoundError(error: unknown): boolean {
    if (!error || typeof error !== 'object') return false
    const message = (error as { message?: unknown }).message
    return typeof message === 'string' && message.includes('runtime not found')
  }

  function clearPendingACPSession() {
    const bid = currentBotId.value ?? ''
    const runtimeId = pendingACPRuntimeId.value
    nextPendingACPGeneration()
    clearPendingACPCreateTracking()
    closeStagedRuntime(bid, runtimeId)
    pendingACPSessionInput.value = null
    pendingACPRuntimeId.value = ''
  }

  // Detaches the staged ACP session without closing its warm runtime, so the
  // first send can bind the runtime to the real session.
  function detachPendingACPSession(): { input: ACPAgentSessionInput; runtimeId: string } | null {
    const pending = pendingACPSessionInput.value
    if (!pending) return null
    const runtimeId = pendingACPRuntimeId.value
    nextPendingACPGeneration()
    clearPendingACPCreateTracking()
    pendingACPSessionInput.value = null
    pendingACPRuntimeId.value = ''
    return { input: { ...pending }, runtimeId }
  }

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
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
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
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
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

  function pendingACPMatchesInput(input: ACPAgentSessionInput): boolean {
    const pending = pendingACPSessionInput.value
    if (!pending || sessionId.value) return false
    const metadata = acpSessionMetadata(input)
    return pending.agentId === metadata.acp_agent_id
      && (pending.sessionMode || 'chat') === (input.sessionMode || 'chat')
      && (pending.projectPath || ACP_DEFAULT_PROJECT_PATH) === metadata.project_path
      && (pending.projectMode || ACP_DEFAULT_PROJECT_MODE) === metadata.acp_project_mode
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
          hasLoadedOlder.value = false
          stopStreams()
          stopWebSocket()

          const bid = await ensureBot()
          if (!bid) {
            replaceSessions([])
            sessionsCursor.value = null
            hasMoreSessions.value = false
            sessionId.value = null
            clearPendingACPSession()
            replaceMessages([])
            hasMoreOlder.value = false
            hasLoadedOlder.value = false
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
            hasMoreOlder.value = false
            hasLoadedOlder.value = false
          } else if (preserveExplicitEmptyComposer) {
            sessionId.value = null
            replaceMessages([])
            hasMoreOlder.value = false
            hasLoadedOlder.value = false
          } else if (preferDefaultACP) {
            sessionId.value = null
            explicitSessionSelection.value = false
            draftIntent.value = false
            replaceMessages([])
            hasMoreOlder.value = false
            hasLoadedOlder.value = false
          } else if (restoredExplicitSession) {
            draftIntent.value = false
          } else if (!visibleSessions.length) {
            sessionId.value = null
            explicitSessionSelection.value = false
            replaceMessages([])
            hasMoreOlder.value = false
            hasLoadedOlder.value = false
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
              replaceMessages([])
              hasMoreOlder.value = false
              hasLoadedOlder.value = false
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
  // pending watcher finally runs `replaceMessages([])`).
  //
  // We deliberately do NOT call `abortAllAssistantStreams()` here: an
  // assistant stream that started in session A keeps running server-side
  // after the user switches to B, and finalizes against A's history when
  // the user comes back (the `appendTurnToSession` / WS handlers are
  // already gated on `sessionId.value === <stream's sessionId>`, so the
  // orphan does not bleed into B's view).
  function switchActiveSession(sid: string) {
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
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
    rememberedSessions.value = {}
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
      replaceMessages([])
      hasMoreOlder.value = false
      hasLoadedOlder.value = false
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

      const turns = await fetchMessagesUI(bid, forked.id, { limit: PAGE_SIZE })
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
      replaceMessages(turns, forked.id)
      hasMoreOlder.value = true
      hasLoadedOlder.value = false
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
        messages.push(userTurn)
        messages.push(assistantTurn)
      }

      const modelId = overrideModelId.value || undefined
      const effort = overrideReasoningEffort.value
      const reasoningEffort = effort || undefined

      const ws = ensureWebSocket(bid)
      if (ws) {
        if (!ws.connected) {
          throw new StreamFailureError('WebSocket is not connected', 'startup')
        }
        const completion = trackAssistantStream(sendStreamId, assistantTurn, bid, sid, composerScope)
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
        const createdSessionId = wsCreatedSessionsByStream.get(sendStreamId) ?? ''
        const fallbackActiveSessionId = (currentBotId.value ?? '').trim() === bid ? sessionId.value ?? '' : ''
        const refreshSessionId = sendSessionId || createdSessionId || fallbackActiveSessionId
        wsCreatedSessionsByStream.delete(sendStreamId)
        if (refreshSessionId) await refreshCurrentSession(bid, refreshSessionId)
      } else {
        if (serverSkillActivation) throw new StreamFailureError('WebSocket is required for skill activation', 'startup')
        await sendLocalChannelMessage(bid, trimmed, attachments, { modelId, reasoningEffort })
        await refreshCurrentSession(bid, sid)
      }

      assistantTurn.streaming = false
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
      const createdSessionId = sendStreamId ? wsCreatedSessionsByStream.get(sendStreamId) ?? '' : ''
      const bid = sendBotId || currentBotId.value || ''
      const sid = sendSessionId || (deferSessionCreation ? '' : sessionId.value || '')

      if (assistantTurn) finalizeStreamFailure(assistantTurn, bid, sid, err)
      if (!isAbort && stage === 'startup' && userTurn) {
        removeTurnFromSession(bid, sid, userTurn)
      }
      if (!isAbort && stage === 'startup' && deferSessionCreation && wasDraft && createdSessionId) {
        await cleanupFailedDeferredSession(bid, createdSessionId, composerScope)
      }

      if (sendStreamId) forgetAssistantStream(sendStreamId)
      if (sendStreamId) wsCreatedSessionsByStream.delete(sendStreamId)
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
      const completion = trackAssistantStream(streamId, assistantTurn, bid, sid)
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
      forgetAssistantStream(streamId)
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
      const completion = trackAssistantStream(streamId, assistantTurn, bid, sid)
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
      forgetAssistantStream(streamId)
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
      messages.push(assistantTurn)
      void trackAssistantStream(streamId, assistantTurn, bid, sid).catch((error: Error) => {
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
        forgetAssistantStream(streamId)
        if (assistantTurn) {
          const idx = messages.indexOf(assistantTurn)
          if (idx >= 0) messages.splice(idx, 1)
        }
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
    messages.push(assistantTurn)
    void trackAssistantStream(streamId, assistantTurn, bid, sid).catch((error: Error) => {
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
      forgetAssistantStream(streamId)
      const idx = messages.indexOf(assistantTurn)
      if (idx >= 0) messages.splice(idx, 1)
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
