<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="flex items-center gap-1 border-b border-border px-2 py-1.5 shrink-0">
      <span class="text-[11px] font-medium text-muted-foreground tracking-wide uppercase">
        {{ t('chat.activityTabMcp') }}
      </span>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0 ml-auto"
        :title="t('chat.mcpManageInSettings')"
        @click="goToSettings()"
      >
        <Plus class="size-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0"
        :disabled="isLoading"
        :title="t('common.refresh')"
        @click="reload"
      >
        <RefreshCw
          class="size-3.5"
          :class="{ 'animate-spin': isLoading }"
        />
      </Button>
    </div>

    <div class="flex-1 min-h-0 relative">
      <ScrollArea class="absolute inset-0">
        <div class="px-2 py-2">
          <div
            v-if="!items.length && !isLoading"
            class="flex flex-col items-center justify-center py-12 text-center text-muted-foreground"
          >
            <Plug class="mb-2 size-6 opacity-40" />
            <p class="text-xs">
              {{ t('chat.mcpEmpty') }}
            </p>
            <Button
              variant="outline"
              size="sm"
              class="mt-3 text-xs h-7"
              @click="goToSettings()"
            >
              {{ t('chat.mcpManageInSettings') }}
            </Button>
          </div>

          <div
            v-else
            class="flex flex-col gap-1"
          >
            <div
              v-for="item in items"
              :key="item.id"
              class="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-sidebar-accent/40 transition-colors"
            >
              <div class="relative shrink-0">
                <Plug class="size-4 text-muted-foreground" />
                <span
                  class="absolute -bottom-0.5 -right-0.5 size-1.5 rounded-full ring-1 ring-sidebar"
                  :class="statusDotClass(item)"
                />
              </div>
              <div class="flex-1 min-w-0">
                <div class="truncate text-xs font-medium text-foreground leading-[18px]">
                  {{ item.name || t('chat.untitledMcp') }}
                </div>
                <div class="text-[11px] text-muted-foreground leading-[14px]">
                  {{ statusText(item) }}
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                class="size-6 p-0 shrink-0"
                :title="t('chat.mcpOpenInSettings')"
                :aria-label="t('chat.mcpOpenInSettings')"
                @click.stop="goToSettings(item.id)"
              >
                <ExternalLink class="size-3.5" />
              </Button>
              <Switch
                :model-value="item.is_active"
                :disabled="!!togglingId && togglingId === item.id"
                class="shrink-0"
                @update:model-value="(val) => onToggle(item, !!val)"
              />
            </div>
          </div>
        </div>
      </ScrollArea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useQuery, useQueryCache } from '@pinia/colada'
import { Plug, Plus, RefreshCw, ExternalLink } from 'lucide-vue-next'
import { Button, ScrollArea, Switch } from '@memohai/ui'
import {
  getBotsByBotIdMcp,
  putBotsByBotIdMcpById,
} from '@memohai/sdk'
import type { McpUpsertRequest } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface McpItem {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  is_active: boolean
  status: string
  status_message: string
  auth_type: string
}

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const router = useRouter()
const queryCache = useQueryCache()

const togglingId = ref<string | null>(null)
const localOverrides = ref<Record<string, boolean>>({})

const { data, isLoading, error } = useQuery({
  key: () => ['bot-mcp', props.botId],
  query: async () => {
    const { data: resp } = await getBotsByBotIdMcp({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return ((resp.items ?? []) as Record<string, unknown>[]).map((raw): McpItem => ({
      id: String(raw.id ?? ''),
      name: String(raw.name ?? ''),
      type: String(raw.type ?? ''),
      config: (raw.config as Record<string, unknown>) ?? {},
      is_active: !!raw.is_active,
      status: String(raw.status ?? 'unknown'),
      status_message: String(raw.status_message ?? ''),
      auth_type: String(raw.auth_type ?? 'none'),
    }))
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: false,
})

const items = computed<McpItem[]>(() => {
  const list = data.value ?? []
  const overrides = localOverrides.value
  return list.map((item) =>
    overrides[item.id] !== undefined
      ? { ...item, is_active: overrides[item.id]! }
      : item,
  )
})

watch(error, (err) => {
  if (err) toast.error(resolveApiErrorMessage(err, t('mcp.loadFailed')))
})

function reload() {
  queryCache.invalidateQueries({ key: ['bot-mcp', props.botId] })
}

function statusDotClass(item: McpItem): string {
  if (!item.is_active) return 'bg-muted-foreground/40'
  switch (item.status) {
    case 'connected': return 'bg-green-500'
    case 'error': return 'bg-destructive'
    default: return 'bg-amber-400'
  }
}

function statusText(item: McpItem): string {
  if (!item.is_active) return t('chat.mcpInactive')
  switch (item.status) {
    case 'connected': return t('mcp.statusConnected')
    case 'error': return t('mcp.statusError')
    default: return t('mcp.statusUnknown')
  }
}

function goToSettings(mcpId?: string) {
  void router.push({
    name: 'bot-detail',
    params: { botId: props.botId },
    query: { tab: 'mcp', ...(mcpId ? { mcpId } : {}) },
  })
}

function buildToggleBody(item: McpItem, nextValue: boolean): McpUpsertRequest {
  const cfg = item.config ?? {}
  const body: McpUpsertRequest = {
    name: item.name,
    is_active: nextValue,
  }
  if (typeof cfg.command === 'string' && cfg.command) {
    body.command = cfg.command
    if (Array.isArray(cfg.args)) body.args = cfg.args as string[]
    if (cfg.env && typeof cfg.env === 'object') body.env = cfg.env as Record<string, string>
    if (typeof cfg.cwd === 'string' && cfg.cwd) body.cwd = cfg.cwd
  } else {
    if (typeof cfg.url === 'string') body.url = cfg.url
    if (cfg.headers && typeof cfg.headers === 'object') body.headers = cfg.headers as Record<string, string>
    if (cfg.transport === 'sse' || cfg.transport === 'http') body.transport = cfg.transport
  }
  if (item.auth_type) body.auth_type = item.auth_type
  return body
}

async function onToggle(item: McpItem, nextValue: boolean) {
  if (!item.id || togglingId.value) return
  togglingId.value = item.id
  localOverrides.value = { ...localOverrides.value, [item.id]: nextValue }
  try {
    await putBotsByBotIdMcpById({
      path: { bot_id: props.botId, id: item.id },
      body: buildToggleBody(item, nextValue),
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-mcp', props.botId] })
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('common.saveFailed')))
    const next = { ...localOverrides.value }
    delete next[item.id]
    localOverrides.value = next
  } finally {
    togglingId.value = null
  }
}

watch(
  () => data.value,
  (list) => {
    if (!list) return
    // Reset local overrides for items whose server state has caught up.
    if (Object.keys(localOverrides.value).length === 0) return
    const next = { ...localOverrides.value }
    for (const item of list) {
      if (next[item.id] !== undefined && next[item.id] === item.is_active) {
        delete next[item.id]
      }
    }
    localOverrides.value = next
  },
)
</script>
