import { defineStore, storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { useChatStore } from '@/store/chat-list'
import { useChatSelectionStore } from '@/store/chat-selection'

export type WorkspaceTab =
  | { id: string; type: 'chat'; sessionId: string; title: string }
  | { id: string; type: 'file'; filePath: string; title: string }
  | { id: string; type: 'terminal'; title: string }
  | { id: string; type: 'draft'; title: string }

const DRAFT_TAB_ID = 'draft'

interface BotTabState {
  tabs: WorkspaceTab[]
  activeId: string | null
  terminalCounter: number
  dirtyFileTabs: Record<string, boolean>
}

type WorkspaceTabsStorage = Record<string, BotTabState>

function chatTabId(sessionId: string): string {
  return `chat:${sessionId}`
}

function fileTabId(filePath: string): string {
  return `file:${filePath}`
}

function terminalTabId(counter: number): string {
  return `terminal:${counter}`
}

function fileBaseName(filePath: string): string {
  const idx = filePath.lastIndexOf('/')
  return idx >= 0 ? filePath.slice(idx + 1) : filePath
}

function emptyBotState(): BotTabState {
  return { tabs: [], activeId: null, terminalCounter: 0, dirtyFileTabs: {} }
}

export const useWorkspaceTabsStore = defineStore('workspace-tabs', () => {
  const selection = useChatSelectionStore()
  const { currentBotId } = storeToRefs(selection)
  const chatStore = useChatStore()

  const storage = useStorage<WorkspaceTabsStorage>('workspace-tabs', {})

  function ensureBot(botId: string | null | undefined): BotTabState | null {
    const bid = (botId ?? '').trim()
    if (!bid) return null
    if (!storage.value[bid]) {
      storage.value = { ...storage.value, [bid]: emptyBotState() }
    } else {
      // Backfill fields added later so old persisted state stays usable.
      const cur = storage.value[bid]!
      if (cur.terminalCounter === undefined || cur.dirtyFileTabs === undefined) {
        storage.value = {
          ...storage.value,
          [bid]: {
            tabs: cur.tabs ?? [],
            activeId: cur.activeId ?? null,
            terminalCounter: cur.terminalCounter ?? 0,
            dirtyFileTabs: cur.dirtyFileTabs ?? {},
          },
        }
      }
    }
    return storage.value[bid] ?? null
  }

  const currentState = computed<BotTabState>(() => {
    const bid = (currentBotId.value ?? '').trim()
    if (!bid) return emptyBotState()
    return storage.value[bid] ?? emptyBotState()
  })

  const tabs = computed<WorkspaceTab[]>(() => currentState.value.tabs)
  const activeId = computed<string | null>(() => currentState.value.activeId)
  const activeTab = computed<WorkspaceTab | null>(() => {
    const id = activeId.value
    if (!id) return null
    return tabs.value.find((t) => t.id === id) ?? null
  })

  function commit(state: BotTabState) {
    const bid = (currentBotId.value ?? '').trim()
    if (!bid) return
    storage.value = {
      ...storage.value,
      [bid]: {
        tabs: [...state.tabs],
        activeId: state.activeId,
        terminalCounter: state.terminalCounter,
        dirtyFileTabs: { ...state.dirtyFileTabs },
      },
    }
  }

  function setActive(id: string | null) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    if (state.activeId === id) return
    commit({ ...state, activeId: id })

    if (!id) return
    const tab = state.tabs.find((t) => t.id === id)
    if (tab?.type === 'chat') {
      void chatStore.selectSession(tab.sessionId)
    } else if (tab?.type === 'draft') {
      void chatStore.createNewSession()
    }
  }

  function openChat(sessionId: string, title?: string) {
    const sid = (sessionId ?? '').trim()
    if (!sid) return
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const id = chatTabId(sid)
    const existing = state.tabs.find((t) => t.id === id)
    if (existing) {
      if (title && existing.type === 'chat' && existing.title !== title) {
        const next = state.tabs.map((t) =>
          t.id === id && t.type === 'chat' ? { ...t, title } : t,
        )
        commit({ ...state, tabs: next, activeId: id })
      } else {
        commit({ ...state, activeId: id })
      }
    } else {
      const tab: WorkspaceTab = {
        id,
        type: 'chat',
        sessionId: sid,
        title: title ?? '',
      }
      commit({ ...state, tabs: [...state.tabs, tab], activeId: id })
    }
    void chatStore.selectSession(sid)
  }

  function openFile(filePath: string) {
    const path = (filePath ?? '').trim()
    if (!path) return
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const id = fileTabId(path)
    const existing = state.tabs.find((t) => t.id === id)
    if (existing) {
      commit({ ...state, activeId: id })
      return
    }
    const tab: WorkspaceTab = {
      id,
      type: 'file',
      filePath: path,
      title: fileBaseName(path),
    }
    commit({ ...state, tabs: [...state.tabs, tab], activeId: id })
  }

  function openDraft() {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const existing = state.tabs.find((t) => t.id === DRAFT_TAB_ID)
    if (existing) {
      commit({ ...state, activeId: DRAFT_TAB_ID })
    } else {
      const tab: WorkspaceTab = { id: DRAFT_TAB_ID, type: 'draft', title: '' }
      commit({ ...state, tabs: [...state.tabs, tab], activeId: DRAFT_TAB_ID })
    }
    void chatStore.createNewSession()
  }

  function promoteDraftToChat(sessionId: string, title?: string) {
    const sid = (sessionId ?? '').trim()
    if (!sid) return
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const draftIdx = state.tabs.findIndex((t) => t.type === 'draft')
    if (draftIdx < 0) return

    const newId = chatTabId(sid)
    const existingChatIdx = state.tabs.findIndex((t) => t.id === newId)

    if (existingChatIdx >= 0) {
      // A chat tab for this session already exists; drop the draft and focus it.
      const nextTabs = state.tabs.filter((_, i) => i !== draftIdx)
      const nextActive = state.activeId === DRAFT_TAB_ID ? newId : state.activeId
      commit({ ...state, tabs: nextTabs, activeId: nextActive })
      return
    }

    const promoted: WorkspaceTab = {
      id: newId,
      type: 'chat',
      sessionId: sid,
      title: title ?? '',
    }
    const nextTabs = [...state.tabs]
    nextTabs[draftIdx] = promoted
    const nextActive = state.activeId === DRAFT_TAB_ID ? newId : state.activeId
    commit({ ...state, tabs: nextTabs, activeId: nextActive })
  }

  function openTerminal() {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const nextCounter = state.terminalCounter + 1
    const id = terminalTabId(nextCounter)
    const tab: WorkspaceTab = {
      id,
      type: 'terminal',
      title: `Terminal ${nextCounter}`,
    }
    commit({
      ...state,
      tabs: [...state.tabs, tab],
      activeId: id,
      terminalCounter: nextCounter,
    })
  }

  function closeTab(id: string) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const idx = state.tabs.findIndex((t) => t.id === id)
    if (idx < 0) return
    const nextTabs = state.tabs.filter((t) => t.id !== id)
    let nextActive = state.activeId
    if (state.activeId === id) {
      if (nextTabs.length === 0) {
        nextActive = null
      } else {
        const fallback = nextTabs[Math.min(idx, nextTabs.length - 1)]
        nextActive = fallback?.id ?? null
      }
    }
    const nextDirty = { ...state.dirtyFileTabs }
    delete nextDirty[id]

    commit({ ...state, tabs: nextTabs, activeId: nextActive, dirtyFileTabs: nextDirty })

    if (nextActive) {
      const tab = nextTabs.find((t) => t.id === nextActive)
      if (tab?.type === 'chat') {
        void chatStore.selectSession(tab.sessionId)
      }
    }
  }

  function closeChatBySession(sessionId: string) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    closeTab(chatTabId(sessionId))
  }

  function closeAll() {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    commit({ ...state, tabs: [], activeId: null, dirtyFileTabs: {} })
  }

  function isTabBusy(tab: WorkspaceTab, dirty: Record<string, boolean>): boolean {
    switch (tab.type) {
      case 'chat':
        return chatStore.streamingSessionId === tab.sessionId
      case 'file':
        return dirty[tab.id] === true
      case 'terminal':
      case 'draft':
        return false
    }
  }

  function closeFinished() {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const remaining = state.tabs.filter((tab) => isTabBusy(tab, state.dirtyFileTabs))
    let nextActive = state.activeId
    if (nextActive && !remaining.some((t) => t.id === nextActive)) {
      nextActive = remaining[0]?.id ?? null
    }
    const nextDirty: Record<string, boolean> = {}
    for (const tab of remaining) {
      if (state.dirtyFileTabs[tab.id]) nextDirty[tab.id] = true
    }
    commit({ ...state, tabs: remaining, activeId: nextActive, dirtyFileTabs: nextDirty })

    if (nextActive) {
      const tab = remaining.find((t) => t.id === nextActive)
      if (tab?.type === 'chat') {
        void chatStore.selectSession(tab.sessionId)
      }
    }
  }

  function setFileDirty(tabId: string, dirty: boolean) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const current = state.dirtyFileTabs[tabId] === true
    if (current === dirty) return
    const nextDirty = { ...state.dirtyFileTabs }
    if (dirty) nextDirty[tabId] = true
    else delete nextDirty[tabId]
    commit({ ...state, dirtyFileTabs: nextDirty })
  }

  function updateChatTitle(sessionId: string, title: string) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const id = chatTabId(sessionId)
    const next = state.tabs.map((t) =>
      t.id === id && t.type === 'chat' ? { ...t, title } : t,
    )
    commit({ ...state, tabs: next })
  }

  // Reset all tabs for a specific bot. Used when the user switches bots.
  function resetBot(botId: string) {
    const bid = (botId ?? '').trim()
    if (!bid) return
    const next = { ...storage.value }
    delete next[bid]
    storage.value = next
  }

  // When the active tab is a chat tab, keep chat-store selection in sync.
  watch(activeTab, (tab) => {
    if (!tab) return
    if (tab.type !== 'chat') return
    if (chatStore.sessionId === tab.sessionId) return
    void chatStore.selectSession(tab.sessionId)
  })

  // Pre-create state for newly seen bots so that the storage object always
  // has a slot for the active bot.
  watch(currentBotId, (bid) => {
    ensureBot(bid)
  }, { immediate: true })

  // When the chat-store session is set externally (e.g. URL navigation, or
  // the first message in a draft tab triggering server-side session creation),
  // promote the draft tab if active, otherwise open or focus the chat tab.
  const draftSessionId = ref<string | null>(null)
  watch(
    () => chatStore.sessionId,
    (sid) => {
      if (!sid) {
        draftSessionId.value = null
        return
      }
      if (draftSessionId.value === sid) return
      draftSessionId.value = sid
      const state = ensureBot(currentBotId.value)
      if (!state) return
      const id = chatTabId(sid)

      // Promote the draft tab in place if it's currently active. This is the
      // path taken when sendMessage in a draft creates the real session.
      if (state.activeId === DRAFT_TAB_ID) {
        promoteDraftToChat(sid)
        return
      }

      const exists = state.tabs.some((t) => t.id === id)
      if (!exists) return
      if (state.activeId !== id) {
        commit({ ...state, activeId: id })
      }
    },
  )

  return {
    tabs,
    activeId,
    activeTab,
    openChat,
    openFile,
    openTerminal,
    openDraft,
    promoteDraftToChat,
    closeTab,
    closeChatBySession,
    closeAll,
    closeFinished,
    setFileDirty,
    updateChatTitle,
    setActive,
    resetBot,
  }
})
