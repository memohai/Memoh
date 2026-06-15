<script setup lang="ts">
import { Minimize2, RefreshCw, History } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import {
  Button, Badge, Empty, EmptyHeader, EmptyMedia, EmptyTitle, Spinner, NativeSelect, Switch, Input, Slider,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from './model-select.vue'
import {
  getBotsByBotIdSettings, putBotsByBotIdSettings,
  getBotsByBotIdCompactionLogs, deleteBotsByBotIdCompactionLogs,
  getModels, getProviders,
} from '@memohai/sdk'
import type { SettingsSettings, SettingsUpsertRequest, CompactionLog } from '@memohai/sdk'
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
  compaction_enabled: false,
  compaction_threshold: 100000,
  compaction_ratio: 80,
  compaction_model_id: '',
})

watch(settings, (val: SettingsSettings | undefined) => {
  if (val) {
    settingsForm.compaction_enabled = val.compaction_enabled ?? false
    settingsForm.compaction_threshold = val.compaction_threshold ?? 100000
    settingsForm.compaction_ratio = val.compaction_ratio ?? 80
    settingsForm.compaction_model_id = val.compaction_model_id ?? ''
  }
}, { immediate: true })

const settingsChanged = computed(() => {
  if (!settings.value) return false
  const s: SettingsSettings = settings.value
  return settingsForm.compaction_enabled !== (s.compaction_enabled ?? false)
    || settingsForm.compaction_threshold !== (s.compaction_threshold ?? 100000)
    || settingsForm.compaction_ratio !== (s.compaction_ratio ?? 80)
    || settingsForm.compaction_model_id !== (s.compaction_model_id ?? '')
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

// ---- Logs ----
const isLoading = ref(false)
const isClearing = ref(false)
const logs = ref<CompactionLog[]>([])
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
  if (status === 'pending') return 'default' as const
  return 'destructive' as const
}

function statusLabel(status: string | undefined) {
  if (status === 'ok') return t('bots.compaction.statusOk')
  if (status === 'pending') return t('bots.compaction.statusPending')
  return t('bots.compaction.statusError')
}

function formatDuration(startedAt: string | undefined, completedAt: string | null | undefined) {
  if (!startedAt || !completedAt) return '—'
  const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
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
    const { data } = await getBotsByBotIdCompactionLogs({
      path: { bot_id: props.botId },
      query: { limit: PAGE_SIZE, offset },
      throwOnError: true,
    })
    logs.value = data?.items ?? []
    totalCount.value = data?.total_count ?? 0
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.compaction.loadFailed')))
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
    await deleteBotsByBotIdCompactionLogs({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    logs.value = []
    totalCount.value = 0
    expandedIds.value.clear()
    toast.success(t('bots.compaction.clearSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.compaction.clearFailed')))
  } finally {
    isClearing.value = false
  }
}

onMounted(() => {
  fetchLogs()
})
</script>

<template>
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <div class="mb-6 flex items-start justify-between gap-4 px-2">
      <div class="min-w-0">
        <h1 class="text-lg font-semibold text-foreground">
          {{ $t('bots.tabs.compaction') }}
        </h1>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ $t('bots.settings.compactionDescription') }}
        </p>
      </div>

      <Button
        variant="outline"
        size="sm"
        class="shrink-0"
        :disabled="isLoading"
        @click="handleRefresh"
      >
        <Spinner
          v-if="isLoading"
          class="size-3"
        />
        <RefreshCw
          v-else
          class="size-4"
        />
        {{ $t('common.refresh') }}
      </Button>
    </div>

    <div class="space-y-8">
      <SettingsSection :title="$t('bots.settings.compactionEnabled')">
        <SettingsRow
          :label="$t('bots.settings.compactionEnabled')"
          :description="$t('bots.settings.compactionDescription')"
        >
          <Switch
            :model-value="settingsForm.compaction_enabled"
            @update:model-value="(val) => settingsForm.compaction_enabled = !!val"
          />
        </SettingsRow>

        <template v-if="settingsForm.compaction_enabled">
          <SettingsRow :label="$t('bots.settings.compactionThreshold')">
            <Input
              v-model.number="settingsForm.compaction_threshold"
              type="number"
              :min="1"
              placeholder="100000"
              class="h-8 w-32 tabular-nums"
            />
          </SettingsRow>

          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.settings.compactionRatio') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.settings.compactionRatioDescription') }}
              </p>
            </div>
            <div class="flex w-48 shrink-0 items-center gap-3">
              <Slider
                :model-value="[settingsForm.compaction_ratio]"
                :min="1"
                :max="100"
                :step="1"
                class="min-w-0 flex-1"
                @update:model-value="(val) => settingsForm.compaction_ratio = val[0]"
              />
              <span class="w-10 text-right text-xs tabular-nums text-muted-foreground">
                {{ settingsForm.compaction_ratio }}%
              </span>
            </div>
          </div>

          <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ $t('bots.settings.compactionModel') }}
              </div>
              <p class="mt-0.5 text-xs text-muted-foreground">
                {{ $t('bots.settings.compactionModelDescription') }}
              </p>
            </div>
            <ModelSelect
              v-model="settingsForm.compaction_model_id"
              :models="models"
              :providers="providers"
              model-type="chat"
              :placeholder="$t('bots.settings.compactionModelPlaceholder')"
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

      <SettingsSection :title="$t('bots.compaction.title')">
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 border-b border-border py-3">
          <div class="flex min-w-0 items-center gap-2">
            <History class="size-4 text-muted-foreground" />
            <span class="text-sm font-medium text-foreground">
              {{ $t('bots.compaction.title') }}
            </span>
          </div>
          <NativeSelect
            v-model="statusFilter"
            class="w-32"
          >
            <option value="">
              {{ $t('bots.compaction.filterAll') }}
            </option>
            <option value="ok">
              {{ $t('bots.compaction.statusOk') }}
            </option>
            <option value="pending">
              {{ $t('bots.compaction.statusPending') }}
            </option>
            <option value="error">
              {{ $t('bots.compaction.statusError') }}
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
          class="m-4 rounded-[var(--radius-menu-shell)] border border-dashed border-border py-12"
        >
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Minimize2 />
            </EmptyMedia>
            <EmptyTitle>{{ $t('bots.compaction.empty') }}</EmptyTitle>
          </EmptyHeader>
        </Empty>

        <template v-else>
          <div class="overflow-x-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="border-b border-border">
                  <th class="px-4 py-2.5 text-left font-medium text-muted-foreground">
                    {{ $t('bots.compaction.status') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-medium text-muted-foreground">
                    {{ $t('bots.compaction.time') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-medium text-muted-foreground">
                    {{ $t('bots.compaction.duration') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-medium text-muted-foreground">
                    {{ $t('bots.compaction.error') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border">
                <template
                  v-for="log in filteredLogs"
                  :key="log.id"
                >
                  <tr
                    class="cursor-pointer transition-colors hover:bg-accent"
                    @click="toggleExpand(log.id)"
                  >
                    <td class="px-4 py-3">
                      <Badge
                        :variant="statusVariant(log.status)"
                        size="sm"
                      >
                        {{ statusLabel(log.status) }}
                      </Badge>
                    </td>
                    <td class="px-4 py-3 text-muted-foreground font-mono">
                      {{ formatDateTime(log.started_at) }}
                    </td>
                    <td class="px-4 py-3 text-muted-foreground font-mono">
                      {{ formatDuration(log.started_at, log.completed_at) }}
                    </td>
                    <td class="px-4 py-3">
                      <span
                        v-if="log.error_message"
                        class="text-destructive truncate max-w-[200px] block"
                      >{{ log.error_message }}</span>
                      <span
                        v-else
                        class="text-muted-foreground"
                      >—</span>
                    </td>
                  </tr>
                  <tr
                    v-if="log.id && expandedIds.has(log.id)"
                    class="border-t border-border"
                  >
                    <td
                      colspan="4"
                      class="px-4 py-4"
                    >
                      <div class="space-y-3">
                        <div
                          v-if="log.error_message"
                          class="rounded-md border border-border bg-card p-3"
                        >
                          <p class="whitespace-pre-wrap font-mono text-xs text-destructive">
                            {{ log.error_message }}
                          </p>
                        </div>
                        <div
                          v-if="log.usage"
                          class="space-y-1"
                        >
                          <span class="text-xs font-medium text-muted-foreground">{{ $t('common.usage') }}</span>
                          <div class="whitespace-pre-wrap rounded-md border border-border bg-card p-3 font-mono text-xs text-muted-foreground">
                            {{ JSON.stringify(log.usage, null, 2) }}
                          </div>
                        </div>
                      </div>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <div
            v-if="totalPages > 1"
            class="flex items-center justify-between border-t border-border p-4"
          >
            <span class="text-xs tabular-nums text-muted-foreground">
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
        </template>
      </SettingsSection>

      <SettingsSection
        v-if="logs.length > 0"
        :title="$t('common.dangerZone')"
      >
        <div class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.compaction.clearLogs') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ $t('bots.compaction.clearConfirm') }}
            </p>
          </div>
          <ConfirmPopover
            :message="$t('bots.compaction.clearConfirm')"
            :loading="isClearing"
            :confirm-text="$t('bots.compaction.clearLogs')"
            @confirm="handleClear"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
                :disabled="isClearing"
              >
                <Spinner
                  v-if="isClearing"
                  class="size-3"
                />
                {{ $t('bots.compaction.clearLogs') }}
              </Button>
            </template>
          </ConfirmPopover>
        </div>
      </SettingsSection>
    </div>
  </div>
</template>
