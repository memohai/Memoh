import { defineStore, storeToRefs } from 'pinia'
import { computed, nextTick, ref, shallowRef, watch } from 'vue'
import { useLocalStorage, useStorage } from '@vueuse/core'
import type { DockviewApi, DockviewGroupPanel, SerializedDockview } from 'dockview-vue'
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

export type SidebarView = 'sessions' | 'files' | 'schedule'

export const CHAT_PANEL_ID = 'chat'

export const TERMINAL_TAB_COMPONENT = 'terminalTab'

const DEFAULT_BROWSER_ADDRESS = 'localhost:5173/'

export type WorkspacePanelComponent = 'chat' | 'file' | 'preview' | 'terminal' | 'browser' | 'display'

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
  if (id.startsWith('preview:')) return 'preview'
  if (id.startsWith('terminal:')) return 'terminal'
  if (id.startsWith('browser:')) return 'browser'
  if (id.startsWith('display:')) return 'display'
  return null
}

function isTerminalOnlyGroup(group: { panels: Array<{ id: string }> }): boolean {
  const panels = group.panels
  return panels.length > 0 && panels.every(p => p.id.startsWith('terminal:'))
}

function syncTerminalGroupChrome(group: DockviewGroupPanel) {
  const terminalOnly = isTerminalOnlyGroup(group)
  group.element.classList.toggle('memoh-terminal-group', terminalOnly)
  const target = terminalOnly ? 'bottom' : 'top'
  if (group.api.getHeaderPosition() !== target) {
    group.api.setHeaderPosition(target)
  }
}

function syncAllTerminalGroupChrome(dock: DockviewApi) {
  for (const group of dock.groups) {
    syncTerminalGroupChrome(group)
  }
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
  // Per-panel unsaved-changes state for file panels. Kept here (NOT baked into
  // the tab title) so the tab dot, the sidebar count badge, and the close-confirm
  // dialog all read ONE reactive source. Keyed by dockview panel id.
  const fileDirty = ref<Record<string, boolean>>({})
  // Save callbacks registered by each mounted file panel, so a dirty tab can be
  // written from the close-confirm dialog even while it sits in the background.
  const saveHandlers = new Map<string, () => Promise<boolean>>()
  // FIFO of dirty panel ids awaiting a close decision (drives the dialog). A
  // batch close (others / all) enqueues several; the dialog walks them in turn.
  const closeQueue = ref<string[]>([])
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

  // Dockview serializes `hideHeader: true` when a group's tab bar is hidden
  // (display:none). Once this enters localStorage it re-applies on every
  // fromJSON call, creating a permanent cycle where the tab bar never appears.
  // Strip the flag from all leaf group data so the tab bar is always visible,
  // both when reading from storage and when writing back to storage.
  function sanitizeLayout(layout: SerializedDockview): SerializedDockview {
    type GridNode = { type: string; data: unknown }
    function walkNode(node: GridNode): GridNode {
      if (!node || typeof node !== 'object') return node
      if (node.type === 'leaf' && node.data && typeof node.data === 'object') {
        const data = { ...(node.data as Record<string, unknown>) }
        delete data.hideHeader
        return { ...node, data }
      }
      if (node.type === 'branch' && Array.isArray(node.data)) {
        return { ...node, data: (node.data as GridNode[]).map(walkNode) }
      }
      return node
    }
    try {
      return {
        ...layout,
        grid: {
          ...layout.grid,
          root: walkNode(layout.grid.root as GridNode) as SerializedDockview['grid']['root'],
        },
      }
    } catch {
      return layout
    }
  }

  function persistLayout() {
    if (!api.value || suppressPersist || !loadedBotId) return
    try {
      patchBotLayout(loadedBotId, { layout: sanitizeLayout(api.value.toJSON()) })
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
          dock.fromJSON(sanitizeLayout(stored))
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
      syncAllTerminalGroupChrome(dock)
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
        cleanupPanelState(panel.id)
        if (panel.id.startsWith('terminal:') && loadedBotId) {
          const bid = loadedBotId
          void nextTick(() => deleteTerminalSnapshot(terminalCacheKey(bid, panel.id)))
        }
        syncAllTerminalGroupChrome(dock)
      }),
      dock.onDidAddPanel(() => {
        syncAllTerminalGroupChrome(dock)
      }),
      dock.onDidMovePanel(() => {
        syncAllTerminalGroupChrome(dock)
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
    // The tab group whose header strip triggered the open (the "+" lives per
    // group). Without it dockview drops the new panel into the active group,
    // which is why a "+" in the right split used to open its terminal back in
    // the left pane.
    groupId?: string
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
      ...(options.groupId
        ? { position: { referenceGroup: options.groupId, direction: 'within' as const } }
        : {}),
    })
    return true
  }

  function defaultTerminalPosition(dock: DockviewApi) {
    const terminalGroupId = dock.groups.find(g =>
      g.panels.some(p => p.id.startsWith('terminal:')),
    )?.id
    if (terminalGroupId) {
      return { referenceGroup: terminalGroupId, direction: 'within' as const }
    }
    const chatGroupId = dock.groups.find(g =>
      g.panels.some(p => p.id === CHAT_PANEL_ID),
    )?.id
    if (chatGroupId) {
      return { referenceGroup: chatGroupId, direction: 'below' as const }
    }
    return undefined
  }

  function addTerminalPanel(options: {
    id: string
    title: string
    groupId?: string
    position?: { referenceGroup: string, direction: 'within' | 'below' | 'right' | 'left' | 'above' }
  }) {
    const dock = api.value
    if (!dock) return false
    const existing = dock.getPanel(options.id)
    if (existing) {
      existing.api.setActive()
      syncAllTerminalGroupChrome(dock)
      return true
    }
    const panelBase = {
      id: options.id,
      component: 'terminal' as const,
      tabComponent: TERMINAL_TAB_COMPONENT,
      title: options.title,
      renderer: 'always' as const,
    }
    if (options.groupId) {
      dock.addPanel({
        ...panelBase,
        position: { referenceGroup: options.groupId, direction: 'within' },
      })
    } else {
      dock.addPanel({
        ...panelBase,
        ...(options.position ? { position: options.position } : {}),
      })
    }
    syncAllTerminalGroupChrome(dock)
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

  // VS Code's "Open Preview to the Side": render a markdown/html file's preview
  // as its own panel in a group beside the source editor, instead of toggling the
  // source view in place. Focuses an existing preview for the same file.
  function openPreview(filePath: string, title?: string, groupId?: string) {
    if (!hasCurrentPermission('workspace_read')) return
    const path = (filePath ?? '').trim()
    if (!path) return
    const dock = api.value
    if (!dock) return
    const id = `preview:${path}`
    const existing = dock.getPanel(id)
    if (existing) {
      existing.api.setActive()
      return
    }
    // Split to the right of the source editor's own group (the preview action
    // lives in that group's header); fall back to the active group.
    const referenceGroup = groupId || dock.activeGroup
    dock.addPanel({
      id,
      component: 'preview',
      title: title || fileBaseName(path),
      params: { filePath: path },
      renderer: 'always',
      ...(referenceGroup ? { position: { referenceGroup, direction: 'right' as const } } : {}),
    })
  }

  function openTerminal(groupId?: string) {
    if (!hasCurrentPermission('workspace_exec')) return
    const bid = (currentBotId.value ?? '').trim()
    const state = ensureBotLayout(bid)
    const dock = api.value
    if (!state || !dock) return
    const next = state.terminalCounter + 1
    patchBotLayout(bid, { terminalCounter: next })
    addTerminalPanel({
      id: `terminal:${next}`,
      title: 'zsh',
      groupId,
      position: groupId ? undefined : defaultTerminalPosition(dock),
    })
  }

  function openBrowser(groupId?: string) {
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
      params: { address: DEFAULT_BROWSER_ADDRESS },
      groupId,
    })
  }

  function openDisplay(groupId?: string) {
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
      groupId,
    })
  }

  function uniqueSplitPanelId(baseId: string): string {
    const dock = api.value
    if (!dock) return baseId
    if (!dock.getPanel(baseId)) return baseId
    let n = 2
    while (dock.getPanel(`${baseId}~${n}`)) n++
    return `${baseId}~${n}`
  }

  /** Split the group's active tab into a new pane beside/below it (VS Code-style).
   * Duplicates the current panel into the new split rather than moving it. */
  function splitGroup(groupId: string, direction: 'right' | 'below') {
    const dock = api.value
    if (!dock) return
    const group = dock.getGroup(groupId)
    const source = group?.activePanel
    if (!source || !group) return

    const comp = panelComponentOf(source.id)
    if (!comp) return

    const position = { referenceGroup: groupId, direction }
    const title = source.title ?? source.api.title ?? ''
    const params = source.params as Record<string, unknown> | undefined

    switch (comp) {
      case 'terminal': {
        if (!hasCurrentPermission('workspace_exec')) return
        const bid = (currentBotId.value ?? '').trim()
        const state = ensureBotLayout(bid)
        if (!state) return
        const next = state.terminalCounter + 1
        patchBotLayout(bid, { terminalCounter: next })
        addTerminalPanel({
          id: `terminal:${next}`,
          title: title || 'zsh',
          position,
        })
        break
      }
      case 'file':
      case 'preview': {
        if (!hasCurrentPermission('workspace_read')) return
        dock.addPanel({
          id: uniqueSplitPanelId(source.id),
          component: comp,
          title,
          params: params ? { ...params } : undefined,
          renderer: 'always',
          position,
        })
        break
      }
      case 'browser': {
        if (!hasCurrentPermission('manage')) return
        const bid = (currentBotId.value ?? '').trim()
        const state = ensureBotLayout(bid)
        if (!state) return
        const next = state.browserCounter + 1
        patchBotLayout(bid, { browserCounter: next })
        dock.addPanel({
          id: `browser:${next}`,
          component: 'browser',
          title: title || `Browser ${next}`,
          params: { address: (params?.address as string | undefined) ?? DEFAULT_BROWSER_ADDRESS },
          renderer: 'always',
          position,
        })
        break
      }
      case 'display': {
        if (!hasCurrentPermission('manage')) return
        const bid = (currentBotId.value ?? '').trim()
        const state = ensureBotLayout(bid)
        if (!state) return
        const next = state.displayCounter + 1
        patchBotLayout(bid, { displayCounter: next })
        dock.addPanel({
          id: `display:${next}`,
          component: 'display',
          title: title || `Desktop ${next}`,
          renderer: 'always',
          position,
        })
        break
      }
      case 'chat': {
        dock.addPanel({
          id: CHAT_PANEL_ID,
          component: 'chat',
          title,
          renderer: 'always',
          position,
        })
        break
      }
    }
  }

  function closeTab(id: string) {
    const panel = api.value?.getPanel(id)
    panel?.api.close()
  }

  function setFileDirty(panelId: string, dirty: boolean) {
    if (!!fileDirty.value[panelId] === dirty) return
    const next = { ...fileDirty.value }
    if (dirty) next[panelId] = true
    else delete next[panelId]
    fileDirty.value = next
  }

  function registerFileSaveHandler(panelId: string, handler: () => Promise<boolean>) {
    saveHandlers.set(panelId, handler)
  }

  function unregisterFileSaveHandler(panelId: string) {
    saveHandlers.delete(panelId)
  }

  // Drop every trace of a panel once it's gone, so a stale dirty flag can't keep
  // a phantom count on the sidebar badge or a phantom entry in the close queue.
  function cleanupPanelState(id: string) {
    if (fileDirty.value[id]) {
      const next = { ...fileDirty.value }
      delete next[id]
      fileDirty.value = next
    }
    saveHandlers.delete(id)
    if (closeQueue.value.includes(id)) {
      closeQueue.value = closeQueue.value.filter(q => q !== id)
    }
  }

  const dirtyFileCount = computed(
    () => Object.values(fileDirty.value).filter(Boolean).length,
  )

  function tabBaseName(id: string): string {
    if (id.startsWith('file:')) return fileBaseName(id.slice('file:'.length))
    if (id.startsWith('preview:')) return fileBaseName(id.slice('preview:'.length))
    return api.value?.getPanel(id)?.api.title ?? id
  }

  // The tab the close-confirm dialog is currently asking about (head of queue).
  const pendingClose = computed<{ panelId: string, title: string } | null>(() => {
    const id = closeQueue.value[0]
    return id ? { panelId: id, title: tabBaseName(id) } : null
  })

  // User-initiated close. Clean tabs close at once; a dirty file is queued for
  // the close-confirm dialog instead of vanishing with its unsaved edits.
  function requestCloseTab(id: string) {
    if (!fileDirty.value[id]) {
      closeTab(id)
      return
    }
    if (!closeQueue.value.includes(id)) {
      closeQueue.value = [...closeQueue.value, id]
    }
  }

  // Batch close (close-others / close-all): clean tabs go immediately, dirty ones
  // queue up and the dialog walks them one at a time.
  function requestCloseTabs(ids: string[]) {
    const queued = [...closeQueue.value]
    for (const id of ids) {
      if (fileDirty.value[id]) {
        if (!queued.includes(id)) queued.push(id)
      } else {
        closeTab(id)
      }
    }
    closeQueue.value = queued
  }

  async function resolvePendingClose(action: 'save' | 'discard' | 'cancel') {
    const id = closeQueue.value[0]
    if (!id) return
    if (action === 'cancel') {
      // Cancel aborts the whole pending batch — matches VS Code.
      closeQueue.value = []
      return
    }
    if (action === 'save') {
      const handler = saveHandlers.get(id)
      const ok = handler ? await handler() : true
      // On save failure leave the tab open (the viewer surfaced the error); just
      // drop it from the queue so the dialog can move to the next one.
      if (ok) closeTab(id)
    } else {
      closeTab(id)
    }
    closeQueue.value = closeQueue.value.filter(q => q !== id)
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
      case 'preview':
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

  // ---- bottom panel actions -------------------------------------------------
  // Terminals use the standard dockview layout but always default to a group
  // at the bottom of the editor area, so the layout feels like a VS Code-style
  // bottom panel while remaining fully draggable and composable.

  function openTerminalInPanel() {
    openTerminal()
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
    fileDirty,
    dirtyFileCount,
    pendingClose,
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
    openPreview,
    openFilesAt,
    consumePendingFilesPath,
    openTerminal,
    openTerminalInPanel,
    openBrowser,
    openDisplay,
    splitGroup,
    closeTab,
    requestCloseTab,
    requestCloseTabs,
    resolvePendingClose,
    setFileDirty,
    registerFileSaveHandler,
    unregisterFileSaveHandler,
    updateBrowserAddress,
    resetBot,
    resetAll,
  }
})
