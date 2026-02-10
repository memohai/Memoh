import { fetchApi } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { type MCPListItem } from '@memoh/shared'

// ---- Types ----

export interface McpListResponse {
  items: MCPListItem[]
}

export interface CreateMcpRequest {
  name: string
  config: Record<string, unknown>
  active: boolean
}

export interface UpdateMcpRequest extends CreateMcpRequest {
  id?: string
}

// ---- Query: 获取 MCP 列表 ----

export function useMcpList() {
  const query = useQuery({
    key: ['mcp'],
    query: async () => {
      const res = await fetchApi<McpListResponse>('/mcp/')
      return res.items
    },
  })
  return query
}

// ---- Mutations ----

export function useCreateOrUpdateMcp() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (data: UpdateMcpRequest) => {
      const isEdit = !!data.id
      return fetchApi(isEdit ? `/mcp/${data.id}` : '/mcp/', {
        method: isEdit ? 'PUT' : 'POST',
        body: data,
      })
    },
    onSettled: () => queryCache.invalidateQueries({ key: ['mcp'] }),
  })
}

export function useDeleteMcp() {
  const queryCache = useQueryCache()
  return useMutation({
    mutation: (id: string) => fetchApi(`/mcp/${id}`, {
      method: 'DELETE',
    }),
    onSettled: () => queryCache.invalidateQueries({ key: ['mcp'] }),
  })
}
