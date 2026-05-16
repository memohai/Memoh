<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground">
          {{ $t('bots.heartbeat.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.settings.heartbeatDescription') }}
        </p>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <NativeSelect
          v-model="statusFilter"
          class="h-8 w-24 text-[11px] shadow-none bg-background/50 border-border/50"
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

        <ConfirmPopover
          v-if="logs.length > 0"
          :message="$t('bots.heartbeat.clearConfirm')"
          :loading="isClearing"
          :confirm-text="$t('bots.heartbeat.clearLogs')"
          @confirm="handleClear"
        >
          <template #trigger>
            <Button
              variant="ghost"
              size="sm"
              class="h-8 text-xs font-medium px-3 shadow-none text-muted-foreground hover:text-destructive"
              :disabled="isClearing"
            >
              <Trash2 class="mr-2 size-3.5" />
              {{ $t('bots.heartbeat.clearLogs') }}
            </Button>
          </template>
        </ConfirmPopover>

        <Button
          variant="outline"
          size="sm"
          class="h-8 text-xs font-medium px-3 shadow-none border-border/50 bg-background/60 backdrop-blur-sm"
          :disabled="isLoading"
          @click="handleRefresh"
        >
          <RotateCw
            class="mr-2 size-3.5"
            :class="{ 'animate-spin': isLoading }"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
    </header>

    <!-- Configuration Panel (Compact) -->
    <section class="rounded-md border border-border/60 bg-background p-4 shadow-sm space-y-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2.5">
          <div class="p-1.5 rounded bg-muted/30">
            <HeartPulse class="size-4 text-muted-foreground" />
          </div>
          <div>
            <h3 class="text-xs font-semibold text-foreground">
              {{ $t('bots.settings.heartbeatEnabled') }}
            </h3>
            <p class="text-[10px] text-muted-foreground">
              {{ $t('bots.settings.heartbeatModelDescription') }}
            </p>
          </div>
        </div>
        <Switch
          :model-value="settingsForm.heartbeat_enabled"
          @update:model-value="(val) => settingsForm.heartbeat_enabled = !!val"
        />
      </div>

      <Transition
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="transform -translate-y-2 opacity-0"
        enter-to-class="transform translate-y-0 opacity-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="transform translate-y-0 opacity-100"
        leave-to-class="transform -translate-y-2 opacity-0"
      >
        <div
          v-if="settingsForm.heartbeat_enabled"
          class="grid gap-4 md:grid-cols-3 pt-2 border-t border-border/40"
        >
          <div class="space-y-1.5">
            <Label class="text-[11px] font-medium text-foreground">{{ $t('bots.settings.heartbeatInterval') }}</Label>
            <Input
              v-model.number="settingsForm.heartbeat_interval"
              type="number"
              :min="1"
              class="h-8 text-[11px] font-mono bg-muted/5 border-border/50 shadow-none"
              :placeholder="'30'"
            />
          </div>
          <div class="space-y-1.5 md:col-span-2">
            <Label class="text-[11px] font-medium text-foreground">{{ $t('bots.settings.heartbeatModel') }}</Label>
            <ModelSelect
              v-model="settingsForm.heartbeat_model_id"
              :models="models"
              :providers="providers"
              model-type="chat"
              class="h-8 text-[11px]"
            />
          </div>
        </div>
      </Transition>

      <div class="flex justify-end pt-1">
        <Button
          size="sm"
          class="h-8 text-xs font-medium px-4 shadow-none bg-foreground text-background hover:bg-foreground/90"
          :disabled="!settingsChanged || isSaving"
          @click="handleSaveSettings"
        >
          <Spinner
            v-if="isSaving"
            class="mr-2 size-3.5"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </section>

    <!-- Logs List -->
    <div class="space-y-3">
      <!-- Loading State -->
      <div
        v-if="isLoading && logs.length === 0"
        class="flex items-center justify-center py-12 text-xs text-muted-foreground"
      >
        <Spinner class="mr-2 size-4" />
        {{ $t('common.loading') }}
      </div>

      <!-- Empty State -->
      <div
        v-else-if="!isLoading && filteredLogs.length === 0"
        class="flex flex-col items-center justify-center py-16 text-center border border-dashed border-border/50 bg-muted/5 rounded-lg relative overflow-hidden"
      >
        <!-- Blueprint Grid Background -->
        <div class="absolute inset-0 opacity-[0.03] pointer-events-none bg-[linear-gradient(to_right,#80808012_1px,transparent_1px),linear-gradient(to_bottom,#80808012_1px,transparent_1px)] bg-[size:24px_24px]" />
        
        <div class="relative z-10 flex flex-col items-center">
          <div class="rounded-md bg-background border border-border/50 p-2.5 mb-4 shadow-sm">
            <Zap class="size-5 text-muted-foreground" />
          </div>
          <h3 class="text-sm font-medium text-foreground">
            {{ $t('bots.heartbeat.empty') }}
          </h3>
          <p class="text-[11px] text-muted-foreground mt-1.5 max-w-sm">
            {{ statusFilter ? $t('bots.heartbeat.filterEmpty') : $t('bots.heartbeat.description') }}
          </p>
        </div>
      </div>

      <!-- Log Cards -->
      <div
        v-else
        class="space-y-2"
      >
        <div
          v-for="log in filteredLogs"
          :key="log.id"
          class="group rounded-md border border-border/60 bg-background hover:bg-accent/30 transition-colors duration-75 p-3 flex flex-col gap-2 relative cursor-pointer"
          @click="toggleExpand(log.id)"
        >
          <!-- Card Header -->
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2.5 min-w-0">
              <div 
                class="size-1.5 rounded-full shrink-0"
                :class="statusColorClass(log.status)"
              />
              <span class="font-mono text-[11px] font-semibold text-foreground uppercase tracking-tight">
                {{ statusLabel(log.status) }}
              </span>
              <span class="font-mono text-[10px] text-muted-foreground/60">
                {{ formatDateTime(log.started_at) }}
              </span>
            </div>

            <div class="flex items-center gap-3 shrink-0">
              <span class="font-mono text-[10px] text-muted-foreground/80 px-1.5 py-0.5 rounded bg-muted/40">
                {{ formatDuration(log.started_at, log.completed_at) }}
              </span>
              <ChevronDown 
                class="size-3 text-muted-foreground/40 transition-transform duration-200"
                :class="{ 'rotate-180': log.id && expandedIds.has(log.id) }"
              />
            </div>
          </div>

          <!-- Card Body / Result Summary -->
          <div 
            v-if="!expandedIds.has(log.id)"
            class="pl-4"
          >
            <p 
              class="text-[11px] leading-snug line-clamp-1 break-words"
              :class="log.status === 'error' ? 'text-destructive/80' : 'text-muted-foreground'"
            >
              {{ log.status === 'error' ? (log.error_message || $t('bots.heartbeat.noResult')) : (truncateText(log.result_text) || $t('bots.heartbeat.noResult')) }}
            </p>
          </div>

          <!-- Expanded Detail Section -->
          <div
            v-if="log.id && expandedIds.has(log.id)"
            class="pl-4 pt-1 space-y-3"
          >
            <div class="p-3 rounded-md bg-muted/20 border border-border/40 overflow-hidden shadow-inner">
              <pre class="font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all text-foreground/90">{{ log.result_text || $t('bots.heartbeat.noResult') }}</pre>
            </div>
            
            <div 
              v-if="log.error_message"
              class="p-3 rounded-md bg-destructive/5 border border-destructive/20"
            >
              <p class="font-mono text-[11px] text-destructive leading-normal">
                {{ log.error_message }}
              </p>
            </div>

            <div 
              v-if="log.usage"
              class="flex flex-wrap gap-2 pt-1"
            >
              <div 
                v-for="(val, key) in (log.usage as any)"
                :key="key"
                class="px-1.5 py-0.5 rounded border border-border/40 bg-muted/10 font-mono text-[9px] text-muted-foreground"
              >
                {{ key }}: {{ val }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Pagination -->
      <div
        v-if="totalPages > 1"
        class="flex items-center justify-between pt-4 border-t border-border/40"
      >
        <span class="font-mono text-[10px] text-muted-foreground whitespace-nowrap">
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
            <PaginationFirst class="h-8 w-8" />
            <PaginationPrevious class="h-8 w-8" />
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
                class="h-8 w-8 text-[11px]"
              />
            </template>
            <PaginationNext class="h-8 w-8" />
            <PaginationLast class="h-8 w-8" />
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { HeartPulse, Trash2, RotateCw, Zap, ChevronDown } from 'lucide-vue-next'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Spinner, NativeSelect, Label, Switch, Input,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
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
  heartbeat_interval: 30,
  heartbeat_model_id: '',
})

watch(settings, (val: SettingsSettings | undefined) => {
  if (val) {
    settingsForm.heartbeat_enabled = val.heartbeat_enabled ?? false
    settingsForm.heartbeat_interval = val.heartbeat_interval ?? 30
    settingsForm.heartbeat_model_id = val.heartbeat_model_id ?? ''
  }
}, { immediate: true })

const settingsChanged = computed(() => {
  if (!settings.value) return false
  const s: SettingsSettings = settings.value
  return settingsForm.heartbeat_enabled !== (s.heartbeat_enabled ?? false)
    || settingsForm.heartbeat_interval !== (s.heartbeat_interval ?? 30)
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

function statusColorClass(status: string | undefined) {
  if (status === 'ok') return 'bg-green-500'
  if (status === 'alert') return 'bg-yellow-500'
  return 'bg-destructive shadow-[0_0_8px_rgba(239,68,68,0.4)]'
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

