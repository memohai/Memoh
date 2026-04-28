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
  type UIMessage,
  type UIReasoningMessage,
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

export interface ToolCallBlock extends UIToolMessage {
  toolCallId: string
  toolName: string
  result: unknown | null
  done: boolean
  approval?: UIToolApproval
}

export type ContentBlock = TextBlock | ThinkingBlock | ToolCallBlock | AttachmentBlock

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

export type ChatMessage = ChatUserTurn | ChatAssistantTurn

interface PendingAssistantStream {
  assistantTurn: ChatAssistantTurn
  done: boolean
  resolve: () => void
  reject: (err: Error) => void
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

  let abortFn: (() => void) | null = null
  let messageEventsSince = ''
  let pendingAssistantStream: PendingAssistantStream | null = null
  let activeWs: ChatWebSocket | null = null
  let refreshTimer: ReturnType<typeof setTimeout> | null = null
  let refreshPromise: Promise<void> | null = null
  let suppressNextStartPlaceholder = false
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

  function normalizeUIMessage(msg: UIMessage): ContentBlock {
    switch (msg.type) {
      case 'tool':
        return {
          ...msg,
          toolCallId: msg.tool_call_id,
          toolName: msg.name,
          result: msg.output ?? null,
          done: !msg.running,
          approval: msg.approval,
          progress: msg.progress ? [...msg.progress] : undefined,
        }
      case 'attachments':
        return {
          ...msg,
          attachments: msg.attachments.map(normalizeAttachment),
        }
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

    return {
      id: String(turn.id ?? nextId()),
      role: 'assistant',
      messages: (turn.messages ?? []).map(normalizeUIMessage),
      timestamp: normalizeTimestamp(turn.timestamp),
      platform: (turn.platform ?? '').trim() || undefined,
      streaming: false,
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
    messageEventsSince = ''
    for (const item of items) {
      updateSince(item.timestamp)
    }
  }

  function replaceMessages(items: UITurn[]) {
    const normalized = items.map(normalizeTurn)
    messages.splice(0, messages.length, ...normalized)
    updateSinceFromMessages(normalized)
  }

  function createCompletionForAssistantTurn(assistantTurn: ChatAssistantTurn): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      pendingAssistantStream = {
        assistantTurn,
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
        url: attachment.base64,
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
  }

  function nextAssistantMessageId(turn: ChatAssistantTurn): number {
    return turn.messages.reduce((maxId, message) => Math.max(maxId, message.id), -1) + 1
  }

  function appendAssistantError(session: PendingAssistantStream, errorMessage: string) {
    const text = errorMessage.trim()
    if (!text) return

    for (let index = session.assistantTurn.messages.length - 1; index >= 0; index -= 1) {
      const current = session.assistantTurn.messages[index]
      if (current?.type === 'text') {
        session.assistantTurn.messages[index] = {
          ...current,
          content: `${current.content}\n\n**Error:** ${text}`.trim(),
        }
        return
      }
    }

    session.assistantTurn.messages.push({
      id: nextAssistantMessageId(session.assistantTurn),
      type: 'text',
      content: `**Error:** ${text}`,
    })
  }

  function pruneEmptyAssistantTurnIfPending() {
    if (!pendingAssistantStream) return
    const turn = pendingAssistantStream.assistantTurn
    if (turn.messages.length > 0) return
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
  }

  function handleWSStreamEvent(event: UIStreamEvent) {
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
        pruneEmptyAssistantTurnIfPending()
        resolvePendingAssistantStream()
        streamingSessionId.value = null
        loading.value = false
        abortFn = null
        void refreshCurrentSession()
        break
      case 'error': {
        const session = ensureDiscussStream()
        appendAssistantError(session, event.message || 'stream error')
        rejectPendingAssistantStream(new Error(event.message || 'stream error'))
        streamingSessionId.value = null
        loading.value = false
        abortFn = null
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

    if (refreshPromise) {
      await refreshPromise
      return
    }

    refreshPromise = (async () => {
      const turns = await fetchMessagesUI(bid, sid, { limit: PAGE_SIZE })
      if (currentBotId.value !== bid || sessionId.value !== sid) return
      replaceMessages(turns)
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
      refreshPromise = null
    })

    await refreshPromise
  }

  function scheduleRefreshCurrentSession(expectedSessionId?: string, delay = 100) {
    const sid = (sessionId.value ?? '').trim()
    if (!sid) return
    if (expectedSessionId?.trim() && expectedSessionId.trim() !== sid) return
    if (refreshTimer) return

    refreshTimer = setTimeout(() => {
      refreshTimer = null
      void refreshCurrentSession()
    }, delay)
  }

  function handleStreamEvent(targetBotId: string, event: MessageStreamEvent) {
    const eventType = (event.type ?? '').toLowerCase()
    const eBotId = (event.bot_id ?? '').trim()
    if (eBotId && eBotId !== targetBotId) return

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
  }

  async function loadOlderMessages(): Promise<number> {
    const bid = currentBotId.value ?? ''
    const sid = sessionId.value ?? ''
    if (!bid || !sid || loadingOlder.value || !hasMoreOlder.value) return 0
    const first = messages[0]
    if (!first?.timestamp) return 0

    loadingOlder.value = true
    try {
      const turns = await fetchMessagesUI(bid, sid, {
        limit: PAGE_SIZE,
        before: first.timestamp,
      })
      const existingIds = new Set(messages.map(message => message.id))
      const older = turns
        .map(normalizeTurn)
        .filter(turn => !existingIds.has(turn.id))
      if (older.length === 0) {
        hasMoreOlder.value = false
        return 0
      }
      messages.unshift(...older)
      return older.length
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
        return
      }

      const visible = await fetchSessions(bid)
      sessions.value = visible
      if (!visible.length) {
        messageEventsSince = ''
        sessionId.value = null
        replaceMessages([])
      } else {
        const activeSessionId = sessionId.value && visible.some(session => session.id === sessionId.value)
          ? sessionId.value
          : (visible.find((s) => s.type === 'chat' || s.type === 'discuss')?.id ?? visible[0]!.id)
        sessionId.value = activeSessionId
        await loadMessages(bid, activeSessionId)
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
    sessionId.value = sid
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      await loadMessages(bid, sid)
    } finally {
      loadingChats.value = false
    }
  }

  async function createNewSession() {
    const bid = await ensureBot()
    if (!bid) return
    sessionId.value = null
    replaceMessages([])
  }

  async function removeSession(targetSessionId: string) {
    const delId = targetSessionId.trim()
    if (!delId) return
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      await requestDeleteSession(bid, delId)
      const remaining = sessions.value.filter(session => session.id !== delId)
      sessions.value = remaining
      if (sessionId.value !== delId) return
      if (!remaining.length) {
        sessionId.value = null
        replaceMessages([])
        return
      }
      sessionId.value = remaining[0]!.id
      await loadMessages(bid, remaining[0]!.id)
    } finally {
      loadingChats.value = false
    }
  }

  async function sendMessage(text: string, attachments?: ChatAttachment[]) {
    const trimmed = text.trim()
    if ((!trimmed && !attachments?.length) || streaming.value || !currentBotId.value) return

    loading.value = true
    let assistantTurn: ChatAssistantTurn | null = null

    try {
      await ensureActiveSession()

      const bid = currentBotId.value!
      const sid = sessionId.value!
      streamingSessionId.value = sid

      messages.push(createOptimisticUserTurn(trimmed, attachments))
      messages.push(createOptimisticAssistantTurn())
      assistantTurn = messages[messages.length - 1] as ChatAssistantTurn

      const modelId = overrideModelId.value || undefined
      const effort = overrideReasoningEffort.value
      const reasoningEffort = effort && effort !== 'off' ? effort : undefined

      const ws = ensureWebSocket(bid)
      if (ws) {
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
    } catch (error) {
      const isAbort = error instanceof Error && error.name === 'AbortError'
      const reason = error instanceof Error ? error.message : 'Unknown error'
      if (!isAbort && assistantTurn) {
        assistantTurn.messages = [{
          id: 0,
          type: 'text',
          content: `Failed to send message: ${reason}`,
        }]
        assistantTurn.streaming = false
      } else if (!isAbort) {
        messages.push({
          id: nextId(),
          role: 'assistant',
          messages: [{
            id: 0,
            type: 'text',
            content: `Failed to send message: ${reason}`,
          }],
          timestamp: new Date().toISOString(),
          streaming: false,
        })
      }
      pendingAssistantStream = null
      streamingSessionId.value = null
      loading.value = false
      abortFn = null
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

