<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="flex items-center gap-1 border-b border-border px-2 py-1.5 shrink-0">
      <span class="text-[11px] font-medium text-muted-foreground tracking-wide uppercase">
        {{ t('chat.activityTabSchedule') }}
      </span>
      <Button
        variant="ghost"
        size="sm"
        class="size-7 p-0 ml-auto"
        :title="t('chat.scheduleManageInSettings')"
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
            <CalendarClock class="mb-2 size-6 opacity-40" />
            <p class="text-xs">
              {{ t('chat.scheduleEmpty') }}
            </p>
            <Button
              variant="outline"
              size="sm"
              class="mt-3 text-xs h-7"
              @click="goToSettings()"
            >
              {{ t('chat.scheduleManageInSettings') }}
            </Button>
          </div>

          <div
            v-else
            class="flex flex-col gap-1"
          >
            <div
              v-for="item in items"
              :key="item.id"
              class="flex items-start gap-2 rounded-md px-2 py-1.5 hover:bg-sidebar-accent/40 transition-colors"
            >
              <CalendarClock class="size-4 text-muted-foreground shrink-0 mt-0.5" />
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  <span class="truncate text-xs font-medium text-foreground leading-[18px]">
                    {{ item.name || t('chat.untitledSchedule') }}
                  </span>
                </div>
                <div class="flex items-center gap-1.5 text-[11px] text-muted-foreground leading-[14px]">
                  <span class="truncate font-mono">
                    {{ item.description || item.pattern || '--' }}
                  </span>
                  <span
                    v-if="callsLabel(item)"
                    class="shrink-0 tabular-nums"
                  >
                    · {{ callsLabel(item) }}
                  </span>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                class="size-6 p-0 shrink-0"
                :title="t('chat.scheduleManageInSettings')"
                :aria-label="t('chat.scheduleManageInSettings')"
                @click.stop="goToSettings()"
              >
                <ExternalLink class="size-3.5" />
              </Button>
              <Switch
                :model-value="item.enabled"
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
import { CalendarClock, Plus, RefreshCw, ExternalLink } from 'lucide-vue-next'
import { Button, ScrollArea, Switch } from '@memohai/ui'
import {
  getBotsByBotIdSchedule,
  putBotsByBotIdScheduleById,
} from '@memohai/sdk'
import type { ScheduleSchedule } from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const router = useRouter()
const queryCache = useQueryCache()

const togglingId = ref<string | null>(null)
const localOverrides = ref<Record<string, boolean>>({})

const { data, isLoading, error } = useQuery({
  key: () => ['bot-schedule', props.botId],
  query: async () => {
    const { data: resp } = await getBotsByBotIdSchedule({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return (resp.items ?? []) as ScheduleSchedule[]
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: false,
})

const items = computed<ScheduleSchedule[]>(() => {
  const list = data.value ?? []
  const overrides = localOverrides.value
  return list.map((item) => {
    const id = item.id ?? ''
    return id && overrides[id] !== undefined
      ? { ...item, enabled: overrides[id]! }
      : item
  })
})

watch(error, (err) => {
  if (err) toast.error(resolveApiErrorMessage(err, t('bots.schedule.loadFailed')))
})

watch(
  () => data.value,
  (list) => {
    if (!list) return
    if (Object.keys(localOverrides.value).length === 0) return
    const next = { ...localOverrides.value }
    for (const item of list) {
      const id = item.id ?? ''
      if (!id) continue
      if (next[id] !== undefined && next[id] === item.enabled) {
        delete next[id]
      }
    }
    localOverrides.value = next
  },
)

function reload() {
  queryCache.invalidateQueries({ key: ['bot-schedule', props.botId] })
}

function goToSettings() {
  void router.push({
    name: 'bot-detail',
    params: { botId: props.botId },
    query: { tab: 'schedule' },
  })
}

function callsLabel(item: ScheduleSchedule): string {
  const max = item.max_calls ?? 0
  if (!max || max <= 0) return ''
  const cur = item.current_calls ?? 0
  return `${cur}/${max}`
}

async function onToggle(item: ScheduleSchedule, nextValue: boolean) {
  const id = item.id
  if (!id || togglingId.value) return
  togglingId.value = id
  localOverrides.value = { ...localOverrides.value, [id]: nextValue }
  try {
    await putBotsByBotIdScheduleById({
      path: { bot_id: props.botId, id },
      body: { enabled: nextValue },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['bot-schedule', props.botId] })
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('bots.schedule.saveFailed')))
    const next = { ...localOverrides.value }
    delete next[id]
    localOverrides.value = next
  } finally {
    togglingId.value = null
  }
}
</script>
