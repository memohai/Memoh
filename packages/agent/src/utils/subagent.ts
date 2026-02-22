import type { AuthFetcher } from '../types'
import type { LanguageModelUsage } from 'ai'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SubagentItem {
  id: string
  name: string
  description: string
  bot_id: string
  messages: Record<string, unknown>[]
  metadata: Record<string, unknown>
  skills: string[]
  usage: SubagentUsage
  created_at: string
  updated_at: string
  deleted: boolean
  deleted_at?: string
}

export interface SubagentUsage {
  inputTokens: number
  outputTokens: number
  totalTokens: number
}

export interface SubagentListResponse {
  items: SubagentItem[]
}

export interface SubagentContextResponse {
  messages: Record<string, unknown>[]
  usage: SubagentUsage
}

// ---------------------------------------------------------------------------
// Usage helpers
// ---------------------------------------------------------------------------

const emptyUsage: SubagentUsage = {
  inputTokens: 0,
  outputTokens: 0,
  totalTokens: 0,
}

export const toSubagentUsage = (raw: unknown): SubagentUsage => {
  if (!raw || typeof raw !== 'object') return { ...emptyUsage }
  const obj = raw as Record<string, unknown>
  return {
    inputTokens: typeof obj.inputTokens === 'number' ? obj.inputTokens : 0,
    outputTokens: typeof obj.outputTokens === 'number' ? obj.outputTokens : 0,
    totalTokens: typeof obj.totalTokens === 'number' ? obj.totalTokens : 0,
  }
}

export const addUsage = (
  existing: SubagentUsage,
  delta: LanguageModelUsage,
): SubagentUsage => ({
  inputTokens: existing.inputTokens + (delta.inputTokens ?? 0),
  outputTokens: existing.outputTokens + (delta.outputTokens ?? 0),
  totalTokens: existing.totalTokens + (delta.totalTokens ?? 0),
})

// ---------------------------------------------------------------------------
// Client factory
// ---------------------------------------------------------------------------

export const createSubagentClient = (fetch: AuthFetcher, botId: string) => {
  const base = `/bots/${botId}/subagents`

  const list = async (): Promise<SubagentListResponse> => {
    const res = await fetch(base, { method: 'GET' })
    return res.json() as Promise<SubagentListResponse>
  }

  const create = async (params: {
    name: string
    description: string
  }): Promise<SubagentItem> => {
    const res = await fetch(base, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    return res.json() as Promise<SubagentItem>
  }

  const get = async (id: string): Promise<SubagentItem> => {
    const res = await fetch(`${base}/${id}`, { method: 'GET' })
    return res.json() as Promise<SubagentItem>
  }

  const getContext = async (id: string): Promise<SubagentContextResponse> => {
    const res = await fetch(`${base}/${id}/context`, { method: 'GET' })
    return res.json() as Promise<SubagentContextResponse>
  }

  const updateContext = async (
    id: string,
    messages: Record<string, unknown>[],
    usage: SubagentUsage,
  ): Promise<SubagentContextResponse> => {
    const res = await fetch(`${base}/${id}/context`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages, usage }),
    })
    return res.json() as Promise<SubagentContextResponse>
  }

  const getOrCreate = async (params: {
    name: string
    description: string
  }): Promise<SubagentItem> => {
    const { items } = await list()
    const existing = items.find((item) => item.name === params.name)
    if (existing) return existing
    return create(params)
  }

  const remove = async (id: string): Promise<{ success: boolean }> => {
    const res = await fetch(`${base}/${id}`, { method: 'DELETE' })
    return res.status === 204 ? { success: true } : (res.json() as Promise<{ success: boolean }>)
  }

  return { list, create, get, getContext, updateContext, getOrCreate, remove }
}

