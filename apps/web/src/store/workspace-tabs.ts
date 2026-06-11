import { defineStore, storeToRefs } from 'pinia'
import { computed, nextTick, ref, shallowRef, watch } from 'vue'
import { useLocalStorage, useStorage } from '@vueuse/core'
import type { DockviewApi, SerializedDockview } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import { useChatSelectionStore } from '@/store/chat-selection'
import { onAuthSessionCleared } from '@/lib/auth-session'
import { hasBotPermission, type BotPermission } from '@/utils/bot-permissions'
import {
  clearTerminalSnapshots,
  clearTerminalSnapshotsForBot,
  deleteTerminalSnapshot,
  terminalCacheKey,
} from '@/composables/useTerminalCache'

// Workspace shell state (activity bar + side panel + dockview layout):
// - the dockview layout in the center area (chat / file / terminal / browser /
//   display panels, splits, tab order) persisted per bot
// - the left activity-bar + side-panel state (active view, collapsed, width)
//
// The active chat session itself lives in the chat-selection store. The chat
// panel is a singleton dockview panel whose content follows the active
// session; files/terminals/browsers/displays are multi-instance panels.

export type SidebarView = 'sessions' | 'files'

export const CHAT_PANEL_ID = 'chat'

export type WorkspacePanelComponent = 'chat' | 'file' | 'terminal' | 'browser' | 'display'

interface BotLayoutState {
  layout: SerializedDockview | null
  terminalCounter: number
  browserCounter: number
  displayCounter: number
}

type WorkspaceLayoutStorage = Record<string, BotLayoutState>

function emptyBotLayout(): BotLayoutState {
  return { layout: null, terminalCounter: 0, browserCounter: 0, displayCounter: 0 }
}

function fileBaseName(filePath: string): string {
  const idx = filePath.lastIndexOf('/')
  return idx >= 0 ? filePath.slice(idx + 1) : filePath
}

function panelComponentOf(id: string): WorkspacePanelComponent | null {
  if (id === CHAT_PANEL_ID) return 'chat'
  if (id.startsWith('file:')) return 'file'
  if (id.startsWith('terminal:')) return 'terminal'
  if (id.startsWith('browser:')) return 'browser'
  if (id.startsWith('display:')) return 'display'
  return null
}

export const useWorkspaceTabsStore = defineStore('workspace-tabs', () => {
  const selection = useChatSelectionStore()
  const { currentBotId } = storeToRefs(selection)
  const chatStore = useChatStore()
  const currentBot = computed(() =>
    chatStore.bots.find(bot => bot.id === currentBotId.value) ?? null,
  )

  // Earlier iterations persisted browser-style tab state under these keys;
  // neither model is compatible with the dockview layout, so drop them.
  if (typeof localStorage !== 'undefined') {
    localStorage.removeItem('workspace-tabs')
    localStorage.removeItem('workspace-panes')
  }
  const storage = useStorage<WorkspaceLayoutStorage>('workspace-layout', {})

  // ---- dockview wiring -----------------------------------------------------

  const api = shallowRef<DockviewApi | null>(null)
  const activePanelId = ref<string | null>(null)
  // The bot whose layout is currently loaded into dockview. Guards persistence
  // so that clearing the view during a bot switch can't wipe the old bot's
  // stored layout.
  let loadedBotId: string | null = null
  let suppressPersist = false
  let apiDisposables: Array<{ dispose(): void }> = []

  function ensureBotLayout(botId: string | null | undefined): BotLayoutState | null {
    const bid = (botId ?? '').trim()
    if (!bid) return null
    if (!storage.value[bid]) {
      storage.value = { ...storage.value, [bid]: emptyBotLayout() }
    }
    return storage.value[bid] ?? null
  }

  function patchBotLayout(botId: string, patch: Partial<BotLayoutState>) {
    const state = ensureBotLayout(botId)
    if (!state) return
    storage.value = { ...storage.value, [botId]: { ...state, ...patch } }
  }

  function persistLayout() {
    if (!api.value || suppressPersist || !loadedBotId) return
    try {
      patchBotLayout(loadedBotId, { layout: api.value.toJSON() })
    } catch {
      // Serialization should never throw, but a failed save must not break UI.
    }
  }

  function restoreLayout(botId: string) {
    const dock = api.value
    if (!dock) return
    suppressPersist = true
    try {
      const stored = storage.value[botId]?.layout ?? null
      if (stored) {
        try {
          dock.fromJSON(stored)
        } catch {
          dock.clear()
          patchBotLayout(botId, { layout: null })
        }
      } else {
        dock.clear()
      }
      loadedBotId = botId
      prunePanels()
      activePanelId.value = dock.activePanel?.id ?? null
    } finally {
      suppressPersist = false
    }
  }

  function registerApi(dock: DockviewApi) {
    for (const d of apiDisposables) d.dispose()
    api.value = dock
    apiDisposables = [
      dock.onDidActivePanelChange((panel) => {
        activePanelId.value = panel?.id ?? null
      }),
      dock.onDidLayoutChange(() => {
        persistLayout()
      }),
      dock.onDidRemovePanel((panel) => {
        if (panel.id.startsWith('terminal:') && loadedBotId) {
          const bid = loadedBotId
          void nextTick(() => deleteTerminalSnapshot(terminalCacheKey(bid, panel.id)))
        }
      }),
    ]
    const bid = (currentBotId.value ?? '').trim()
    if (bid) {
      restoreLayout(bid)
    }
  }

  function releaseApi() {
    for (const d of apiDisposables) d.dispose()
    apiDisposables = []
    api.value = null
    activePanelId.value = null
    loadedBotId = null
  }

  // ---- panel operations ----------------------------------------------------

  const activeId = computed<string | null>(() => activePanelId.value)

  function hasCurrentPermission(permission: BotPermission): boolean {
    return hasBotPermission(currentBot.value?.current_user_permissions, permission)
  }

  function focusOrAdd(options: {
    id: string
    component: WorkspacePanelComponent
    title: string
    params?: Record<string, unknown>
  }): boolean {
    const dock = api.value
    if (!dock) return false
    const existing = dock.getPanel(options.id)
    if (existing) {
      existing.api.setActive()
      return true
    }
    dock.addPanel({
      id: options.id,
      component: options.component,
      title: options.title,
      params: options.params,
      // Keep panel DOM mounted while hidden: terminals, WebRTC display and
      // chat scroll state do not survive detach/reattach cycles.
      renderer: 'always',
    })
    return true
  }

  /** Open or focus the singleton chat panel. Content follows the active session. */
  function openChat(title?: string) {
    focusOrAdd({ id: CHAT_PANEL_ID, component: 'chat', title: title ?? '' })
  }

  function setChatTitle(title: string) {
    const panel = api.value?.getPanel(CHAT_PANEL_ID)
    if (panel && panel.api.title !== title) {
      panel.api.setTitle(title)
    }
  }

  function openFile(filePath: string) {
    if (!hasCurrentPermission('workspace_read')) return
    const path = (filePath ?? '').trim()
    if (!path) return
    focusOrAdd({
      id: `file:${path}`,
      component: 'file',
      title: fileBaseName(path),
      params: { filePath: path },
    })
  }

  function openTerminal() {
    if (!hasCurrentPermission('workspace_exec')) return
    const bid = (currentBotId.value ?? '').trim()
    const state = ensureBotLayout(bid)
    if (!state || !api.value) return
    const next = state.terminalCounter + 1
    patchBotLayout(bid, { terminalCounter: next })
    focusOrAdd({
      id: `terminal:${next}`,
      component: 'terminal',
      title: `Terminal ${next}`,
    })
  }

  function openBrowser(address = 'localhost:5173/') {
    if (!hasCurrentPermission('manage')) return
    const bid = (currentBotId.value ?? '').trim()
    const state = ensureBotLayout(bid)
    if (!state || !api.value) return
    const next = state.browserCounter + 1
    patchBotLayout(bid, { browserCounter: next })
    focusOrAdd({
      id: `browser:${next}`,
      component: 'browser',
      title: `Browser ${next}`,
      params: { address },
    })
  }

  function openDisplay() {
    if (!hasCurrentPermission('manage')) return
    const dock = api.value
    if (!dock) return
    const existing = dock.panels.find((panel) => panel.id.startsWith('display:'))
    if (existing) {
      existing.api.setActive()
      return
    }
    const bid = (currentBotId.value ?? '').trim()
    const state = ensureBotLayout(bid)
    if (!state) return
    const next = state.displayCounter + 1
    patchBotLayout(bid, { displayCounter: next })
    focusOrAdd({
      id: `display:${next}`,
      component: 'display',
      title: `Desktop ${next}`,
    })
  }

  function closeTab(id: string) {
    const panel = api.value?.getPanel(id)
    panel?.api.close()
  }

  function setFileDirty(panelId: string, dirty: boolean) {
    const panel = api.value?.getPanel(panelId)
    if (!panel) return
    const base = fileBaseName(panelId.startsWith('file:') ? panelId.slice('file:'.length) : panelId)
    const next = dirty ? `● ${base}` : base
    if (panel.api.title !== next) {
      panel.api.setTitle(next)
    }
  }

  function updateBrowserAddress(panelId: string, address: string) {
    const panel = api.value?.getPanel(panelId)
    if (!panel) return
    panel.api.updateParameters({ address })
    if (address) {
      panel.api.setTitle(address)
    }
  }

  function isPanelAllowed(id: string): boolean {
    switch (panelComponentOf(id)) {
      case 'chat':
        return true
      case 'file':
        return hasCurrentPermission('workspace_read')
      case 'terminal':
        return hasCurrentPermission('workspace_exec')
      case 'browser':
      case 'display':
        return hasCurrentPermission('manage')
      default:
        return false
    }
  }

  function prunePanels() {
    const dock = api.value
    if (!dock) return
    const perms = currentBot.value?.current_user_permissions
    if (!perms || perms.length === 0) return
    for (const panel of [...dock.panels]) {
      if (!isPanelAllowed(panel.id)) {
        panel.api.close()
      }
    }
  }

  // ---- sidebar (activity bar + side panel) ---------------------------------

  const sidebarView = useLocalStorage<SidebarView>('workspace-sidebar-view', 'sessions')
  const sidebarOpen = useLocalStorage('workspace-sidebar-open', true)
  const sidebarWidth = useLocalStorage('workspace-sidebar-width', 256)
  const workbenchOpen = useLocalStorage('workspace-workbench-open', true)

  // Push/pull model (see main-section + sidebar): the rail is in flow and slides
  // out to the left (margin-left) while the dock, a flex sibling, grows to fill
  // the space — content shifts. Toggling just flips this boolean; the rail
  // animates its margin on a shared curve and dockview relays out per frame as
  // the dock width changes, so the right-side actions ("+") stay pinned (right
  // edge never moves) instead of snapping.
  function setWorkbench(open: boolean) {
    workbenchOpen.value = open
  }

  // Horizontal nav only switches the active view and ensures the sidebar is
  // shown. Collapsing the whole sidebar lives on the workbench toggle (the
  // chrome button over the dock), not on the nav items.
  function selectSidebarView(view: SidebarView) {
    sidebarView.value = view
    sidebarOpen.value = true
    setWorkbench(true)
  }

  function toggleWorkbench() {
    setWorkbench(!workbenchOpen.value)
  }

  function showWorkbench() {
    setWorkbench(true)
  }

  function hideWorkbench() {
    setWorkbench(false)
  }

  // One-shot navigation request consumed by the sidebar files panel.
  const pendingFilesPath = ref<string | null>(null)

  function openFilesAt(path: string) {
    if (!hasCurrentPermission('workspace_read')) return
    setWorkbench(true)
    sidebarView.value = 'files'
    sidebarOpen.value = true
    pendingFilesPath.value = (path ?? '').trim() || null
  }

  function consumePendingFilesPath(): string | null {
    const path = pendingFilesPath.value
    pendingFilesPath.value = null
    return path
  }

  // ---- lifecycle -----------------------------------------------------------

  function resetBot(botId: string) {
    const bid = (botId ?? '').trim()
    if (!bid) return
    const next = { ...storage.value }
    delete next[bid]
    storage.value = next
    if (loadedBotId === bid && api.value) {
      suppressPersist = true
      try {
        api.value.clear()
      } finally {
        suppressPersist = false
      }
    }
    void nextTick(() => clearTerminalSnapshotsForBot(bid))
  }

  function resetAll() {
    storage.value = {}
    pendingFilesPath.value = null
    if (api.value) {
      suppressPersist = true
      try {
        api.value.clear()
      } finally {
        suppressPersist = false
      }
    }
    loadedBotId = null
    void nextTick(() => clearTerminalSnapshots())
  }

  onAuthSessionCleared(() => resetAll())

  watch(currentBotId, (bid) => {
    const next = (bid ?? '').trim()
    if (!next) {
      loadedBotId = null
      return
    }
    ensureBotLayout(next)
    if (api.value && loadedBotId !== next) {
      restoreLayout(next)
    }
  }, { immediate: true })

  watch(
    () => [currentBotId.value, ...(currentBot.value?.current_user_permissions ?? [])].join('|'),
    () => prunePanels(),
  )

  return {
    api,
    activeId,
    sidebarView,
    sidebarOpen,
    workbenchOpen,
    sidebarWidth,
    pendingFilesPath,
    registerApi,
    releaseApi,
    selectSidebarView,
    toggleWorkbench,
    showWorkbench,
    hideWorkbench,
    openChat,
    setChatTitle,
    openFile,
    openFilesAt,
    consumePendingFilesPath,
    openTerminal,
    openBrowser,
    openDisplay,
    closeTab,
    setFileDirty,
    updateBrowserAddress,
    resetBot,
    resetAll,
  }
})
