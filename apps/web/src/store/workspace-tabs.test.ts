import { nextTick, reactive, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatSelectionStore } from './chat-selection'
import { useWorkspaceTabsStore } from './workspace-tabs'

// workspace-tabs imports @/i18n to localize the desktop panel title; that
// module reads localStorage at load to pick the initial locale, which isn't
// polyfilled in this test's environment. Mock it so the import stays
// side-effect free here.
vi.mock('@/i18n', () => ({
  default: { global: { t: (key: string) => key } },
}))

const chatStoreMock = vi.hoisted(() => ({
  sessionId: null as string | null,
  hasExplicitSessionSelection: false,
  sessions: [] as Array<{ id: string, title?: string }>,
  knownSessions: [] as Array<{ id: string, title?: string, type?: string }>,
  createNewSession: vi.fn(async () => {}),
  deletedSession: undefined as unknown as ReturnType<typeof ref<{ id: string, botId: string, seq: number } | null>>,
  selectSession: vi.fn(async (sessionId: string) => {
    chatStoreMock.sessionId = sessionId
    chatStoreMock.hasExplicitSessionSelection = true
  }),
  selectDraft: vi.fn((options?: { explicitSelection?: boolean }) => {
    chatStoreMock.sessionId = null
    chatStoreMock.hasExplicitSessionSelection = options?.explicitSelection === true
  }),
  knownSessionSummary: vi.fn((sessionId: string) =>
    chatStoreMock.knownSessions.find(session => session.id === sessionId)
    ?? chatStoreMock.sessions.find(session => session.id === sessionId)
    ?? null,
  ),
}))

vi.mock('@/store/chat-list', () => ({
  useChatStore: () => ({
    get sessionId() {
      return chatStoreMock.sessionId
    },
    get hasExplicitSessionSelection() {
      return chatStoreMock.hasExplicitSessionSelection
    },
    sessions: chatStoreMock.sessions,
    knownSessions: chatStoreMock.knownSessions,
    loadingChats: false,
    activeSession: null,
    userSentInSession: null,
    get deletedSession() {
      return chatStoreMock.deletedSession.value
    },
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
    knownSessionSummary: chatStoreMock.knownSessionSummary,
    createNewSession: chatStoreMock.createNewSession,
    selectSession: chatStoreMock.selectSession,
    selectDraft: chatStoreMock.selectDraft,
  }),
}))

interface FakePanel {
  id: string
  component: string
  params: Record<string, unknown>
  title: string
  group?: FakeGroup
  api: {
    setActive: () => void
    close: () => void
    setTitle: (title: string) => void
    updateParameters: (params: Record<string, unknown>) => void
    readonly title: string
  }
}

interface FakeGroup {
  id: string
  element: HTMLElement
  api: {
    getHeaderPosition: () => 'top' | 'bottom'
    setHeaderPosition: ReturnType<typeof vi.fn>
    setActivePanel: (panel: FakePanel) => void
    setActive: () => void
    setSize: ReturnType<typeof vi.fn>
  }
  readonly panels: FakePanel[]
  readonly activePanel: FakePanel | null
}

function createFakeDock() {
  const panels: FakePanel[] = []
  const removeListeners: Array<(panel: FakePanel) => void> = []
  const activePanelListeners: Array<(panel: FakePanel | null) => void> = []
  const layoutListeners: Array<() => void> = []
  const closeVetoPanelIds = new Set<string>()
  let activePanel: FakePanel | null = null
  const setActivePanel = (panel: FakePanel | null) => {
    activePanel = panel
    activePanelListeners.forEach(listener => listener(panel))
  }
  const noopDisposable = () => ({ dispose: () => {} })
  const layoutDisposable = (listener: () => void) => {
    layoutListeners.push(listener)
    return {
      dispose: () => {
        const idx = layoutListeners.indexOf(listener)
        if (idx >= 0) layoutListeners.splice(idx, 1)
      },
    }
  }
  const emitLayoutChange = () => {
    queueMicrotask(() => {
      layoutListeners.forEach(listener => listener())
    })
  }
  const activePanelDisposable = (listener: (panel: FakePanel | null) => void) => {
    activePanelListeners.push(listener)
    return {
      dispose: () => {
        const idx = activePanelListeners.indexOf(listener)
        if (idx >= 0) activePanelListeners.splice(idx, 1)
      },
    }
  }
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
  } as unknown as HTMLElement
  const group = {
    id: 'group-1',
    element: groupElement,
    api: {
      getHeaderPosition: () => 'top' as const,
      setHeaderPosition: vi.fn(),
      setActivePanel: (panel: FakePanel) => setActivePanel(panel),
      setActive: () => {},
      setSize: vi.fn(),
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
    groups: [group],
    get activePanel() {
      return activePanel
    },
    get activeGroup() {
      return activePanel?.group ?? group
    },
    onDidActivePanelChange: activePanelDisposable,
    onDidLayoutChange: layoutDisposable,
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
    addPanel(options: {
      id: string
      component: string
      title?: string
      params?: Record<string, unknown>
      position?: { referenceGroup: string; direction: string }
    }) {
      const panel: FakePanel = {
        id: options.id,
        component: options.component,
        params: { ...(options.params ?? {}) },
        title: options.title ?? '',
        api: {
          setActive: () => {
            setActivePanel(panel)
          },
          close: () => {
            if (closeVetoPanelIds.has(panel.id)) return
            const idx = panels.indexOf(panel)
            if (idx >= 0) panels.splice(idx, 1)
            removeListeners.forEach(listener => listener(panel))
            if (activePanel === panel) setActivePanel(panels[0] ?? null)
            emitLayoutChange()
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
      panel.group = group
      panels.push(panel)
      setActivePanel(panel)
      emitLayoutChange()
      return panel
    },
    clear() {
      panels.splice(0, panels.length)
      setActivePanel(null)
      emitLayoutChange()
    },
    vetoCloseUntilAllowed(id: string) {
      closeVetoPanelIds.add(id)
    },
    allowClose(id: string) {
      closeVetoPanelIds.delete(id)
    },
    activePanelListenerCount() {
      return activePanelListeners.length
    },
    toJSON() {
      return {
        grid: { root: { type: 'leaf', data: {} } },
        panels: Object.fromEntries(panels.map(panel => [
          panel.id,
          {
            id: panel.id,
            contentComponent: panel.component,
            title: panel.title,
            params: { ...panel.params },
          },
        ])),
      }
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
  afterEach(() => {
    useWorkspaceTabsStore().releaseApi()
  })

  beforeEach(() => {
    const storage = new Map<string, string>()
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
      clear: () => storage.clear(),
    })
    chatStoreMock.createNewSession.mockClear()
    chatStoreMock.selectSession.mockClear()
    chatStoreMock.selectDraft.mockClear()
    chatStoreMock.deletedSession = ref(null)
    chatStoreMock.sessionId = null
    chatStoreMock.hasExplicitSessionSelection = false
    chatStoreMock.sessions = reactive([]) as typeof chatStoreMock.sessions
    chatStoreMock.knownSessions = reactive([]) as typeof chatStoreMock.knownSessions
    chatStoreMock.knownSessionSummary.mockImplementation((sessionId: string) =>
      chatStoreMock.knownSessions.find(session => session.id === sessionId)
      ?? chatStoreMock.sessions.find(session => session.id === sessionId)
      ?? null,
    )
    setActivePinia(createPinia())
    useChatSelectionStore().setBot('bot-1')
    chatStoreMock.selectSession.mockImplementation(async (sessionId: string) => {
      useChatSelectionStore().setSession(sessionId)
    })
    chatStoreMock.selectDraft.mockImplementation(() => {
      useChatSelectionStore().setSession(null)
    })
  })

  function emitDeletedSession(id: string, botId = 'bot-1') {
    const prevSeq = chatStoreMock.deletedSession.value?.seq ?? 0
    chatStoreMock.deletedSession.value = { id, botId, seq: prevSeq + 1 }
  }

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

  it('opens a browser tab at an address and focuses the existing one on the same URL', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    // First click normalizes the address and titles the tab with it.
    expect(store.openBrowserAt('http://localhost:5173/app')).toBe(true)
    const first = dock.getPanel('browser:1')
    expect(first?.params.address).toBe('localhost:5173/app')
    expect(first?.title).toBe('localhost:5173/app')

    // A different path on the same port opens a second tab.
    expect(store.openBrowserAt('127.0.0.1:5173/other')).toBe(true)
    expect(dock.getPanel('browser:2')?.params.address).toBe('localhost:5173/other')

    // The exact same URL (after normalization) focuses the existing tab.
    expect(store.openBrowserAt('localhost:5173/app')).toBe(true)
    expect(dock.panels.filter(p => p.component === 'browser')).toHaveLength(2)
    expect(dock.activePanel?.id).toBe('browser:1')
  })

  it('refuses to open a browser tab for a non-local address', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    expect(store.openBrowserAt('https://example.com/')).toBe(false)
    expect(dock.panels.filter(p => p.component === 'browser')).toHaveLength(0)
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

  it('focuses the single draft chat tab instead of duplicating it', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openDraftChat({ title: 'First' })
    store.openTerminal()
    store.openDraftChat({ title: 'Second' })

    expect(dock.panels.filter((p) => p.component === 'chat')).toHaveLength(1)
    expect(dock.activePanel?.component).toBe('chat')
  })

  it('marks sidebar-created draft chats as non-explicit so default ACP can stage', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openDraftChat({ title: 'New Session', explicitSelection: false })

    const draft = dock.panels.find((p) => p.component === 'chat')
    expect(draft?.params).toMatchObject({ sessionId: null, explicitSelection: false })

    store.openTerminal()
    store.openDraftChat({ title: 'New Session', explicitSelection: false })

    expect(chatStoreMock.selectDraft).toHaveBeenLastCalledWith({ explicitSelection: false })
  })

  it('does not let restored draft panel params clear an explicit empty composer', async () => {
    const staleLayout = {
      panels: {
        draft: {
          id: 'chat:bot-1:1',
          contentComponent: 'chat',
          title: 'New Session',
          params: { sessionId: null, explicitSelection: false },
        },
      },
    }
    localStorage.setItem('workspace-layout', JSON.stringify({
      'bot-1': {
        layout: staleLayout,
        ephemeralIds: ['chat:bot-1:1'],
      },
    }))
    chatStoreMock.hasExplicitSessionSelection = true

    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)
    await nextTick()

    expect(chatStoreMock.selectDraft).toHaveBeenCalledWith({ explicitSelection: true })
    expect(chatStoreMock.hasExplicitSessionSelection).toBe(true)
  })

  it('opens one chat tab per session and reuses the ephemeral slot', () => {
    chatStoreMock.sessions.push(
      { id: 's1', title: 'S1' },
      { id: 's2', title: 'S2' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'S1' })
    const firstId = dock.panels.find((p) => p.component === 'chat')!.id

    // Same session: focus the existing tab, no new panel.
    store.openSessionChat({ sessionId: 's1', title: 'S1' })
    expect(dock.panels.filter((p) => p.component === 'chat')).toHaveLength(1)

    // Different session: repoint the group's ephemeral chat slot in place.
    store.openSessionChat({ sessionId: 's2', title: 'S2' })
    const chatPanels = dock.panels.filter((p) => p.component === 'chat')
    expect(chatPanels).toHaveLength(1)
    expect(chatPanels[0]!.id).toBe(firstId)
    expect(chatPanels[0]!.params.sessionId).toBe('s2')
  })

  it('reuses an ephemeral chat tab even when its session is off the current page', () => {
    chatStoreMock.sessions.push({ id: 's2', title: 'S2' })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 'older-session', title: 'Older' })
    const firstId = dock.panels.find((p) => p.component === 'chat')!.id

    store.openSessionChat({ sessionId: 's2', title: 'S2' })

    const chatPanels = dock.panels.filter((p) => p.component === 'chat')
    expect(chatPanels).toHaveLength(1)
    expect(chatPanels[0]!.id).toBe(firstId)
    expect(chatPanels[0]!.params.sessionId).toBe('s2')
  })

  it('uses remembered hidden session summaries for subagent chat tabs', () => {
    chatStoreMock.knownSessionSummary.mockImplementation((sessionId: string) => {
      if (sessionId === 'subagent-1') {
        return {
          id: 'subagent-1',
          title: 'Subagent task',
          type: 'subagent',
        }
      }
      return chatStoreMock.sessions.find(session => session.id === sessionId) ?? null
    })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 'subagent-1' })

    const chatPanel = dock.panels.find(panel => panel.component === 'chat')
    expect(chatPanel?.params.sessionId).toBe('subagent-1')
    expect(chatPanel?.title).toBe('Subagent task')
  })

  it('updates open hidden subagent tab titles when their summary is remembered later', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 'subagent-1', title: 'agent-a' })
    const chatPanel = dock.panels.find(panel => panel.component === 'chat')!
    expect(chatPanel.title).toBe('agent-a')

    chatStoreMock.knownSessions.push({
      id: 'subagent-1',
      title: 'Fetched subagent task',
      type: 'subagent',
    })
    await nextTick()

    expect(dock.getPanel(chatPanel.id)?.title).toBe('Fetched subagent task')
  })

  it('does not reconcile remembered hidden subagent chat tabs as deleted', async () => {
    chatStoreMock.knownSessionSummary.mockImplementation((sessionId: string) => {
      if (sessionId === 'subagent-1') {
        return {
          id: 'subagent-1',
          title: 'Subagent task',
          type: 'subagent',
        }
      }
      return chatStoreMock.sessions.find(session => session.id === sessionId) ?? null
    })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)
    store.openSessionChat({ sessionId: 'subagent-1' })
    const chatPanel = dock.panels.find(panel => panel.component === 'chat')!

    chatStoreMock.sessions.push({ id: 'parent-1', title: 'Parent' })
    await nextTick()

    expect(dock.getPanel(chatPanel.id)?.params.sessionId).toBe('subagent-1')
    expect(chatStoreMock.selectDraft).not.toHaveBeenCalled()
  })

  it('closes the active deleted chat tab and opens the newly selected session', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push(
      { id: 's1', title: 'Deleted session' },
      { id: 's2', title: 'Next session' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const chatPanel = dock.panels.find(panel => panel.component === 'chat')!

    emitDeletedSession('s1')
    chatStoreMock.sessions.splice(0, chatStoreMock.sessions.length, { id: 's2', title: 'Next session' })
    selection.setSession('s2')
    await nextTick()

    expect(dock.getPanel(chatPanel.id)).toBeUndefined()
    const nextChatPanel = dock.panels.find(panel => panel.component === 'chat')
    expect(nextChatPanel?.params.sessionId).toBe('s2')
    expect(nextChatPanel?.title).toBe('Next session')
    expect(chatStoreMock.selectDraft).not.toHaveBeenCalled()
  })

  it('does not let deleted-tab auto-activation override the fallback session', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push(
      { id: 's1', title: 'Deleted session' },
      { id: 's2', title: 'Fallback session' },
      { id: 's3', title: 'Neighbor session' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat' && panel.params.sessionId === 's1')!
    store.pinPanel(deletedPanel.id)
    store.openSessionChat({ sessionId: 's3', title: 'Neighbor session' })
    deletedPanel.api.setActive()
    expect(selection.sessionId).toBe('s1')
    chatStoreMock.selectSession.mockClear()

    chatStoreMock.sessions.splice(0, chatStoreMock.sessions.length,
      { id: 's2', title: 'Fallback session' },
      { id: 's3', title: 'Neighbor session' },
    )
    emitDeletedSession('s1')
    selection.setSession('s2')
    await nextTick()
    await nextTick()

    expect(dock.getPanel(deletedPanel.id)).toBeUndefined()
    expect(selection.sessionId).toBe('s2')
    expect(chatStoreMock.selectSession).not.toHaveBeenCalledWith('s3')
    expect(dock.activePanel?.params.sessionId).toBe('s2')
    expect(chatStoreMock.selectDraft).not.toHaveBeenCalled()
  })

  it('keeps open chat tabs when a paginated session list refresh drops their id', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push(
      { id: 's1', title: 'Open older session' },
      { id: 's2', title: 'Other session' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Open older session' })
    const openPanel = dock.panels.find(panel => panel.component === 'chat')!

    chatStoreMock.sessions.splice(0, chatStoreMock.sessions.length,
      { id: 's2', title: 'Other session' },
      { id: 's3', title: 'Replacement page item' },
    )
    await nextTick()

    expect(dock.getPanel(openPanel.id)?.params.sessionId).toBe('s1')
    expect(chatStoreMock.selectDraft).not.toHaveBeenCalled()
  })

  it('buffers deleted-session signals for inactive bots and applies them after switching back', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push({ id: 's1', title: 'Deleted session' })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat')!

    selection.setBot('bot-without-layout')
    emitDeletedSession('s1', 'bot-1')
    await nextTick()
    expect(dock.getPanel(deletedPanel.id)).toBeUndefined()

    selection.setBot('bot-1')
    await nextTick()
    await nextTick()

    expect(dock.getPanel(deletedPanel.id)).toBeUndefined()
    expect(dock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's1')).toBe(false)
  })

  it('keeps buffered delete reconciliation across dock api re-registration', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push({ id: 's1', title: 'Deleted session' })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat')!

    selection.setBot('bot-without-layout')
    emitDeletedSession('s1', 'bot-1')
    await nextTick()
    store.releaseApi()
    expect(dock.activePanelListenerCount()).toBe(0)

    const nextDock = createFakeDock()
    store.registerApi(nextDock as never)
    selection.setBot('bot-1')
    await nextTick()
    await nextTick()

    expect(nextDock.getPanel(deletedPanel.id)).toBeUndefined()
    expect(nextDock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's1')).toBe(false)
  })

  it('reconciles buffered deletes when the same bot dock api re-registers', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push({ id: 's1', title: 'Deleted session' })
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat')!
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))

    store.releaseApi()
    emitDeletedSession('s1', 'bot-1')
    await nextTick()

    chatStoreMock.selectSession.mockClear()
    const nextDock = createFakeDock()
    store.registerApi(nextDock as never)
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(nextDock.getPanel(deletedPanel.id)).toBeUndefined()
    expect(nextDock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's1')).toBe(false)
    expect(chatStoreMock.selectSession).not.toHaveBeenCalledWith('s1')
  })

  it('opens the selected fallback session after re-registering a layout with deleted chat and other panels', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push(
      { id: 's1', title: 'Deleted session' },
      { id: 's2', title: 'Fallback session' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat')!
    store.openFile('/data/notes.md')
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))

    store.releaseApi()
    selection.setSession('s2')
    emitDeletedSession('s1', 'bot-1')
    await nextTick()

    const nextDock = createFakeDock()
    store.registerApi(nextDock as never)
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))
    await nextTick()

    expect(nextDock.getPanel(deletedPanel.id)).toBeUndefined()
    expect(nextDock.panels.some(panel => panel.component === 'file')).toBe(true)
    expect(nextDock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's2')).toBe(true)
    expect(nextDock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's1')).toBe(false)
  })

  it('does not reopen a tombstoned chat session from a stale caller', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    emitDeletedSession('s1', 'bot-1')
    await nextTick()

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })

    expect(dock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 's1')).toBe(false)
    expect(chatStoreMock.selectSession).not.toHaveBeenCalledWith('s1')
  })

  it('keeps older tombstones available to block stale opens after reconciliation', async () => {
    const selection = useChatSelectionStore()
    selection.setSession('s1')
    chatStoreMock.sessions.push(
      { id: 's1', title: 'Deleted session' },
      { id: 's2', title: 'Existing session' },
    )
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Deleted session' })
    const deletedPanel = dock.panels.find(panel => panel.component === 'chat')!
    store.pinPanel(deletedPanel.id)
    store.openSessionChat({ sessionId: 's2', title: 'Existing session' })
    emitDeletedSession('missing-session')
    await nextTick()
    emitDeletedSession('s1')
    selection.setSession('s2')
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(dock.getPanel(deletedPanel.id)).toBeUndefined()

    selection.setBot('bot-without-layout')
    await nextTick()
    selection.setBot('bot-1')
    await nextTick()
    await new Promise(resolve => setTimeout(resolve, 0))

    chatStoreMock.selectSession.mockClear()
    store.openSessionChat({ sessionId: 'missing-session', title: 'Missing deleted session' })
    expect(dock.panels.some(panel => panel.component === 'chat' && panel.params.sessionId === 'missing-session')).toBe(false)

    store.openSessionChat({ sessionId: 's2', title: 'Existing session' })
    const s2Panel = dock.panels.find(panel => panel.component === 'chat' && panel.params.sessionId === 's2')
    expect(s2Panel).toBeTruthy()
    s2Panel?.api.setActive()

    expect(chatStoreMock.selectSession).toHaveBeenCalledWith('s2')
  })

  it('opens files into the ephemeral slot and replaces until pinned', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openFile('/data/a.md')
    expect(store.ephemeralPanels['file:/data/a.md']).toBe(true)

    // Another file replaces the ephemeral one in the same group.
    store.openFile('/data/b.md')
    expect(dock.getPanel('file:/data/a.md')).toBeUndefined()
    expect(dock.getPanel('file:/data/b.md')).toBeTruthy()

    // Pin b (mimics a first edit); opening c now keeps b.
    store.pinPanel('file:/data/b.md')
    expect(store.ephemeralPanels['file:/data/b.md']).toBeUndefined()
    store.openFile('/data/c.md')
    expect(dock.getPanel('file:/data/b.md')).toBeTruthy()
    expect(dock.getPanel('file:/data/c.md')).toBeTruthy()
  })

  it('keeps the final draft chat tab open when it is closed', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openDraftChat({ title: 'Existing' })
    const id = dock.panels.find((p) => p.component === 'chat')!.id
    store.closeTab(id)

    expect(dock.panels).toHaveLength(1)
    expect(dock.getPanel(id)?.component).toBe('chat')
  })

  it('respawns a fresh draft when the last chat tab (a real session) is closed', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'S1' })
    const id = dock.panels.find((p) => p.component === 'chat')!.id

    store.closeTab(id)
    // Closing the last chat resets the global view to a draft so the respawn is a
    // fresh New Session page, not the session that was just closed.
    expect(chatStoreMock.selectDraft).toHaveBeenCalled()

    await flushDraftChatFallback()
    const chatPanels = dock.panels.filter((p) => p.component === 'chat')
    expect(chatPanels).toHaveLength(1)
    expect(chatPanels[0]!.params.sessionId ?? null).toBeNull()
  })

  it('opens a draft chat when a non-chat tab leaves the dock empty', async () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openTerminal()
    store.closeTab('terminal:1')

    await flushDraftChatFallback()

    expect(dock.panels).toHaveLength(1)
    const chat = dock.panels[0]!
    expect(chat.component).toBe('chat')
    expect(chat.params.sessionId ?? null).toBeNull()
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

    expect(dock.panels).toHaveLength(1)
    const chat = dock.panels[0]!
    expect(chat.component).toBe('chat')
    expect(chat.title).toBe('New Session')
  })

  it('does not split chat panels (single global session)', () => {
    const store = useWorkspaceTabsStore()
    const dock = createFakeDock()
    store.registerApi(dock as never)

    store.openSessionChat({ sessionId: 's1', title: 'Session' })
    store.splitGroup('group-1', 'right')

    expect(dock.panels.filter((p) => p.component === 'chat')).toHaveLength(1)
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
