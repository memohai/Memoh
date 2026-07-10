import { reactive, type Ref } from 'vue'
import type {
  ChatAttachment,
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
  isSameLogicalTurn,
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
}

export interface TranscriptDeps {
  currentBotId: Ref<string | null>
  sessionId: Ref<string | null>
  rememberBackgroundTask: (task: BackgroundTask) => BackgroundTask
  applyPendingBackgroundEventsToTool: (block: ToolCallBlock) => void
  bumpFsChangedAtIfFsMutation: (message: UIMessage) => void
}

type SnapshotHook = (targetSessionId: string | undefined, turns: UITurn[]) => void

// Owns the single active transcript view and every mutation of that view.
// Streams for inactive sessions may keep mutating their detached turn objects,
// but only this controller can add, remove, reconcile, or reorder visible turns.
export function createTranscriptController({
  currentBotId,
  sessionId,
  rememberBackgroundTask,
  applyPendingBackgroundEventsToTool,
  bumpFsChangedAtIfFsMutation,
}: TranscriptDeps) {
  const messages = reactive<ChatMessage[]>([])
  const ephemeralAssistantErrors = new Map<string, EphemeralAssistantError[]>()
  let onSnapshot: SnapshotHook = () => {}

  function setSnapshotHook(hook: SnapshotHook) {
    onSnapshot = hook
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
      messages: [{ id: 0, type: 'error', content: error.content }],
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
      if (!text || hasAssistantError(items, text)) continue
      const anchorIndex = findAnchorUserIndex(items, error)
      const assistantTurn = anchorIndex >= 0
        ? findAssistantAfterAnchor(items, anchorIndex)
        : findAssistantTurnForEphemeralError(items, error.timestamp)
      if (assistantTurn) {
        assistantTurn.messages.push({ id: nextAssistantMessageId(assistantTurn), type: 'error', content: text })
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
    onSnapshot(targetSessionId, items)
    const next = normalizeTurns(items, targetSessionId)
    adoptRenderIdentity(next)
    messages.splice(0, messages.length, ...next)
  }

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
    messages.splice(0, messages.length, ...sortChatMessages([...merged.values()]))
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

  function appendToView(...turns: ChatMessage[]) {
    messages.push(...turns)
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
      attachments: (attachments ?? []).map(attachment => ({
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
          snapshots.push({ block, approval: cloneToolApprovalState(block.approval) })
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
          snapshots.push({ block, userInput: cloneUserInputState(block.userInput) })
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

  function rememberAssistantError(errorMessage: string, targetSessionId: string, assistantTurn: ChatAssistantTurn) {
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

  // Stream errors are not persisted server-side. Keep a small session-scoped
  // replay set so a terminal REST refresh cannot make a visible failure vanish.
  function appendAssistantError(assistantTurn: ChatAssistantTurn, targetSessionId: string, errorMessage: string) {
    const text = errorMessage.trim()
    if (!text) return
    rememberAssistantError(text, targetSessionId, assistantTurn)
    assistantTurn.messages.push({ id: nextAssistantMessageId(assistantTurn), type: 'error', content: text })
  }

  function finalizeStreamFailure(assistantTurn: ChatAssistantTurn, botId: string, targetSessionId: string, error: Error) {
    if (!hasVisibleAssistantBlocks(assistantTurn)) {
      removeTurnFromSession(botId, targetSessionId, assistantTurn)
      return
    }
    if (error.name === 'AbortError') return
    if (assistantTurn.messages.some(block => block.type === 'error')) return
    appendAssistantError(assistantTurn, targetSessionId, error.message)
  }

  function latestOptimisticUserText(): string {
    for (let i = messages.length - 1; i >= 0; i -= 1) {
      const message = messages[i]
      if (message?.role === 'user') return message.text.trim()
    }
    return ''
  }

  function markToolApprovalDecision(approvalId: string, status: 'approved' | 'rejected' | 'pending') {
    const id = approvalId.trim()
    if (!id) return
    for (const message of messages) {
      if (message.role !== 'assistant') continue
      for (const block of message.messages) {
        if (block.type !== 'tool' || block.approval?.approval_id !== id) continue
        block.approval = { ...block.approval, status, can_approve: status === 'pending' }
      }
    }
  }

  function resetUserScope() {
    replaceMessages([])
    ephemeralAssistantErrors.clear()
  }

  return {
    messages,
    setSnapshotHook,
    normalizeUIMessage,
    normalizeTurn,
    normalizeTurns,
    replaceMessages,
    mergeMessages,
    isActiveSessionTarget,
    appendTurnToSession,
    appendToView,
    prependToView,
    removeFromView,
    removeTurnFromSession,
    replaceTailFromTurn,
    restoreTailFromOptimistic,
    createOptimisticAssistantTurn,
    createOptimisticUserTurn,
    upsertAssistantUIMessage,
    hasVisibleAssistantBlocks,
    snapshotToolApprovalStates,
    restoreToolApprovalStates,
    snapshotUserInputStates,
    restoreUserInputStates,
    finalizeStreamFailure,
    latestOptimisticUserText,
    markToolApprovalDecision,
    resetUserScope,
  }
}
