import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatSelectionStore } from './chat-selection'
import { useWorkspaceTabsStore } from './workspace-tabs'

vi.mock('@/store/chat-list', () => ({
  useChatStore: () => ({
    sessionId: null,
    sessions: [],
    bots: [
      {
        id: 'bot-1',
        current_user_permissions: ['manage', 'workspace_exec', 'workspace_read'],
      },
    ],
    isSessionStreaming: vi.fn(() => false),
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

function createFakeDock() {
  const panels: FakePanel[] = []
  let activePanel: FakePanel | null = null
  const noopDisposable = () => ({ dispose: () => {} })

  const dock = {
    panels,
    get activePanel() {
      return activePanel
    },
    onDidActivePanelChange: noopDisposable,
    onDidLayoutChange: noopDisposable,
    onDidRemovePanel: noopDisposable,
    getPanel(id: string) {
      return panels.find((p) => p.id === id)
    },
    addPanel(options: { id: string; component: string; title?: string; params?: Record<string, unknown> }) {
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
    fromJSON() {},
  }
  return dock
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

  it('drops legacy persisted tab models', () => {
    localStorage.setItem('workspace-tabs', '{"bot-1":{"tabs":[]}}')
    localStorage.setItem('workspace-panes', '{"bot-1":{"panes":[]}}')
    useWorkspaceTabsStore()
    expect(localStorage.getItem('workspace-tabs')).toBeNull()
    expect(localStorage.getItem('workspace-panes')).toBeNull()
  })
})
