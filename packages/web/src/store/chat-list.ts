import { defineStore } from 'pinia'
import { computed, reactive, ref, watch } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { useRetryingStream } from '@/composables/useRetryingStream'
import { useUserStore } from '@/store/user'
import {
  createChat,
  deleteChat as requestDeleteChat,
  type Bot,
  type ChatSummary,
  type Message,
  type StreamEvent,
  fetchBots,
  fetchMessages,
  fetchChats,
  extractMessageText,
  extractToolCalls,
  extractAllToolResults,
  sendLocalChannelMessage,
  streamLocalChannel,
  streamMessageEvents,
  type ChatAttachment,
} from '@/composables/api/useChat'

// ---- Message model (blocks-based, aligned with main branch) ----

export interface TextBlock {
  type: 'text'
  content: string
}

export interface ThinkingBlock {
  type: 'thinking'
  content: string
  done: boolean
}

export interface ToolCallBlock {
  type: 'tool_call'
  toolCallId: string
  toolName: string
  input: unknown
  result: unknown | null
  done: boolean
}

export interface AttachmentBlock {
  type: 'attachment'
  attachments: Array<Record<string, unknown>>
}

export type ContentBlock = TextBlock | ThinkingBlock | ToolCallBlock | AttachmentBlock

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  blocks: ContentBlock[]
  timestamp: Date
  streaming: boolean
  platform?: string
  senderDisplayName?: string
  senderAvatarUrl?: string
  isSelf?: boolean
}

interface PendingAssistantStream {
  assistantMsg: ChatMessage
  textBlockIdx: number
  thinkingBlockIdx: number
  done: boolean
  resolve: () => void
  reject: (err: Error) => void
}

// ---- Store ----

export const useChatStore = defineStore('chat', () => {
  const messages = reactive<ChatMessage[]>([])
  const streaming = ref(false)
  const chats = ref<ChatSummary[]>([])
  const loading = ref(false)
  const loadingChats = ref(false)
  const loadingOlder = ref(false)
  const hasMoreOlder = ref(true)
  const initializing = ref(false)
  const currentBotId = useLocalStorage<string | null>('chat-bot-id', null)
  const chatId = useLocalStorage<string | null>('chat-id', null)
  const bots = ref<Bot[]>([])

  let abortFn: (() => void) | null = null
  let messageEventsSince = ''
  let pendingAssistantStream: PendingAssistantStream | null = null
  const messageEventsStream = useRetryingStream()
  const localStream = useRetryingStream()

  const participantChats = computed(() =>
    chats.value.filter((c) => (c.access_mode ?? 'participant') === 'participant'),
  )
  const observedChats = computed(() =>
    chats.value.filter((c) => c.access_mode === 'channel_identity_observed'),
  )
  const activeChat = computed(() =>
    chats.value.find((c) => c.id === chatId.value) ?? null,
  )
  const activeChatReadOnly = computed(() =>
    activeChat.value?.access_mode === 'channel_identity_observed',
  )

  watch(currentBotId, (newId) => {
    if (newId) {
      void initialize()
    } else {
      stopMessageEvents()
      stopLocalStream()
      rejectPendingAssistantStream(new Error('Bot stream stopped'))
      messageEventsSince = ''
      chats.value = []
      chatId.value = null
      replaceMessages([])
    }
  })

  const nextId = () => `${Date.now()}-${Math.floor(Math.random() * 1000)}`

  const isPendingBot = (bot: Bot | null | undefined) =>
    bot?.status === 'creating' || bot?.status === 'deleting'

  // ---- Message adapter: convert server Message to ChatMessage ----

  function mediaTypeFromMime(mime: string): string {
    const m = (mime || '').toLowerCase().trim()
    if (m.startsWith('image/')) return 'image'
    if (m.startsWith('audio/')) return 'audio'
    if (m.startsWith('video/')) return 'video'
    return 'file'
  }

  function buildAssetBlocks(raw: Message): AttachmentBlock[] {
    if (!raw.assets?.length) return []
    const items: Array<Record<string, unknown>> = raw.assets.map((a) => ({
      type: mediaTypeFromMime(a.mime),
      content_hash: a.content_hash,
      bot_id: raw.bot_id,
      mime: a.mime,
      size: a.size_bytes,
      storage_key: a.storage_key,
    }))
    return [{ type: 'attachment', attachments: items }]
  }

  function messageToChat(raw: Message): ChatMessage | null {
    if (raw.role !== 'user' && raw.role !== 'assistant') return null

    const text = extractMessageText(raw)
    const assetBlocks = buildAssetBlocks(raw)
    if (!text && assetBlocks.length === 0) return null

    const blocks: ContentBlock[] = []
    if (text) blocks.push({ type: 'text', content: text })
    blocks.push(...assetBlocks)

    const createdAt = raw.created_at ? new Date(raw.created_at) : new Date()
    const timestamp = Number.isNaN(createdAt.getTime()) ? new Date() : createdAt
    const platform = (raw.platform ?? '').trim().toLowerCase()
    const channelTag = platform && platform !== 'web' ? platform : undefined

    if (raw.role === 'user') {
      const isSelf = resolveIsSelf(raw)
      const senderName = (raw.sender_display_name ?? '').trim() || undefined
      const senderAvatar = (raw.sender_avatar_url ?? '').trim() || undefined
      return {
        id: raw.id || nextId(),
        role: 'user',
        blocks,
        timestamp,
        streaming: false,
        isSelf,
        ...(channelTag && { platform: channelTag }),
        ...(channelTag && { senderDisplayName: senderName, senderAvatarUrl: senderAvatar }),
      }
    }

    return {
      id: raw.id || nextId(),
      role: 'assistant',
      blocks,
      timestamp,
      streaming: false,
      ...(channelTag && { platform: channelTag }),
    }
  }

  /**
   * Convert an ordered array of raw messages into ChatMessages,
   * merging consecutive assistant(tool_calls) + tool + assistant(text)
   * sequences into a single ChatMessage with ToolCallBlocks.
   */
  function convertMessagesToChats(rows: Message[]): ChatMessage[] {
    const result: ChatMessage[] = []
    let pendingAssistant: ChatMessage | null = null
    const pendingToolCallMap = new Map<string, ToolCallBlock>()

    function flushPending() {
      if (!pendingAssistant) return
      for (const block of pendingAssistant.blocks) {
        if (block.type === 'tool_call' && !block.done) block.done = true
      }
      result.push(pendingAssistant)
      pendingAssistant = null
      pendingToolCallMap.clear()
    }

    function makeTimestamp(raw: Message): Date {
      const d = raw.created_at ? new Date(raw.created_at) : new Date()
      return Number.isNaN(d.getTime()) ? new Date() : d
    }

    for (const raw of rows) {
      if (raw.role === 'user') {
        flushPending()
        const chat = messageToChat(raw)
        if (chat) result.push(chat)
        continue
      }

      if (raw.role === 'assistant') {
        const toolCalls = extractToolCalls(raw)
        const text = extractMessageText(raw)

        if (toolCalls.length > 0) {
          if (!pendingAssistant) {
            const platform = (raw.platform ?? '').trim().toLowerCase()
            const channelTag = platform && platform !== 'web' ? platform : undefined
            pendingAssistant = {
              id: raw.id || nextId(),
              role: 'assistant',
              blocks: [],
              timestamp: makeTimestamp(raw),
              streaming: false,
              ...(channelTag && { platform: channelTag }),
            }
          }
          if (text) {
            pendingAssistant.blocks.push({ type: 'text', content: text })
          }
          for (const tc of toolCalls) {
            const block: ToolCallBlock = {
              type: 'tool_call',
              toolCallId: tc.id ?? '',
              toolName: tc.name,
              input: tc.input,
              result: null,
              done: false,
            }
            pendingAssistant.blocks.push(block)
            if (tc.id) pendingToolCallMap.set(tc.id, block)
          }
          continue
        }

        // Assistant message without tool_calls
        if (pendingAssistant && text) {
          pendingAssistant.blocks.push({ type: 'text', content: text })
          flushPending()
          continue
        }

        flushPending()
        const chat = messageToChat(raw)
        if (chat) result.push(chat)
        continue
      }

      if (raw.role === 'tool') {
        const results = extractAllToolResults(raw)
        for (const r of results) {
          if (r.toolCallId && pendingToolCallMap.has(r.toolCallId)) {
            const block = pendingToolCallMap.get(r.toolCallId)!
            block.result = r.output
            block.done = true
          }
        }
        continue
      }
    }

    flushPending()
    return result
  }

  function resolveIsSelf(raw: Message): boolean {
    const platform = (raw.platform ?? '').trim().toLowerCase()
    if (!platform || platform === 'web') return true
    const senderUserId = (raw.sender_user_id ?? '').trim()
    if (!senderUserId) return false
    const userStore = useUserStore()
    const currentUserId = (userStore.userInfo.id ?? '').trim()
    if (!currentUserId) return false
    return senderUserId === currentUserId
  }

  function resolveCrossChannelIsSelf(senderUserId: string): boolean {
    if (!senderUserId) return false
    const userStore = useUserStore()
    const currentUserId = (userStore.userInfo.id ?? '').trim()
    if (!currentUserId) return false
    return senderUserId === currentUserId
  }

  // ---- Abort ----

  function abort() {
    abortFn?.()
    abortFn = null
    for (const msg of messages) {
      if (msg.streaming) msg.streaming = false
    }
    streaming.value = false
  }

  // ---- Message list management ----

  function replaceMessages(items: ChatMessage[]) {
    messages.splice(0, messages.length, ...items)
  }

  // ---- SSE real-time events ----

  function stopMessageEvents() {
    messageEventsStream.stop()
  }

  function stopLocalStream() {
    localStream.stop()
  }

  function pushAssistantBlock(session: PendingAssistantStream, block: ContentBlock): number {
    session.assistantMsg.blocks.push(block)
    return session.assistantMsg.blocks.length - 1
  }

  function appendAssistantError(session: PendingAssistantStream, errorMessage: string) {
    const message = errorMessage.trim()
    if (!message) return
    ensureAssistantTextBlock(session).content += `\n\n**Error:** ${message}`
  }

  function ensureAssistantTextBlock(session: PendingAssistantStream): TextBlock {
    if (session.textBlockIdx < 0 || session.assistantMsg.blocks[session.textBlockIdx]?.type !== 'text') {
      session.textBlockIdx = pushAssistantBlock(session, { type: 'text', content: '' })
    }
    return session.assistantMsg.blocks[session.textBlockIdx] as TextBlock
  }

  function resolveStreamErrorMessage(event: StreamEvent, fallback: string): string {
    if (typeof event.message === 'string') return event.message
    if (typeof event.error === 'string') return event.error
    return fallback
  }

  function resolvePendingAssistantStream() {
    if (!pendingAssistantStream || pendingAssistantStream.done) return
    const session = pendingAssistantStream
    session.done = true
    pendingAssistantStream = null
    session.resolve()
  }

  function rejectPendingAssistantStream(err: Error) {
    if (!pendingAssistantStream || pendingAssistantStream.done) return
    const session = pendingAssistantStream
    session.done = true
    pendingAssistantStream = null
    session.reject(err)
  }

  function handleLocalStreamEvent(event: StreamEvent) {
    // Cross-channel events arrive without a pending session. Detect them via
    // source_channel metadata injected by the RouteHubBroadcaster.
    const meta = (event as Record<string, unknown>).metadata as Record<string, unknown> | undefined
    const sourceChannel = meta?.source_channel as string | undefined
    const isCrossChannel = !!sourceChannel

    // Cross-channel user message (the inbound message from Telegram/Feishu user).
    if (isCrossChannel && (event.type ?? '').toLowerCase() === 'final' && meta?.role === 'user') {
      const finalPayload = (event as Record<string, unknown>).final as Record<string, unknown> | undefined
      const msg = finalPayload?.message as Record<string, unknown> | undefined
      if (msg) {
        const text = String(msg.text ?? '').trim()
        const msgMeta = (msg.metadata as Record<string, unknown> | undefined)
        const senderName = (msgMeta?.sender_display_name as string) ?? sourceChannel
        const senderUserId = String(meta?.sender_user_id ?? '').trim()
        const blocks: ContentBlock[] = []
        if (text) blocks.push({ type: 'text', content: text })
        const rawAtts = (msg.attachments ?? msg.Attachments) as Array<Record<string, unknown>> | undefined
        if (Array.isArray(rawAtts) && rawAtts.length > 0) {
          const items = rawAtts.map((a) => ({
            type: mediaTypeFromMime(String(a.mime ?? '')),
            content_hash: String(a.content_hash ?? ''),
            bot_id: currentBotId.value ?? '',
            mime: String(a.mime ?? ''),
            size: Number(a.size ?? 0),
            storage_key: String((a.metadata as Record<string, unknown> | undefined)?.storage_key ?? ''),
          }))
          blocks.push({ type: 'attachment', attachments: items })
        }
        if (blocks.length > 0) {
          messages.push({
            id: nextId(),
            role: 'user',
            blocks,
            timestamp: new Date(),
            streaming: false,
            isSelf: resolveCrossChannelIsSelf(senderUserId),
            platform: sourceChannel,
            senderDisplayName: senderName,
          })
        }
      }
      return
    }

    // Cross-channel assistant events: auto-create a session when none exists.
    if (isCrossChannel && !pendingAssistantStream) {
      const type = (event.type ?? '').toLowerCase()
      // Only start a session for events that carry actual content.
      if (type === 'delta' || type === 'text_delta' || type === 'text_start'
        || type === 'reasoning_start' || type === 'reasoning_delta'
        || type === 'tool_call_start' || type === 'attachment_delta'
        || type === 'status' || type === 'processing_started') {
        messages.push({
          id: nextId(),
          role: 'assistant',
          blocks: [],
          timestamp: new Date(),
          streaming: true,
          platform: sourceChannel,
        })
        // IMPORTANT: get the reactive proxy from the array, not the plain object.
        // Vue 3 wraps pushed objects in a Proxy; using the original ref bypasses reactivity.
        const reactiveMsg = messages[messages.length - 1]!
        pendingAssistantStream = {
          assistantMsg: reactiveMsg,
          textBlockIdx: -1,
          thinkingBlockIdx: -1,
          done: false,
          resolve: () => { reactiveMsg.streaming = false },
          reject: () => { reactiveMsg.streaming = false },
        }
      } else {
        return
      }
    }

    const session = pendingAssistantStream
    if (!session || session.done) return

    const type = (event.type ?? '').toLowerCase()
    switch (type) {
      case 'text_start':
        session.textBlockIdx = pushAssistantBlock(session, { type: 'text', content: '' })
        break
      case 'text_delta':
        if (typeof event.delta === 'string') {
          ensureAssistantTextBlock(session).content += event.delta
        }
        break
      case 'text_end':
        session.textBlockIdx = -1
        break
      case 'reasoning_start':
        session.thinkingBlockIdx = pushAssistantBlock(session, { type: 'thinking', content: '', done: false })
        break
      case 'reasoning_delta':
        if (typeof event.delta === 'string') {
          if (session.thinkingBlockIdx < 0 || session.assistantMsg.blocks[session.thinkingBlockIdx]?.type !== 'thinking') {
            session.thinkingBlockIdx = pushAssistantBlock(session, { type: 'thinking', content: '', done: false })
          }
          ;(session.assistantMsg.blocks[session.thinkingBlockIdx] as ThinkingBlock).content += event.delta
        }
        break
      case 'reasoning_end':
        if (session.thinkingBlockIdx >= 0 && session.assistantMsg.blocks[session.thinkingBlockIdx]?.type === 'thinking') {
          ;(session.assistantMsg.blocks[session.thinkingBlockIdx] as ThinkingBlock).done = true
        }
        session.thinkingBlockIdx = -1
        break
      case 'tool_call_start':
        pushAssistantBlock(session, {
          type: 'tool_call',
          toolCallId: (event.toolCallId as string) ?? '',
          toolName: (event.toolName as string) ?? 'unknown',
          input: event.input ?? null,
          result: null,
          done: false,
        })
        session.textBlockIdx = -1
        break
      case 'tool_call_end': {
        const callId = (event.toolCallId as string) ?? ''
        let matched = false
        if (callId) {
          for (let i = 0; i < session.assistantMsg.blocks.length; i++) {
            const block = session.assistantMsg.blocks[i]
            if (block && block.type === 'tool_call' && block.toolCallId === callId && !block.done) {
              block.result = event.result ?? null
              block.input = event.input ?? block.input
              block.done = true
              matched = true
              break
            }
          }
        }
        if (!matched) {
          for (let i = 0; i < session.assistantMsg.blocks.length; i++) {
            const block = session.assistantMsg.blocks[i]
            if (block && block.type === 'tool_call' && block.toolName === event.toolName && !block.done) {
              block.result = event.result ?? null
              block.input = event.input ?? block.input
              block.done = true
              break
            }
          }
        }
        break
      }
      case 'attachment_delta': {
        const items = event.attachments
        if (Array.isArray(items) && items.length > 0) {
          const lastBlock = session.assistantMsg.blocks[session.assistantMsg.blocks.length - 1]
          if (lastBlock && lastBlock.type === 'attachment') {
            lastBlock.attachments.push(...items)
          } else {
            pushAssistantBlock(session, { type: 'attachment', attachments: [...items] })
          }
        }
        break
      }
      case 'final':
        // Text and attachments already accumulated via deltas/attachment_delta.
        // For cross-channel, finalize via processing_completed instead.
        break
      case 'processing_started':
        if (session.assistantMsg.blocks.length === 0) {
          session.textBlockIdx = pushAssistantBlock(session, { type: 'text', content: '' })
        }
        break
      case 'processing_completed':
        resolvePendingAssistantStream()
        break
      case 'processing_failed': {
        const message = resolveStreamErrorMessage(event, 'processing failed')
        appendAssistantError(session, message)
        rejectPendingAssistantStream(new Error(message))
        break
      }
      case 'error': {
        const message = resolveStreamErrorMessage(event, 'stream error')
        appendAssistantError(session, message)
        rejectPendingAssistantStream(new Error(message))
        break
      }
      case 'agent_start':
      case 'agent_end':
      default: {
        const fallback = extractFallbackText(event)
        if (fallback) {
          ensureAssistantTextBlock(session).content += fallback
        }
        break
      }
    }
  }

  function startLocalStream(targetBotId: string) {
    const bid = targetBotId.trim()
    stopLocalStream()
    if (!bid) return

    localStream.start(async (signal) => {
      await streamLocalChannel(bid, signal, handleLocalStreamEvent)
    })
  }

  function updateSince(createdAt?: string) {
    const v = (createdAt ?? '').trim()
    if (!v) return
    if (!messageEventsSince) { messageEventsSince = v; return }
    const cur = Date.parse(messageEventsSince)
    const next = Date.parse(v)
    if (!Number.isNaN(next) && (Number.isNaN(cur) || next > cur)) {
      messageEventsSince = v
    }
  }

  function updateSinceFromRows(rows: Message[]) {
    messageEventsSince = ''
    for (const row of rows) updateSince(row.created_at)
  }

  function hasMessageWithId(id: string) {
    const tid = id.trim()
    return tid ? messages.some((m) => String(m.id).trim() === tid) : false
  }

  function appendRealtimeMessage(raw: Message) {
    updateSince(raw.created_at)
    const platform = (raw.platform ?? '').trim().toLowerCase()
    if (platform === 'web') return
    const mid = String(raw.id ?? '').trim()
    if (mid && hasMessageWithId(mid)) return
    const item = messageToChat(raw)
    if (!item) return
    messages.push(item)
    messages.sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime())
    if (chatId.value) touchChat(chatId.value)
  }

  function handleStreamEvent(targetBotId: string, event: Record<string, unknown>) {
    if (String(event.type ?? '').toLowerCase() !== 'message_created') return
    const eBotId = String(event.bot_id ?? '').trim()
    if (eBotId && eBotId !== targetBotId) return
    const payload = event.message
    if (!payload || typeof payload !== 'object') return
    const raw = payload as Message
    const pBotId = String(raw.bot_id ?? '').trim()
    if (pBotId && pBotId !== targetBotId) return
    appendRealtimeMessage(raw)
  }

  function startMessageEvents(targetBotId: string) {
    const bid = targetBotId.trim()
    stopMessageEvents()
    if (!bid) return

    messageEventsStream.start(async (signal) => {
      await streamMessageEvents(
        bid,
        signal,
        (e) => handleStreamEvent(bid, e as unknown as Record<string, unknown>),
        messageEventsSince || undefined,
      )
    })
  }

  // ---- Bot management ----

  async function ensureBot(): Promise<string | null> {
    try {
      const list = await fetchBots()
      bots.value = list
      if (!list.length) { currentBotId.value = null; return null }
      if (currentBotId.value) {
        const found = list.find((b) => b.id === currentBotId.value)
        if (found && !isPendingBot(found)) return currentBotId.value
      }
      const ready = list.find((b) => !isPendingBot(b))
      currentBotId.value = ready ? ready.id : list[0]!.id
      return currentBotId.value
    } catch (err) {
      console.error('Failed to fetch bots:', err)
      return currentBotId.value
    }
  }

  // ---- Pagination ----

  const PAGE_SIZE = 30

  async function loadMessages(botId: string, cid: string) {
    const rows = await fetchMessages(botId, cid, { limit: PAGE_SIZE })
    const items = convertMessagesToChats(rows)
    replaceMessages(items)
    hasMoreOlder.value = true
    updateSinceFromRows(rows)
  }

  async function loadOlderMessages(): Promise<number> {
    const bid = currentBotId.value ?? ''
    const cid = chatId.value ?? ''
    if (!bid || !cid || loadingOlder.value || !hasMoreOlder.value) return 0
    const first = messages[0]
    if (!first?.timestamp) return 0

    const before = first.timestamp.toISOString()
    loadingOlder.value = true
    try {
      const rows = await fetchMessages(bid, cid, { limit: PAGE_SIZE, before })
      const items = convertMessagesToChats(rows)
      if (rows.length < PAGE_SIZE) hasMoreOlder.value = false
      messages.unshift(...items)
      return items.length
    } finally {
      loadingOlder.value = false
    }
  }

  // ---- Chat CRUD ----

  function touchChat(targetChatId: string) {
    const idx = chats.value.findIndex((c) => c.id === targetChatId)
    if (idx < 0) return
    const [target] = chats.value.splice(idx, 1)
    if (!target) return
    target.updated_at = new Date().toISOString()
    chats.value.unshift(target)
  }

  async function ensureActiveChat() {
    if (chatId.value) return
    const bid = currentBotId.value ?? await ensureBot()
    if (!bid) throw new Error('Bot not ready')
    const created = await createChat(bid)
    chats.value = [created, ...chats.value.filter((c) => c.id !== created.id)]
    chatId.value = created.id
    replaceMessages([])
  }

  // ---- Initialize ----

  async function initialize() {
    if (initializing.value) return
    initializing.value = true
    loadingChats.value = true
    stopMessageEvents()
    stopLocalStream()
    try {
      const bid = await ensureBot()
      if (!bid) {
        messageEventsSince = ''
        chats.value = []
        chatId.value = null
        replaceMessages([])
        return
      }
      const visible = await fetchChats(bid)
      chats.value = visible
      if (!visible.length) {
        messageEventsSince = ''
        chatId.value = null
        replaceMessages([])
        return
      }
      const activeChatId = chatId.value && visible.some((c) => c.id === chatId.value)
        ? chatId.value
        : visible[0]!.id
      chatId.value = activeChatId
      await loadMessages(bid, activeChatId)
      startMessageEvents(bid)
      startLocalStream(bid)
    } finally {
      loadingChats.value = false
      initializing.value = false
    }
  }

  async function selectBot(targetBotId: string) {
    if (currentBotId.value === targetBotId) return
    abort()
    currentBotId.value = targetBotId
    chatId.value = null
    await initialize()
  }

  async function selectChat(targetChatId: string) {
    const cid = targetChatId.trim()
    if (!cid || cid === chatId.value) return
    chatId.value = cid
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      await loadMessages(bid, cid)
    } finally {
      loadingChats.value = false
    }
  }

  async function createNewChat() {
    loadingChats.value = true
    try {
      const bid = await ensureBot()
      if (!bid) return
      const created = await createChat(bid)
      chats.value = [created, ...chats.value.filter((c) => c.id !== created.id)]
      chatId.value = created.id
      replaceMessages([])
    } finally {
      loadingChats.value = false
    }
  }

  async function removeChat(targetChatId: string) {
    const delId = targetChatId.trim()
    if (!delId) return
    loadingChats.value = true
    try {
      const bid = currentBotId.value ?? ''
      if (!bid) throw new Error('Bot not selected')
      await requestDeleteChat(bid, delId)
      const remaining = chats.value.filter((c) => c.id !== delId)
      chats.value = remaining
      if (chatId.value !== delId) return
      if (!remaining.length) {
        chatId.value = null
        replaceMessages([])
        return
      }
      chatId.value = remaining[0]!.id
      await loadMessages(bid, remaining[0]!.id)
    } finally {
      loadingChats.value = false
    }
  }

  // ---- Send message (blocks-based streaming) ----

  async function sendMessage(text: string, attachments?: ChatAttachment[]) {
    const trimmed = text.trim()
    if ((!trimmed && !attachments?.length) || streaming.value || !currentBotId.value) return

    loading.value = true
    streaming.value = true

    try {
      await ensureActiveChat()
      if (activeChatReadOnly.value) throw new Error('Chat is read-only')

      const bid = currentBotId.value!
      const cid = chatId.value!

      // Add user message
      const userBlocks: ContentBlock[] = []
      if (trimmed) userBlocks.push({ type: 'text', content: trimmed })
      if (attachments?.length) {
        userBlocks.push({
          type: 'attachment',
          attachments: attachments.map((a) => ({
            type: a.type,
            name: a.name ?? '',
            mime: a.mime ?? '',
            url: a.base64,
          })),
        })
      }
      messages.push({
        id: nextId(),
        role: 'user',
        blocks: userBlocks,
        timestamp: new Date(),
        streaming: false,
      })

      // Add assistant placeholder
      messages.push({
        id: nextId(),
        role: 'assistant',
        blocks: [],
        timestamp: new Date(),
        streaming: true,
      })
      const assistantMsg = messages[messages.length - 1]!
      const completion = new Promise<void>((resolve, reject) => {
        pendingAssistantStream = {
          assistantMsg,
          textBlockIdx: -1,
          thinkingBlockIdx: -1,
          done: false,
          resolve,
          reject,
        }
      })

      abortFn = () => {
        const abortError = new Error('aborted')
        abortError.name = 'AbortError'
        rejectPendingAssistantStream(abortError)
      }

      await sendLocalChannelMessage(bid, trimmed, attachments)
      await completion

      assistantMsg.streaming = false
      streaming.value = false
      loading.value = false
      abortFn = null
      touchChat(cid)
    } catch (err) {
      const isAbort = err instanceof Error && err.name === 'AbortError'
      const reason = err instanceof Error ? err.message : 'Unknown error'
      const last = messages[messages.length - 1]
      if (!isAbort && last?.role === 'assistant' && last.streaming) {
        last.blocks = [{ type: 'text', content: `Failed to send message: ${reason}` }]
        last.streaming = false
      } else if (!isAbort) {
        messages.push({
          id: nextId(),
          role: 'assistant',
          blocks: [{ type: 'text', content: `Failed to send message: ${reason}` }],
          timestamp: new Date(),
          streaming: false,
        })
      }
      pendingAssistantStream = null
      streaming.value = false
      loading.value = false
      abortFn = null
    }
  }

  function clearMessages() {
    abort()
    replaceMessages([])
  }

  return {
    messages,
    streaming,
    chats,
    participantChats,
    observedChats,
    chatId,
    currentBotId,
    bots,
    activeChat,
    activeChatReadOnly,
    loading,
    loadingChats,
    loadingOlder,
    hasMoreOlder,
    initializing,
    initialize,
    selectBot,
    selectChat,
    createNewChat,
    removeChat,
    deleteChat: removeChat,
    sendMessage,
    clearMessages,
    loadOlderMessages,
    abort,
  }
})

function extractFallbackText(event: StreamEvent): string | null {
  if (typeof event.delta === 'string') return event.delta
  if (typeof (event as Record<string, unknown>).text === 'string') return (event as Record<string, unknown>).text as string
  if (typeof (event as Record<string, unknown>).content === 'string') return (event as Record<string, unknown>).content as string
  return null
}
