import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatSelectionStore } from './chat-selection'
import { useWorkspaceTabsStore } from './workspace-tabs'

const chatStoreMock = vi.hoisted(() => ({
  createNewSession: vi.fn(async () => {}),
}))

vi.mock('@/store/chat-list', () => ({
  useChatStore: () => ({
    sessionId: null,
    sessions: [],
    activeSession: null,
    bots: [
      {
        id: 'bot-1',
        current_user_permissions: ['manage', 'workspace_exec', 'workspace_read'],
      },
      {
        id: 'bot-without-layout',
        current_user_permissions: ['manage', 'workspace_exec', 'workspace_read'],
      },
    ],
    isSessionStreaming: vi.fn(() => false),
    createNewSession: chatStoreMock.createNewSession,
  }),
}))

interface FakePanel {
  id: string
  component: string
  params: Record<string, unknown>
  title: string
  group?: {
    api: {
      setActivePanel: (panel: FakePanel) => void
      setActive: () => void
    }
  }
  api: {
    setActive: () => void
    close: () => void
    setTitle: (title: string) => void
    updateParameters: (params: Record<string, unknown>) => void
    readonly title: string
  }
}

interface FakeAddPanelOptions {
  id: string
  component: string
  title?: string
  params?: Record<string, unknown>
  position?: { referenceGroup: string; direction: string }
}

function createFakeDock() {
  const panels: FakePanel[] = []
  const removeListeners: Array<(panel: FakePanel) => void> = []
  const addPanelCalls: FakeAddPanelOptions[] = []
  let activePanel: FakePanel | null = null
  const noopDisposable = () => ({ dispose: () => {} })
  const removeDisposable = (listener: (panel: FakePanel) => void) => {
    removeListeners.push(listener)
    return {
      dispose: () => {
        const idx = removeListeners.indexOf(listener)
        if (idx >= 0) removeListeners.splice(idx, 1)
      },
    }
  }
  const groupElement = {
    classList: {
      toggle: vi.fn(),
    },
    getBoundingClientRect: vi.fn(() => ({
      top: 0,
      left: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100,
    })),
  } as unknown as HTMLElement
  const group = {
    id: 'group-1',
    element: groupElement,
    api: {
      getHeaderPosition: () => 'top' as const,
      setHeaderPosition: vi.fn(),
    },
    get panels() {
      return panels
    },
    get activePanel() {
      return activePanel
    },
  }

  const dock = {
    panels,
    addPanelCalls,
    groups: [group],
    get activeGroup() {
      return group
    },
    get activePanel() {
      return activePanel
    },
    onDidActivePanelChange: noopDisposable,
    onDidLayoutChange: noopDisposable,
    onDidRemovePanel: removeDisposable,
    onDidAddPanel: noopDisposable,
    onDidMovePanel: noopDisposable,
    onWillShowOverlay: noopDisposable,
    onWillDrop: noopDisposable,
    onWillDragPanel: noopDisposable,
    onWillDragGroup: noopDisposable,
    getGroup(id: string) {
      return id === 'group-1' ? group : undefined
    },
    getPanel(id: string) {
      return panels.find((p) => p.id === id)
    },
    addPanel(options: FakeAddPanelOptions) {
      addPanelCalls.push(options)
      const panel: FakePanel = {
        id: options.id,
        component: options.component,
        params: { ...(options.params ?? {}) },
        title: options.title ?? '',
        api: {
          setActive: () => {
            activePanel = panel
          },
          close: () => {
            const idx = panels.indexOf(panel)
            if (idx >= 0) panels.splice(idx, 1)
            if (activePanel === panel) activePanel = panels[0] ?? null
            removeListeners.forEach(listener => listener(panel))
          },
          setTitle: (title: string) => {
            panel.title = title
          },
          updateParameters: (params: Record<string, unknown>) => {
            Object.assign(panel.params, params)
          },
          get title() {
            return panel.title
          },
        },
      }
      panel.group = {
        api: {
          setActivePanel: (nextPanel: FakePanel) => {
            activePanel = nextPanel
          },
          setActive: () => {},
        },
      }
      panels.push(panel)
      activePanel = panel
      return panel
    },
    clear() {
      panels.splice(0, panels.length)
      activePanel = null
    },
    toJSON() {
      return { fake: true }
    },
    fromJSON(data?: {
      panels?: Record<string, {
        id?: string
        contentComponent?: string
        title?: string
        params?: Record<string, unknown>
      }>
    }) {
      dock.clear()
      for (const [id, panel] of Object.entries(data?.panels ?? {})) {
        dock.addPanel({
          id: panel.id ?? id,
          component: panel.contentComponent ?? 'unknown',
          title: panel.title,
          params: panel.params,
        })
      }
    },
  }
  return dock
}

async function flushDraftChatFallback() {
  await nextTick()
  await new Promise(resolve => setTimeout(resolve, 0))
}

describe('workspace layout store', () => {
  beforeEach(() => {
    const storage = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    })
    chatStoreMock.createNewSession.mockClear()
    setActivePinia(createPinia())
    useChatSelectionStore().setBot('bot-1')
  })

  it('opens browser panels and updates their address', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openBrowser()

    const panel = dock.getPanel('browser:1')
    expect(panel).toBeTruthy()
    expect(panel?.component).toBe('browser')
    expect(panel?.params.address).toBe('localhost:5173/')

    store.updateBrowserAddress('browser:1', 'localhost:3000/app')
    expect(panel?.params.address).toBe('localhost:3000/app')
    expect(panel?.title).toBe('localhost:3000/app')
  })

  it('keeps terminal ids monotonic per bot', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openTerminal()
    store.openTerminal()
    store.closeTab('terminal:1')
    store.openTerminal()

    expect(dock.getPanel('terminal:1')).toBeUndefined()
    expect(dock.getPanel('terminal:2')).toBeTruthy()
    expect(dock.getPanel('terminal:3')).toBeTruthy()
  })

  it('duplicates the active file into a split pane with a unique panel id', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/notes/todo.md')
    store.splitGroup('group-1', 'right')

    expect(dock.getPanel('file:/data/notes/todo.md')).toBeTruthy()
    expect(dock.getPanel('file:/data/notes/todo.md~2')).toBeTruthy()
    expect(dock.panels).toHaveLength(2)
  })

  it('focuses the singleton chat panel instead of duplicating it', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openChat('First')
    store.openTerminal()
    store.openChat('Second')

    expect(dock.panels.filter((p) => p.component === 'chat')).toHaveLength(1)
    expect(dock.activePanel?.id).toBe('chat')

    store.setChatTitle('Renamed')
    expect(dock.getPanel('chat')?.title).toBe('Renamed')
  })

  it('keeps the final draft chat tab open when it is closed', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openChat('Existing')
    store.closeTab('chat')

    expect(dock.panels).toHaveLength(1)
    expect(dock.getPanel('chat')?.component).toBe('chat')
    expect(chatStoreMock.createNewSession).not.toHaveBeenCalled()
  })

  it('opens a draft chat when a non-chat tab leaves the dock empty', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openTerminal()
    store.closeTab('terminal:1')

    await flushDraftChatFallback()

    expect(chatStoreMock.createNewSession).toHaveBeenCalledOnce()
    expect(dock.panels).toHaveLength(1)
    expect(dock.getPanel('chat')?.component).toBe('chat')
  })

  it('opens a draft chat when switching to a bot without a saved layout', async () => {
    const selection = useChatSelectionStore()
    selection.setBot(null)
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    expect(dock.panels).toHaveLength(0)

    selection.setBot('bot-without-layout')
    await flushDraftChatFallback()

    expect(chatStoreMock.createNewSession).toHaveBeenCalledOnce()
    expect(dock.panels).toHaveLength(1)
    expect(dock.getPanel('chat')?.component).toBe('chat')
    expect(dock.getPanel('chat')?.title).toBe('New Session')
  })

  it('opens chat above a terminal-only group instead of joining it', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openTerminal()
    store.openChat('Chat')

    const chatOpen = dock.addPanelCalls.find(call => call.id === 'chat')
    expect(chatOpen?.position).toEqual({ referenceGroup: 'group-1', direction: 'above' })
  })

  it('sets a fallback title when focusing an existing blank chat tab', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    dock.addPanel({ id: 'chat', component: 'chat', title: '' })
    store.openChat()

    expect(dock.getPanel('chat')?.title).toBe('New Session')
  })

  it('keeps sidebar toggles usable after closing the last tab', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openChat('Chat')
    store.hideWorkbench()
    store.closeTab('chat')

    expect(dock.panels).toHaveLength(1)
    expect(dock.getPanel('chat')?.component).toBe('chat')
    expect(store.workbenchOpen).toBe(false)

    store.toggleWorkbench()
    expect(store.workbenchOpen).toBe(true)

    store.toggleBranchSidebar()
    expect(store.branchSidebarOpen).toBe(false)
  })

  it('splits the chat panel into a duplicate chat pane', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openChat('Session')
    store.splitGroup('group-1', 'right')

    expect(dock.getPanel('chat')).toBeTruthy()
    expect(dock.getPanel('chat~2')).toBeTruthy()
    expect(dock.getPanel('chat~2')?.component).toBe('chat')

    store.setChatTitle('Renamed')
    expect(dock.getPanel('chat')?.title).toBe('Renamed')
    expect(dock.getPanel('chat~2')?.title).toBe('Renamed')
  })

  it('opens multiple schedule panels and focuses an existing schedule', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSchedule('schedule-a', 'Morning')
    store.openSchedule('schedule-b', 'Evening')
    store.openSchedule('schedule-a', 'Morning renamed')

    expect(dock.panels.filter((p) => p.component === 'schedule')).toHaveLength(2)
    expect(dock.activePanel?.id).toBe('schedule:schedule-a')
    expect(dock.getPanel('schedule:schedule-a')?.title).toBe('Morning renamed')
  })

  it('tracks file dirty state without mangling the tab title', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/notes/todo.md')
    const panel = dock.getPanel('file:/data/notes/todo.md')
    expect(panel?.title).toBe('todo.md')

    store.setFileDirty('file:/data/notes/todo.md', true)
    expect(store.fileDirty['file:/data/notes/todo.md']).toBe(true)
    expect(store.dirtyFileCount).toBe(1)
    // Title stays the clean base name — the dot is now a tab-rendered affordance.
    expect(panel?.title).toBe('todo.md')

    store.setFileDirty('file:/data/notes/todo.md', false)
    expect(store.fileDirty['file:/data/notes/todo.md']).toBeUndefined()
    expect(store.dirtyFileCount).toBe(0)
  })

  it('queues a dirty tab for confirmation instead of closing it', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/a.md')
    store.setFileDirty('file:/data/a.md', true)

    // A dirty close is blocked; the tab is queued for the confirm dialog.
    store.requestCloseTab('file:/data/a.md')
    expect(dock.getPanel('file:/data/a.md')).toBeTruthy()
    expect(store.pendingClose?.panelId).toBe('file:/data/a.md')
    expect(store.pendingClose?.title).toBe('a.md')

    // Discard closes it and clears the queue.
    await store.resolvePendingClose('discard')
    expect(dock.getPanel('file:/data/a.md')).toBeUndefined()
    expect(store.pendingClose).toBeNull()
  })

  it('saves a dirty tab via its handler before closing', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/b.md')
    store.setFileDirty('file:/data/b.md', true)
    const save = vi.fn(async () => true)
    store.registerFileSaveHandler('file:/data/b.md', save)

    store.requestCloseTab('file:/data/b.md')
    await store.resolvePendingClose('save')

    expect(save).toHaveBeenCalledOnce()
    expect(dock.getPanel('file:/data/b.md')).toBeUndefined()
  })

  it('keeps a dirty tab open when its save fails', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/c.md')
    store.setFileDirty('file:/data/c.md', true)
    store.registerFileSaveHandler('file:/data/c.md', async () => false)

    store.requestCloseTab('file:/data/c.md')
    await store.resolvePendingClose('save')

    // Save failed → tab stays, but it leaves the queue so the dialog dismisses.
    expect(dock.getPanel('file:/data/c.md')).toBeTruthy()
    expect(store.pendingClose).toBeNull()
  })

  it('switches the active view and keeps the sidebar open', () => {
    const store = useWorkspaceTabsStore()

    store.sidebarView = 'sessions'
    store.sidebarOpen = true

    store.selectSidebarView('files')
    expect(store.sidebarView).toBe('files')
    expect(store.sidebarOpen).toBe(true)

    // Re-selecting the active view keeps the sidebar open instead of toggling it closed.
    store.selectSidebarView('files')
    expect(store.sidebarView).toBe('files')
    expect(store.sidebarOpen).toBe(true)
  })

  it('keeps activity panel collapse separate from whole workbench collapse', () => {
    const store = useWorkspaceTabsStore()

    store.sidebarView = 'sessions'
    store.sidebarOpen = true
    store.workbenchOpen = true

    store.toggleWorkbench()
    expect(store.workbenchOpen).toBe(false)
    expect(store.sidebarOpen).toBe(true)

    store.selectSidebarView('files')
    expect(store.workbenchOpen).toBe(true)
    expect(store.sidebarOpen).toBe(true)
    expect(store.sidebarView).toBe('files')

    store.hideWorkbench()
    expect(store.workbenchOpen).toBe(false)

    store.showWorkbench()
    expect(store.workbenchOpen).toBe(true)
  })

  it('keeps the branch history sidebar visible by default', () => {
    const store = useWorkspaceTabsStore()

    expect(store.branchSidebarOpen).toBe(true)
  })

  it('toggles the branch history sidebar', () => {
    const store = useWorkspaceTabsStore()

    store.toggleBranchSidebar()
    expect(store.branchSidebarOpen).toBe(false)

    store.toggleBranchSidebar()
    expect(store.branchSidebarOpen).toBe(true)
  })

  it('drops legacy persisted tab models', () => {
    localStorage.setItem('workspace-tabs', '{"bot-1":{"tabs":[]}}')
    localStorage.setItem('workspace-panes', '{"bot-1":{"panes":[]}}')
    useWorkspaceTabsStore()
    expect(localStorage.getItem('workspace-tabs')).toBeNull()
    expect(localStorage.getItem('workspace-panes')).toBeNull()
  })
})
