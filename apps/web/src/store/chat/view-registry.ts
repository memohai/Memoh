import { readonly, ref } from 'vue'
import type { UITurn } from '@/composables/api/useChat.types'
import type {
  ChatMessage,
  ChatViewTarget,
  ChatWorkspaceTargetSelectionSource,
  ChatWorkspaceTargetSnapshot,
} from './types'

export const CHAT_SESSION_VIEW_CACHE_LIMIT = 12

export type { ChatViewTarget } from './types'

export type ChatViewKind = 'session' | 'draft'
export interface ChatTranscriptReader {
  readonly messages: readonly ChatMessage[]
}

export type ChatTranscriptController = ChatTranscriptReader

export interface ChatTranscriptHooks {
  onSnapshot: (targetSessionId: string | undefined, turns: UITurn[]) => void
  onRefreshApplied: (botId: string, targetSessionId: string, latestTimestamp?: string) => void
}

export interface ChatViewEntry<
  TTranscript extends ChatTranscriptReader = ChatTranscriptController,
> {
  key: string
  kind: ChatViewKind
  botId: string
  sessionId: string | null
  viewId: string
  transcript: TTranscript
  attachedPanelIds: Set<string>
  visiblePanelIds: Set<string>
  initialized: boolean
  workspaceTargetId: Ref<string>
  workspaceTargetSnapshot: Ref<ChatWorkspaceTargetSnapshot | null>
  workspaceTargetSelectionSource: Ref<ChatWorkspaceTargetSelectionSource>
  lastAccess: number
}

export interface ChatViewRegistryDeps<TTranscript extends ChatTranscriptReader> {
  cacheLimit?: number
  isSessionStreaming?: (botId: string, sessionId: string) => boolean
  createTranscript: (target: ChatViewTarget, hooks: ChatTranscriptHooks) => TTranscript
  onPromoteTranscript: (target: TTranscript, source: TTranscript) => void
  onDisposeTranscript: (transcript: TTranscript) => void
  onEvict?: (view: ChatViewEntry<TTranscript>) => void
  onSnapshot?: (view: ChatViewEntry<TTranscript>, targetSessionId: string | undefined, turns: UITurn[]) => void
  onRefreshApplied?: (view: ChatViewEntry<TTranscript>, targetSessionId: string, latestTimestamp?: string) => void
}

export interface ChatViewBindingChange<
  TTranscript extends ChatTranscriptReader = ChatTranscriptController,
> {
  view: ChatViewEntry<TTranscript>
  activatedSession: ChatViewEntry<TTranscript> | null
  deactivatedSession: ChatViewEntry<TTranscript> | null
}

function normalize(value: string | null | undefined): string {
  return value?.trim() ?? ''
}

export function chatSessionViewKey(botId: string, sessionId: string): string {
  return `session:${normalize(botId)}:${normalize(sessionId)}`
}

export function chatDraftViewKey(botId: string, viewId: string): string {
  return `draft:${normalize(botId)}:${normalize(viewId)}`
}

export function chatViewKey(target: ChatViewTarget): string {
  const botId = normalize(target.botId)
  const sessionId = normalize(target.sessionId)
  const viewId = normalize(target.viewId)
  return sessionId
    ? chatSessionViewKey(botId, sessionId)
    : chatDraftViewKey(botId, viewId)
}

function hasPendingInteraction(messages: readonly ChatMessage[]): boolean {
  for (const message of messages) {
    if (message.role !== 'assistant') continue
    for (const block of message.messages) {
      if (block.type !== 'tool') continue
      if (
        block.approval?.status === 'pending'
        && block.approval.can_approve !== false
      ) return true
      if (
        block.userInput?.status === 'pending'
        && block.userInput.can_respond !== false
      ) return true
    }
  }
  return false
}

// Owns the in-memory working set for chat panes. Session entries are shared by
// (bot, session); drafts are isolated by their stable dockview panel id.
export function createChatViewRegistry<TTranscript extends ChatTranscriptReader>(
  deps: ChatViewRegistryDeps<TTranscript>,
) {
  const cacheLimit = Math.max(0, deps.cacheLimit ?? CHAT_SESSION_VIEW_CACHE_LIMIT)
  const views = new Map<string, ChatViewEntry<TTranscript>>()
  const panelKeys = new Map<string, string>()
  const revision = ref(0)
  let accessClock = 0

  function touch(view: ChatViewEntry<TTranscript>) {
    view.lastAccess = ++accessClock
    return view
  }

  function createView(target: ChatViewTarget): ChatViewEntry<TTranscript> {
    const botId = normalize(target.botId)
    const sessionId = normalize(target.sessionId) || null
    const viewId = normalize(target.viewId)
    if (!botId) throw new Error('Chat view requires a bot id')
    if (!sessionId && !viewId) throw new Error('Draft chat view requires a stable view id')

    const normalizedTarget = { botId, sessionId, viewId }
    const viewRef: { current?: ChatViewEntry<TTranscript> } = {}
    const transcript = deps.createTranscript(normalizedTarget, {
      onSnapshot: (targetSessionId, turns) => {
        if (viewRef.current) deps.onSnapshot?.(viewRef.current, targetSessionId, turns)
      },
      onRefreshApplied: (_botId, targetSessionId, latestTimestamp) => {
        if (!viewRef.current) return
        viewRef.current.initialized = true
        deps.onRefreshApplied?.(viewRef.current, targetSessionId, latestTimestamp)
      },
    })
    const view: ChatViewEntry<TTranscript> = {
      key: chatViewKey({ botId, sessionId, viewId }),
      kind: sessionId ? 'session' : 'draft',
      botId,
      sessionId,
      viewId,
      transcript,
      attachedPanelIds: new Set<string>(),
      visiblePanelIds: new Set<string>(),
      initialized: false,
      workspaceTargetId: ref(''),
      workspaceTargetSnapshot: ref(null),
      workspaceTargetSelectionSource: ref('unset'),
      lastAccess: 0,
    }
    viewRef.current = view
    views.set(view.key, view)
    return touch(view)
  }

  function get(target: ChatViewTarget): ChatViewEntry<TTranscript> | undefined {
    const view = views.get(chatViewKey(target))
    return view ? touch(view) : undefined
  }

  function getOrCreate(target: ChatViewTarget): ChatViewEntry<TTranscript> {
    return get(target) ?? createView(target)
  }

  function getSession(botId: string, sessionId: string): ChatViewEntry<TTranscript> | undefined {
    const view = views.get(chatSessionViewKey(botId, sessionId))
    return view ? touch(view) : undefined
  }

  function getDraft(botId: string, viewId: string): ChatViewEntry<TTranscript> | undefined {
    const view = views.get(chatDraftViewKey(botId, viewId))
    return view ? touch(view) : undefined
  }

  function getPanel(panelId: string): ChatViewEntry<TTranscript> | undefined {
    const key = panelKeys.get(normalize(panelId))
    const view = key ? views.get(key) : undefined
    return view ? touch(view) : undefined
  }

  function mustRetain(view: ChatViewEntry<TTranscript>): boolean {
    // A dock tab is only an address back to this view. Keeping it attached must
    // not turn an arbitrary number of hidden tabs into an unbounded cache.
    if (view.visiblePanelIds.size > 0) return true
    if (view.kind !== 'session' || !view.sessionId) return false
    if (deps.isSessionStreaming?.(view.botId, view.sessionId)) return true
    return hasPendingInteraction(view.transcript.messages)
  }

  function evict(view: ChatViewEntry<TTranscript>) {
    views.delete(view.key)
    for (const panelId of view.attachedPanelIds) {
      if (panelKeys.get(panelId) === view.key) panelKeys.delete(panelId)
    }
    view.attachedPanelIds.clear()
    view.visiblePanelIds.clear()
    deps.onDisposeTranscript(view.transcript)
    deps.onEvict?.(view)
    revision.value += 1
  }

  function prune() {
    const hiddenSessions = [...views.values()]
      .filter(view => view.kind === 'session'
        && view.visiblePanelIds.size === 0)
      .sort((left, right) => left.lastAccess - right.lastAccess)
    let excess = hiddenSessions.length - cacheLimit
    if (excess <= 0) return
    for (const view of hiddenSessions) {
      if (excess <= 0) break
      if (mustRetain(view)) continue
      evict(view)
      excess -= 1
    }
  }

  function removePanelFromView(
    panelId: string,
    view: ChatViewEntry<TTranscript>,
  ): ChatViewEntry<TTranscript> | null {
    const wasLastVisibleSession = view.kind === 'session'
      && view.visiblePanelIds.has(panelId)
      && view.visiblePanelIds.size === 1
    view.attachedPanelIds.delete(panelId)
    view.visiblePanelIds.delete(panelId)
    if (panelKeys.get(panelId) === view.key) panelKeys.delete(panelId)
    touch(view)
    if (view.kind === 'draft' && view.attachedPanelIds.size === 0) {
      evict(view)
    }
    return wasLastVisibleSession ? view : null
  }

  function bindPanel(
    panelId: string,
    target: ChatViewTarget,
    visible: boolean,
  ): ChatViewBindingChange<TTranscript> {
    const id = normalize(panelId)
    if (!id) throw new Error('Chat view binding requires a panel id')
    const next = getOrCreate(target)
    const previousKey = panelKeys.get(id)
    let deactivatedSession: ChatViewEntry<TTranscript> | null = null
    if (previousKey && previousKey !== next.key) {
      const previous = views.get(previousKey)
      if (previous) deactivatedSession = removePanelFromView(id, previous)
    }

    const wasVisible = next.visiblePanelIds.size > 0
    next.attachedPanelIds.add(id)
    if (visible) next.visiblePanelIds.add(id)
    else next.visiblePanelIds.delete(id)
    panelKeys.set(id, next.key)
    touch(next)
    const activatedSession = next.kind === 'session' && visible && !wasVisible ? next : null
    prune()
    return { view: next, activatedSession, deactivatedSession }
  }

  function setPanelVisible(
    panelId: string,
    visible: boolean,
  ): ChatViewBindingChange<TTranscript> | null {
    const id = normalize(panelId)
    const key = panelKeys.get(id)
    const view = key ? views.get(key) : undefined
    if (!view) return null
    const wasVisible = view.visiblePanelIds.size > 0
    const panelWasVisible = view.visiblePanelIds.has(id)
    if (visible) view.visiblePanelIds.add(id)
    else view.visiblePanelIds.delete(id)
    touch(view)
    const activatedSession = view.kind === 'session' && visible && !wasVisible ? view : null
    const deactivatedSession = view.kind === 'session'
      && !visible
      && panelWasVisible
      && view.visiblePanelIds.size === 0
      ? view
      : null
    prune()
    return { view, activatedSession, deactivatedSession }
  }

  function unbindPanel(panelId: string): ChatViewEntry<TTranscript> | null {
    const id = normalize(panelId)
    const key = panelKeys.get(id)
    const view = key ? views.get(key) : undefined
    if (!view) return null
    const deactivated = removePanelFromView(id, view)
    prune()
    return deactivated
  }

  function promoteDraft(botId: string, viewId: string, sessionId: string): ChatViewEntry<TTranscript> {
    const bid = normalize(botId)
    const vid = normalize(viewId)
    const sid = normalize(sessionId)
    if (!bid || !vid || !sid) throw new Error('Draft promotion requires bot, view, and session ids')
    const draft = getDraft(bid, vid)
    if (!draft) return getOrCreate({ botId: bid, sessionId: sid, viewId: vid })

    const sessionKey = chatSessionViewKey(bid, sid)
    const existing = views.get(sessionKey)
    if (existing && existing !== draft) {
      deps.onPromoteTranscript(existing.transcript, draft.transcript)
      existing.initialized ||= draft.initialized
      if (draft.workspaceTargetSelectionSource.value !== 'unset') {
        existing.workspaceTargetId.value = draft.workspaceTargetId.value
        existing.workspaceTargetSnapshot.value = draft.workspaceTargetSnapshot.value
          ? { ...draft.workspaceTargetSnapshot.value }
          : null
        existing.workspaceTargetSelectionSource.value = draft.workspaceTargetSelectionSource.value
      }
      for (const panelId of draft.attachedPanelIds) {
        existing.attachedPanelIds.add(panelId)
        panelKeys.set(panelId, existing.key)
      }
      for (const panelId of draft.visiblePanelIds) existing.visiblePanelIds.add(panelId)
      views.delete(draft.key)
      draft.attachedPanelIds.clear()
      draft.visiblePanelIds.clear()
      deps.onDisposeTranscript(draft.transcript)
      touch(existing)
      prune()
      return existing
    }

    views.delete(draft.key)
    const replacement = createView({ botId: bid, sessionId: sid, viewId: vid })
    deps.onPromoteTranscript(replacement.transcript, draft.transcript)
    replacement.initialized = draft.initialized
    replacement.workspaceTargetId.value = draft.workspaceTargetId.value
    replacement.workspaceTargetSnapshot.value = draft.workspaceTargetSnapshot.value
      ? { ...draft.workspaceTargetSnapshot.value }
      : null
    replacement.workspaceTargetSelectionSource.value = draft.workspaceTargetSelectionSource.value
    for (const panelId of draft.attachedPanelIds) {
      replacement.attachedPanelIds.add(panelId)
      panelKeys.set(panelId, replacement.key)
    }
    for (const panelId of draft.visiblePanelIds) replacement.visiblePanelIds.add(panelId)
    draft.attachedPanelIds.clear()
    draft.visiblePanelIds.clear()
    deps.onDisposeTranscript(draft.transcript)
    touch(replacement)
    prune()
    return replacement
  }

  function removeSession(botId: string, sessionId: string) {
    const view = views.get(chatSessionViewKey(botId, sessionId))
    if (view) evict(view)
  }

  function resetBot(botId: string) {
    const bid = normalize(botId)
    for (const view of [...views.values()]) {
      if (view.botId === bid) evict(view)
    }
  }

  function resetAll() {
    for (const view of [...views.values()]) evict(view)
    panelKeys.clear()
  }

  function entries(): ChatViewEntry<TTranscript>[] {
    return [...views.values()]
  }

  return {
    revision: readonly(revision),
    get,
    getOrCreate,
    getSession,
    getDraft,
    getPanel,
    bindPanel,
    setPanelVisible,
    unbindPanel,
    promoteDraft,
    removeSession,
    prune,
    resetBot,
    resetAll,
    entries,
  }
}
