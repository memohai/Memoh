import { defineStore, storeToRefs } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useUserStore } from '@/store/user'
import { useChatSelectionStore } from '@/store/chat-selection'
import { shouldRefreshFromMessageCreated, upsertById } from './chat-list.utils'
import {
  createSession,
  deleteSession as requestDeleteSession,
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
} from '@/composables/api/useChat'

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
  timestamp: string
  platform?: string
  senderDisplayName?: string
  senderAvatarUrl?: string
  senderUserId?: string
  streaming: boolean
  isSelf: boolean
}

export interface ChatAssistantTurn {
  id: string
  role: 'assistant'
  messages: ContentBlock[]
  timestamp: string
  platform?: string
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
  assistantTurn: ChatAssistantTurn
  botId: string
  sessionId: string
  done: boolean
  resolve: () => void
  reject: (err: Error) => void
  streamError?: StreamFailureError
}

export type SendMessageStage = 'startup' | 'stream'

export interface SendMessageResult {
  ok: boolean
  stage?: SendMessageStage
  error?: string
  restoreInput?: string
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

export const useChatStore = defineStore('chat', () => {
  const selectionStore = useChatSelectionStore()
  const { currentBotId, sessionId } = storeToRefs(selectionStore)

  const messages = reactive<ChatMessage[]>([])
  const streamingSessionId = ref<string | null>(null)
  const streaming = computed(() => streamingSessionId.value !== null && streamingSessionId.value === sessionId.value)
  const sessions = ref<SessionSummary[]>([])
  const loading = ref(false)
  const loadingChats = ref(false)
  const loadingOlder = ref(false)
  const hasMoreOlder = ref(true)
  const initializing = ref(false)
  const bots = ref<Bot[]>([])
  const overrideModelId = ref<string>('')
  const overrideReasoningEffort = ref<string>('')

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

  let abortFn: (() => void) | null = null
  let messageEventsSince = ''
  let pendingAssistantStream: PendingAssistantStream | null = null
  let activeWs: ChatWebSocket | null = null
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refreshPromise: { key: string; promise: Promise<void> } | null = null
  let suppressNextStartPlaceholder = false
  const pendingBackgroundEvents = new Map<string, BackgroundTask[]>()
  const latestBackgroundTasks = new Map<string, BackgroundTask>()
  // Open chat tabs share this store, so keep a small per-session view cache.
  // Switching tabs saves/restores from here; the active session remains the
  // only live `messages` array rendered by ChatPane.
  const sessionMessageStates = new Map<string, SessionMessageState>()
  const ephemeralAssistantErrors = new Map<string, string[]>()
  const messageEventsStream = useRetryingStream()

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

  watch(currentBotId, (newId) => {
    if (newId) {
      void initialize()
    } else {
      stopMessageEvents()
      stopWebSocket()
      rejectPendingAssistantStream(new Error('Bot stream stopped'))
      messageEventsSince = ''
      sessions.value = []
      sessionId.value = null
      replaceMessages([])
    }
  }, { immediate: true })

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
    const writable = merged as Record<string, unknown>
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
        timestamp: normalizeTimestamp(turn.timestamp),
        platform: (turn.platform ?? '').trim() || undefined,
        senderDisplayName: (turn.sender_display_name ?? '').trim() || undefined,
        senderAvatarUrl: (turn.sender_avatar_url ?? '').trim() || undefined,
        senderUserId: (turn.sender_user_id ?? '').trim() || undefined,
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
      updateSince(item.timestamp)
    }
  }

  function appendEphemeralErrors(items: ChatMessage[], targetSessionId?: string) {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!sid) return
    const errors = ephemeralAssistantErrors.get(sid)
    if (!errors?.length) return
    const assistantTurn = [...items].reverse().find((item): item is ChatAssistantTurn => item.role === 'assistant')
    if (!assistantTurn) return
    for (const error of errors) {
      const text = error.trim()
      if (!text) continue
      if (assistantTurn.messages.some(block => block.type === 'error' && block.content === text)) continue
      assistantTurn.messages.push({
        id: nextAssistantMessageId(assistantTurn),
        type: 'error',
        content: text,
      })
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
      updateSince(item.timestamp)
    }
  }

  function clearCachedMessages(botId?: string | null, sid?: string | null) {
    const key = sessionMessageKey(botId, sid)
    if (key) sessionMessageStates.delete(key)
  }

  function createCompletionForAssistantTurn(assistantTurn: ChatAssistantTurn): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      pendingAssistantStream = {
        assistantTurn,
        botId: currentBotId.value ?? '',
        sessionId: streamingSessionId.value ?? sessionId.value ?? '',
        done: false,
        resolve,
        reject,
      }
    })
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

  function resolvePendingAssistantStream() {
    if (!pendingAssistantStream || pendingAssistantStream.done) return
    const session = pendingAssistantStream
    session.assistantTurn.streaming = false
    session.done = true
    pendingAssistantStream = null
    if (session.streamError) {
      session.reject(session.streamError)
      return
    }
    session.resolve()
  }

  function rejectPendingAssistantStream(err: Error) {
    if (!pendingAssistantStream || pendingAssistantStream.done) return
    const session = pendingAssistantStream
    session.assistantTurn.streaming = false
    session.done = true
    pendingAssistantStream = null
    session.reject(err)
  }

  function ensureDiscussStream(): PendingAssistantStream {
    if (pendingAssistantStream && !pendingAssistantStream.done) {
      return pendingAssistantStream
    }
    messages.push(createOptimisticAssistantTurn())
    const assistantTurn = messages[messages.length - 1] as ChatAssistantTurn
    void createCompletionForAssistantTurn(assistantTurn).catch(() => {})
    return pendingAssistantStream!
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

  function rememberAssistantError(errorMessage: string) {
    const sid = (streamingSessionId.value ?? sessionId.value ?? '').trim()
    const text = errorMessage.trim()
    if (!sid || !text) return
    const current = ephemeralAssistantErrors.get(sid) ?? []
    if (current.includes(text)) return
    ephemeralAssistantErrors.set(sid, [...current, text].slice(-5))
  }

  function appendAssistantError(session: PendingAssistantStream, errorMessage: string) {
    const text = errorMessage.trim()
    if (!text) return

    rememberAssistantError(text)
    session.assistantTurn.messages.push({
      id: nextAssistantMessageId(session.assistantTurn),
      type: 'error',
      content: text,
    })
  }

  function pruneEmptyAssistantTurnIfPending() {
    if (!pendingAssistantStream) return
    const turn = pendingAssistantStream.assistantTurn
    if (turn.messages.length > 0) return
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
  }

  function handleWSStreamEvent(event: UIStreamEvent, targetSessionId?: string) {
    switch (event.type) {
      case 'start':
        if (suppressNextStartPlaceholder) {
          suppressNextStartPlaceholder = false
        } else {
          ensureDiscussStream()
        }
        break
      case 'message':
        upsertAssistantUIMessage(ensureDiscussStream().assistantTurn, event.data)
        break
      case 'end':
        const endedBotId = pendingAssistantStream?.botId
        const endedSessionId = targetSessionId || pendingAssistantStream?.sessionId
        pruneEmptyAssistantTurnIfPending()
        resolvePendingAssistantStream()
        streamingSessionId.value = null
        loading.value = false
        abortFn = null
        void refreshCurrentSession(endedBotId, endedSessionId)
        break
      case 'error': {
        const session = ensureDiscussStream()
        const message = event.message || 'stream error'
        const stage: SendMessageStage = hasVisibleAssistantBlocks(session.assistantTurn) ? 'stream' : 'startup'
        if (stage === 'stream') {
          appendAssistantError(session, message)
          session.assistantTurn.streaming = false
          session.streamError = new StreamFailureError(message, stage)
        } else {
          const idx = messages.indexOf(session.assistantTurn)
          if (idx >= 0) messages.splice(idx, 1)
          rejectPendingAssistantStream(new StreamFailureError(message, stage))
        }
        loading.value = false
        if (stage === 'startup') {
          streamingSessionId.value = null
          abortFn = null
        }
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
      const streamStillActive = streamingSessionId.value === sid && pendingAssistantStream && !pendingAssistantStream.done
      if (!streamStillActive && pendingAssistantStream) {
        pendingAssistantStream.assistantTurn.streaming = false
        pendingAssistantStream = null
      }
      if (!streamStillActive) {
        streamingSessionId.value = null
      }
    })().finally(() => {
      if (refreshPromise?.promise === promise) {
        refreshPromise = null
      }
    })
    refreshPromise = { key, promise }

    await promise
  }

  function scheduleRefreshCurrentSession(expectedSessionId?: string, delay = 100) {
    const sid = (sessionId.value ?? '').trim()
    if (!sid) return
    if (expectedSessionId?.trim() && expectedSessionId.trim() !== sid) return
    if (refreshTimer) return

    refreshTimer = setTimeout(() => {
      refreshTimer = null
      const sidNow = (sessionId.value ?? '').trim()
      const streamActive = streamingSessionId.value === sidNow && pendingAssistantStream && !pendingAssistantStream.done
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
      const isKnownBackgroundStream = streamingSessionId.value === sid && pendingAssistantStream && !pendingAssistantStream.done
      if (!isKnownBackgroundStream) return
    }

    if (stream.type === 'start' || stream.type === 'message') {
      if (sid) streamingSessionId.value = sid
      loading.value = true
      suppressNextStartPlaceholder = false
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
    if (activeWs?.connected) {
      activeWs.abort()
    }
    abortFn?.()
    abortFn = null
    for (const message of messages) {
      if (message.role === 'assistant' && message.streaming) {
        message.streaming = false
      }
    }
    streamingSessionId.value = null
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

  function touchSession(targetSessionId: string) {
    const index = sessions.value.findIndex(session => session.id === targetSessionId)
    if (index < 0) return
    const [target] = sessions.value.splice(index, 1)
    if (!target) return
    target.updated_at = new Date().toISOString()
    sessions.value.unshift(target)
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

  async function sendMessage(text: string, attachments?: ChatAttachment[]): Promise<SendMessageResult> {
    const trimmed = text.trim()
    if ((!trimmed && !attachments?.length) || streaming.value || !currentBotId.value) return { ok: false, stage: 'startup' }

    loading.value = true
    let assistantTurn: ChatAssistantTurn | null = null
    let userTurn: ChatUserTurn | null = null

    try {
      await ensureActiveSession()

      const bid = currentBotId.value!
      const sid = sessionId.value!
      streamingSessionId.value = sid

      userTurn = createOptimisticUserTurn(trimmed, attachments)
      messages.push(userTurn)
      messages.push(createOptimisticAssistantTurn())
      assistantTurn = messages[messages.length - 1] as ChatAssistantTurn

      const modelId = overrideModelId.value || undefined
      const effort = overrideReasoningEffort.value
      const reasoningEffort = effort && effort !== 'off' ? effort : undefined

      const ws = ensureWebSocket(bid)
      if (ws) {
        if (!ws.connected) {
          throw new StreamFailureError('WebSocket is not connected', 'startup')
        }
        const completion = createCompletionForAssistantTurn(assistantTurn)
        abortFn = () => {
          const abortError = new Error('aborted')
          abortError.name = 'AbortError'
          ws.abort()
          rejectPendingAssistantStream(abortError)
        }
        ws.send({
          type: 'message',
          text: trimmed,
          session_id: sid,
          attachments,
          model_id: modelId,
          reasoning_effort: reasoningEffort,
        })
        await completion
        await refreshCurrentSession(bid, sid)
      } else {
        void createCompletionForAssistantTurn(assistantTurn).catch(() => {})
        await sendLocalChannelMessage(bid, trimmed, attachments, { modelId, reasoningEffort })
        await refreshCurrentSession(bid, sid)
      }

      assistantTurn.streaming = false
      streamingSessionId.value = null
      loading.value = false
      abortFn = null
      touchSession(sid)
      return { ok: true }
    } catch (error) {
      const isAbort = error instanceof Error && error.name === 'AbortError'
      const reason = error instanceof Error ? error.message : 'Unknown error'
      const stage: SendMessageStage = error instanceof StreamFailureError
        ? error.stage
        : (assistantTurn && hasVisibleAssistantBlocks(assistantTurn) ? 'stream' : 'startup')

      if (!isAbort && stage === 'startup') {
        if (assistantTurn) {
          const idx = messages.indexOf(assistantTurn)
          if (idx >= 0) messages.splice(idx, 1)
        }
        if (userTurn) {
          const idx = messages.indexOf(userTurn)
          if (idx >= 0) messages.splice(idx, 1)
        }
      } else if (!isAbort && assistantTurn && stage === 'stream') {
        if (!assistantTurn.messages.some(block => block.type === 'error')) {
          appendAssistantError({ assistantTurn, done: false, resolve: () => {}, reject: () => {} }, reason)
        }
        assistantTurn.streaming = false
      } else if (!isAbort) {
        messages.push({
          id: nextId(),
          role: 'assistant',
          messages: [{
            id: 0,
            type: 'error',
            content: reason,
          }],
          timestamp: new Date().toISOString(),
          streaming: false,
        })
      }
      pendingAssistantStream = null
      streamingSessionId.value = null
      loading.value = false
      abortFn = null
      if (isAbort) return { ok: false, stage: 'stream', error: reason }
      if (stage === 'startup') {
        return {
          ok: false,
          stage,
          error: reason,
          restoreInput: text,
          restoreAttachments: attachments,
        }
      }
      return { ok: false, stage, error: reason }
    }
  }

  async function respondToolApproval(approval: UIToolApproval, decision: 'approve' | 'reject') {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid || !approval.approval_id || streaming.value) return
    const ws = ensureWebSocket(bid)
    streamingSessionId.value = sid
    loading.value = true
    suppressNextStartPlaceholder = true
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
    abortFn = () => {
      const abortError = new Error('aborted')
      abortError.name = 'AbortError'
      ws?.abort()
      rejectPendingAssistantStream(abortError)
      streamingSessionId.value = null
      loading.value = false
      abortFn = null
    }
    ws?.send({
      type: 'tool_approval_response',
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
    chats,
    chatId,
    sessionId,
    currentBotId,
    bots,
    activeSession,
    activeChatReadOnly,
    loading,
    loadingChats,
    loadingOlder,
    hasMoreOlder,
    initializing,
    overrideModelId,
    overrideReasoningEffort,
    fsChangedAt,
    initialize,
    selectBot,
    selectSession,
    selectChat: selectSession,
    createNewSession,
    createNewChat: createNewSession,
    removeSession,
    removeChat: removeSession,
    deleteChat: removeSession,
    sendMessage,
    respondToolApproval,
    clearMessages,
    loadOlderMessages,
    abort,
  }
})
