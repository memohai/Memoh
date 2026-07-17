import { reactive, ref, toRaw, type Ref } from 'vue'
import type {
  ChatAttachment,
  FetchMessagesOptions,
  UIMessage,
  UISystemTurn,
  UIToolApproval,
  UIUserInput,
  UITurn,
} from '@/composables/api/useChat.types'
import {
  cloneToolApprovalState,
  cloneUserInputState,
  isOptimisticTurn,
  hasSameTurnIdentity,
  mergeApprovalState,
  nextId,
  normalizeAttachment,
  normalizeForwardRef,
  normalizeReplyRef,
  normalizeTimestamp,
  resolveIsSelf,
  serverMessageId,
  skillActivationTextFromRaw,
  sortChatMessages,
} from '../chat-list.normalize'
import { upsertById } from '../chat-list.utils'
import {
  isBackgroundTaskActive,
  normalizeBackgroundTask,
  reconcileBackgroundTasksInMessages,
} from './background-tasks'
import type {
  BackgroundTask,
  ChatAssistantTurn,
  ChatMessage,
  ChatMessageSyncState,
  ChatUserTurn,
  ContentBlock,
  ToolCallBlock,
} from './types'

interface UserInputStateSnapshot {
  block: ToolCallBlock
  userInput: UIUserInput
}

interface ToolApprovalStateSnapshot {
  block: ToolCallBlock
  approval: UIToolApproval
}

interface EphemeralAssistantError {
  content: string
  timestamp: string
  userText?: string
  userId?: string
  userServerId?: string
  externalMessageId?: string
  externalMessageOrdinal?: number
  standalone?: boolean
  runtimeStreamId?: string
  runtimeGeneration?: string
}

export interface RuntimeAssistantErrorIdentity {
  streamId: string
  generation: string
}

export interface TranscriptDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  rememberBackgroundTask: (task: BackgroundTask) => BackgroundTask
  applyPendingBackgroundEventsToTool: (block: ToolCallBlock) => void
  mergeBackgroundTaskIntoMatchingTools: (task: BackgroundTask, messages: ChatMessage[]) => void
  bumpFsChangedAtIfFsMutation: (message: UIMessage) => void
  fetchMessages: (botId: string, sessionId: string, options?: FetchMessagesOptions) => Promise<UITurn[]>
  locateMessage: (botId: string, sessionId: string, externalMessageId: string, before?: number, after?: number) => Promise<LocateMessageResult>
}

type SnapshotHook = (targetSessionId: string | undefined, turns: UITurn[]) => void
type RefreshAppliedHook = (botId: string, targetSessionId: string, latestTimestamp?: string) => void

export interface LocateMessageResult {
  items: UITurn[]
  target_id?: string
}

// Owns the single active transcript view and every mutation of that view.
// Streams for inactive sessions may keep mutating their detached turn objects,
// but only this controller can add, remove, reconcile, or reorder visible turns.
export function createTranscriptController({
  currentBotId,
  sessionId,
  rememberBackgroundTask,
  applyPendingBackgroundEventsToTool,
  mergeBackgroundTaskIntoMatchingTools,
  bumpFsChangedAtIfFsMutation,
  fetchMessages,
  locateMessage,
}: TranscriptDeps) {
  const messages = reactive<ChatMessage[]>([])
  const runtimeAssistantErrorIdentity = new WeakMap<ChatAssistantTurn, string>()
  const loadingMessages = ref(false)
  const loadingOlder = ref(false)
  const hasMoreOlder = ref(true)
  const hasLoadedOlder = ref(false)
  const ephemeralAssistantErrors = new Map<string, EphemeralAssistantError[]>()
  let onSnapshot: SnapshotHook = () => {}
  let onRefreshApplied: RefreshAppliedHook = () => {}
  let refreshPromise: { key: string; promise: Promise<void> } | null = null
  let initialMessagesLoad: {
    botId: string
    sessionId: string
    generation: number
    version: number
    turns: Promise<UITurn[]>
  } | null = null
  let historyGeneration = 0
  let loadingMessagesVersion = 0
  let loadingOlderVersion = 0

  function setSnapshotHook(hook: SnapshotHook) {
    onSnapshot = hook
  }

  function setRefreshAppliedHook(hook: RefreshAppliedHook) {
    onRefreshApplied = hook
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
        turnPosition: turn.turn_position,
        turnMessageSeq: turn.turn_message_seq,
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
        syncState: { run: 'completed', presence: 'settled', persistence: 'persisted' },
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
        turnPosition: turn.turn_position,
        turnMessageSeq: turn.turn_message_seq,
        role: 'system',
        kind: 'background_task',
        backgroundTask: latest,
        timestamp: normalizeTimestamp(turn.timestamp),
        platform: (turn.platform ?? '').trim() || undefined,
        streaming: false,
        syncState: { run: 'completed', presence: 'settled', persistence: 'persisted' },
      }
    }

    return {
      id: String(turn.id ?? nextId()),
      turnPosition: turn.turn_position,
      turnMessageSeq: turn.turn_message_seq,
      role: 'assistant',
      messages: (turn.messages ?? []).map(normalizeUIMessage),
      timestamp: normalizeTimestamp(turn.timestamp),
      platform: (turn.platform ?? '').trim() || undefined,
      externalMessageId: (turn.external_message_id ?? '').trim() || undefined,
      streaming: false,
      syncState: { run: 'completed', presence: 'settled', persistence: 'persisted' },
    }
  }

  function ephemeralErrorId(sessionID: string, error: EphemeralAssistantError): string {
    let hash = 0
    const input = [
      error.timestamp,
      error.content,
      error.runtimeStreamId ?? '',
      error.runtimeGeneration ?? '',
    ].join('\u0000')
    for (let i = 0; i < input.length; i += 1) {
      hash = ((hash << 5) - hash + input.charCodeAt(i)) | 0
    }
    return `ephemeral-error-${sessionID}-${Math.abs(hash).toString(36)}`
  }

  function runtimeErrorIdentityKey(identity: RuntimeAssistantErrorIdentity | EphemeralAssistantError | undefined): string {
    const streamId = ('streamId' in (identity ?? {})
      ? (identity as RuntimeAssistantErrorIdentity).streamId
      : (identity as EphemeralAssistantError | undefined)?.runtimeStreamId)?.trim() ?? ''
    const generation = ('generation' in (identity ?? {})
      ? (identity as RuntimeAssistantErrorIdentity).generation
      : (identity as EphemeralAssistantError | undefined)?.runtimeGeneration)?.trim() ?? ''
    return streamId && generation ? `${streamId}\u0000${generation}` : ''
  }

  function associateRuntimeError(turn: ChatAssistantTurn, error: EphemeralAssistantError) {
    const key = runtimeErrorIdentityKey(error)
    if (key) runtimeAssistantErrorIdentity.set(toRaw(turn), key)
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
      if (!Number.isNaN(errorTime) && !Number.isNaN(itemTime) && itemTime > errorTime) break
      if (item.role === 'user') {
        target = null
      } else if (item.role === 'assistant') {
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
    const targetUserId = (error.userId ?? '').trim()
    const targetUserServerId = (error.userServerId ?? '').trim()
    const targetExternalMessageId = (error.externalMessageId ?? error.runtimeStreamId ?? '').trim()
    const targetExternalMessageOrdinal = error.externalMessageOrdinal ?? 0
    if (targetExternalMessageId && targetExternalMessageOrdinal > 0) {
      let ordinal = 0
      for (let i = 0; i < items.length; i += 1) {
        const item = items[i]
        if (item?.role !== 'user' || item.externalMessageId !== targetExternalMessageId) continue
        ordinal += 1
        if (ordinal === targetExternalMessageOrdinal) return i
      }
    }
    let fallback = -1
    for (let i = items.length - 1; i >= 0; i -= 1) {
      const item = items[i]
      if (item?.role !== 'user') continue
      if (fallback < 0) fallback = i
      if (targetUserServerId && (item.serverId === targetUserServerId || item.id === targetUserServerId)) return i
      if (targetUserId && item.id === targetUserId) return i
      if (targetExternalMessageId && item.externalMessageId === targetExternalMessageId) return i
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
    const turn: ChatAssistantTurn = {
      id: ephemeralErrorId(sessionID, error),
      role: 'assistant',
      messages: [{ id: 0, type: 'error', content: error.content }],
      timestamp,
      streaming: false,
      __ephemeral: true,
    }
    associateRuntimeError(turn, error)
    return turn
  }

  function appendEphemeralErrors(items: ChatMessage[], targetSessionId?: string) {
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!sid) return
    const errors = ephemeralAssistantErrors.get(sid)
    if (!errors?.length) return
    for (const error of errors) {
      const text = error.content.trim()
      const runtimeKey = runtimeErrorIdentityKey(error)
      if (!text || (!runtimeKey && hasAssistantError(items, text))) continue
      const anchorIndex = findAnchorUserIndex(items, error)
      const assistantTurn = anchorIndex >= 0
        ? findAssistantAfterAnchor(items, anchorIndex)
        : findAssistantTurnForEphemeralError(items, error.timestamp)
      if (assistantTurn?.messages.some(block => block.type === 'error' && block.content === text)) {
        associateRuntimeError(assistantTurn, error)
        continue
      }
      if (
        runtimeKey
        && assistantTurn
        && runtimeAssistantErrorIdentity.get(toRaw(assistantTurn)) === runtimeKey
        && assistantTurn.messages.some(block => block.type === 'error' && block.content === text)
      ) continue
      if (error.standalone) {
        const assistantIndex = assistantTurn ? items.indexOf(assistantTurn) : -1
        const insertAt = assistantIndex >= 0 ? assistantIndex + 1 : anchorIndex >= 0 ? anchorIndex + 1 : items.length
        items.splice(insertAt, 0, createEphemeralErrorTurn(sid, { ...error, content: text }))
        continue
      }
      if (assistantTurn) {
        assistantTurn.messages.push({ id: nextAssistantMessageId(assistantTurn), type: 'error', content: text })
        associateRuntimeError(assistantTurn, error)
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

  // Preserve client render keys across REST snapshots. Server ids remain in
  // serverId so Vue does not remount the just-streamed tail and break scroll pinning.
  function isUnsettledTurn(turn: ChatMessage) {
    return isOptimisticTurn(turn) || turn.syncState?.presence === 'live'
  }

  function adoptRenderIdentity(incoming: ChatMessage[]) {
    if (messages.length === 0 || incoming.length === 0) return
    const adopted = new Set<ChatMessage>()
    const adopt = (twin: ChatMessage, existing: ChatMessage) => {
      adopted.add(twin)
      if (
        existing.syncState?.presence === 'settled'
        && existing.syncState.persistence === 'persisted'
      ) {
        twin.syncState = { ...existing.syncState }
      }
      if (twin.id === existing.id) return
      twin.serverId = twin.serverId ?? twin.id
      twin.id = existing.id
    }
    const byServerId = new Map<string, ChatMessage>()
    for (const existing of messages) {
      const id = serverMessageId(existing)
      if (id) byServerId.set(id, existing)
    }
    for (const twin of incoming) {
      const prior = byServerId.get(twin.serverId ?? twin.id)
      if (prior) adopt(twin, prior)
    }
    for (let i = 0; i < messages.length; i += 1) {
      const existing = messages[i]
      if (!existing || existing.role !== 'user' || !isUnsettledTurn(existing)) continue
      const twinIndex = incoming.findIndex(turn => !adopted.has(turn) && hasSameTurnIdentity(existing, turn))
      if (twinIndex === -1) continue
      adopt(incoming[twinIndex]!, existing)
      const existingNext = messages[i + 1]
      const incomingNext = incoming[twinIndex + 1]
      if (
        existingNext?.role === 'assistant' && isUnsettledTurn(existingNext)
        && incomingNext?.role === 'assistant' && !adopted.has(incomingNext)
        && hasSameTurnIdentity(existingNext, incomingNext)
      ) {
        adopt(incomingNext, existingNext)
      }
    }
  }

  function replaceMessages(items: UITurn[], targetSessionId?: string, options: { preserveOptimistic?: boolean } = {}) {
    onSnapshot(targetSessionId, items)
    const next = normalizeTurns(items, targetSessionId)
    adoptRenderIdentity(next)
    if (options.preserveOptimistic) {
      // A refresh snapshot can race the server persisting a just-sent turn:
      // the first-send history fetch resolves after the optimistic user turn
      // was appended but before the server has stored it, so the snapshot
      // comes back without it. Carry unmatched optimistic USER turns over —
      // assistants are excluded because the live stream re-attaches its own
      // turn (reattachTurnToSession) and carrying it here would duplicate it
      // against a persisted twin. Gated on an in-flight send (a streaming
      // optimistic assistant): once the stream settled, the snapshot is
      // authoritative and failed sends must not resurrect their turns.
      const sendInFlight = messages.some(turn =>
        isOptimisticTurn(turn) && turn.role === 'assistant' && turn.streaming)
      if (sendInFlight) {
        const orphans = messages.filter(turn => isOptimisticTurn(turn)
          && turn.role === 'user'
          && !next.some(candidate => hasSameTurnIdentity(turn, candidate)))
        messages.splice(0, messages.length, ...next, ...orphans)
        return
      }
    }
    messages.splice(0, messages.length, ...next)
  }

  function mergeMessages(items: UITurn[], targetSessionId?: string) {
    const incoming = normalizeTurns(items, targetSessionId)
    adoptRenderIdentity(incoming)
    const matched = new Set<string>()
    for (let i = 0; i < messages.length; i += 1) {
      const unsettled = messages[i]
      if (!unsettled || !isUnsettledTurn(unsettled)) continue
      const replacement = incoming.find(turn => !matched.has(turn.id) && hasSameTurnIdentity(unsettled, turn))
      if (replacement) {
        messages[i] = replacement
        matched.add(replacement.id)
      }
    }
    const merged = new Map<string, ChatMessage>()
    for (const item of messages) merged.set(item.id, item)
    for (const item of incoming) merged.set(item.id, item)
    messages.splice(0, messages.length, ...sortChatMessages([...merged.values()]))
  }

  // A terminal runtime handoff must invalidate any older hydration that is
  // still in flight, while merging only the one authoritative persisted turn.
  function mergePersistedTurn(items: UITurn[], targetSessionId?: string) {
    historyGeneration += 1
    onSnapshot(targetSessionId, items)
    mergeMessages(items, targetSessionId)
  }

  function replacePersistedWindow(items: UITurn[], targetSessionId?: string) {
    historyGeneration += 1
    onSnapshot(targetSessionId, items)
    const incoming = sortChatMessages(normalizeTurns(items, targetSessionId))
    adoptRenderIdentity(incoming)
    const retained = messages.filter(turn => {
      if (turn.syncState?.persistence === 'persisted') return false
      return !incoming.some(candidate => hasSameTurnIdentity(turn, candidate))
    })
    const retainedSet = new Set(retained)
    const slots = Array.from({ length: incoming.length + 1 }, () => [] as ChatMessage[])
    const incomingIndexFor = (turn: ChatMessage) => incoming.findIndex(candidate => hasSameTurnIdentity(turn, candidate))

    for (let index = 0; index < messages.length; index += 1) {
      const turn = messages[index]
      if (!turn || !retainedSet.has(turn)) continue
      let slot = -1
      for (let previous = index - 1; previous >= 0; previous -= 1) {
        const anchor = messages[previous]
        if (!anchor || anchor.syncState?.persistence !== 'persisted') continue
        const incomingIndex = incomingIndexFor(anchor)
        if (incomingIndex >= 0) {
          slot = incomingIndex + 1
          break
        }
      }
      if (slot < 0) {
        for (let next = index + 1; next < messages.length; next += 1) {
          const anchor = messages[next]
          if (!anchor || anchor.syncState?.persistence !== 'persisted') continue
          const incomingIndex = incomingIndexFor(anchor)
          if (incomingIndex >= 0) {
            slot = incomingIndex
            break
          }
        }
      }
      slots[slot < 0 ? incoming.length : slot]!.push(turn)
    }

    const reconciled = [...slots[0]!]
    for (let index = 0; index < incoming.length; index += 1) {
      reconciled.push(incoming[index]!, ...slots[index + 1]!)
    }
    messages.splice(0, messages.length, ...reconciled)
  }

  // Applies a bounded authoritative history response only when both views
  // contain the requested durable anchor. This lets callers fall back to a
  // full reset without first destroying a still-valid local transcript.
  function replacePersistedSuffix(items: UITurn[], anchorMessageId: string, targetSessionId?: string): boolean {
    const anchorId = anchorMessageId.trim()
    if (!anchorId) return false

    const localAnchorIndex = messages.findIndex(turn =>
      turn.syncState?.persistence === 'persisted' && serverMessageId(turn) === anchorId)
    if (localAnchorIndex < 0) return false
    if (!items.some(turn => String(turn.id ?? '').trim() === anchorId)) return false

    const normalized = sortChatMessages(normalizeTurns(items, targetSessionId))
    const incomingAnchorIndex = normalized.findIndex(turn => serverMessageId(turn) === anchorId)
    if (incomingAnchorIndex < 0) return false

    const incoming = normalized.slice(incomingAnchorIndex)
    historyGeneration += 1
    onSnapshot(targetSessionId, items)
    adoptRenderIdentity(incoming)

    const prefix = messages.slice(0, localAnchorIndex)
    const localSuffix = messages.slice(localAnchorIndex)
    const retained = localSuffix.filter(turn =>
      isUnsettledTurn(turn) && !incoming.some(candidate => hasSameTurnIdentity(turn, candidate)))
    const retainedSet = new Set(retained)
    const slots = Array.from({ length: incoming.length + 1 }, () => [] as ChatMessage[])
    const incomingIndexFor = (turn: ChatMessage) => incoming.findIndex(candidate =>
      hasSameTurnIdentity(turn, candidate))

    for (let index = 0; index < localSuffix.length; index += 1) {
      const turn = localSuffix[index]
      if (!turn || !retainedSet.has(turn)) continue
      let slot = -1
      for (let previous = index - 1; previous >= 0; previous -= 1) {
        const anchor = localSuffix[previous]
        if (!anchor) continue
        const incomingIndex = incomingIndexFor(anchor)
        if (incomingIndex >= 0) {
          slot = incomingIndex + 1
          break
        }
      }
      if (slot < 0) {
        for (let next = index + 1; next < localSuffix.length; next += 1) {
          const anchor = localSuffix[next]
          if (!anchor) continue
          const incomingIndex = incomingIndexFor(anchor)
          if (incomingIndex >= 0) {
            slot = incomingIndex
            break
          }
        }
      }
      slots[slot < 0 ? incoming.length : slot]!.push(turn)
    }

    const reconciled = [...prefix, ...slots[0]!]
    for (let index = 0; index < incoming.length; index += 1) {
      reconciled.push(incoming[index]!, ...slots[index + 1]!)
    }
    messages.splice(0, messages.length, ...reconciled)
    return true
  }

  const PAGE_SIZE = 30

  function isCurrentHistoryContext(botId: string, targetSessionId: string, generation: number): boolean {
    return generation === historyGeneration && isActiveSessionTarget(botId, targetSessionId)
  }

  function clearHistoryView(options: { hasMoreOlder?: boolean } = {}) {
    historyGeneration += 1
    loadingMessagesVersion += 1
    loadingOlderVersion += 1
    refreshPromise = null
    initialMessagesLoad = null
    replaceMessages([])
    hasMoreOlder.value = options.hasMoreOlder === true
    hasLoadedOlder.value = false
    loadingMessages.value = false
    loadingOlder.value = false
  }

  function prepareForInitialization() {
    historyGeneration += 1
    loadingMessagesVersion += 1
    loadingOlderVersion += 1
    refreshPromise = null
    initialMessagesLoad = null
    hasLoadedOlder.value = false
    loadingMessages.value = false
    loadingOlder.value = false
  }

  function markHistoryEmpty() {
    hasMoreOlder.value = false
    hasLoadedOlder.value = false
  }

  function replaceHistoryView(items: UITurn[], targetSessionId: string) {
    historyGeneration += 1
    loadingOlderVersion += 1
    refreshPromise = null
    initialMessagesLoad = null
    replaceMessages(items, targetSessionId)
    hasMoreOlder.value = true
    hasLoadedOlder.value = false
    loadingOlder.value = false
  }

  async function refreshCurrentSession(
    targetBotId?: string,
    targetSessionId?: string,
    options: { afterCurrent?: boolean } = {},
  ) {
    const bid = (targetBotId ?? currentBotId.value ?? '').trim()
    const sid = (targetSessionId ?? sessionId.value ?? '').trim()
    if (!bid || !sid) return
    const key = `${bid}:${sid}`
    const generation = historyGeneration
    let waitedForCurrentKey = false
    for (;;) {
      const currentRefresh = refreshPromise
      if (!currentRefresh) break
      try {
        await currentRefresh.promise
      } catch (error) {
        if (
          currentRefresh.key !== key
          || !options.afterCurrent
          || waitedForCurrentKey
        ) {
          throw error
        }
      }
      if (!isCurrentHistoryContext(bid, sid, generation)) return
      if (currentRefresh.key !== key) continue
      if (!options.afterCurrent || waitedForCurrentKey) return
      waitedForCurrentKey = true
    }

    const promise = (async () => {
      const turns = await fetchMessages(bid, sid, { limit: PAGE_SIZE })
      if (!isCurrentHistoryContext(bid, sid, generation)) return
      if (hasLoadedOlder.value) {
        mergeMessages(turns, sid)
      } else {
        // preserveOptimistic: this refresh races the server persisting a
        // just-sent turn (first send from a draft resolves history while the
        // user message is still only optimistic locally) — a plain replace
        // would blank the user's own message until the next refresh.
        replaceMessages(turns, sid, { preserveOptimistic: true })
        // The API pages raw DB rows but returns merged UI turns, so a short
        // page is not proof that history ended. Only pagination can settle it.
        hasMoreOlder.value = true
      }
      onRefreshApplied(bid, sid, messages[messages.length - 1]?.timestamp)
    })().finally(() => {
      if (refreshPromise?.promise === promise) refreshPromise = null
    })
    refreshPromise = { key, promise }
    await promise
  }

  async function loadInitialMessages(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return
    loadingMessages.value = true
    const version = ++loadingMessagesVersion
    try {
      await refreshCurrentSession(bid, sid)
    } finally {
      if (version === loadingMessagesVersion) loadingMessages.value = false
    }
  }

  function beginInitialMessagesLoad(botId: string, targetSessionId: string) {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (!bid || !sid) return null
    if (
      initialMessagesLoad
      && initialMessagesLoad.botId === bid
      && initialMessagesLoad.sessionId === sid
      && initialMessagesLoad.generation === historyGeneration
    ) return initialMessagesLoad
    loadingMessages.value = true
    const version = ++loadingMessagesVersion
    const generation = historyGeneration
    initialMessagesLoad = {
      botId: bid,
      sessionId: sid,
      generation,
      version,
      turns: fetchMessages(bid, sid, { limit: PAGE_SIZE }),
    }
    return initialMessagesLoad
  }

  function applyInitialMessagesLoad(load: NonNullable<ReturnType<typeof beginInitialMessagesLoad>>, turns: UITurn[]) {
    if (!isCurrentHistoryContext(load.botId, load.sessionId, load.generation)) return false
    if (hasLoadedOlder.value) {
      mergeMessages(turns, load.sessionId)
    } else {
      replaceMessages(turns, load.sessionId, { preserveOptimistic: true })
      hasMoreOlder.value = true
    }
    onRefreshApplied(load.botId, load.sessionId, messages[messages.length - 1]?.timestamp)
    return true
  }

  function finishInitialMessagesLoad(load: NonNullable<ReturnType<typeof beginInitialMessagesLoad>>) {
    if (initialMessagesLoad === load) initialMessagesLoad = null
    if (load.version === loadingMessagesVersion) loadingMessages.value = false
  }

  function fetchSessionWindow(botId: string, targetSessionId: string): Promise<UITurn[]> {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    if (
      initialMessagesLoad
      && initialMessagesLoad.botId === bid
      && initialMessagesLoad.sessionId === sid
      && initialMessagesLoad.generation === historyGeneration
    ) return initialMessagesLoad.turns
    return fetchMessages(bid, sid, { limit: PAGE_SIZE })
  }

  async function loadOlderMessages(): Promise<number> {
    const bid = (currentBotId.value ?? '').trim()
    const sid = (sessionId.value ?? '').trim()
    if (!bid || !sid || loadingOlder.value || !hasMoreOlder.value) return 0
    const firstId = serverMessageId(messages[0])
    if (!firstId) return 0

    const generation = historyGeneration
    const version = ++loadingOlderVersion
    loadingOlder.value = true
    try {
      const maxDedupHops = 4
      let cursor = firstId
      for (let hop = 0; hop < maxDedupHops; hop++) {
        const turns = await fetchMessages(bid, sid, { limit: PAGE_SIZE, beforeMessageId: cursor })
        if (!isCurrentHistoryContext(bid, sid, generation)) return 0
        if (turns.length === 0) {
          hasMoreOlder.value = false
          return 0
        }

        const existingIds = new Set(messages.map(message => message.id))
        const normalized = normalizeTurns(turns, sid)
        const older = normalized.filter(turn => !existingIds.has(turn.id))
        if (older.length > 0) {
          prependToView(...older)
          hasLoadedOlder.value = true
          return older.length
        }

        const earliest = normalized[0] ? serverMessageId(normalized[0]) : ''
        if (!earliest || earliest === cursor) {
          hasMoreOlder.value = false
          return 0
        }
        cursor = earliest
      }
      hasMoreOlder.value = false
      return 0
    } catch (error) {
      console.error('Failed to load older messages:', error)
      return 0
    } finally {
      if (version === loadingOlderVersion) loadingOlder.value = false
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

    const bid = (currentBotId.value ?? '').trim()
    const sid = (sessionId.value ?? '').trim()
    const target = externalMessageId.trim()
    if (!bid || !sid || !target) return null
    const generation = historyGeneration

    try {
      const result = await locateMessage(bid, sid, target, PAGE_SIZE, PAGE_SIZE)
      if (!isCurrentHistoryContext(bid, sid, generation) || !result.items.length) return null
      mergeMessages(result.items, sid)
      hasMoreOlder.value = true
      hasLoadedOlder.value = true
      return result.target_id?.trim() || findMessageIdByExternalId(target)
    } catch (error) {
      console.error('Failed to locate message:', error)
      return null
    }
  }

  function isActiveSessionTarget(botId: string, targetSessionId: string): boolean {
    const bid = botId.trim()
    const sid = targetSessionId.trim()
    return Boolean(bid && sid && currentBotId.value === bid && sessionId.value === sid)
  }

  // Context-gated operations prevent a late stream or rollback for session A
  // from writing into the visible transcript after the user switches to B.
  function appendTurnToSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    if (isActiveSessionTarget(botId, targetSessionId)) messages.push(turn)
  }

  function reattachTurnToSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    if (!isActiveSessionTarget(botId, targetSessionId)) return
    if (messages.includes(turn)) return
    const adoptedIndex = messages.findIndex(message => message.id === turn.id)
    if (adoptedIndex >= 0) {
      messages.splice(adoptedIndex, 1, turn)
      return
    }
    const tailIndex = messages.length - 1
    const hydratedTail = messages[tailIndex]
    if (hydratedTail?.role === 'assistant' && !hydratedTail.streaming) {
      messages.splice(tailIndex, 1, turn)
      return
    }
    messages.push(turn)
  }

  function appendToView(...turns: ChatMessage[]) {
    messages.push(...turns)
  }

  function insertTurnAt(index: number, turn: ChatMessage) {
    messages.splice(Math.max(0, Math.min(index, messages.length)), 0, turn)
  }

  function replaceTurnAt(index: number, turn: ChatMessage) {
    if (index < 0 || index >= messages.length) return false
    messages.splice(index, 1, turn)
    return true
  }

  function prependToView(...turns: ChatMessage[]) {
    messages.unshift(...turns)
  }

  function removeFromView(turn: ChatMessage) {
    const idx = messages.indexOf(turn)
    if (idx >= 0) messages.splice(idx, 1)
  }

  function removeTurnFromSession(botId: string, targetSessionId: string, turn: ChatMessage) {
    if (botId.trim() && targetSessionId.trim() && !isActiveSessionTarget(botId, targetSessionId)) return
    removeFromView(turn)
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
      appendToView(...replacements)
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
      messages.splice(idx, optimisticUserTurn ? 2 : 1, ...replacedTurns)
      return
    }
    if (optimisticUserTurn) removeTurnFromSession(botId, targetSessionId, optimisticUserTurn)
    removeTurnFromSession(botId, targetSessionId, assistantTurn)
    if (replacedTurns.length > 0) appendToView(...replacedTurns)
  }

  function createOptimisticAssistantTurn(id = nextId()): ChatAssistantTurn {
    return {
      id,
      role: 'assistant',
      messages: [],
      timestamp: new Date().toISOString(),
      streaming: true,
      __optimistic: true,
      syncState: { run: 'admitting', presence: 'optimistic', persistence: 'unknown' },
    }
  }

  function createOptimisticUserTurn(text: string, attachments?: ChatAttachment[], externalMessageId?: string): ChatUserTurn {
    return {
      id: nextId(),
      role: 'user',
      text,
      attachments: (attachments ?? []).map(attachment => ({
        type: attachment.type,
        base64: attachment.base64,
        name: attachment.name ?? '',
        mime: attachment.mime ?? '',
      })),
      timestamp: new Date().toISOString(),
      externalMessageId: externalMessageId?.trim() || undefined,
      streaming: false,
      isSelf: true,
      __optimistic: true,
      syncState: {
        run: 'admitting',
        presence: 'optimistic',
        persistence: 'unknown',
        streamId: externalMessageId?.trim() || undefined,
      },
    }
  }

  function setMessageSyncState(turn: ChatMessage, state: ChatMessageSyncState) {
    turn.syncState = {
      ...state,
      streamId: state.streamId?.trim() || undefined,
      generation: state.generation?.trim() || undefined,
    }
    if (state.presence !== 'optimistic') turn.__optimistic = false
  }

  function adoptRuntimeUserTurn(existing: ChatUserTurn, canonical: ChatUserTurn) {
    const id = existing.id
    const serverId = existing.serverId ?? canonical.serverId
    const isSelf = existing.isSelf || canonical.isSelf
    Object.assign(existing, canonical, { id, serverId, isSelf, __optimistic: false })
  }

  function setAssistantRowIdentity(
    turn: ChatAssistantTurn,
    identity: { stableId?: string, turnPosition?: number, turnMessageSeq?: number },
  ) {
    turn.serverId = identity.stableId?.trim() || turn.serverId
    turn.turnPosition = identity.turnPosition
    turn.turnMessageSeq = identity.turnMessageSeq
  }

  function setAssistantStreaming(turn: ChatAssistantTurn, streaming: boolean) {
    turn.streaming = streaming
  }

  function clearAssistantMessages(turn: ChatAssistantTurn) {
    turn.messages = []
  }

  function appendAssistantContent(
    turn: ChatAssistantTurn,
    id: number,
    type: 'text' | 'reasoning',
    content: string,
  ): boolean {
    const block = turn.messages.find(message => message.id === id && message.type === type)
    if (!block || (block.type !== 'text' && block.type !== 'reasoning')) return false
    block.content += content
    return true
  }

  function appendToolProgress(
    turn: ChatAssistantTurn,
    id: number,
    progress: unknown,
    input: { present: boolean, value?: unknown },
  ): boolean {
    const block = turn.messages.find((message): message is ToolCallBlock =>
      message.id === id && message.type === 'tool')
    if (!block) return false
    block.progress = [...(block.progress ?? []), progress]
    if (input.present) block.input = input.value
    return true
  }

  function applyBackgroundTask(task: BackgroundTask) {
    mergeBackgroundTaskIntoMatchingTools(rememberBackgroundTask(task), messages)
  }

  // Tool updates are partial snapshots. Preserve fields that an earlier stream
  // already filled, and never let a stale pending approval undo a local decision.
  function mergeToolCallBlock(existing: ToolCallBlock, incoming: ToolCallBlock) {
    Object.assign(existing, incoming, {
      id: existing.id,
      name: incoming.name || existing.name,
      toolName: incoming.toolName || existing.toolName,
      input: incoming.input ?? existing.input,
      result: incoming.result ?? existing.result,
      output: incoming.output ?? existing.output,
      approval: mergeApprovalState(existing.approval, incoming.approval),
      execution_location: incoming.execution_location ?? existing.execution_location,
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
      const existing = turn.messages.find((block): block is ToolCallBlock =>
        block.type === 'tool' && block.toolCallId === normalized.toolCallId,
      )
      if (existing) {
        mergeToolCallBlock(existing, normalized)
        bumpFsChangedAtIfFsMutation(message)
        return existing.id
      }
    }
    turn.messages = upsertById(turn.messages, normalized)
    bumpFsChangedAtIfFsMutation(message)
    return normalized.id
  }

  function replaceAssistantUIMessageSnapshot(
    turn: ChatAssistantTurn,
    incoming: UIMessage[],
    previousRuntimeMessageIds: ReadonlySet<number>,
  ): Set<number> {
    const incomingIds = new Set(incoming.map(message => upsertAssistantUIMessage(turn, message)))
    turn.messages = turn.messages.filter(block =>
      !previousRuntimeMessageIds.has(block.id) || incomingIds.has(block.id),
    )
    return incomingIds
  }

  function nextAssistantMessageId(turn: ChatAssistantTurn): number {
    return turn.messages.reduce((maxId, message) => Math.max(maxId, message.id), -1) + 1
  }

  function hasVisibleAssistantBlocks(turn: ChatAssistantTurn): boolean {
    return turn.messages.some(block => block.type !== 'error')
  }

  function finishAssistantTurn(turn: ChatAssistantTurn) {
    turn.streaming = false
  }

  function forEachToolBlock(visitor: (block: ToolCallBlock) => void) {
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type === 'tool') visitor(block)
      }
    }
  }

  function snapshotToolApprovalStates(approvalId: string): ToolApprovalStateSnapshot[] {
    const id = approvalId.trim()
    if (!id) return []
    const snapshots: ToolApprovalStateSnapshot[] = []
    forEachToolBlock((block) => {
      if (block.approval?.approval_id === id) {
        snapshots.push({ block, approval: cloneToolApprovalState(block.approval) })
      }
    })
    return snapshots
  }

  function assistantTurnForApproval(approvalId: string): ChatAssistantTurn | null {
    const id = approvalId.trim()
    if (!id) return null
    return messages.find((message): message is ChatAssistantTurn =>
      message.role === 'assistant'
      && message.messages.some(block => block.type === 'tool' && block.approval?.approval_id === id),
    ) ?? null
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
    forEachToolBlock((block) => {
      if (block.userInput?.user_input_id === id) {
        snapshots.push({ block, userInput: cloneUserInputState(block.userInput) })
      }
    })
    return snapshots
  }

  function assistantTurnForUserInput(userInputId: string): ChatAssistantTurn | null {
    const id = userInputId.trim()
    if (!id) return null
    return messages.find((message): message is ChatAssistantTurn =>
      message.role === 'assistant'
      && message.messages.some(block => block.type === 'tool' && block.userInput?.user_input_id === id),
    ) ?? null
  }

  function restoreUserInputStates(snapshots: UserInputStateSnapshot[]) {
    for (const snapshot of snapshots) {
      if (snapshot.block.userInput?.user_input_id !== snapshot.userInput.user_input_id) continue
      snapshot.block.userInput = cloneUserInputState(snapshot.userInput)
    }
  }

  function rememberAssistantError(
    errorMessage: string,
    targetSessionId: string,
    assistantTurn: ChatAssistantTurn,
    standalone = false,
    identity?: RuntimeAssistantErrorIdentity,
  ) {
    const sid = targetSessionId.trim()
    const text = errorMessage.trim()
    if (!sid || !text) return
    const current = ephemeralAssistantErrors.get(sid) ?? []
    const runtimeKey = runtimeErrorIdentityKey(identity)
    const existing = current.find((item) => {
      const itemRuntimeKey = runtimeErrorIdentityKey(item)
      return runtimeKey ? itemRuntimeKey === runtimeKey : !itemRuntimeKey && item.content === text
    })
    if (existing) {
      associateRuntimeError(assistantTurn, existing)
      return
    }
    const anchorUser = findUserTurnBeforeAssistant(assistantTurn)
    const externalMessageId = anchorUser?.externalMessageId?.trim() || undefined
    const anchorIndex = anchorUser ? messages.indexOf(anchorUser) : -1
    const externalMessageOrdinal = externalMessageId && anchorIndex >= 0
      ? messages.slice(0, anchorIndex + 1).filter(turn =>
          turn.role === 'user' && turn.externalMessageId === externalMessageId).length
      : undefined
    const error: EphemeralAssistantError = {
      content: text,
      timestamp: new Date().toISOString(),
      userText: anchorUser?.text.trim() || undefined,
      userId: anchorUser?.id.trim() || undefined,
      userServerId: anchorUser?.serverId?.trim() || undefined,
      externalMessageId,
      externalMessageOrdinal,
      standalone,
      runtimeStreamId: identity?.streamId.trim() || undefined,
      runtimeGeneration: identity?.generation.trim() || undefined,
    }
    ephemeralAssistantErrors.set(sid, [...current, error].slice(-5))
    associateRuntimeError(assistantTurn, error)
  }

  // Stream errors are not persisted server-side. Keep a small session-scoped
  // replay set so a terminal REST refresh cannot make a visible failure vanish.
  function appendAssistantError(
    assistantTurn: ChatAssistantTurn,
    targetSessionId: string,
    errorMessage: string,
    standalone = false,
    identity?: RuntimeAssistantErrorIdentity,
  ) {
    const text = errorMessage.trim()
    if (!text) return
    rememberAssistantError(text, targetSessionId, assistantTurn, standalone, identity)
    if (assistantTurn.messages.some(block => block.type === 'error' && block.content === text)) return
    assistantTurn.messages.push({ id: nextAssistantMessageId(assistantTurn), type: 'error', content: text })
  }

  function finalizeStreamFailure(
    assistantTurn: ChatAssistantTurn,
    botId: string,
    targetSessionId: string,
    error: Error,
    identity?: RuntimeAssistantErrorIdentity,
  ) {
    if (!hasVisibleAssistantBlocks(assistantTurn)) {
      removeTurnFromSession(botId, targetSessionId, assistantTurn)
      return
    }
    if (error.name === 'AbortError') return
    if (assistantTurn.messages.some(block => block.type === 'error')) return
    appendAssistantError(assistantTurn, targetSessionId, error.message, false, identity)
  }

  function assistantTurnForRuntimeError(targetSessionId: string, identity: RuntimeAssistantErrorIdentity): ChatAssistantTurn | null {
    if ((sessionId.value ?? '').trim() !== targetSessionId.trim()) return null
    const key = runtimeErrorIdentityKey(identity)
    if (!key) return null
    return messages.find((turn): turn is ChatAssistantTurn =>
      turn.role === 'assistant' && runtimeAssistantErrorIdentity.get(toRaw(turn)) === key,
    ) ?? null
  }

  function latestOptimisticUserText(): string {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      const message = messages[i]
      if (message?.role === 'user') return message.text.trim()
    }
    return ''
  }

  function hasTurn(turn: ChatMessage): boolean {
    return messages.includes(turn)
  }

  function findTurnByServerId(messageId: string): ChatMessage | null {
    const id = messageId.trim()
    if (!id) return null
    return messages.find(turn => serverMessageId(turn) === id) ?? null
  }

  function latestVisibleTurn(role: 'user'): ChatUserTurn | null
  function latestVisibleTurn(role: 'assistant'): ChatAssistantTurn | null
  function latestVisibleTurn(role: ChatMessage['role']): ChatUserTurn | ChatAssistantTurn | null {
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      const turn = messages[index]
      if (
        turn?.role === role
        && !turn.__optimistic
        && (turn.role !== 'assistant' || !turn.__ephemeral)
      ) return turn as ChatUserTurn | ChatAssistantTurn
    }
    return null
  }

  function isLatestVisibleUserTurn(turn: ChatMessage): turn is ChatUserTurn {
    if (turn.role !== 'user') return false
    const latest = latestVisibleTurn('user')
    return Boolean(latest && serverMessageId(latest) === serverMessageId(turn))
  }

  function isLatestVisibleAssistantTurn(turn: ChatMessage): turn is ChatAssistantTurn {
    if (turn.role !== 'assistant') return false
    const latest = latestVisibleTurn('assistant')
    return Boolean(latest && serverMessageId(latest) === serverMessageId(turn))
  }

  function markToolApprovalDecision(approvalId: string, status: 'approved' | 'rejected' | 'pending') {
    const id = approvalId.trim()
    if (!id) return
    forEachToolBlock((block) => {
      if (block.approval?.approval_id === id) {
        block.approval = { ...block.approval, status, can_approve: status === 'pending' }
      }
    })
  }

  function markUserInputDecision(userInputId: string, status: 'submitted' | 'canceled') {
    const id = userInputId.trim()
    if (!id) return
    forEachToolBlock((block) => {
      if (block.userInput?.user_input_id === id) {
        block.userInput = { ...block.userInput, status, can_respond: false }
      }
    })
  }

  function resetUserScope() {
    clearHistoryView({ hasMoreOlder: true })
    ephemeralAssistantErrors.clear()
  }

  return {
    messages,
    loadingMessages,
    loadingOlder,
    hasMoreOlder,
    hasLoadedOlder,
    setSnapshotHook,
    setRefreshAppliedHook,
    normalizeUIMessage,
    normalizeTurn,
    normalizeTurns,
    replaceMessages,
    mergeMessages,
    mergePersistedTurn,
    replacePersistedWindow,
    replacePersistedSuffix,
    clearHistoryView,
    prepareForInitialization,
    markHistoryEmpty,
    replaceHistoryView,
    refreshCurrentSession,
    loadInitialMessages,
    beginInitialMessagesLoad,
    applyInitialMessagesLoad,
    finishInitialMessagesLoad,
    fetchSessionWindow,
    loadOlderMessages,
    findMessageIdByExternalId,
    locateMessageByExternalId,
    isActiveSessionTarget,
    appendTurnToSession,
    reattachTurnToSession,
    appendToView,
    insertTurnAt,
    replaceTurnAt,
    prependToView,
    removeFromView,
    removeTurnFromSession,
    replaceTailFromTurn,
    restoreTailFromOptimistic,
    createOptimisticAssistantTurn,
    createOptimisticUserTurn,
    adoptRuntimeUserTurn,
    setAssistantRowIdentity,
    setMessageSyncState,
    setAssistantStreaming,
    clearAssistantMessages,
    appendAssistantContent,
    appendToolProgress,
    applyBackgroundTask,
    upsertAssistantUIMessage,
    replaceAssistantUIMessageSnapshot,
    hasVisibleAssistantBlocks,
    finishAssistantTurn,
    snapshotToolApprovalStates,
    assistantTurnForApproval,
    restoreToolApprovalStates,
    snapshotUserInputStates,
    assistantTurnForUserInput,
    restoreUserInputStates,
    appendAssistantError,
    finalizeStreamFailure,
    assistantTurnForRuntimeError,
    latestOptimisticUserText,
    hasTurn,
    findTurnByServerId,
    isLatestVisibleUserTurn,
    isLatestVisibleAssistantTurn,
    markToolApprovalDecision,
    markUserInputDecision,
    resetUserScope,
  }
}
