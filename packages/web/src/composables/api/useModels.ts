import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { type ModelInfo } from '@memoh/shared'
import type { Ref } from 'vue'

// ---- Types ----

export interface CreateModelRequest {
  model_id: string
  type: string
  llm_provider_id: string
  name?: string
  dimensions?: number
  is_multimodal?: boolean
}

// ---- Query: 获取 Provider 下的模型列表 ----

export function useModelList(providerId: Ref<string | undefined>) {
  const queryCache = useQueryCache()

  const query = useQuery({
    key: ['model'],
    query: () => fetchApi<ModelInfo[]>(
      `/providers/${providerId.value}/models`,
    ),
  })

  return {
    ...query,
    /** 当 providerId 变化时手动刷新 */
    invalidate: () => queryCache.invalidateQueries({ key: ['model'] }),
  }
}

// ---- Query: 获取所有模型（跨 Provider） ----

export function useAllModels() {
  return useQuery({
    key: ['all-models'],
    query: () => fetchApi<ModelInfo[]>('/models'),
  })
}

// ---- Mutations ----

export function useCreateModel() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: CreateModelRequest) => fetchApi('/models', {
      method: 'POST',
      body: data,
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['model'], exact: true }),
  })
}

export function useDeleteModel() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (modelName: string) => fetchApi(`/models/model/${modelName}`, {
      method: 'DELETE',
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['model'] }),
  })
}
