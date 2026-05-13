<script setup lang="ts">
import { Minimize2, RefreshCw, History } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Badge, Spinner, NativeSelect, Label, Switch, Input, Separator, Slider,
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
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between gap-4">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
          {{ $t('bots.compaction.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.settings.compactionDescription') }}
        </p>
      </div>
      <div class="flex shrink-0 flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="isLoading"
          class="shadow-none"
          @click="handleRefresh"
        >
          <Spinner
            v-if="isLoading"
            class="mr-1.5 size-3.5"
          />
          <RefreshCw
            v-else
            class="mr-1.5 size-3.5 text-muted-foreground"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
    </header>

    <!-- Settings Bento -->
    <div class="rounded-md border border-border/60 bg-background overflow-hidden shadow-none">
      <div class="p-4 space-y-4">
        <div class="flex items-center justify-between gap-4">
          <div class="space-y-0.5">
            <Label class="text-xs font-medium">{{ $t('bots.settings.compactionEnabled') }}</Label>
            <p class="text-[11px] text-muted-foreground leading-snug">
              {{ $t('bots.settings.compactionDescription') }}
            </p>
          </div>
          <Switch
            :model-value="settingsForm.compaction_enabled"
            class="scale-90"
            @update:model-value="(val) => settingsForm.compaction_enabled = !!val"
          />
        </div>

        <template v-if="settingsForm.compaction_enabled">
          <Separator class="bg-border/40" />

          <div class="grid gap-4 sm:grid-cols-2">
            <div class="space-y-1.5">
              <Label class="text-xs font-medium">{{ $t('bots.settings.compactionThreshold') }}</Label>
              <Input
                v-model.number="settingsForm.compaction_threshold"
                type="number"
                :min="1"
                :placeholder="'100000'"
                class="h-8 text-xs bg-transparent border-border/60 shadow-none font-mono"
              />
            </div>
            <div class="space-y-1.5">
              <Label class="text-xs font-medium">{{ $t('bots.settings.compactionRatio') }}</Label>
              <p class="text-[11px] text-muted-foreground leading-snug">
                {{ $t('bots.settings.compactionRatioDescription') }}
              </p>
              <div class="flex items-center gap-3 pt-1">
                <Slider
                  :model-value="[settingsForm.compaction_ratio]"
                  :min="1"
                  :max="100"
                  :step="1"
                  class="flex-1"
                  @update:model-value="(val) => settingsForm.compaction_ratio = val[0]"
                />
                <span class="text-[11px] text-muted-foreground w-8 text-right tabular-nums font-mono">{{ settingsForm.compaction_ratio }}%</span>
              </div>
            </div>
            <div class="space-y-1.5 sm:col-span-2">
              <Label class="text-xs font-medium">{{ $t('bots.settings.compactionModel') }}</Label>
              <p class="text-[11px] text-muted-foreground leading-snug">
                {{ $t('bots.settings.compactionModelDescription') }}
              </p>
              <ModelSelect
                v-model="settingsForm.compaction_model_id"
                :models="models"
                :providers="providers"
                model-type="chat"
                :placeholder="$t('bots.settings.compactionModelPlaceholder')"
                class="mt-1"
              />
            </div>
          </div>
        </template>

        <Separator class="bg-border/40" />

        <div class="flex justify-end">
          <Button
            size="sm"
            :disabled="!settingsChanged || isSaving"
            class="h-8 text-xs font-medium px-4 shadow-none"
            @click="handleSaveSettings"
          >
            <Spinner
              v-if="isSaving"
              class="mr-1.5"
            />
            {{ $t('bots.settings.save') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Logs Bento -->
    <div class="space-y-4">
      <div class="rounded-md border border-border/60 bg-background overflow-hidden shadow-none">
        <div class="p-4 space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="space-y-0.5">
              <h4 class="text-xs font-medium text-foreground flex items-center gap-2">
                <History class="size-3.5 text-muted-foreground" />
                {{ $t('bots.compaction.title') }}
              </h4>
              <p class="text-[11px] text-muted-foreground leading-snug">
                {{ $t('bots.settings.compactionDescription') }}
              </p>
            </div>
            <div class="flex items-center gap-2 shrink-0 sm:justify-end">
              <NativeSelect
                v-model="statusFilter"
                class="h-8 w-28 text-[11px] bg-transparent border-border/60 shadow-none"
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
          </div>
        </div>

        <Separator class="bg-border/40" />

        <!-- Loading -->
        <div
          v-if="isLoading && logs.length === 0"
          class="flex items-center justify-center py-12 text-[11px] text-muted-foreground"
        >
          <Spinner class="mr-2" />
          {{ $t('common.loading') }}
        </div>

        <!-- Empty -->
        <div
          v-else-if="!isLoading && filteredLogs.length === 0"
          class="flex flex-col items-center justify-center py-12 text-center"
        >
          <div class="size-10 rounded-full bg-muted/20 flex items-center justify-center mb-4">
            <Minimize2 class="size-5 text-muted-foreground" />
          </div>
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.compaction.empty') }}
          </p>
        </div>

        <!-- Logs Table -->
        <template v-else>
          <div class="overflow-x-auto">
            <table class="w-full text-[11px]">
              <thead>
                <tr class="bg-muted/40 border-b border-border/50">
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
              <tbody class="divide-y divide-border/40">
                <template
                  v-for="log in filteredLogs"
                  :key="log.id"
                >
                  <tr
                    class="hover:bg-muted/30 transition-colors cursor-pointer group"
                    @click="toggleExpand(log.id)"
                  >
                    <td class="px-4 py-3">
                      <Badge
                        :variant="statusVariant(log.status)"
                        class="h-5 text-[10px] px-1.5 font-mono shadow-none"
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
                        class="text-muted-foreground/40"
                      >—</span>
                    </td>
                  </tr>
                  <!-- Expanded detail -->
                  <tr
                    v-if="log.id && expandedIds.has(log.id)"
                    class="bg-muted/5 border-t border-border/40"
                  >
                    <td
                      colspan="4"
                      class="px-4 py-4"
                    >
                      <div class="space-y-3">
                        <div
                          v-if="log.error_message"
                          class="rounded-md bg-destructive/5 border border-destructive/10 p-3"
                        >
                          <p class="text-destructive font-mono text-[10px] whitespace-pre-wrap">
                            {{ log.error_message }}
                          </p>
                        </div>
                        <div
                          v-if="log.usage"
                          class="space-y-1"
                        >
                          <span class="text-[9px] uppercase tracking-wider font-bold text-muted-foreground/60">Usage</span>
                          <div class="rounded-md bg-muted/20 border border-border/40 p-3 font-mono text-[10px] text-muted-foreground whitespace-pre-wrap">
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

          <!-- Pagination -->
          <div
            v-if="totalPages > 1"
            class="flex items-center justify-between p-4 border-t border-border/40"
          >
            <span class="text-[10px] text-muted-foreground font-mono">
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
                <PaginationFirst class="h-7 w-7" />
                <PaginationPrevious class="h-7 w-7" />
                <template
                  v-for="(item, index) in items"
                  :key="index"
                >
                  <PaginationEllipsis
                    v-if="item.type === 'ellipsis'"
                    :index="index"
                    class="h-7 w-7"
                  />
                  <PaginationItem
                    v-else
                    :value="item.value"
                    :is-active="item.value === currentPage"
                    class="h-7 w-7 text-[11px]"
                  />
                </template>
                <PaginationNext class="h-7 w-7" />
                <PaginationLast class="h-7 w-7" />
              </PaginationContent>
            </Pagination>
          </div>
        </template>
      </div>
    </div>

    <!-- Danger Zone -->
    <div
      v-if="logs.length > 0"
      class="pt-4"
    >
      <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div class="space-y-0.5">
            <h4 class="text-xs font-medium text-destructive">
              {{ $t('common.dangerZone') }}
            </h4>
            <p class="text-[11px] text-muted-foreground">
              Permanently delete all compaction history logs.
            </p>
          </div>
          <div class="flex justify-end shrink-0">
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
                  class="inline-flex items-center justify-center whitespace-nowrap transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-lg gap-1.5 px-3 min-w-28 h-8 text-xs font-medium shadow-none"
                >
                  <Spinner
                    v-if="isClearing"
                    class="mr-1.5"
                  />
                  {{ $t('bots.compaction.clearLogs') }}
                </Button>
              </template>
            </ConfirmPopover>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
