import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { type ProviderInfo } from '@memoh/shared'
import type { Ref } from 'vue'

// ---- Types ----

export type ProviderWithId = ProviderInfo & { id: string }

export interface CreateProviderRequest {
  name: string
  api_key: string
  base_url: string
  client_type: string
  metadata?: Record<string, unknown>
}

export type UpdateProviderRequest = Partial<CreateProviderRequest>

// ---- Query: 获取 Provider 列表 ----

export function useProviderList(clientType: Ref<string>) {
  return useQuery({
    key: ['provider'],
    query: () => fetchApi<ProviderWithId[]>(
      `/providers?client_type=${clientType.value}`,
    ),
  })
}

/** 获取所有 Provider（无过滤） */
export function useAllProviders() {
  return useQuery({
    key: ['all-providers'],
    query: () => fetchApi<ProviderWithId[]>('/providers'),
  })
}

// ---- Mutations ----

export function useCreateProvider() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: CreateProviderRequest) => fetchApi('/providers', {
      method: 'POST',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['provider'] }),
  })
}

export function useUpdateProvider(providerId: Ref<string | undefined>) {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: UpdateProviderRequest) => fetchApi(`/providers/${providerId.value}`, {
      method: 'PUT',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['provider'] }),
  })
}

export function useDeleteProvider(providerId: Ref<string | undefined>) {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: () => fetchApi(`/providers/${providerId.value}`, {
      method: 'DELETE',
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['provider'] }),
  })
}
