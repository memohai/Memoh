import { defineStore, storeToRefs } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useUserStore } from '@/store/user'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { shouldRefreshFromMessageCreated, upsertById } from './chat-list.utils'
import {
  createSession,
  deleteSession as requestDeleteSession,
  ensureACPRuntime as requestEnsureACPRuntime,
  setACPRuntimeModel as requestSetACPRuntimeModel,
  updateSessionAgent as requestUpdateSessionAgent,
  updateSessionTitle as requestUpdateSessionTitle,
  fetchSessions,
  type Bot,
  type SessionSummary,
  type MessageStreamEvent,
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
  type UITurn,
  type UIUserTurn,
  type UIStreamEvent,
  fetchBots,
  fetchMessagesUI,
  sendLocalChannelMessage,
  streamMessageEvents,
  connectWebSocket,
  locateMessageUI,
} from '@/composables/api/useChat'
import { ACP_NO_PROJECT_MODE, createACPNoProjectPath } from '@/utils/acp'
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
  backgroundTask?: BackgroundTask
}

export type ContentBlock = TextBlock | ThinkingBlock | ToolCallBlock | AttachmentBlock | ErrorBlock

export interface ChatUserTurn {
  id: string
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
}

export interface ChatAssistantTurn {
  id: string
  role: 'assistant'
  messages: ContentBlock[]
  timestamp: string
  platform?: string
  externalMessageId?: string
  streaming: boolean
}

export interface BackgroundTask {
  taskId: string
  status: string
  event?: string
  botId?: string
  sessionId?: string
  command?: string
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
  role: 'system'
  kind: 'background_task'
  backgroundTask: BackgroundTask
  timestamp: string
  platform?: string
  streaming: boolean
}

export type ChatMessage = ChatUserTurn | ChatAssistantTurn | ChatSystemTurn

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
  title?: string
  startRuntime?: boolean
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

interface SessionMessageState {
  items: ChatMessage[]
  hasMoreOlder: boolean
}

interface EphemeralAssistantError {
  content: string
  timestamp: string
  userText?: string
}

export const useChatStore = defineStore('chat', () => {
  const selectionStore = useChatSelectionStore()
  const { currentBotId, sessionId } = storeToRefs(selectionStore)

  const messages = reactive<ChatMessage[]>([])
  const pendingAssistantStreams = reactive(new Map<string, PendingAssistantStream>())
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
  const loadingChats = ref(false)
  const loadingOlder = ref(false)
  const hasMoreOlder = ref(true)
  const initializing = ref(false)
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')
  const startupSendFailure = ref<StartupSendFailure | null>(null)

  // Bumps every time a fs-mutating tool call (write/edit/exec) finishes for the
  // current bot. File-manager components watch this to refresh their listings
  // and any open file viewers without polling.
  const fsChangedAt = ref(0)
  const FS_MUTATING_TOOLS = new Set(['write', 'edit', 'exec'])

  function bumpFsChangedAtIfFsMutation(message: UIMessage) {
    if (message.type !== 'tool') return
    if (message.running) return
    if (!FS_MUTATING_TOOLS.has(message.name)) return
    fsChangedAt.value = Date.now()
  }

  let messageEventsSince = ''
  let activeWs: ChatWebSocket | null = null
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refreshPromise: { key: string; promise: Promise<void> } | null = null
  let sessionListRefreshPromise: { botId: string; promise: Promise<void> } | null = null
  const pendingBackgroundEvents = new Map<string, BackgroundTask[]>()
  const latestBackgroundTasks = new Map<string, BackgroundTask>()
  // Open chat tabs share this store, so keep a small per-session view cache.
  // Switching tabs saves/restores from here; the active session remains the
  // only live `messages` array rendered by ChatPane.
  const sessionMessageStates = new Map<string, SessionMessageState>()
  const ephemeralAssistantErrors = new Map<string, EphemeralAssistantError[]>()
  const messageEventsStream = useRetryingStream()
  const acpRuntimeStatuses = ref<Record<string, AcpagentRuntimeStatus | undefined>>({})
  const acpRuntimePending = ref<Record<string, boolean>>({})
  const acpRuntimeRequests = new Map<string, Promise<AcpagentRuntimeStatus>>()

  const activeSession = computed(() =>
    sessions.value.find((s) => s.id === sessionId.value) ?? null,
  )

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
    return status === 'running' || status === 'stalled'
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
    const pending = pendingBackgroundEvents.get(taskId)
    if (pending?.length) {
      for (const task of pending) {
        mergeBackgroundTaskIntoToolBlock(block, task)
      }
      pendingBackgroundEvents.delete(taskId)
    }
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

  function updateSince(value?: string) {
    const next = (value ?? '').trim()
    if (!next) return
    if (!messageEventsSince) {
      messageEventsSince = next
      return
    }
    const currentTs = Date.parse(messageEventsSince)
    const nextTs = Date.parse(next)
    if (!Number.isNaN(nextTs) && (Number.isNaN(currentTs) || nextTs > currentTs)) {
      messageEventsSince = next
    }
  }

  function updateSinceFromMessages(items: ChatMessage[]) {
    // Advance only. Restoring an older tab snapshot must not move the event
    // cursor backwards and replay unrelated stream events.
    for (const item of items) {
      if (isEphemeralAssistantErrorTurn(item)) continue
      updateSince(item.timestamp)
    }
  }

  function isEphemeralAssistantErrorTurn(item: ChatMessage): boolean {
    return item.role === 'assistant' && item.id.startsWith('ephemeral-error-')
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

  function findUserTurnBeforeAssistant(assistantTurn: ChatAssistantTurn, botId = currentBotId.value ?? '', targetSessionId = sessionId.value ?? ''): ChatUserTurn | null {
    const items = collectionForTurn(botId, targetSessionId, assistantTurn)
    const index = items.indexOf(assistantTurn)
    if (index < 0) return null
    for (let i = index - 1; i >= 0; i -= 1) {
      const item = items[i]
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

  function setMessages(items: ChatMessage[]) {
    messages.splice(0, messages.length, ...items)
    updateSinceFromMessages(items)
  }

  function replaceMessages(items: UITurn[], targetSessionId?: string) {
    setMessages(normalizeTurns(items, targetSessionId))
  }

  function sortChatMessages(items: ChatMessage[]): ChatMessage[] {
    return [...items].sort((a, b) => {
      const at = Date.parse(a.timestamp)
      const bt = Date.parse(b.timestamp)
      if (!Number.isNaN(at) && !Number.isNaN(bt) && at !== bt) return at - bt
      return a.id.localeCompare(b.id)
    })
  }

  function mergeMessages(items: UITurn[], targetSessionId?: string) {
    const merged = new Map<string, ChatMessage>()
    for (const item of messages) {
      merged.set(item.id, item)
    }
    for (const item of normalizeTurns(items, targetSessionId)) {
      merged.set(item.id, item)
    }
    setMessages(sortChatMessages([...merged.values()]))
  }

  function sessionMessageKey(botId?: string | null, sid?: string | null): string {
    const bid = (botId ?? '').trim()
    const session = (sid ?? '').trim()
    return bid && session ? `${bid}:${session}` : ''
  }

  function cacheCurrentMessages() {
    const key = sessionMessageKey(currentBotId.value, sessionId.value)
    if (!key) return
    sessionMessageStates.set(key, {
      items: [...messages],
      hasMoreOlder: hasMoreOlder.value,
    })
  }

  function restoreCachedMessages(botId: string, sid: string): boolean {
    const key = sessionMessageKey(botId, sid)
    const cached = key ? sessionMessageStates.get(key) : undefined
    if (!cached) return false
    messages.splice(0, messages.length, ...cached.items)
    hasMoreOlder.value = cached.hasMoreOlder
    updateSinceFromMessages(cached.items)
    return true
  }

  function cacheFetchedMessages(botId: string, sid: string, items: ChatMessage[], moreOlder: boolean) {
    const key = sessionMessageKey(botId, sid)
    if (!key) return
    sessionMessageStates.set(key, {
      items: [...items],
      hasMoreOlder: moreOlder,
    })
    for (const item of items) {
      if (isEphemeralAssistantErrorTurn(item)) continue
      updateSince(item.timestamp)
    }
  }

  function clearCachedMessages(botId?: string | null, sid?: string | null) {
    const key = sessionMessageKey(botId, sid)
    if (key) sessionMessageStates.delete(key)
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

  function appendTurnToSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    if (currentBotId.value === bid && sessionId.value === sid) {
      messages.push(turn)
      return
    }
    const key = sessionMessageKey(bid, sid)
    if (!key) return
    const cached = sessionMessageStates.get(key)
    if (cached) {
      cached.items.push(turn)
    } else {
      sessionMessageStates.set(key, {
        items: [turn],
        hasMoreOlder: true,
      })
    }
  }

  function removeTurnFromSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
    const key = sessionMessageKey(botId, targetSessionId)
    const cached = key ? sessionMessageStates.get(key) : undefined
    if (!cached) return
    const cachedIdx = cached.items.indexOf(turn)
    if (cachedIdx >= 0) cached.items.splice(cachedIdx, 1)
  }

  function collectionForTurn(botId: string, targetSessionId: string, turn: ChatMessage): ChatMessage[] {
    if (messages.includes(turn)) return messages
    const key = sessionMessageKey(botId, targetSessionId)
    const cached = key ? sessionMessageStates.get(key) : undefined
    if (cached?.items.includes(turn)) return cached.items
    return messages
  }

  function createOptimisticAssistantTurn(): ChatAssistantTurn {
    return {
      id: nextId(),
      role: 'assistant',
      messages: [],
      timestamp: new Date().toISOString(),
      streaming: true,
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

  function upsertAssistantUIMessage(turn: ChatAssistantTurn, message: UIMessage) {
    const normalized = normalizeUIMessage(message)
    turn.messages = upsertById(turn.messages, normalized)
    bumpFsChangedAtIfFsMutation(message)
  }

  function nextAssistantMessageId(turn: ChatAssistantTurn): number {
    return turn.messages.reduce((maxId, message) => Math.max(maxId, message.id), -1) + 1
  }

  function hasVisibleAssistantBlocks(turn: ChatAssistantTurn): boolean {
    return turn.messages.some(block => block.type !== 'error')
  }

  function rememberAssistantError(errorMessage: string, botId: string, targetSessionId: string, assistantTurn: ChatAssistantTurn) {
    const sid = targetSessionId.trim()
    const text = errorMessage.trim()
    if (!sid || !text) return
    const current = ephemeralAssistantErrors.get(sid) ?? []
    if (current.some(item => item.content === text)) return
    const anchorUser = findUserTurnBeforeAssistant(assistantTurn, botId, sid)
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

  function handleWSStreamEvent(event: UIStreamEvent, targetSessionId?: string) {
    const sid = (event.session_id ?? targetSessionId ?? sessionId.value ?? '').trim()
    const streamId = streamIdForEvent(event, sid)

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
        const endedSessionId = sid || endedSession?.sessionId
        pruneEmptyAssistantTurnIfPending(streamId)
        resolveAssistantStream(streamId)
        loading.value = isSessionStreaming(sessionId.value)
        void refreshCurrentSession(endedBotId, endedSessionId)
        break
      case 'error': {
        const session = getAssistantStream(streamId) ?? ensureDiscussStream(streamId, sid)
        const message = event.message || 'stream error'
        const stage: SendMessageStage = hasVisibleAssistantBlocks(session.assistantTurn) ? 'stream' : 'startup'
        rejectAssistantStream(streamId, new StreamFailureError(message, stage))
        loading.value = isSessionStreaming(sessionId.value)
        break
      }
    }
  }

  function stopMessageEvents() {
    messageEventsStream.stop()
  }

  function stopWebSocket() {
    if (activeWs) {
      activeWs.close()
      activeWs = null
    }
  }

  function resetUserScopedState(options: { clearSelection?: boolean } = {}) {
    stopMessageEvents()
    abortAllAssistantStreams()
    stopWebSocket()

    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
    refreshPromise = null
    sessionListRefreshPromise = null
    messageEventsSince = ''

    sessions.value = []
    bots.value = []
    sessionId.value = null
    if (options.clearSelection && currentBotId.value) {
      currentBotId.value = null
    }
    replaceMessages([])
    hasMoreOlder.value = true
    loading.value = false
    loadingChats.value = false
    loadingOlder.value = false
    initializing.value = false
    overrideModelId.value = ''
    overrideReasoningEffort.value = ''
    startupSendFailure.value = null
    fsChangedAt.value = 0

    pendingAssistantStreams.clear()
    pendingBackgroundEvents.clear()
    latestBackgroundTasks.clear()
    sessionMessageStates.clear()
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
    const key = sessionMessageKey(bid, sid)

    if (refreshPromise) {
      if (refreshPromise.key === key) {
        await refreshPromise.promise
        return
      }
      await refreshPromise.promise
    }

    const promise = (async () => {
      const turns = await fetchMessagesUI(bid, sid, { limit: PAGE_SIZE })
      const normalized = normalizeTurns(turns, sid)
      const moreOlder = turns.length > 0
      if (currentBotId.value === bid && sessionId.value === sid) {
        setMessages(normalized)
        hasMoreOlder.value = moreOlder
        cacheCurrentMessages()
      } else {
        cacheFetchedMessages(bid, sid, normalized, moreOlder)
      }
      touchSession(sid)
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
      .then((visible) => {
        if ((currentBotId.value ?? '').trim() !== bid) return
        sessions.value = visible
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

  function findBackgroundToolBlockIn(items: ChatMessage[], taskId: string): ToolCallBlock | null {
    const id = taskId.trim()
    if (!id) return null
    for (const item of items) {
      if (item.role !== 'assistant') continue
      for (const block of item.messages) {
        if (block.type !== 'tool') continue
        if (taskIdFromToolBlock(block) === id) return block
      }
    }
    return null
  }

  function findBackgroundToolBlock(taskId: string): ToolCallBlock | null {
    return findBackgroundToolBlockIn(messages, taskId)
  }

  function applyBackgroundTaskToCachedMessages(botId: string, task: BackgroundTask) {
    const key = sessionMessageKey(botId, task.sessionId)
    const cached = key ? sessionMessageStates.get(key) : undefined
    if (!cached) return
    const block = findBackgroundToolBlockIn(cached.items, task.taskId)
    if (block) mergeBackgroundTaskIntoToolBlock(block, task)
  }

  function queuePendingBackgroundEvent(task: BackgroundTask) {
    const current = pendingBackgroundEvents.get(task.taskId) ?? []
    current.push(task)
    pendingBackgroundEvents.set(task.taskId, current.slice(-40))
  }

  function applyBackgroundTaskEvent(targetBotId: string, event: MessageStreamEvent) {
    const incoming = normalizeBackgroundTask(event.task ?? (event as UIBackgroundTask), event.event)
    if (!incoming) return

    const sid = (sessionId.value ?? '').trim()

    const task = rememberBackgroundTask(incoming)

    if (incoming.sessionId && sid && incoming.sessionId !== sid) {
      applyBackgroundTaskToCachedMessages(targetBotId, task)
      return
    }

    const block = findBackgroundToolBlock(task.taskId)
    if (block) {
      mergeBackgroundTaskIntoToolBlock(block, task)
      if (!isBackgroundTaskActive(block.backgroundTask)) {
        fsChangedAt.value = Date.now()
      }
    } else {
      queuePendingBackgroundEvent(task)
    }

    if (!isBackgroundTaskActive(task) || task.status === 'stalled') {
      scheduleRefreshCurrentSession(task.sessionId, 250)
    }
  }

  function applyAgentStreamEvent(event: MessageStreamEvent) {
    const stream = event.stream
    if (!stream) return

    const sid = (event.session_id ?? '').trim()
    const activeSid = (sessionId.value ?? '').trim()
    if (sid && activeSid && sid !== activeSid) {
      const isKnownBackgroundStream = !!stream.stream_id && pendingAssistantStreams.has(stream.stream_id)
      if (!isKnownBackgroundStream) return
    }

    if (stream.type === 'start' || stream.type === 'message') {
      loading.value = true
    }

    handleWSStreamEvent(stream, sid || undefined)

    if (stream.type === 'end' || stream.type === 'error') {
      if (sid) touchSession(sid)
    }
  }

  function handleStreamEvent(targetBotId: string, event: MessageStreamEvent) {
    const eventType = (event.type ?? '').toLowerCase()
    const eBotId = (event.bot_id ?? '').trim()
    if (eBotId && eBotId !== targetBotId) return

    if (eventType === 'background_task') {
      applyBackgroundTaskEvent(targetBotId, event)
      return
    }

    if (eventType === 'agent_stream') {
      applyAgentStreamEvent(event)
      return
    }

    if (eventType === 'message_created') {
      const raw = event.message
      if (!raw) return
      updateSince(raw.created_at)
      const messageSessionId = String(raw.session_id ?? event.session_id ?? '').trim()
      if (messageSessionId) {
        if (sessions.value.some((session) => session.id === messageSessionId)) {
          touchSession(messageSessionId)
        } else {
          void refreshSessionsList(targetBotId)
        }
      }
      if (shouldRefreshFromMessageCreated(targetBotId, sessionId.value, streamingSessionId.value, event)) {
        scheduleRefreshCurrentSession((raw.session_id ?? '').trim())
      }
      return
    }

    if (eventType === 'session_title_updated') {
      const sid = (event.session_id ?? '').trim()
      const title = (event.title ?? '').trim()
      if (!sid || !title) return
      const target = sessions.value.find((session) => session.id === sid)
      if (target) target.title = title
    }
  }

  function startMessageEvents(targetBotId: string) {
    const bid = targetBotId.trim()
    stopMessageEvents()
    if (!bid) return

    messageEventsStream.start(async (signal) => {
      await streamMessageEvents(
        bid,
        signal,
        (event) => handleStreamEvent(bid, event),
        messageEventsSince || undefined,
      )
    })
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

  const PAGE_SIZE = 30

  async function loadMessages(botId: string, sid: string) {
    const turns = await fetchMessagesUI(botId, sid, { limit: PAGE_SIZE })
    replaceMessages(turns)
    hasMoreOlder.value = turns.length > 0
    cacheCurrentMessages()
  }

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
          // Cursor cannot advance — bail out to avoid a request loop.
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
      cacheCurrentMessages()
      return result.target_id?.trim() || findMessageIdByExternalId(target)
    } catch (error) {
      console.error('Failed to locate message:', error)
      return null
    }
  }

  function touchSession(targetSessionId: string) {
    const index = sessions.value.findIndex(session => session.id === targetSessionId)
    if (index < 0) return
    const [target] = sessions.value.splice(index, 1)
    if (!target) return
    target.updated_at = new Date().toISOString()
    sessions.value.unshift(target)
  }

  function acpSessionMetadata(input: ACPAgentSessionInput): Record<string, unknown> {
    const agentId = input.agentId.trim()
    const projectMode = input.projectMode?.trim() || ACP_NO_PROJECT_MODE
    const projectPath = input.projectPath?.trim() || createACPNoProjectPath()
    return {
      acp_agent_id: agentId,
      project_path: projectPath,
      acp_project_mode: projectMode,
    }
  }

  function upsertSession(updated: SessionSummary) {
    sessions.value = [updated, ...sessions.value.filter(session => session.id !== updated.id)]
  }

  async function createACPSession(input: ACPAgentSessionInput): Promise<{ session: SessionSummary; runtime?: AcpagentRuntimeStatus }> {
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    const metadata = acpSessionMetadata(input)
    const created = await createSession(bid, {
      title: input.title ?? '',
      type: 'acp_agent',
      metadata,
    })
    upsertSession(created)
    sessionId.value = created.id
    replaceMessages([])
    hasMoreOlder.value = false
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
    const index = sessions.value.findIndex(session => session.id === updated.id)
    if (index >= 0) {
      sessions.value[index] = updated
    } else {
      upsertSession(updated)
    }
    clearACPRuntimeStatus(bid, sid)
    const runtime = input.startRuntime ? await ensureACPRuntime(sid) : undefined
    return { session: updated, runtime }
  }

  async function updateCurrentSessionToMemoh(): Promise<SessionSummary | null> {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid) return null
    const updated = await requestUpdateSessionAgent(bid, sid, 'chat', {})
    const index = sessions.value.findIndex(session => session.id === updated.id)
    if (index >= 0) {
      sessions.value[index] = updated
    } else {
      upsertSession(updated)
    }
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

  async function ensureActiveSession() {
    if (sessionId.value) return
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    const created = await createSession(bid)
    sessions.value = [created, ...sessions.value.filter(session => session.id !== created.id)]
    sessionId.value = created.id
    replaceMessages([])
    hasMoreOlder.value = false
  }

  async function initialize() {
    if (initializing.value) return
    initializing.value = true
    loadingChats.value = true
    stopMessageEvents()
    stopWebSocket()
    try {
      const bid = await ensureBot()
      if (!bid) {
        messageEventsSince = ''
        sessions.value = []
        sessionId.value = null
        replaceMessages([])
        hasMoreOlder.value = false
        return
      }

      const visible = await fetchSessions(bid)
      sessions.value = visible
      if (!visible.length) {
        messageEventsSince = ''
        sessionId.value = null
        replaceMessages([])
        hasMoreOlder.value = false
      } else {
        const activeSessionId = sessionId.value && visible.some(session => session.id === sessionId.value)
          ? sessionId.value
          : (visible.find((s) => s.type === 'chat' || s.type === 'discuss')?.id ?? visible[0]!.id)
        sessionId.value = activeSessionId
        if (!restoreCachedMessages(bid, activeSessionId)) {
          await loadMessages(bid, activeSessionId)
        }
      }

      startWebSocket(bid)
      startMessageEvents(bid)
    } finally {
      loadingChats.value = false
      initializing.value = false
    }
  }

  async function selectBot(targetBotId: string) {
    if (currentBotId.value === targetBotId) return
    abort()
    abortAllAssistantStreams()
    currentBotId.value = targetBotId
    sessionId.value = null
    await initialize()
  }

  async function selectSession(targetSessionId: string) {
    const sid = targetSessionId.trim()
    if (!sid || sid === sessionId.value) return
    cacheCurrentMessages()
    sessionId.value = sid
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      if (restoreCachedMessages(bid, sid)) return
      await loadMessages(bid, sid)
    } finally {
      loadingChats.value = false
    }
  }

  async function createNewSession() {
    cacheCurrentMessages()
    const bid = await ensureBot()
    if (!bid) return
    sessionId.value = null
    replaceMessages([])
    hasMoreOlder.value = false
  }

  async function removeSession(targetSessionId: string) {
    const delId = targetSessionId.trim()
    if (!delId) return
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      await requestDeleteSession(bid, delId)
      clearACPRuntimeStatus(bid, delId)
      clearCachedMessages(bid, delId)
      const remaining = sessions.value.filter(session => session.id !== delId)
      sessions.value = remaining
      if (sessionId.value !== delId) return
      if (!remaining.length) {
        sessionId.value = null
        replaceMessages([])
        hasMoreOlder.value = false
        return
      }
      sessionId.value = remaining[0]!.id
      await loadMessages(bid, remaining[0]!.id)
    } finally {
      loadingChats.value = false
    }
  }

  async function renameSession(targetSessionId: string, title: string): Promise<SessionSummary> {
    const sid = targetSessionId.trim()
    const nextTitle = title.trim()
    if (!sid) throw new Error('Session not selected')
    const bid = currentBotId.value ?? ''
    if (!bid) throw new Error('Bot not selected')
    const updated = await requestUpdateSessionTitle(bid, sid, nextTitle)
    const target = sessions.value.find(session => session.id === sid)
    if (target) {
      target.title = updated.title ?? nextTitle
      target.updated_at = updated.updated_at ?? target.updated_at
    }
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

    try {
      await ensureActiveSession()

      const bid = currentBotId.value!
      const sid = sessionId.value!
      sendBotId = bid
      sendSessionId = sid
      sendStreamId = createStreamId()

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
      touchSession(sid)
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
    if (!bid || !sid || !approval.approval_id || streaming.value) return
    const ws = ensureWebSocket(bid)
    const streamId = createStreamId()
    const assistantTurn = createOptimisticAssistantTurn()
    messages.push(assistantTurn)
    void trackAssistantStream(streamId, assistantTurn, bid, sid).catch((error: Error) => {
      finalizeStreamFailure(assistantTurn, bid, sid, error)
    })
    loading.value = true
    // Optimistically update the approved/rejected tool block before the
    // server snapshot arrives so the buttons disappear immediately.
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type === 'tool' && block.approval?.approval_id === approval.approval_id) {
          block.approval = {
            ...block.approval,
            status: decision === 'approve' ? 'approved' : 'rejected',
            can_approve: false,
          }
        }
      }
    }
    ws?.send({
      type: 'tool_approval_response',
      stream_id: streamId,
      session_id: sid,
      approval_id: approval.approval_id,
      short_id: approval.short_id,
      decision,
    })
  }

  function clearMessages() {
    abort()
    replaceMessages([])
    hasMoreOlder.value = false
    cacheCurrentMessages()
  }

  const chats = sessions
  const chatId = sessionId

  return {
    messages,
    streaming,
    streamingSessionId,
    sessions,
    acpRuntimeStatuses,
    acpRuntimePending,
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
    loadingOlder,
    hasMoreOlder,
    initializing,
    overrideModelId,
    overrideReasoningEffort,
    startupSendFailure,
    fsChangedAt,
    initialize,
    selectBot,
    selectSession,
    selectChat: selectSession,
    createACPSession,
    updateCurrentSessionAgent,
    updateCurrentSessionToMemoh,
    acpRuntimeKey,
    ensureACPRuntime,
    setACPRuntimeModel,
    createNewSession,
    createNewChat: createNewSession,
    removeSession,
    removeChat: removeSession,
    deleteChat: removeSession,
    renameSession,
    sendMessage,
    respondToolApproval,
    clearMessages,
    resetUserScopedState,
    loadOlderMessages,
    findMessageIdByExternalId,
    locateMessageByExternalId,
    clearStartupSendFailure,
    abort,
  }
})
