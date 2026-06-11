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

  it('marks file panels dirty through the tab title', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/notes/todo.md')
    const panel = dock.getPanel('file:/data/notes/todo.md')
    expect(panel?.title).toBe('todo.md')

    store.setFileDirty('file:/data/notes/todo.md', true)
    expect(panel?.title).toBe('● todo.md')

    store.setFileDirty('file:/data/notes/todo.md', false)
    expect(panel?.title).toBe('todo.md')
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
