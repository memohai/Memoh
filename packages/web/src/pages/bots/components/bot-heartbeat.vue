<template>
  <div class="space-y-6">
    <!-- Settings -->
    <div class="space-y-4 mx-auto">
      <div class="flex items-center justify-between">
        <div>
          <Label>{{ $t('bots.settings.heartbeatEnabled') }}</Label>
          <p class="text-xs text-muted-foreground mt-0.5">
            {{ $t('bots.settings.heartbeatDescription') }}
          </p>
        </div>
        <Switch
          :model-value="settingsForm.heartbeat_enabled"
          @update:model-value="(val) => settingsForm.heartbeat_enabled = !!val"
        />
      </div>
      <div
        v-if="settingsForm.heartbeat_enabled"
        class="space-y-4"
      >
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.heartbeatInterval') }}</Label>
          <Input
            v-model.number="settingsForm.heartbeat_interval"
            type="number"
            :min="1"
            :placeholder="'30'"
            :aria-label="$t('bots.settings.heartbeatInterval')"
          />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.heartbeatModel') }}</Label>
          <p class="text-xs text-muted-foreground mt-0.5">
            {{ $t('bots.settings.heartbeatModelDescription') }}
          </p>
          <ModelSelect
            v-model="settingsForm.heartbeat_model_id"
            :models="models"
            :providers="providers"
            model-type="chat"
            :placeholder="$t('bots.settings.heartbeatModelPlaceholder')"
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
      <h3 class="text-lg font-medium">
        {{ $t('bots.heartbeat.title') }}
      </h3>
      <div class="flex items-center gap-2">
        <NativeSelect
          v-model="statusFilter"
          class="h-9 w-28 text-sm"
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
              variant="outline"
              size="sm"
              :disabled="isClearing"
            >
              {{ $t('bots.heartbeat.clearLogs') }}
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
      class="flex items-center justify-center py-8 text-sm text-muted-foreground"
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
        <FontAwesomeIcon
          :icon="['fas', 'heartbeat']"
          class="size-6 text-muted-foreground"
        />
      </div>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.heartbeat.empty') }}
      </p>
    </div>

    <!-- Logs -->
    <template v-else>
      <div class="rounded-md border">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b bg-muted/50">
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.heartbeat.status') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.heartbeat.time') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.heartbeat.duration') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.heartbeat.result') }}
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
                  v-if="log.status === 'error'"
                  class="text-destructive"
                >{{ log.error_message || $t('bots.heartbeat.noResult') }}</span>
                <span v-else>{{ truncateText(log.result_text) || $t('bots.heartbeat.noResult') }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Expanded detail -->
      <div
        v-for="log in filteredLogs.filter(l => l.id && expandedIds.has(l.id))"
        :key="'detail-' + log.id"
        class="rounded-md border bg-muted/20 p-4 text-sm whitespace-pre-wrap wrap-break-word"
      >
        <p v-if="log.result_text">
          {{ log.result_text }}
        </p>
        <p
          v-if="log.error_message"
          class="text-destructive mt-2"
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

      <!-- Load more -->
      <div
        v-if="hasMore"
        class="flex justify-center pt-2"
      >
        <Button
          variant="outline"
          size="sm"
          :disabled="isLoading"
          @click="loadMore"
        >
          <Spinner
            v-if="isLoading"
            class="mr-2 size-4"
          />
          {{ $t('bots.heartbeat.loadMore') }}
        </Button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Badge, Spinner, NativeSelect, Label, Switch, Input, Separator,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from './model-select.vue'
import {
  getBotsByBotIdSettings, putBotsByBotIdSettings,
  getBotsByBotIdHeartbeatLogs, deleteBotsByBotIdHeartbeatLogs,
  getModels, getProviders,
} from '@memoh/sdk'
import type { SettingsSettings, SettingsUpsertRequest, HeartbeatLog } from '@memoh/sdk'
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
const statusFilter = ref('')
const expandedIds = ref(new Set<string>())
const hasMore = ref(false)

const PAGE_SIZE = 50

const filteredLogs = computed(() => {
  if (!statusFilter.value) return logs.value
  return logs.value.filter(l => l.status === statusFilter.value)
})

function statusVariant(status: string | undefined) {
  if (status === 'ok') return 'secondary' as const
  if (status === 'alert') return 'default' as const
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

function truncateText(text: string | undefined, maxLen = 80) {
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

async function fetchLogs(before?: string) {
  if (!props.botId) return
  isLoading.value = true
  try {
    const { data } = await getBotsByBotIdHeartbeatLogs({
      path: { bot_id: props.botId },
      query: { limit: PAGE_SIZE, ...(before ? { before } : {}) },
      throwOnError: true,
    })
    const items = data?.items ?? []
    if (!before) {
      logs.value = items
    } else {
      logs.value.push(...items)
    }
    hasMore.value = items.length >= PAGE_SIZE
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.heartbeat.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

async function loadMore() {
  if (logs.value.length === 0) return
  const lastLog = logs.value[logs.value.length - 1]
  await fetchLogs(lastLog?.started_at)
}

async function handleRefresh() {
  expandedIds.value.clear()
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
