<template>
  <div class="space-y-6">
    <!-- Settings -->
    <div class="space-y-4 mx-auto">
      <div class="flex items-center justify-between">
        <div>
          <Label>{{ $t('bots.settings.compactionEnabled') }}</Label>
          <p class="text-xs text-muted-foreground mt-0.5">
            {{ $t('bots.settings.compactionDescription') }}
          </p>
        </div>
        <Switch
          :model-value="settingsForm.compaction_enabled"
          @update:model-value="(val) => settingsForm.compaction_enabled = !!val"
        />
      </div>
      <div
        v-if="settingsForm.compaction_enabled"
        class="space-y-4"
      >
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.compactionThreshold') }}</Label>
          <Input
            v-model.number="settingsForm.compaction_threshold"
            type="number"
            :min="1"
            :placeholder="'100000'"
            :aria-label="$t('bots.settings.compactionThreshold')"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.compactionRatio') }}</Label>
          <p class="text-xs text-muted-foreground mt-0.5">
            {{ $t('bots.settings.compactionRatioDescription') }}
          </p>
          <div class="flex items-center gap-3">
            <Slider
              :model-value="[settingsForm.compaction_ratio]"
              :min="1"
              :max="100"
              :step="1"
              class="flex-1"
              @update:model-value="(val) => settingsForm.compaction_ratio = val[0]"
            />
            <span class="text-sm text-muted-foreground w-10 text-right tabular-nums">{{ settingsForm.compaction_ratio }}%</span>
          </div>
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.compactionModel') }}</Label>
          <p class="text-xs text-muted-foreground mt-0.5">
            {{ $t('bots.settings.compactionModelDescription') }}
          </p>
          <ModelSelect
            v-model="settingsForm.compaction_model_id"
            :models="models"
            :providers="providers"
            model-type="chat"
            :placeholder="$t('bots.settings.compactionModelPlaceholder')"
          />
        </div>
      </div>
      <div class="flex justify-end">
        <Button
          size="sm"
          :disabled="!settingsChanged || isSaving"
          @click="handleSaveSettings"
        >
          <Spinner
            v-if="isSaving"
            class="mr-2 size-4"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <Separator />

    <!-- Logs header -->
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-medium">
        {{ $t('bots.compaction.title') }}
      </h3>
      <div class="flex items-center gap-2">
        <NativeSelect
          v-model="statusFilter"
          class="h-9 w-28 text-xs"
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
        <ConfirmPopover
          v-if="logs.length > 0"
          :message="$t('bots.compaction.clearConfirm')"
          :loading="isClearing"
          :confirm-text="$t('bots.compaction.clearLogs')"
          @confirm="handleClear"
        >
          <template #trigger>
            <Button
              variant="outline"
              size="sm"
              :disabled="isClearing"
            >
              {{ $t('bots.compaction.clearLogs') }}
            </Button>
          </template>
        </ConfirmPopover>
        <Button
          variant="outline"
          size="sm"
          :disabled="isLoading"
          @click="handleRefresh"
        >
          <Spinner
            v-if="isLoading"
            class="mr-2 size-4"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="isLoading && logs.length === 0"
      class="flex items-center justify-center py-8 text-xs text-muted-foreground"
    >
      <Spinner class="mr-2" />
      {{ $t('common.loading') }}
    </div>

    <!-- Empty -->
    <div
      v-else-if="!isLoading && filteredLogs.length === 0"
      class="flex flex-col items-center justify-center py-12 text-center"
    >
      <div class="rounded-full bg-muted p-3 mb-4">
        <Minimize2
          class="size-6 text-muted-foreground"
        />
      </div>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.compaction.empty') }}
      </p>
    </div>

    <!-- Logs -->
    <template v-else>
      <div class="rounded-md border">
        <table class="w-full text-xs">
          <thead>
            <tr class="border-b bg-muted/50">
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.compaction.status') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.compaction.time') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.compaction.duration') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.compaction.error') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="log in filteredLogs"
              :key="log.id"
              class="border-b last:border-0 hover:bg-muted/30 cursor-pointer"
              @click="toggleExpand(log.id)"
            >
              <td class="px-4 py-2">
                <Badge :variant="statusVariant(log.status)">
                  {{ statusLabel(log.status) }}
                </Badge>
              </td>
              <td class="px-4 py-2 text-muted-foreground">
                {{ formatDateTime(log.started_at) }}
              </td>
              <td class="px-4 py-2 text-muted-foreground">
                {{ formatDuration(log.started_at, log.completed_at) }}
              </td>
              <td class="px-4 py-2">
                <span
                  v-if="log.error_message"
                  class="text-destructive"
                >{{ log.error_message }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Expanded detail -->
      <div
        v-for="log in filteredLogs.filter(l => l.id && expandedIds.has(l.id))"
        :key="'detail-' + log.id"
        class="rounded-md border bg-muted/20 p-4 text-xs whitespace-pre-wrap wrap-break-word"
      >
        <p
          v-if="log.error_message"
          class="text-destructive"
        >
          {{ log.error_message }}
        </p>
        <p
          v-if="log.usage"
          class="text-muted-foreground mt-2 text-xs"
        >
          Usage: {{ JSON.stringify(log.usage) }}
        </p>
      </div>

      <!-- Pagination -->
      <div
        v-if="totalPages > 1"
        class="flex items-center justify-between pt-2"
      >
        <span class="text-xs text-muted-foreground">
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
              />
            </template>
            <PaginationNext />
            <PaginationLast />
          </PaginationContent>
        </Pagination>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { Minimize2 } from 'lucide-vue-next'
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
  key: ['all-models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: ['all-providers'],
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
