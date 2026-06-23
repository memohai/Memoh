import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useSubagentList } from './useSubagentList'

const api = vi.hoisted(() => ({
  connectWebSocket: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  fetchSession: vi.fn(),
  fetchSessions: vi.fn(),
  streamBotSessionsActivityEvents: vi.fn(),
  streamSessionMessageEvents: vi.fn(),
}))

const colada = vi.hoisted(() => ({
  data: { value: undefined as unknown },
  options: [] as Array<{
    key: () => unknown[]
    query: () => Promise<unknown>
    enabled: () => boolean
  }>,
  useQuery: vi.fn((options) => {
    colada.options.push(options)
    return { data: colada.data }
  }),
}))

vi.mock('@/composables/api/useChat', () => api)
vi.mock('@pinia/colada', () => ({ useQuery: colada.useQuery }))

describe('useSubagentList', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    colada.data.value = undefined
    colada.options = []
    api.connectWebSocket.mockReturnValue({ close: vi.fn(), send: vi.fn() })
    api.fetchBots.mockResolvedValue([{ id: 'bot-1', name: 'Bot', status: 'ready' }])
    api.fetchMessagesUI.mockResolvedValue([])
    api.fetchSession.mockResolvedValue({
      id: 'parent-1',
      bot_id: 'bot-1',
      title: 'Parent',
      type: 'chat',
    })
    api.streamBotSessionsActivityEvents.mockResolvedValue(undefined)
    api.streamSessionMessageEvents.mockResolvedValue(undefined)
  })

  it('loads subagent sessions from the current session parent', async () => {
    const chatStore = useChatStore()
    chatStore.currentBotId = 'bot-1'
    chatStore.sessionId = 'parent-1'
    api.fetchSessions.mockResolvedValue({
      items: [{
        id: 'child-1',
        bot_id: 'bot-1',
        title: 'Child task',
        type: 'subagent',
        metadata: { agent_id: 'agent-a' },
      }],
      nextCursor: null,
    })

    const { subagents } = useSubagentList()
    const result = await colada.options[0]!.query()
    colada.data.value = result

    expect(colada.options[0]!.key()).toEqual(['session-subagents', 'bot-1', 'parent-1'])
    expect(colada.options[0]!.enabled()).toBe(true)
    expect(api.fetchSessions).toHaveBeenCalledWith('bot-1', {
      types: ['subagent'],
      parentSessionId: 'parent-1',
      limit: 50,
    })
    expect(subagents.value).toEqual([{
      id: 'child-1',
      agentId: 'agent-a',
      title: 'Child task',
    }])
  })

  it('opens the subagent through workspace tabs', () => {
    const chatStore = useChatStore()
    chatStore.currentBotId = 'bot-1'
    chatStore.sessionId = 'parent-1'
    colada.data.value = [{
      id: 'child-1',
      agentId: 'agent-a',
      title: 'Child task',
    }]
    const workspaceTabs = useWorkspaceTabsStore()
    const openSessionChat = vi.spyOn(workspaceTabs, 'openSessionChat').mockImplementation(() => {})

    const { navigateToSession } = useSubagentList()
    navigateToSession('child-1')

    expect(openSessionChat).toHaveBeenCalledWith({
      sessionId: 'child-1',
      title: 'Child task',
    })
  })
})
