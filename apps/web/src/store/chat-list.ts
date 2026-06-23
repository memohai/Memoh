import { defineStore, storeToRefs } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { toast } from '@memohai/ui'
import enMessages from '@/i18n/locales/en.json'
import zhMessages from '@/i18n/locales/zh.json'
import jaMessages from '@/i18n/locales/ja.json'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useUserStore } from '@/store/user'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { provisionalSessionTitle, shouldRefreshFromMessageCreated, upsertById } from './chat-list.utils'
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
  fetchSessions,
  type Bot,
  type BotSessionActivityEvent,
  type SessionSummary,
  type SessionMessageStreamEvent,
  type ChatAttachment,
  type ChatWebSocket,
  type UIAttachment,
  type UIAttachmentsMessage,
  type UIErrorMessage,
  type UIBackgroundTask,
  type UIMessage,
  type UIReasoningMessage,
  type UIReplyRef,
  type UIForwardRef,
  type UISystemTurn,
  type UITextMessage,
  type UIToolApproval,
  type UIToolMessage,
  type UIUserInput,
  type WSUserInputAnswer,
  type UITurn,
  type UIUserTurn,
  type UIStreamEvent,
  fetchBots,
  fetchMessagesUI,
  sendLocalChannelMessage,
  streamBotSessionsActivityEvents,
  streamSessionMessageEvents,
  connectWebSocket,
  locateMessageUI,
} from '@/composables/api/useChat'
import { ACP_DEFAULT_PROJECT_MODE, ACP_DEFAULT_PROJECT_PATH } from '@/utils/acp'
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

export interface FsChangeBatch {
  at: number
  // null = unknown / wildcard (exec completion, manual refresh, user-driven mutation)
  paths: ReadonlySet<string> | null
}

export type FsToolKind = 'write' | 'edit' | 'apply_patch' | 'exec'

// Rich metadata for fs-mutating tool calls that landed on a known absolute
// path. Stored per-path so the file viewer can show context (which agent, when,
// what was written) and so the Compare flow can diff against the agent's
// content without an extra round-trip.
export interface FsChangeEvent {
  at: number
  path: string
  kind: FsToolKind
  toolCallId: string
  sessionId: string
  writeContent?: string
  editOldText?: string
  editNewText?: string
}

export interface ChatUserTurn {
  id: string
  serverId?: string
  role: 'user'
  text: string
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

function userInputConnectionLostMessage() {
  const locale = currentLocale()
  const messages = locale === 'zh' ? zhMessages : locale === 'ja' ? jaMessages : enMessages
  return messages.chat.tools.userInputConnectionLost
}

interface PendingAssistantStream {
  streamId: string
  assistantTurn: ChatAssistantTurn
  botId: string
  sessionId: string
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
}

export interface ACPAgentSessionInput {
  agentId: string
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
  error: string
  restoreInput: string
  restoreAttachments?: ChatAttachment[]
}

class StreamFailureError extends Error {
  stage: SendMessageStage

  constructor(message: string, stage: SendMessageStage) {
    super(message)
    this.name = 'StreamFailureError'
    this.stage = stage
  }
}

interface EphemeralAssistantError {
  content: string
  timestamp: string
  userText?: string
}

export const useChatStore = defineStore('chat', () => {
  const selectionStore = useChatSelectionStore()
  const { currentBotId, sessionId, draftIntent } = storeToRefs(selectionStore)

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
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')
  const startupSendFailure = ref<StartupSendFailure | null>(null)
  // Bumps when the user sends a message, carrying the resolved session id and
  // whether that send just promoted a draft (created the session). The workspace
  // tab store watches this to pin the chat tab — a session you have sent in is no
  // longer an ephemeral "preview" tab. seq forces the watch to fire on repeats.
  const userSentInSession = ref<{ id: string, wasDraft: boolean, seq: number } | null>(null)
  let userSendSeq = 0

  // Bumps every time a fs-mutating tool call (write/edit/apply_patch/exec) finishes for the
  // current bot. File-manager components watch this to refresh their listings
  // and any open file viewers without polling. Trailing fixed-delay throttle so
  // a burst of edits within one window collapses into one refresh. Each batch
  // carries the set of paths touched in that window (or null = wildcard, for
  // exec and other unknown-impact triggers) so consumers can filter by path.
  const fsChangedAt = ref(0)
  const lastFsChange = ref<FsChangeBatch | null>(null)
  // Most recent rich event per absolute path. Powers the file-viewer chip's
  // "who did what" context and the Compare view's diff baseline. Wildcard
  // events (exec / apply_patch / relative paths) are intentionally absent —
  // those still fire fsChangedAt but contribute no per-path metadata.
  const lastFsEvents = ref<Map<string, FsChangeEvent>>(new Map())
  const FS_MUTATING_TOOLS = new Set(['write', 'edit', 'apply_patch', 'exec'])
  const FS_CHANGED_DEBOUNCE_MS = 150
  let fsChangedBumpTimer: ReturnType<typeof setTimeout> | null = null
  let pendingFsPaths: Set<string> | null = new Set()
  let pendingFsEvents = new Map<string, FsChangeEvent>()
  // Bot at the moment the in-flight batch started. If currentBotId changes
  // before the timer fires, the batch belongs to the old bot and we drop it
  // rather than leak it into the new bot's UI.
  let pendingFsBotId: string | null = null
  // Tool calls we've already bumped (or seen at load time) for the current
  // bot. Prevents double-bumping when a tool first arrives via the WS stream
  // and then re-appears via the stream-end / message_created refresh path.
  const seenFsToolCallIds = new Set<string>()

  function markFsChanged(path?: string | null) {
    if (path === undefined || path === null) {
      pendingFsPaths = null
    } else if (pendingFsPaths !== null) {
      pendingFsPaths.add(path)
    }
    if (fsChangedBumpTimer != null) return
    pendingFsBotId = currentBotId.value
    fsChangedBumpTimer = setTimeout(() => {
      fsChangedBumpTimer = null
      const recordedBotId = pendingFsBotId
      const paths = pendingFsPaths
      const events = pendingFsEvents
      pendingFsBotId = null
      pendingFsPaths = new Set()
      pendingFsEvents = new Map()
      if (recordedBotId !== currentBotId.value) return
      const at = Date.now()
      lastFsChange.value = { at, paths }
      fsChangedAt.value = at
      if (events.size > 0) {
        const next = new Map(lastFsEvents.value)
        for (const [p, ev] of events) next.set(p, ev)
        lastFsEvents.value = next
      }
    }, FS_CHANGED_DEBOUNCE_MS)
  }

  function cancelPendingFsBump() {
    if (fsChangedBumpTimer != null) {
      clearTimeout(fsChangedBumpTimer)
      fsChangedBumpTimer = null
    }
    pendingFsPaths = new Set()
    pendingFsEvents = new Map()
    pendingFsBotId = null
  }

  function affectsPath(path: string): boolean {
    const change = lastFsChange.value
    if (!change) return false
    if (change.paths === null) return true
    return change.paths.has(path)
  }

  function fsEventForPath(path: string): FsChangeEvent | null {
    return lastFsEvents.value.get(path) ?? null
  }

  function extractToolMessagePath(message: UIMessage): string | null {
    if (message.type !== 'tool') return null
    const input = message.input
    if (typeof input !== 'object' || input === null) return null
    const path = (input as Record<string, unknown>).path
    if (typeof path !== 'string' || !path) return null
    // Only emit absolute paths as path-targeted hints. Viewer filePaths are
    // always absolute (the FS list API normalizes them); a relative path here
    // can't be safely compared without knowing the agent's cwd, so fall through
    // to wildcard and let every viewer decide whether to refresh.
    if (!path.startsWith('/')) return null
    return path
  }

  function buildFsChangeEvent(message: UIMessage, path: string, callId: string): FsChangeEvent | null {
    if (message.type !== 'tool') return null
    const input = message.input
    const event: FsChangeEvent = {
      at: Date.now(),
      path,
      kind: message.name as FsToolKind,
      toolCallId: callId,
      sessionId: (sessionId.value ?? '').trim(),
    }
    if (typeof input === 'object' && input !== null) {
      const rec = input as Record<string, unknown>
      if (message.name === 'write' && typeof rec.content === 'string') {
        event.writeContent = rec.content
      } else if (message.name === 'edit') {
        if (typeof rec.old_text === 'string') event.editOldText = rec.old_text
        if (typeof rec.new_text === 'string') event.editNewText = rec.new_text
      }
    }
    return event
  }

  function bumpFsChangedAtIfFsMutation(message: UIMessage) {
    if (message.type !== 'tool') return
    if (message.running) return
    if (!FS_MUTATING_TOOLS.has(message.name)) return
    const callId = message.tool_call_id?.trim() ?? ''
    if (callId && seenFsToolCallIds.has(callId)) return
    if (callId) seenFsToolCallIds.add(callId)
    // write / edit carry their target `path` in input. apply_patch can target
    // many files (multi-path parsing belongs to the view layer, not the store)
    // and exec is opaque — both fall back to wildcard.
    const path = (message.name === 'write' || message.name === 'edit')
      ? extractToolMessagePath(message)
      : null
    if (path) {
      const event = buildFsChangeEvent(message, path, callId)
      if (event) pendingFsEvents.set(path, event)
    }
    markFsChanged(path)
  }

  let activeWs: ChatWebSocket | null = null
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refreshPromise: { key: string; promise: Promise<void> } | null = null
  let sessionListRefreshPromise: { botId: string; promise: Promise<void> } | null = null
  const latestBackgroundTasks = new Map<string, BackgroundTask>()
  const ephemeralAssistantErrors = new Map<string, EphemeralAssistantError[]>()
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
  const sessionsCursor = ref<string | null>(null)
  const hasMoreSessions = ref(false)
  const loadingMoreSessions = ref(false)
  const acpRuntimeStatuses = ref<Record<string, AcpagentRuntimeStatus | undefined>>({})
  const acpRuntimePending = ref<Record<string, boolean>>({})
  const acpRuntimeRequests = new Map<string, Promise<AcpagentRuntimeStatus>>()
  const pendingACPSessionInput = ref<ACPAgentSessionInput | null>(null)
  // Server-generated ID of the staged runtime; the client never invents
  // runtime identifiers.
  const pendingACPRuntimeId = ref('')
  const pendingACPCreating = ref(false)
  let pendingACPCreateRequest: Promise<AcpagentRuntimeStatus | undefined> | null = null
  let pendingACPCreateKey = ''
  let pendingACPGeneration = 0

  // NOTE: activeSession reads from the plain (non-reactive) sessionById Map
  // and only re-evaluates when sessionId.value changes — it is NOT live-updated
  // by patchSessionInList (which swaps a *new* object into the Map). That is
  // fine today because consumers here read only immutable-ish fields (.type /
  // .metadata / .id), never a live title. If you bind a title display to
  // activeSession it will go stale — read from chatStore.sessions instead.
  const activeSession = computed(() => sessionById.get(sessionId.value ?? '') ?? null)

  function replaceSessions(items: SessionSummary[]) {
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
    const merged = items.map(s => {
      const known = sessionById.get(s.id)
      if (known && !(s.title ?? '').trim() && (known.title ?? '').trim()) {
        return { ...s, title: known.title }
      }
      return s
    })
    sessions.value = merged
    sessionById.clear()
    for (const s of merged) sessionById.set(s.id, s)
  }

  function appendSessions(items: SessionSummary[]) {
    if (items.length === 0) return
    const fresh = items.filter(s => !sessionById.has(s.id))
    if (fresh.length === 0) return
    sessions.value = [...sessions.value, ...fresh]
    for (const s of fresh) sessionById.set(s.id, s)
  }

  function upsertSession(updated: SessionSummary) {
    const existing = sessionById.get(updated.id)
    if (existing) {
      const rest = sessions.value.filter(session => session.id !== updated.id)
      sessions.value = [updated, ...rest]
    } else {
      sessions.value = [updated, ...sessions.value]
    }
    sessionById.set(updated.id, updated)
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
    const next = { ...existing, ...patch }
    sessionById.set(id, next)
    sessions.value = sessions.value.map(session => (session.id === id ? next : session))
  }

  function removeSessionFromList(id: string) {
    if (!sessionById.has(id)) return
    sessions.value = sessions.value.filter(session => session.id !== id)
    sessionById.delete(id)
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

  const nextId = () => `${Date.now()}-${Math.floor(Math.random() * 1000)}`

  const isPendingBot = (bot: Bot | null | undefined) =>
    bot?.status === 'creating' || bot?.status === 'deleting'

  function normalizeTimestamp(value?: string): string {
    const raw = (value ?? '').trim()
    if (!raw) return new Date().toISOString()
    const parsed = new Date(raw)
    return Number.isNaN(parsed.getTime()) ? new Date().toISOString() : parsed.toISOString()
  }

  function resolveIsSelf(turn: UIUserTurn): boolean {
    const platform = (turn.platform ?? '').trim().toLowerCase()
    if (!platform || platform === 'local') return true
    const senderUserId = (turn.sender_user_id ?? '').trim()
    if (!senderUserId) return false
    const userStore = useUserStore()
    const currentUserId = (userStore.userInfo.id ?? '').trim()
    if (!currentUserId) return false
    return senderUserId === currentUserId
  }

  function normalizeAttachment(att: UIAttachment): AttachmentItem {
    return { ...att }
  }

  function normalizeReplyRef(reply?: UIReplyRef): UIReplyRef | undefined {
    if (!reply) return undefined
    const normalized = {
      message_id: (reply.message_id ?? '').trim(),
      sender: (reply.sender ?? '').trim(),
      preview: (reply.preview ?? '').trim(),
      attachments: (reply.attachments ?? []).map(normalizeAttachment),
    }
    return normalized.message_id || normalized.sender || normalized.preview || normalized.attachments.length ? normalized : undefined
  }

  function normalizeForwardRef(forward?: UIForwardRef): UIForwardRef | undefined {
    if (!forward) return undefined
    const normalized = {
      message_id: (forward.message_id ?? '').trim(),
      from_user_id: (forward.from_user_id ?? '').trim(),
      from_conversation_id: (forward.from_conversation_id ?? '').trim(),
      sender: (forward.sender ?? '').trim(),
      date: typeof forward.date === 'number' && Number.isFinite(forward.date) ? forward.date : undefined,
    }
    return normalized.message_id || normalized.from_user_id || normalized.from_conversation_id || normalized.sender || normalized.date
      ? normalized
      : undefined
  }

  function asRecord(value: unknown): Record<string, unknown> {
    return value && typeof value === 'object' ? value as Record<string, unknown> : {}
  }

  function pickString(obj: Record<string, unknown>, ...keys: string[]): string {
    for (const key of keys) {
      const value = obj[key]
      if (typeof value === 'string' && value.trim()) return value.trim()
    }
    return ''
  }

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
    const record = task as Record<string, unknown>
    const taskId = pickString(record, 'task_id', 'taskId')
    if (!taskId) return null
    const event = pickString(record, 'event') || (eventType ?? '').trim()
    const status = normalizeBackgroundStatus(pickString(record, 'status'), event) || 'running'
    const exitCode = record.exit_code ?? record.exitCode
    return {
      taskId,
      status,
      event: event || undefined,
      botId: pickString(record, 'bot_id', 'botId') || undefined,
      sessionId: pickString(record, 'session_id', 'sessionId') || undefined,
      command: pickString(record, 'command') || undefined,
      agentId: pickString(record, 'agent_id', 'agentId') || undefined,
      agentSessionId: pickString(record, 'agent_session_id', 'agentSessionId') || undefined,
      outputFile: pickString(record, 'output_file', 'outputFile') || undefined,
      outputTail: pickString(record, 'output_tail', 'outputTail', 'tail') || undefined,
      stream: pickString(record, 'stream') || undefined,
      chunk: pickString(record, 'chunk') || undefined,
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

  function structuredToolResult(result: unknown): Record<string, unknown> {
    const record = asRecord(result)
    const structured = asRecord(record.structuredContent)
    return Object.keys(structured).length > 0 ? structured : record
  }

  function taskIdFromToolBlock(block: ToolCallBlock): string {
    if (block.backgroundTask?.taskId) return block.backgroundTask.taskId
    const structured = structuredToolResult(block.result)
    const result = asRecord(block.result)
    return pickString(structured, 'task_id', 'taskId') || pickString(result, 'task_id', 'taskId')
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
      return {
        id: String(turn.id ?? nextId()),
        role: 'user',
        text: turn.text ?? '',
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

  function replaceMessages(items: UITurn[], targetSessionId?: string) {
    const next = normalizeTurns(items, targetSessionId)
    messages.splice(0, messages.length, ...next)
  }

  function sortChatMessages(items: ChatMessage[]): ChatMessage[] {
    return [...items].sort((a, b) => {
      const at = Date.parse(a.timestamp)
      const bt = Date.parse(b.timestamp)
      if (!Number.isNaN(at) && !Number.isNaN(bt) && at !== bt) return at - bt
      return a.id.localeCompare(b.id)
    })
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

  // Optimistic turns set `__optimistic: true` at construction
  // (createOptimisticUserTurn / createOptimisticAssistantTurn). Server-derived
  // turns from fetchMessagesUI and SSE never carry this flag, so an opaque id
  // shape (numeric, UUID, slug) is irrelevant here.
  function isOptimisticTurn(turn: ChatMessage): boolean {
    return turn.__optimistic === true
  }

  const SAME_TURN_TIMESTAMP_TOLERANCE_MS = 5_000

  function isSameLogicalTurn(local: ChatMessage, incoming: ChatMessage): boolean {
    if (local.role !== incoming.role) return false
    const localExt = (local as { externalMessageId?: string }).externalMessageId
    const incomingExt = (incoming as { externalMessageId?: string }).externalMessageId
    if (localExt && incomingExt) return localExt === incomingExt
    if (local.role === 'user' && incoming.role === 'user') {
      if (local.text.trim() !== incoming.text.trim()) return false
    } else if (local.role === 'assistant' && incoming.role === 'assistant') {
      // Assistant turns rarely overlap as optimistic + server in this path
      // because optimistic assistants stay attached to a live stream; bail
      // out conservatively rather than guessing on opaque content blocks.
      return false
    } else {
      return false
    }
    const dt = Math.abs(new Date(local.timestamp).getTime() - new Date(incoming.timestamp).getTime())
    return Number.isFinite(dt) && dt <= SAME_TURN_TIMESTAMP_TOLERANCE_MS
  }

  function createStreamId(): string {
    const randomUUID = globalThis.crypto?.randomUUID
    if (typeof randomUUID === 'function') return randomUUID.call(globalThis.crypto)
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
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

  function trackAssistantStream(streamId: string, assistantTurn: ChatAssistantTurn, botId: string, targetSessionId: string): Promise<void> {
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

  function removeTurnFromSession(_botId: string, _targetSessionId: string, turn: ChatMessage) {
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
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

  function isPendingApproval(approval?: UIToolApproval) {
    return approval?.status?.trim().toLowerCase() === 'pending'
  }

  function isSameApproval(left?: UIToolApproval, right?: UIToolApproval) {
    const leftId = left?.approval_id?.trim()
    const rightId = right?.approval_id?.trim()
    return Boolean(leftId && rightId && leftId === rightId)
  }

  function mergeApprovalState(existing?: UIToolApproval, incoming?: UIToolApproval) {
    if (!incoming) return existing
    if (isSameApproval(existing, incoming) && !isPendingApproval(existing) && isPendingApproval(incoming)) {
      return existing
    }
    return incoming
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

  function cloneUserInputState(userInput: UIUserInput): UIUserInput {
    return {
      ...userInput,
      questions: userInput.questions?.map(question => ({
        ...question,
        options: question.options?.map(option => ({ ...option })),
      })),
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

  function rememberStartupSendFailure(failure: Omit<StartupSendFailure, 'id'>) {
    startupSendFailure.value = {
      ...failure,
      id: nextId(),
      restoreAttachments: failure.restoreAttachments ? [...failure.restoreAttachments] : undefined,
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

  function handleWSStreamEvent(event: UIStreamEvent, targetSessionId?: string) {
    const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
    const streamId = streamIdForEvent(event, sid)

    if (approvalResponseStreams.get(streamId)?.silent) {
      if (event.type === 'end' || event.type === 'error') {
        if (event.type === 'error') {
          rollbackApprovalResponse(streamId)
          toast.error(event.message || 'tool approval failed')
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
    sessionsCursor.value = null
    hasMoreSessions.value = false
    loadingMoreSessions.value = false
    bots.value = []
    sessionId.value = null
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
    overrideModelId.value = ''
    overrideReasoningEffort.value = ''
    startupSendFailure.value = null
    cancelPendingFsBump()
    fsChangedAt.value = 0
    lastFsChange.value = null
    lastFsEvents.value = new Map()
    seenFsToolCallIds.clear()
    clearPendingACPSession()

    pendingAssistantStreams.clear()
    approvalResponseStreams.clear()
    latestBackgroundTasks.clear()
    ephemeralAssistantErrors.clear()
  }

  function startWebSocket(targetBotId: string) {
    const bid = targetBotId.trim()
    stopWebSocket()
    if (!bid) return
    activeWs = connectWebSocket(bid, handleWSStreamEvent)
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

    if (event.type === 'session_title_updated') {
      const sid = event.session_id.trim()
      const title = event.title.trim()
      if (!sid || !title) return
      patchSessionInList(sid, { title })
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
    if (!first?.timestamp) return 0

    loadingOlder.value = true
    try {
      // Page through history with cursor advancement. When merged-turn de-dup
      // collapses an entire page to zero net-new entries, we must keep
      // advancing the `before` cursor (using the earliest timestamp from the
      // raw server response, not from our local list, otherwise the cursor
      // never moves and we'd terminate prematurely).
      const MAX_DEDUP_HOPS = 4
      let cursor = first.timestamp
      for (let hop = 0; hop < MAX_DEDUP_HOPS; hop++) {
        const turns = await fetchMessagesUI(bid, sid, {
          limit: PAGE_SIZE,
          before: cursor,
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
        // past the earliest one we just saw and try again on the next hop.
        const earliest = normalized.reduce<string | null>((acc, turn) => {
          const ts = turn.timestamp?.trim()
          if (!ts) return acc
          if (!acc || ts < acc) return ts
          return acc
        }, null)
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

  const pendingACPSessionMetadata = computed<Record<string, unknown> | null>(() =>
    pendingACPSessionInput.value ? acpSessionMetadata(pendingACPSessionInput.value) : null,
  )
  const pendingACPModelId = computed(() => pendingACPSessionInput.value?.modelId?.trim() ?? '')
  const pendingACPRuntimeStatus = computed(() => {
    const bid = currentBotId.value ?? ''
    const rid = pendingACPRuntimeId.value
    const key = acpRuntimeKey(bid, rid)
    return key ? acpRuntimeStatuses.value[key] : undefined
  })
  const pendingACPRuntimeEnsuring = computed(() => pendingACPCreating.value)

  function pendingACPIdentityKey(botId: string, input: ACPAgentSessionInput): string {
    return [botId, input.agentId, input.projectPath ?? '', input.projectMode ?? ''].join('\u0000')
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

  function stageACPSession(input: ACPAgentSessionInput) {
    const metadata = acpSessionMetadata(input)
    const existing = pendingACPSessionInput.value
    const samePendingAgent = Boolean(existing
      && existing.agentId === metadata.acp_agent_id
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
    // The warm staged runtime is bound server-side inside session creation;
    // no separate adopt/bind round trip and nothing for a watcher to race.
    const created = await createSession(bid, {
      title: input.title ?? '',
      type: 'acp_agent',
      metadata,
      acpRuntimeId: runtimeId || undefined,
    })
    upsertSession(created)
    sessionId.value = created.id
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
    const updated = await requestUpdateSessionAgent(bid, sid, 'acp_agent', metadata)
    upsertSession(updated)
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
    const updated = await requestUpdateSessionAgent(bid, sid, 'chat', {})
    upsertSession(updated)
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
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  async function initialize() {
    if (initializing.value) return
    initializing.value = true
    loadingChats.value = true
    // Every entry into initialize starts from a clean transcript window. We
    // reset here unconditionally so the success path that hydrates
    // `sessionId` without clearing messages can't carry a stale
    // `hasLoadedOlder = true` from a previous bot into the new bot's first
    // refresh (which would take the merge branch and duplicate optimistic
    // turns).
    hasLoadedOlder.value = false
    stopStreams()
    stopWebSocket()
    try {
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
        return
      }

      const response = await fetchSessions(bid)
      replaceSessions(response.items)
      sessionsCursor.value = response.nextCursor
      hasMoreSessions.value = response.nextCursor !== null

      if (!response.items.length) {
        sessionId.value = null
        replaceMessages([])
        hasMoreOlder.value = false
        hasLoadedOlder.value = false
      } else {
        // Keep a VALID persisted session; otherwise, if the user intentionally
        // closed down to the draft "New Session" page, keep that on reload instead
        // of force-opening a random session; otherwise pick the most recent (the
        // server already filtered to user-facing types and sorted by recency).
        // Transcript hydration is driven by startSessionMessagesStream below — no
        // eager loadMessages REST round trip from here.
        if (sessionId.value && sessionById.has(sessionId.value)) {
          draftIntent.value = false
        } else if (draftIntent.value) {
          sessionId.value = null
          replaceMessages([])
          hasMoreOlder.value = false
          hasLoadedOlder.value = false
        } else {
          sessionId.value = response.items[0]!.id
        }
      }

      startWebSocket(bid)
      startBotSessionsActivityStream(bid)
      if (sessionId.value) startSessionMessagesStream(bid, sessionId.value)
    } finally {
      loadingChats.value = false
      initializing.value = false
    }
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
    abort()
    abortAllAssistantStreams()
    clearPendingACPSession()
    cancelPendingFsBump()
    lastFsChange.value = null
    lastFsEvents.value = new Map()
    seenFsToolCallIds.clear()
    currentBotId.value = targetBotId
    sessionId.value = null
    await initialize()
  }

  function selectSession(targetSessionId: string) {
    const sid = targetSessionId.trim()
    if (!sid || sid === sessionId.value) return
    clearPendingACPSession()
    sessionId.value = sid
    draftIntent.value = false
    switchActiveSession(sid)
  }

  async function createNewSession() {
    const bid = await ensureBot()
    if (!bid) return
    clearPendingACPSession()
    sessionId.value = null
    draftIntent.value = true
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  // Switch the global view to the draft (no real session yet). Unlike
  // createNewSession this assumes the bot is already active and only resets the
  // view, so per-session chat tabs can activate their draft tab without minting a
  // session. selectSession early-returns on an empty id, so a draft needs this.
  function selectDraft() {
    draftIntent.value = true
    if (!sessionId.value) return
    clearPendingACPSession()
    sessionId.value = null
    sessionMessagesStream.stop()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  async function removeSession(targetSessionId: string) {
    const delId = targetSessionId.trim()
    if (!delId) return
    const bid = currentBotId.value ?? ''
    if (!bid) throw new Error('Bot not selected')
    await requestDeleteSession(bid, delId)
    clearACPRuntimeStatus(bid, delId)
    removeSessionFromList(delId)
    if (sessionId.value !== delId) return
    if (sessions.value.length === 0) {
      sessionId.value = null
      sessionMessagesStream.stop()
      replaceMessages([])
      hasMoreOlder.value = false
      hasLoadedOlder.value = false
      return
    }
    const next = sessions.value[0]!.id
    sessionId.value = next
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

  async function sendMessage(text: string, attachments?: ChatAttachment[]): Promise<SendMessageResult> {
    const trimmed = text.trim()
    if ((!trimmed && !attachments?.length) || streaming.value || !currentBotId.value) return { ok: false, stage: 'startup' }

    loading.value = true
    let assistantTurn: ChatAssistantTurn | null = null
    let userTurn: ChatUserTurn | null = null
    let sendBotId = ''
    let sendSessionId = ''
    let sendStreamId = ''

    const wasDraft = !sessionId.value
    try {
      await ensureActiveSession(wasDraft ? trimmed : undefined)

      const bid = currentBotId.value!
      const sid = sessionId.value!
      sendBotId = bid
      sendSessionId = sid
      sendStreamId = createStreamId()
      // Tell the tab store to pin (and, for a draft, repoint) this session's tab.
      userSentInSession.value = { id: sid, wasDraft, seq: ++userSendSeq }

      userTurn = createOptimisticUserTurn(trimmed, attachments)
      messages.push(userTurn)
      messages.push(createOptimisticAssistantTurn())
      assistantTurn = messages[messages.length - 1] as ChatAssistantTurn

      const modelId = overrideModelId.value || undefined
      const effort = overrideReasoningEffort.value
      const reasoningEffort = effort || undefined

      const ws = ensureWebSocket(bid)
      if (ws) {
        if (!ws.connected) {
          throw new StreamFailureError('WebSocket is not connected', 'startup')
        }
        const completion = trackAssistantStream(sendStreamId, assistantTurn, bid, sid)
        ws.send({
          type: 'message',
          stream_id: sendStreamId,
          text: trimmed,
          session_id: sid,
          attachments,
          model_id: modelId,
          reasoning_effort: reasoningEffort,
        })
        await completion
        await refreshCurrentSession(bid, sid)
      } else {
        await sendLocalChannelMessage(bid, trimmed, attachments, { modelId, reasoningEffort })
        await refreshCurrentSession(bid, sid)
      }

      assistantTurn.streaming = false
      loading.value = false
      return { ok: true }
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Unknown error')
      const isAbort = err.name === 'AbortError'
      const reason = err.message
      const stage: SendMessageStage = err instanceof StreamFailureError
        ? err.stage
        : (assistantTurn && hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')
      const bid = sendBotId || currentBotId.value || ''
      const sid = sendSessionId || sessionId.value || ''

      if (assistantTurn) finalizeStreamFailure(assistantTurn, bid, sid, err)
      if (!isAbort && stage === 'startup' && userTurn) {
        removeTurnFromSession(bid, sid, userTurn)
      }

      if (sendStreamId) forgetAssistantStream(sendStreamId)
      loading.value = false

      if (isAbort) return { ok: false, stage: 'stream', error: reason }
      if (stage === 'startup') {
        rememberStartupSendFailure({ botId: bid, sessionId: sid, error: reason, restoreInput: text, restoreAttachments: attachments })
        return { ok: false, stage, error: reason, restoreInput: text, restoreAttachments: attachments }
      }
      return { ok: false, stage, error: reason }
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
    const streamId = createStreamId()
    const silent = isSessionStreaming(sid)
    approvalResponseStreams.set(streamId, { approvalId, silent, at: Date.now() })
    if (!silent) {
      const assistantTurn = createOptimisticAssistantTurn()
      messages.push(assistantTurn)
      void trackAssistantStream(streamId, assistantTurn, bid, sid).catch((error: Error) => {
        finalizeStreamFailure(assistantTurn, bid, sid, error)
      })
      loading.value = true
    }
    // Optimistically update the approved/rejected tool block before the
    // server snapshot arrives so the buttons disappear immediately.
    markToolApprovalDecision(approvalId, decision === 'approve' ? 'approved' : 'rejected')
    ws?.send({
      type: 'tool_approval_response',
      stream_id: streamId,
      session_id: sid,
      approval_id: approvalId,
      short_id: approval.short_id,
      decision,
    })
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

    ws?.send({
      type: 'user_input_response',
      stream_id: streamId,
      session_id: sid,
      user_input_id: userInput.user_input_id,
      short_id: userInput.short_id,
      answers: payload.answers,
      canceled: payload.canceled === true,
      reason: payload.reason,
    })
  }

  function clearMessages() {
    abort()
    replaceMessages([])
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  const chats = sessions
  const chatId = sessionId

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
    chats,
    chatId,
    sessionId,
    currentBotId,
    bots,
    activeSession,
    activeChatReadOnly,
    isSessionStreaming,
    loading,
    loadingChats,
    loadingMessages,
    loadingOlder,
    hasMoreOlder,
    // Exposed for tests only — do not branch on this in components. The
    // leading underscore reflects the test-only contract at the call site.
    _hasLoadedOlder: hasLoadedOlder,
    initializing,
    overrideModelId,
    overrideReasoningEffort,
    startupSendFailure,
    fsChangedAt,
    lastFsChange,
    lastFsEvents,
    markFsChanged,
    affectsPath,
    fsEventForPath,
    initialize,
    refreshBots,
    selectBot,
    selectSession,
    selectChat: selectSession,
    stageACPSession,
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
    createNewChat: createNewSession,
    selectDraft,
    userSentInSession,
    removeSession,
    removeChat: removeSession,
    deleteChat: removeSession,
    renameSession,
    sendMessage,
    respondToolApproval,
    respondUserInput,
    clearMessages,
    resetUserScopedState,
    loadOlderMessages,
    findMessageIdByExternalId,
    locateMessageByExternalId,
    clearStartupSendFailure,
    abort,
  }
})
