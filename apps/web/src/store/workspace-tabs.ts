import { defineStore, storeToRefs } from 'pinia'
import { computed, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { useChatStore } from '@/store/chat-list'
import { useChatSelectionStore } from '@/store/chat-selection'

export type WorkspaceTab =
  | { id: string; type: 'chat'; sessionId: string; title: string }
  | { id: string; type: 'file'; filePath: string; title: string }

interface BotTabState {
  tabs: WorkspaceTab[]
  activeId: string | null
}

type WorkspaceTabsStorage = Record<string, BotTabState>

function chatTabId(sessionId: string): string {
  return `chat:${sessionId}`
}

function fileTabId(filePath: string): string {
  return `file:${filePath}`
}

function fileBaseName(filePath: string): string {
  const idx = filePath.lastIndexOf('/')
  return idx >= 0 ? filePath.slice(idx + 1) : filePath
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
      storage.value = { ...storage.value, [bid]: { tabs: [], activeId: null } }
    }
    return storage.value[bid] ?? null
  }

  const currentState = computed<BotTabState>(() => {
    const bid = (currentBotId.value ?? '').trim()
    if (!bid) return { tabs: [], activeId: null }
    return storage.value[bid] ?? { tabs: [], activeId: null }
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
      [bid]: { tabs: [...state.tabs], activeId: state.activeId },
    }
  }

  function setActive(id: string | null) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    if (state.activeId === id) return
    commit({ tabs: state.tabs, activeId: id })

    if (!id) return
    const tab = state.tabs.find((t) => t.id === id)
    if (tab?.type === 'chat') {
      void chatStore.selectSession(tab.sessionId)
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
        commit({ tabs: next, activeId: id })
      } else {
        commit({ tabs: state.tabs, activeId: id })
      }
    } else {
      const tab: WorkspaceTab = {
        id,
        type: 'chat',
        sessionId: sid,
        title: title ?? '',
      }
      commit({ tabs: [...state.tabs, tab], activeId: id })
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
      commit({ tabs: state.tabs, activeId: id })
      return
    }
    const tab: WorkspaceTab = {
      id,
      type: 'file',
      filePath: path,
      title: fileBaseName(path),
    }
    commit({ tabs: [...state.tabs, tab], activeId: id })
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
    commit({ tabs: nextTabs, activeId: nextActive })

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

  function updateChatTitle(sessionId: string, title: string) {
    const state = ensureBot(currentBotId.value)
    if (!state) return
    const id = chatTabId(sessionId)
    const next = state.tabs.map((t) =>
      t.id === id && t.type === 'chat' ? { ...t, title } : t,
    )
    commit({ tabs: next, activeId: state.activeId })
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

  // When the chat-store session is set externally (e.g. URL navigation),
  // open or focus the corresponding chat tab.
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
      const exists = state.tabs.some((t) => t.id === id)
      if (!exists) return
      if (state.activeId !== id) {
        commit({ tabs: state.tabs, activeId: id })
      }
    },
  )

  return {
    tabs,
    activeId,
    activeTab,
    openChat,
    openFile,
    closeTab,
    closeChatBySession,
    updateChatTitle,
    setActive,
    resetBot,
  }
})
