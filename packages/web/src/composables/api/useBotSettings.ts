import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import type { Ref } from 'vue'

// ---- Types ----

export interface BotSettings {
  chat_model_id: string
  memory_model_id: string
  embedding_model_id: string
  max_context_load_time: number
  language: string
  allow_guest: boolean
}

export interface UpsertBotSettingsRequest {
  chat_model_id?: string
  memory_model_id?: string
  embedding_model_id?: string
  max_context_load_time?: number
  language?: string
  allow_guest?: boolean
}

// ---- Query ----

export function useBotSettings(botId: Ref<string>) {
  return useQuery({
    key: () => ['bot-settings', botId.value],
    query: () => fetchApi<BotSettings>(`/bots/${botId.value}/settings`),
    enabled: () => !!botId.value,
  })
}

// ---- Mutation ----

export function useUpdateBotSettings(botId: Ref<string>) {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: UpsertBotSettingsRequest) => fetchApi<BotSettings>(
      `/bots/${botId.value}/settings`,
      { method: 'PUT', body: data },
    ),
    onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', botId.value] }),
  })
}
