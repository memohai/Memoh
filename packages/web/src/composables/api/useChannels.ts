import { fetchApi, ApiError } from '@/utils/request'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import type { Ref } from 'vue'

// ---- Types ----

export interface FieldSchema {
  title: string
  description?: string
  type: 'string' | 'secret' | 'bool' | 'number' | 'enum'
  required?: boolean
  enum?: string[]
  example?: unknown
}

export interface ConfigSchema {
  version: number
  fields: Record<string, FieldSchema>
}

export interface ChannelCapabilities {
  text: boolean
  markdown: boolean
  rich_text: boolean
  attachments: boolean
  media: boolean
  reactions: boolean
  buttons: boolean
  reply: boolean
  threads: boolean
  streaming: boolean
  polls: boolean
  edit: boolean
  unsend: boolean
  native_commands: boolean
  block_streaming: boolean
  chat_types?: string[]
}

export interface ChannelMeta {
  type: string
  display_name: string
  configless: boolean
  capabilities: ChannelCapabilities
  config_schema: ConfigSchema
  user_config_schema: ConfigSchema
  target_spec: { format: string; description: string }
}

export interface ChannelConfig {
  id: string
  botID: string
  channelType: string
  credentials: Record<string, unknown>
  externalIdentity: string
  selfIdentity: Record<string, unknown>
  routing: Record<string, unknown>
  status: string
  verifiedAt: string
  createdAt: string
  updatedAt: string
}

export interface UpsertConfigRequest {
  credentials: Record<string, unknown>
  external_identity?: string
  self_identity?: Record<string, unknown>
  routing?: Record<string, unknown>
  status?: string
}

export interface BotChannelItem {
  meta: ChannelMeta
  config: ChannelConfig | null
  configured: boolean
}

// ---- Query: 获取可用渠道类型元信息 ----

export function useChannelMetas() {
  return useQuery({
    key: ['channel-metas'],
    query: () => fetchApi<ChannelMeta[]>('/channels'),
  })
}

// ---- Query: 获取某 Bot 的所有渠道配置（组合查询） ----

export function useBotChannels(botId: Ref<string>) {
  const queryCache = useQueryCache()

  const query = useQuery({
    key: () => ['bot-channels', botId.value],
    query: async (): Promise<BotChannelItem[]> => {
      // 1. 获取所有渠道元信息
      const metas = await fetchApi<ChannelMeta[]>('/channels')

      // 2. 过滤掉 configless 的类型（cli / web 等本地渠道）
      const configurableTypes = metas.filter((m) => !m.configless)

      // 3. 并行获取每种类型的 bot 配置
      const results = await Promise.all(
        configurableTypes.map(async (meta) => {
          try {
            const config = await fetchApi<ChannelConfig>(
              `/bots/${botId.value}/channel/${meta.type}`,
            )
            return { meta, config, configured: true } as BotChannelItem
          } catch (err) {
            // 404 = 尚未配置，其他错误也视为未配置
            if (err instanceof ApiError && err.status === 404) {
              return { meta, config: null, configured: false } as BotChannelItem
            }
            return { meta, config: null, configured: false } as BotChannelItem
          }
        }),
      )

      return results
    },
    enabled: () => !!botId.value,
  })

  return {
    ...query,
    invalidate: () => queryCache.invalidateQueries({ key: ['bot-channels', botId.value] }),
  }
}

// ---- Mutation: 创建/更新 Bot 渠道配置 ----

export function useUpsertBotChannel(botId: Ref<string>) {
  const queryCache = useQueryCache()

  return useMutation({
    mutation: ({ platform, data }: { platform: string; data: UpsertConfigRequest }) =>
      fetchApi<ChannelConfig>(`/bots/${botId.value}/channel/${platform}`, {
        method: 'PUT',
        body: data,
      }),
    onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botId.value] }),
  })
}
