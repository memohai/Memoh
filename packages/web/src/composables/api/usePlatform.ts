import { client } from '@memoh/sdk/client'
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

// ---- Query: platform list ----

export function usePlatformList() {
  return useQuery({
    key: ['platform'],
    query: async () => {
      const { data } = await client.get<PlatformItem[]>({
        url: '/platform/',
        throwOnError: true,
      })
      return data
    },
  })
}

// ---- Mutations ----

export function useCreatePlatform() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: CreatePlatformRequest) =>
      client.post<void>({ url: '/platform/', body: data, throwOnError: true }),
    onSettled: () => queryCache.invalidateQueries({ key: ['platform'] }),
  })
}
