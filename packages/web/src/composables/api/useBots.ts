import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import type { Ref } from 'vue'

// ---- Types ----

export interface BotInfo {
  id: string
  display_name: string
  avatar_url: string
  type: string
  is_active: boolean
  owner_user_id: string
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ListBotsResponse {
  items: BotInfo[]
}

export interface CreateBotRequest {
  display_name: string
  avatar_url?: string
  type?: string
  is_active?: boolean
  metadata?: Record<string, unknown>
}

export interface UpdateBotRequest {
  display_name?: string
  avatar_url?: string
  is_active?: boolean
  metadata?: Record<string, unknown>
}

// ---- Query: 获取 Bot 列表 ----

export function useBotList() {
  const queryCache = useQueryCache()

  const query = useQuery({
    key: ['bots'],
    query: async (): Promise<BotInfo[]> => {
      const res = await fetchApi<ListBotsResponse>('/bots')
      return res.items ?? []
    },
  })

  return {
    ...query,
    invalidate: () => queryCache.invalidateQueries({ key: ['bots'] }),
  }
}

// ---- Query: 获取单个 Bot 详情 ----

export function useBotDetail(botId: Ref<string>) {
  return useQuery({
    key: () => ['bot', botId.value],
    query: () => fetchApi<BotInfo>(`/bots/${botId.value}`),
    enabled: () => !!botId.value,
  })
}

// ---- Mutations ----

export function useCreateBot() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: CreateBotRequest) => fetchApi<BotInfo>('/bots', {
      method: 'POST',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['bots'] }),
  })
}

export function useDeleteBot() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (botId: string) => fetchApi(`/bots/${botId}`, {
      method: 'DELETE',
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['bots'] }),
  })
}

export function useUpdateBot() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: ({ id, ...data }: UpdateBotRequest & { id: string }) => fetchApi<BotInfo>(`/bots/${id}`, {
      method: 'PUT',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['bots'] }),
  })
}
