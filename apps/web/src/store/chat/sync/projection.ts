import { readonly, ref, type Ref } from 'vue'
import type { ConversationUiMessage } from '@memohai/sdk'
import type {
  UIAttachment,
  UIBackgroundTask,
  UIMessage,
  UIToolApproval,
  UIUserInput,
} from '@/composables/api/useChat.types'
import {
  createTranscriptController,
  type TranscriptDeps,
} from '../transcript'
import type { ChatMessage, ChatViewTarget } from '../types'

type TranscriptController = ReturnType<typeof createTranscriptController>

// This is the mutation surface owned by chat/sync. Keeping an explicit list
// prevents a new transcript container helper from silently becoming writable
// from the store or rendering layers.
export type SynchronizedTranscript = Pick<TranscriptController,
  | 'messages'
  | 'loadingMessages'
  | 'loadingOlder'
  | 'hasMoreOlder'
  | 'hasLoadedOlder'
  | 'setSnapshotHook'
  | 'setRefreshAppliedHook'
  | 'normalizeTurn'
  | 'mergePersistedTurn'
  | 'replacePersistedWindow'
  | 'replacePersistedSuffix'
  | 'clearHistoryView'
  | 'prepareForInitialization'
  | 'markHistoryEmpty'
  | 'replaceHistoryView'
  | 'beginInitialMessagesLoad'
  | 'applyInitialMessagesLoad'
  | 'finishInitialMessagesLoad'
  | 'fetchSessionWindow'
  | 'appendTurnToSession'
  | 'reattachTurnToSession'
  | 'appendToView'
  | 'insertTurnAt'
  | 'removeTurnFromSession'
  | 'replaceTailFromTurn'
  | 'restoreTailFromOptimistic'
  | 'createOptimisticAssistantTurn'
  | 'createOptimisticUserTurn'
  | 'adoptRuntimeUserTurn'
  | 'setAssistantRowIdentity'
  | 'setMessageSyncState'
  | 'setAssistantStreaming'
  | 'clearAssistantMessages'
  | 'appendAssistantContent'
  | 'appendToolProgress'
  | 'applyBackgroundTask'
  | 'upsertAssistantUIMessage'
  | 'replaceAssistantUIMessageSnapshot'
  | 'hasVisibleAssistantBlocks'
  | 'finishAssistantTurn'
  | 'snapshotToolApprovalStates'
  | 'assistantTurnForApproval'
  | 'restoreToolApprovalStates'
  | 'snapshotUserInputStates'
  | 'assistantTurnForUserInput'
  | 'restoreUserInputStates'
  | 'appendAssistantError'
  | 'finalizeStreamFailure'
  | 'assistantTurnForRuntimeError'
  | 'latestOptimisticUserText'
  | 'hasTurn'
  | 'findTurnByServerId'
  | 'isLatestVisibleUserTurn'
  | 'isLatestVisibleAssistantTurn'
  | 'markToolApprovalDecision'
  | 'markUserInputDecision'
  | 'resetUserScope'
>

export interface SynchronizedTranscriptView {
  messages: ChatMessage[]
  loadingMessages: Readonly<Ref<boolean>>
  loadingOlder: Readonly<Ref<boolean>>
  hasMoreOlder: Readonly<Ref<boolean>>
}

const readViews = new WeakMap<SynchronizedTranscript, SynchronizedTranscriptView>()
const readOwners = new WeakMap<SynchronizedTranscriptView, SynchronizedTranscript>()

export function readSynchronizedTranscript(transcript: SynchronizedTranscript): SynchronizedTranscriptView {
  const existing = readViews.get(transcript)
  if (existing) return existing
  const view = Object.freeze({
    // Keep the component-facing array type compatible with existing props;
    // Vue's deep readonly proxy enforces the ownership boundary at runtime.
    messages: readonly(transcript.messages) as unknown as ChatMessage[],
    loadingMessages: readonly(transcript.loadingMessages),
    loadingOlder: readonly(transcript.loadingOlder),
    hasMoreOlder: readonly(transcript.hasMoreOlder),
  })
  readViews.set(transcript, view)
  readOwners.set(view, transcript)
  return view
}

export function seedSynchronizedTranscriptForTest(
  view: SynchronizedTranscriptView,
  ...turns: ChatMessage[]
) {
  if (import.meta.env.MODE !== 'test') throw new Error('test transcript seeding is unavailable')
  const transcript = readOwners.get(view)
  if (!transcript) throw new Error('unknown synchronized transcript view')
  transcript.appendToView(...turns)
}

interface SynchronizedTranscriptHooks {
  onSnapshot: (targetSessionId: string | undefined, turns: import('@/composables/api/useChat.types').UITurn[]) => void
  onRefreshApplied: (botId: string, targetSessionId: string, latestTimestamp?: string) => void
}

function runtimeMessageId(message: ConversationUiMessage, fallback: number): number {
  return typeof message.id === 'number' && Number.isFinite(message.id) ? message.id : fallback
}

export function projectRuntimeMessage(message: ConversationUiMessage, fallbackId: number): UIMessage | null {
  const coordinates = {
    stable_id: message.stable_id?.trim() || undefined,
    turn_position: message.turn_position,
    turn_message_seq: message.turn_message_seq,
    ...('row_identities' in message && Array.isArray(message.row_identities)
      ? { row_identities: structuredClone(message.row_identities) }
      : {}),
  }
  switch (message.type) {
    case 'text':
      return { ...coordinates, id: runtimeMessageId(message, fallbackId), type: 'text', content: message.content ?? '' }
    case 'reasoning':
      return { ...coordinates, id: runtimeMessageId(message, fallbackId), type: 'reasoning', content: message.content ?? '' }
    case 'attachments':
      return {
        ...coordinates,
        id: runtimeMessageId(message, fallbackId),
        type: 'attachments',
        attachments: (message.attachments ?? []) as UIAttachment[],
      }
    case 'tool': {
      const name = (message.name ?? '').trim()
      const toolCallId = (message.tool_call_id ?? '').trim()
      if (!name || !toolCallId) return null
      return {
        ...coordinates,
        id: runtimeMessageId(message, fallbackId),
        type: 'tool',
        name,
        input: message.input,
        output: message.output,
        tool_call_id: toolCallId,
        running: message.running === true,
        progress: message.progress,
        approval: message.approval as UIToolApproval | undefined,
        user_input: message.user_input as UIUserInput | undefined,
        background_task: message.background_task as UIBackgroundTask | undefined,
      }
    }
    default:
      return null
  }
}

export function projectRuntimeMessages(messages: ConversationUiMessage[]): UIMessage[] {
  return messages
    .map((message, index) => projectRuntimeMessage(message, index))
    .filter((message): message is UIMessage => message !== null)
}

export function createSynchronizedTranscript(
  target: ChatViewTarget,
  deps: Omit<TranscriptDeps, 'currentBotId' | 'sessionId'>,
  hooks: SynchronizedTranscriptHooks,
): SynchronizedTranscript {
  const transcript = createTranscriptController({
    ...deps,
    currentBotId: ref(target.botId),
    sessionId: ref(target.sessionId),
  })
  transcript.setSnapshotHook(hooks.onSnapshot)
  transcript.setRefreshAppliedHook(hooks.onRefreshApplied)
  return transcript
}

function stableBlockIds(turn: ChatMessage): Set<string> {
  if (turn.role !== 'assistant') return new Set()
  return new Set(turn.messages
    .map(block => block.stable_id?.trim())
    .filter((id): id is string => Boolean(id)))
}

function sameDurableTurn(left: ChatMessage, right: ChatMessage): boolean {
  const leftServerId = left.serverId?.trim() ?? ''
  const rightServerId = right.serverId?.trim() ?? ''
  if (leftServerId && rightServerId && leftServerId === rightServerId) return true
  if (
    Number.isSafeInteger(left.turnPosition)
    && Number.isSafeInteger(left.turnMessageSeq)
    && left.turnPosition === right.turnPosition
    && left.turnMessageSeq === right.turnMessageSeq
  ) return true
  if (
    left.role === 'user'
    && right.role === 'user'
    && left.externalMessageId
    && left.externalMessageId === right.externalMessageId
  ) return true
  const leftBlocks = stableBlockIds(left)
  if (leftBlocks.size === 0) return false
  return [...stableBlockIds(right)].some(id => leftBlocks.has(id))
}

// Draft promotion is a sync concern because it reconciles two transcript
// identities. The view registry only moves panel metadata and delegates here.
export function promoteSynchronizedTranscript(
  target: SynchronizedTranscript,
  source: SynchronizedTranscript,
) {
  const additions = source.messages.filter(sourceTurn =>
    !target.messages.some(targetTurn => targetTurn === sourceTurn || sameDurableTurn(targetTurn, sourceTurn)))
  if (additions.length > 0) target.appendToView(...additions)
}

export function disposeSynchronizedTranscript(transcript: SynchronizedTranscript) {
  transcript.resetUserScope()
}
