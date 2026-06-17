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
import type { OpenAssetPreviewArgs } from '@/pages/home/composables/useFileManagerProvider'

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

// Default share of the editor height the bottom terminal panel claims when it
// first splits off below the chat. ~1/3 mirrors VS Code's editor:panel ratio
// (≈554:269) — enough room to work in without burying the conversation.
const TERMINAL_PANEL_HEIGHT_RATIO = 1 / 3

export type WorkspacePanelComponent = 'chat' | 'file' | 'preview' | 'asset' | 'terminal' | 'browser' | 'display' | 'schedule'

interface BotLayoutState {
  layout: SerializedDockview | null
  terminalCounter: number
  browserCounter: number
  displayCounter: number
  scheduleCounter: number
}

type WorkspaceLayoutStorage = Record<string, BotLayoutState>

function emptyBotLayout(): BotLayoutState {
  return { layout: null, terminalCounter: 0, browserCounter: 0, displayCounter: 0, scheduleCounter: 0 }
}

function fileBaseName(filePath: string): string {
  const idx = filePath.lastIndexOf('/')
  return idx >= 0 ? filePath.slice(idx + 1) : filePath
}

function panelComponentOf(id: string): WorkspacePanelComponent | null {
  if (id === CHAT_PANEL_ID) return 'chat'
  if (id.startsWith(`${CHAT_PANEL_ID}~`)) return 'chat'
  if (id.startsWith('file:')) return 'file'
  if (id.startsWith('preview:')) return 'preview'
  if (id.startsWith('asset:')) return 'asset'
  if (id.startsWith('terminal:')) return 'terminal'
  if (id.startsWith('browser:')) return 'browser'
  if (id.startsWith('display:')) return 'display'
  if (id.startsWith('schedule:')) return 'schedule'
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
  // True while ANY panel/group is being dragged. On the desktop shell the blank
  // header strip (the growing void container) doubles as a window-drag region
  // (`-webkit-app-region: drag`), which swallows native drag-and-drop events so a
  // tab can't land on the blank part of another group's tab bar. chat-workspace
  // mirrors this onto the dock root as `.memoh-dock-dragging`, and the theme
  // flips the void to `no-drag` while it's set so the whole header accepts drops,
  // then back to a window handle on drag end. Also gates the terminal "no-drop"
  // cursor below.
  const panelDragging = ref(false)
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
  // Whether the in-flight drag started from a terminal. Non-terminal drags lock
  // the exclusive terminal groups (see beginDrag) so an editor can never merge
  // into the bottom panel; terminal drags leave them open so terminals can be
  // reordered or stacked. Reset on drag end.
  let dragSourceTerminal = false
  // The most recent NON-terminal group to hold focus. A terminal group is
  // exclusive — opening a file/browser/etc. while it is the active group must
  // land in an editor group, not contaminate the terminals — so we remember the
  // last editor group to route those opens back to (see nonTerminalTarget).
  let lastNonTerminalGroupId: string | null = null

  function ensureBotLayout(botId: string | null | undefined): BotLayoutState | null {
    const bid = (botId ?? '').trim()
    if (!bid) return null
    if (!storage.value[bid]) {
      storage.value = { ...storage.value, [bid]: emptyBotLayout() }
    }
    const state = storage.value[bid]
    if (state && typeof state.scheduleCounter !== 'number') {
      storage.value = { ...storage.value, [bid]: { ...emptyBotLayout(), ...state, scheduleCounter: 0 } }
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
        // Defensively strip `locked`: older builds toggled 'no-drop-target' on
        // terminal groups mid-drag and a layout-change persist could bake it
        // into storage, permanently sealing the group on the next restore. Drop
        // rejection is now an onWillDrop veto (see registerApi), so we never set
        // it — this just scrubs any value an old snapshot still carries.
        delete data.locked
        return { ...node, data }
      }
      if (node.type === 'branch' && Array.isArray(node.data)) {
        return { ...node, data: (node.data as GridNode[]).map(walkNode) }
      }
      return node
    }
    // Strip any persisted per-panel tabComponent (older layouts pinned terminals
    // to 'terminalTab'): terminals now resolve their tab through the default host
    // by group, so a baked component would override that and keep a moved-out
    // terminal stuck as a chip.
    function stripTabComponents(panels: SerializedDockview['panels']): SerializedDockview['panels'] {
      if (!panels || typeof panels !== 'object') return panels
      const out: Record<string, unknown> = {}
      for (const [id, panel] of Object.entries(panels)) {
        if (panel && typeof panel === 'object' && 'tabComponent' in panel) {
          const { tabComponent: _tabComponent, ...rest } = panel as unknown as Record<string, unknown>
          out[id] = rest
        } else {
          out[id] = panel
        }
      }
      return out as SerializedDockview['panels']
    }
    try {
      return {
        ...layout,
        grid: {
          ...layout.grid,
          root: walkNode(layout.grid.root as GridNode) as SerializedDockview['grid']['root'],
        },
        panels: stripTabComponents(layout.panels),
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

  function focusPanel(panel: unknown) {
    (panel as { api?: { setActive?: () => void } }).api?.setActive?.()
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

  function domListener<K extends keyof DocumentEventMap>(
    type: K,
    handler: (event: DocumentEventMap[K]) => void,
    options?: AddEventListenerOptions,
  ): { dispose(): void } {
    if (typeof document === 'undefined') return { dispose() {} }
    document.addEventListener(type, handler as EventListener, options)
    return {
      dispose() {
        document.removeEventListener(type, handler as EventListener, options)
      },
    }
  }

  // A terminal group is exclusive: an editor (file/preview/chat/…) can never
  // become a tab inside it, mirroring how a VS Code terminal panel refuses
  // editor drops. We enforce this per drag with onWillShowOverlay (veto the
  // drop overlay) and onWillDrop (veto the drop) — see registerApi. Preventing
  // the overlay routes through dockview's removeDropTarget, so no stale
  // "can-drop" highlight lingers over the terminal. The group-level `locked`
  // flag can't do that: it makes canDisplayOverlay bail out early and the
  // shared overlay element is left untouched, so the highlight from the group
  // hovered just before stays painted. beginDrag only records WHAT is being
  // dragged so those vetoes can tell a foreign editor from a terminal session
  // being reordered.
  function beginDrag(sourceTerminal: boolean) {
    panelDragging.value = true
    dragSourceTerminal = sourceTerminal
  }

  function endDrag() {
    panelDragging.value = false
    dragSourceTerminal = false
  }

  // Is the in-flight drag a NON-terminal panel/group? A single-tab drag carries
  // its panelId; a whole-group drag has a null panelId, so we fall back to the
  // terminal-vs-editor classification recorded at drag start.
  function draggingNonTerminal(
    event: { getData(): { panelId?: string | null } | undefined },
  ): boolean {
    const panelId = event.getData()?.panelId
    if (typeof panelId === 'string') return !panelId.startsWith('terminal:')
    return !dragSourceTerminal
  }

  function registerApi(dock: DockviewApi) {
    for (const d of apiDisposables) d.dispose()
    api.value = dock
    apiDisposables = [
      dock.onDidActivePanelChange((panel) => {
        activePanelId.value = panel?.id ?? null
        const group = panel?.group
        if (group && !isTerminalOnlyGroup(group)) {
          lastNonTerminalGroupId = group.id
        }
      }),
      dock.onDidLayoutChange(() => {
        // Catch-all: keep terminal-group chrome (bottom header + class) in
        // lockstep with composition for ANY layout mutation, including drags
        // dockview may not surface through onDidMovePanel. The per-tab host
        // also keys off layout changes, so syncing here guarantees the group
        // chrome and the chip-vs-normal tab never disagree. setHeaderPosition
        // is guarded by a current-value check, so this cannot loop. Sync first
        // so the persisted snapshot already reflects the resolved chrome.
        syncAllTerminalGroupChrome(dock)
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
      // Record what is being dragged (a terminal session vs a foreign editor)
      // and flip the desktop window-drag region off so blank header space
      // accepts drops.
      dock.onWillDragPanel((event) => {
        beginDrag(event.panel.id.startsWith('terminal:'))
      }),
      dock.onWillDragGroup((event) => {
        beginDrag(isTerminalOnlyGroup(event.group))
      }),
      // Terminal groups are exclusive. Veto the drop overlay so no "can-drop"
      // highlight ever paints over the terminal, and veto the drop itself,
      // whenever a foreign editor is dragged onto one (header, tab strip or
      // content, any edge). Terminal sessions stay droppable among themselves.
      dock.onWillShowOverlay((event) => {
        if (isTerminalOnlyGroup(event.group) && draggingNonTerminal(event)) {
          event.preventDefault()
        }
      }),
      dock.onWillDrop((event) => {
        if (isTerminalOnlyGroup(event.group) && draggingNonTerminal(event)) {
          event.preventDefault()
        }
      }),
      // Clear the drag flags however the drag ends (drop, cancel, Esc). Native
      // 'dragend' always fires for the HTML5 backend; 'pointerup' covers the
      // pointer/touch backend.
      domListener('dragend', () => endDrag(), { capture: true }),
      domListener('pointerup', () => endDrag(), { capture: true }),
      // The vetoes above already refuse foreign editors, but dockview still
      // preventDefaults the native dragover, so the OS would otherwise paint a
      // droppable cursor. Force the "no-drop" cursor over the terminal group
      // while a non-terminal is in flight. Capture phase so this runs before
      // dockview's own dragover sets the effect.
      domListener('dragover', (event) => {
        if (!panelDragging.value || dragSourceTerminal) return
        const target = event.target
        if (!(target instanceof Element)) return
        if (!target.closest('.memoh-terminal-group')) return
        if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
      }, { capture: true }),
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
    panelDragging.value = false
    dragSourceTerminal = false
  }

  // ---- panel operations ----------------------------------------------------

  const activeId = computed<string | null>(() => activePanelId.value)

  function hasCurrentPermission(permission: BotPermission): boolean {
    return hasBotPermission(currentBot.value?.current_user_permissions, permission)
  }

  // Resolve a group an EDITOR-class panel may join. A terminal-only group is
  // exclusive, so we never let a non-terminal panel land in one: prefer the
  // caller's group (if not a terminal group), then the active group, then the
  // last editor group to hold focus, then any non-terminal group. If only
  // terminal groups exist, open ABOVE one so the panel gets its own group
  // instead of joining the terminals. Undefined → let dockview create the first
  // group (empty dock).
  function nonTerminalTarget(
    dock: DockviewApi,
    groupId?: string,
  ): { referenceGroup: string, direction: 'within' | 'above' } | undefined {
    const explicit = groupId ? dock.getGroup(groupId) : undefined
    if (explicit && !isTerminalOnlyGroup(explicit)) {
      return { referenceGroup: explicit.id, direction: 'within' }
    }
    const active = dock.activeGroup
    if (active && !isTerminalOnlyGroup(active)) {
      return { referenceGroup: active.id, direction: 'within' }
    }
    if (lastNonTerminalGroupId) {
      const last = dock.getGroup(lastNonTerminalGroupId)
      if (last && !isTerminalOnlyGroup(last)) {
        return { referenceGroup: last.id, direction: 'within' }
      }
    }
    const other = dock.groups.find(g => !isTerminalOnlyGroup(g))
    if (other) return { referenceGroup: other.id, direction: 'within' }
    const fallback = dock.groups[0]
    if (fallback) return { referenceGroup: fallback.id, direction: 'above' }
    return undefined
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
      focusPanel(existing)
      return true
    }
    const target = nonTerminalTarget(dock, options.groupId)
    dock.addPanel({
      id: options.id,
      component: options.component,
      title: options.title,
      params: options.params,
      // Keep panel DOM mounted while hidden: terminals, WebRTC display and
      // chat scroll state do not survive detach/reattach cycles.
      renderer: 'always',
      ...(target ? { position: target } : {}),
    })
    return true
  }

  function defaultTerminalPosition(dock: DockviewApi, belowGroupId?: string) {
    // Reuse the bottom terminal panel (a terminal-ONLY group) if one exists, so a
    // new session stacks as another tab instead of spawning a parallel panel. We
    // match terminal-ONLY (not "has a terminal"): a terminal dragged up into the
    // chat group lives in a MIXED group, which is no longer the bottom panel.
    const terminalGroupId = dock.groups.find(g => isTerminalOnlyGroup(g))?.id
    if (terminalGroupId) {
      return { referenceGroup: terminalGroupId, direction: 'within' as const }
    }
    // No bottom panel yet: open one directly BELOW the column the request came
    // from — the editor group whose "+" was clicked, else the active editor
    // group, else the chat group. This is why a terminal opened from the RIGHT
    // split now appears under the right split instead of jumping back under the
    // chat on the left.
    const below
      = (belowGroupId ? dock.getGroup(belowGroupId) : undefined)
        ?? (dock.activeGroup && !isTerminalOnlyGroup(dock.activeGroup) ? dock.activeGroup : undefined)
        ?? dock.groups.find(g => g.panels.some(p => p.id === CHAT_PANEL_ID))
    if (below && !isTerminalOnlyGroup(below)) {
      return { referenceGroup: below.id, direction: 'below' as const }
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
    // No per-panel tabComponent: terminals use the default tab host, which
    // renders the file-chip tab ONLY while the panel sits in a terminal-only
    // group and falls back to the normal editor tab once a terminal is dragged
    // into a mixed group — so a moved-out terminal blends into the dock strip.
    const panelBase = {
      id: options.id,
      component: 'terminal' as const,
      title: options.title,
      renderer: 'always' as const,
    }
    const position = options.groupId
      ? { referenceGroup: options.groupId, direction: 'within' as const }
      : options.position
    const panel = dock.addPanel({
      ...panelBase,
      ...(position ? { position } : {}),
    })
    // Only when the terminal opens its OWN new group BELOW the chat: give that
    // group a sensible default height instead of dockview's even 50/50 split.
    // Joining an existing terminal/editor group keeps whatever height it has.
    if (position?.direction === 'below' && dock.height > 0) {
      panel.group.api.setSize({
        height: Math.round(dock.height * TERMINAL_PANEL_HEIGHT_RATIO),
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
    const dock = api.value
    if (!dock) return
    for (const panel of dock.panels) {
      if (panel.id !== CHAT_PANEL_ID && !panel.id.startsWith(`${CHAT_PANEL_ID}~`)) continue
      if (panel.api.title !== title) {
        panel.api.setTitle(title)
      }
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
      focusPanel(existing)
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

  // Open or focus a tab that renders a message attachment (a stored media asset).
  // Unlike openFile/openPreview this is NOT a workspace path: the tab re-resolves
  // its source from the content hash (or falls back to a direct URL), so it works
  // for the user's own uploads without a workspace_read grant. Lands in the editor
  // area like a file tab; refocuses an existing tab for the same asset.
  function openAsset(args: OpenAssetPreviewArgs) {
    const dock = api.value
    if (!dock) return
    const key = (args.key ?? '').trim()
    if (!key) return
    const id = `asset:${key}`
    const existing = dock.getPanel(id)
    if (existing) {
      existing.api.setActive()
      return
    }
    const target = nonTerminalTarget(dock)
    dock.addPanel({
      id,
      component: 'asset',
      title: args.name || 'file',
      params: {
        name: args.name,
        botId: args.botId,
        contentHash: args.contentHash,
        src: args.src,
      },
      renderer: 'always',
      ...(target ? { position: target } : {}),
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
    // The "+" lives per group. Fired from a terminal-only group, the new session
    // JOINS it (another tab in the bottom panel). Fired from an editor group's
    // "+" menu — or opened programmatically — it lands in the bottom terminal
    // panel, which opens directly below the INITIATING editor column when none
    // exists yet (see defaultTerminalPosition). Passing a non-terminal groupId
    // straight through would wrongly merge the terminal into that editor group.
    const initiating = groupId ? dock.getGroup(groupId) : undefined
    const joinTerminalGroup = !!initiating && isTerminalOnlyGroup(initiating)
    addTerminalPanel({
      id: `terminal:${next}`,
      title: 'zsh',
      groupId: joinTerminalGroup ? groupId : undefined,
      position: joinTerminalGroup ? undefined : defaultTerminalPosition(dock, groupId),
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
      focusPanel(existing)
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
      case 'schedule': {
        if (!hasCurrentPermission('manage')) return
        dock.addPanel({
          id: uniqueSplitPanelId(source.id),
          component: 'schedule',
          title,
          params: params ? { ...params } : undefined,
          renderer: 'always',
          position,
        })
        break
      }
      case 'asset': {
        dock.addPanel({
          id: uniqueSplitPanelId(source.id),
          component: 'asset',
          title,
          params: params ? { ...params } : undefined,
          renderer: 'always',
          position,
        })
        break
      }
      case 'chat':
        dock.addPanel({
          id: uniqueSplitPanelId(source.id),
          component: 'chat',
          title,
          renderer: 'always',
          position,
        })
        break
    }
  }

  function openSchedule(scheduleId?: string, title?: string, groupId?: string) {
    if (!hasCurrentPermission('manage')) return
    const bid = (currentBotId.value ?? '').trim()
    if (!bid) return
    const panelTitle = title?.trim() || 'Schedule'
    const state = ensureBotLayout(bid)
    if (!state) return
    const panelId = scheduleId
      ? `schedule:${scheduleId}`
      : `schedule:new:${state.scheduleCounter + 1}`
    const panel = api.value?.getPanel(panelId)
    if (panel) {
      panel.api.setTitle(panelTitle)
      focusPanel(panel)
      return
    }
    if (!scheduleId) {
      patchBotLayout(bid, { scheduleCounter: state.scheduleCounter + 1 })
    }
    focusOrAdd({
      id: panelId,
      component: 'schedule',
      title: panelTitle,
      params: { scheduleId },
      groupId,
    })
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
      case 'asset':
        // A message attachment is the user's own content, not a workspace file —
        // viewing it never requires the workspace_read grant.
        return true
      case 'terminal':
        return hasCurrentPermission('workspace_exec')
      case 'browser':
      case 'display':
      case 'schedule':
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

  // groupId is the header strip that triggered the "+" (the terminal group's own
  // bar). Passing it keeps the new session in THAT group; without it (editor "+"
  // menu) the terminal routes to the default bottom slot.
  function openTerminalInPanel(groupId?: string) {
    openTerminal(groupId)
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
    panelDragging,
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
    openAsset,
    openFilesAt,
    consumePendingFilesPath,
    openTerminal,
    openTerminalInPanel,
    openBrowser,
    openDisplay,
    splitGroup,
    openSchedule,
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
