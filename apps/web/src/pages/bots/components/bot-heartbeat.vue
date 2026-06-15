<template>
  <PageShell
    variant="tab"
    :title="$t('bots.heartbeat.title')"
  >
    <template #actions>
      <Button
        variant="outline"
        :disabled="isLoading"
        @click="handleRefresh"
      >
        <RotateCw
          class="size-4"
          :class="{ 'animate-spin': isLoading }"
        />
        {{ $t('common.refresh') }}
      </Button>
    </template>

    <div class="space-y-8">
      <SettingsSection :title="$t('bots.settings.heartbeatEnabled')">
        <SettingsRow
          :label="$t('bots.settings.heartbeatEnabled')"
          :description="$t('bots.settings.heartbeatDescription')"
        >
          <Switch
            :model-value="settingsForm.heartbeat_enabled"
            @update:model-value="(val) => settingsForm.heartbeat_enabled = !!val"
          />
        </SettingsRow>

        <template v-if="settingsForm.heartbeat_enabled">
          <SettingsRow :label="$t('bots.settings.heartbeatInterval')">
            <Input
              v-model.number="settingsForm.heartbeat_interval"
              type="number"
              :min="1"
              placeholder="1440"
              class="h-8 w-28 tabular-nums"
            />
          </SettingsRow>

          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.settings.heartbeatModel') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.settings.heartbeatModelDescription') }}
              </p>
            </div>
            <ModelSelect
              v-model="settingsForm.heartbeat_model_id"
              :models="models"
              :providers="providers"
              model-type="chat"
              class="w-72 shrink-0"
            />
          </div>
        </template>

        <div class="mx-4 flex min-h-[3.75rem] items-center justify-end py-3">
          <Button
            size="sm"
            :disabled="!settingsChanged || isSaving"
            @click="handleSaveSettings"
          >
            <Spinner
              v-if="isSaving"
              class="size-3"
            />
            {{ $t('bots.settings.save') }}
          </Button>
        </div>
      </SettingsSection>

      <SettingsSection :title="$t('bots.heartbeat.title')">
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
          <span class="text-sm font-medium text-foreground">
            {{ $t('common.status') }}
          </span>
          <NativeSelect
            v-model="statusFilter"
            class="w-32"
          >
            <option value="">
              {{ $t('bots.heartbeat.filterAll') }}
            </option>
            <option value="ok">
              {{ $t('bots.heartbeat.statusOk') }}
            </option>
            <option value="alert">
              {{ $t('bots.heartbeat.statusAlert') }}
            </option>
            <option value="error">
              {{ $t('bots.heartbeat.statusError') }}
            </option>
          </NativeSelect>
        </div>

        <div
          v-if="isLoading && logs.length === 0"
          class="mx-4 flex min-h-[12rem] items-center justify-center gap-2 py-12 text-sm text-muted-foreground"
        >
          <Spinner class="size-4" />
          {{ $t('common.loading') }}
        </div>

        <Empty
          v-else-if="!isLoading && filteredLogs.length === 0"
          class="py-12"
        >
          <EmptyHeader>
            <EmptyTitle>{{ $t('bots.heartbeat.empty') }}</EmptyTitle>
            <EmptyDescription>
              {{ statusFilter ? $t('bots.heartbeat.filterEmpty') : $t('bots.heartbeat.description') }}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>

        <div v-else>
          <div
            v-for="log in filteredLogs"
            :key="log.id"
            class="mx-4 border-b border-border py-3 last:border-b-0"
          >
            <button
              type="button"
              class="flex w-full items-center justify-between gap-3 text-left"
              @click="toggleExpand(log.id)"
            >
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <Badge
                    :variant="statusVariant(log.status)"
                    size="sm"
                  >
                    {{ statusLabel(log.status) }}
                  </Badge>
                  <span class="text-xs tabular-nums text-muted-foreground">
                    {{ formatDateTime(log.started_at) }}
                  </span>
                </div>
                <p
                  v-if="!expandedIds.has(log.id!)"
                  class="mt-1 truncate text-xs"
                  :class="log.status === 'error' ? 'text-destructive' : 'text-muted-foreground'"
                >
                  {{ log.status === 'error' ? (log.error_message || $t('bots.heartbeat.noResult')) : (truncateText(log.result_text) || $t('bots.heartbeat.noResult')) }}
                </p>
              </div>

              <div class="flex shrink-0 items-center gap-3">
                <span class="text-xs tabular-nums text-muted-foreground">
                  {{ formatDuration(log.started_at, log.completed_at) }}
                </span>
                <ChevronDown
                  class="size-4 text-muted-foreground transition-transform"
                  :class="{ 'rotate-180': log.id && expandedIds.has(log.id) }"
                />
              </div>
            </button>

            <div
              v-if="log.id && expandedIds.has(log.id)"
              class="mt-3 space-y-3"
            >
              <div class="overflow-hidden rounded-md border border-border bg-card p-3">
                <pre class="whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-foreground">{{ log.result_text || $t('bots.heartbeat.noResult') }}</pre>
              </div>

              <div
                v-if="log.error_message"
                class="rounded-md border border-border bg-card p-3"
              >
                <p class="font-mono text-xs leading-normal text-destructive">
                  {{ log.error_message }}
                </p>
              </div>

              <div
                v-if="log.usage"
                class="flex flex-wrap gap-2"
              >
                <span
                  v-for="(val, key) in (log.usage as any)"
                  :key="key"
                  class="rounded-sm border border-border px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground"
                >
                  {{ key }}: {{ val }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="totalPages > 1"
          class="flex items-center justify-between border-t border-border p-4"
        >
          <span class="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
            {{ paginationSummary }}
          </span>
          <Pagination
            :total="totalCount"
            :items-per-page="PAGE_SIZE"
            :sibling-count="1"
            :page="currentPage"
            show-edges
            @update:page="currentPage = $event"
          >
            <PaginationContent v-slot="{ items }">
              <PaginationFirst />
              <PaginationPrevious />
              <template
                v-for="(item, index) in items"
                :key="index"
              >
                <PaginationEllipsis
                  v-if="item.type === 'ellipsis'"
                  :index="index"
                />
                <PaginationItem
                  v-else
                  :value="item.value"
                  :is-active="item.value === currentPage"
                />
              </template>
              <PaginationNext />
              <PaginationLast />
            </PaginationContent>
          </Pagination>
        </div>
      </SettingsSection>

      <SettingsSection
        v-if="logs.length > 0"
        :title="$t('common.dangerZone')"
      >
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.heartbeat.clearLogs') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ $t('bots.heartbeat.clearConfirm') }}
            </p>
          </div>
          <ConfirmPopover
            :message="$t('bots.heartbeat.clearConfirm')"
            :loading="isClearing"
            :confirm-text="$t('bots.heartbeat.clearLogs')"
            @confirm="handleClear"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
                :disabled="isClearing"
              >
                <Trash2 class="size-4" />
                {{ $t('bots.heartbeat.clearLogs') }}
              </Button>
            </template>
          </ConfirmPopover>
        </div>
      </SettingsSection>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import { Trash2, RotateCw, ChevronDown } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import {
  Badge, Button, Empty, EmptyDescription, EmptyHeader, EmptyTitle, Spinner, NativeSelect, Switch, Input,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import PageShell from '@/components/page-shell/index.vue'
import ModelSelect from './model-select.vue'
import {
  getBotsByBotIdSettings, putBotsByBotIdSettings,
  getBotsByBotIdHeartbeatLogs, deleteBotsByBotIdHeartbeatLogs,
  getModels, getProviders,
} from '@memohai/sdk'
import type { SettingsSettings, SettingsUpsertRequest, HeartbeatLog } from '@memohai/sdk'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import type { Ref } from 'vue'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId) as Ref<string>

// ---- Settings ----
const queryCache = useQueryCache()

const { data: settings } = useQuery({
  key: () => ['bot-settings', botIdRef.value],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({ path: { bot_id: botIdRef.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botIdRef.value,
})

const { data: modelData } = useQuery({
  key: ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])

const settingsForm = reactive({
  heartbeat_enabled: false,
  heartbeat_interval: 1440,
  heartbeat_model_id: '',
})

watch(settings, (val: SettingsSettings | undefined) => {
  if (val) {
    settingsForm.heartbeat_enabled = val.heartbeat_enabled ?? false
    settingsForm.heartbeat_interval = val.heartbeat_interval ?? 1440
    settingsForm.heartbeat_model_id = val.heartbeat_model_id ?? ''
  }
}, { immediate: true })

const settingsChanged = computed(() => {
  if (!settings.value) return false
  const s: SettingsSettings = settings.value
  return settingsForm.heartbeat_enabled !== (s.heartbeat_enabled ?? false)
    || settingsForm.heartbeat_interval !== (s.heartbeat_interval ?? 1440)
    || settingsForm.heartbeat_model_id !== (s.heartbeat_model_id ?? '')
})

const { mutateAsync: updateSettings, isLoading: isSaving } = useMutation({
  mutation: async (body: SettingsUpsertRequest) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] }),
})

async function handleSaveSettings() {
  try {
    await updateSettings({ ...settingsForm })
    toast.success(t('bots.settings.saveSuccess'))
  } catch {
    return
  }
}

const isLoading = ref(false)
const isClearing = ref(false)
const logs = ref<HeartbeatLog[]>([])
const totalCount = ref(0)
const statusFilter = ref('')
const expandedIds = ref(new Set<string>())
const currentPage = ref(1)

const PAGE_SIZE = 20

const filteredLogs = computed(() => {
  if (!statusFilter.value) return logs.value
  return logs.value.filter(l => l.status === statusFilter.value)
})

const totalPages = computed(() => Math.ceil(totalCount.value / PAGE_SIZE))

const paginationSummary = computed(() => {
  const total = totalCount.value
  if (total === 0) return ''
  const start = (currentPage.value - 1) * PAGE_SIZE + 1
  const end = Math.min(currentPage.value * PAGE_SIZE, total)
  return `${start}-${end} / ${total}`
})

watch(currentPage, () => {
  fetchLogs()
})

function statusVariant(status: string | undefined) {
  if (status === 'ok') return 'secondary' as const
  if (status === 'alert') return 'warning' as const
  return 'destructive' as const
}

function statusLabel(status: string | undefined) {
  if (status === 'ok') return t('bots.heartbeat.statusOk')
  if (status === 'alert') return t('bots.heartbeat.statusAlert')
  return t('bots.heartbeat.statusError')
}

function formatDuration(startedAt: string | undefined, completedAt: string | null | undefined) {
  if (!startedAt || !completedAt) return '—'
  const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function truncateText(text: string | undefined, maxLen = 120) {
  if (!text) return ''
  if (text === 'HEARTBEAT_OK') return 'HEARTBEAT_OK'
  return text.length > maxLen ? text.slice(0, maxLen) + '…' : text
}

function toggleExpand(id: string | undefined) {
  if (!id) return
  if (expandedIds.value.has(id)) {
    expandedIds.value.delete(id)
  } else {
    expandedIds.value.add(id)
  }
}

async function fetchLogs() {
  if (!props.botId) return
  isLoading.value = true
  try {
    const offset = (currentPage.value - 1) * PAGE_SIZE
    const { data } = await getBotsByBotIdHeartbeatLogs({
      path: { bot_id: props.botId },
      query: { limit: PAGE_SIZE, offset },
      throwOnError: true,
    })
    logs.value = data?.items ?? []
    totalCount.value = data?.total_count ?? 0
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.heartbeat.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

async function handleRefresh() {
  expandedIds.value.clear()
  currentPage.value = 1
  await fetchLogs()
}

async function handleClear() {
  isClearing.value = true
  try {
    await deleteBotsByBotIdHeartbeatLogs({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    logs.value = []
    totalCount.value = 0
    expandedIds.value.clear()
    toast.success(t('bots.heartbeat.clearSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.heartbeat.clearFailed')))
  } finally {
    isClearing.value = false
  }
}

onMounted(() => {
  fetchLogs()
})
</script>
