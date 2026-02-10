import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'

// ---- Types ----

export interface PlatformItem {
  name: string
  active: boolean
  config: Record<string, string>
}

export interface CreatePlatformRequest {
  name: string
  config: Record<string, unknown>
  active: boolean
}

// ---- Query: 获取平台列表 ----

export function usePlatformList() {
  return useQuery({
    key: ['platform'],
    query: () => fetchApi<PlatformItem[]>('/platform/'),
  })
}

// ---- Mutations ----

export function useCreatePlatform() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: CreatePlatformRequest) => fetchApi('/platform/', {
      method: 'POST',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['platform'] }),
  })
}
