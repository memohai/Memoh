<template>
  <div class="max-w-2xl mx-auto space-y-6">
    <!-- Chat Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.chatModel') }}</Label>
      <ModelSelect
        v-model="form.chat_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.chatModel')"
      />
    </div>

    <!-- Title Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.titleModel') }}</Label>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.settings.titleModelDescription') }}
      </p>
      <ModelSelect
        v-model="form.title_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.titleModelPlaceholder')"
      />
    </div>

    <!-- Memory Provider -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.memoryProvider') }}</Label>
      <MemoryProviderSelect
        v-model="form.memory_provider_id"
        :providers="memoryProviders"
        :placeholder="$t('bots.settings.memoryProviderPlaceholder')"
      />
      <div
        v-if="selectedBuiltinMemoryProvider"
        class="rounded-md border border-border bg-card px-3 py-2 text-xs text-muted-foreground"
      >
        {{ $t('bots.settings.memoryModePreview', {
          mode: $t(`memory.modeNames.${selectedBuiltinMemoryMode}`),
        }) }}
      </div>
      <div
        v-if="showMemoryProviderStatusCard"
        class="rounded-lg border border-border bg-card p-4 space-y-4"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="space-y-1">
            <p class="text-xs font-medium text-foreground">
              {{ indexedMemoryStatusTitle }}
            </p>
            <p class="text-xs text-muted-foreground">
              {{ isSelectedMemoryProviderPersisted
                ? indexedMemoryStatusHint
                : $t('bots.settings.indexedMemoryStatusPendingSave') }}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            :disabled="!isSelectedMemoryProviderPersisted || isRebuilding || !memoryStatus?.can_manual_sync"
            @click="handleMemorySync"
          >
            <Spinner
              v-if="isRebuilding"
              class="mr-1.5"
            />
            {{ $t('bots.settings.memorySyncAction') }}
          </Button>
        </div>

        <div
          v-if="isMemoryStatusLoading"
          class="text-xs text-muted-foreground"
        >
          {{ $t('common.loading') }}
        </div>

        <div
          v-else-if="statusCardData"
          class="grid gap-3 md:grid-cols-2"
        >
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memorySourceDir') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.source_dir || '-' }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memoryOverviewPath') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.overview_path || '-' }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memoryMarkdownFiles') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.markdown_file_count ?? 0 }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memorySourceEntries') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.source_count ?? 0 }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memoryIndexedEntries') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.indexed_count ?? 0 }}
            </p>
          </div>
          <div
            v-if="showQdrantDetails"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memoryQdrantCollection') }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.qdrant_collection || '-' }}
            </p>
          </div>
          <div
            v-if="showEncoderHealth"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ encoderHealthLabel }}
            </p>
            <p
              class="mt-1 text-xs font-medium"
              :class="healthTextClass(statusCardData.encoder?.ok)"
            >
              {{ healthLabel(statusCardData.encoder?.ok, statusCardData.encoder?.error) }}
            </p>
          </div>
          <div
            v-if="showQdrantHealth"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ $t('bots.settings.memoryQdrantHealth') }}
            </p>
            <p
              class="mt-1 text-xs font-medium"
              :class="healthTextClass(statusCardData.qdrant?.ok)"
            >
              {{ healthLabel(statusCardData.qdrant?.ok, statusCardData.qdrant?.error) }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Search Provider -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.searchProvider') }}</Label>
      <SearchProviderSelect
        v-model="form.search_provider_id"
        :providers="searchProviders"
        :placeholder="$t('bots.settings.searchProviderPlaceholder')"
      />
    </div>

    <!-- TTS Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.ttsModel') }}</Label>
      <TtsModelSelect
        v-model="form.tts_model_id"
        :models="ttsModels"
        :providers="ttsProviders"
        :placeholder="$t('bots.settings.ttsModelPlaceholder')"
      />
    </div>

    <!-- Image Generation Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.imageModel') }}</Label>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.settings.imageModelDescription') }}
      </p>
      <ModelSelect
        v-model="form.image_model_id"
        :models="imageCapableModels"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.imageModelPlaceholder')"
      />
    </div>

    <!-- Browser Context -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.browserContext') }}</Label>
      <BrowserContextSelect
        v-model="form.browser_context_id"
        :contexts="browserContexts"
        :placeholder="$t('bots.settings.browserContextPlaceholder')"
      />
    </div>

    <Separator />

    <!-- Language -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.language') }}</Label>
      <Input
        v-model="form.language"
        type="text"
        :aria-label="$t('bots.settings.language')"
      />
    </div>

    <!-- Reasoning -->
    <Separator />
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.reasoningEffort') }}</Label>
      <Popover v-model:open="reasoningPopoverOpen">
        <PopoverTrigger as-child>
          <Button
            variant="outline"
            role="combobox"
            :disabled="!chatModelSupportsReasoning"
            class="w-full justify-between font-normal"
          >
            <span class="flex items-center gap-2">
              <Lightbulb
                class="size-3.5"
                :style="{ opacity: EFFORT_OPACITY[reasoningFormValue] ?? 0.5 }"
              />
              {{ reasoningFormValue === 'off' ? $t('chat.reasoningOff') : $t(EFFORT_LABELS[reasoningFormValue] ?? reasoningFormValue) }}
            </span>
            <ChevronDown class="size-3.5 shrink-0 text-muted-foreground" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          class="w-[--reka-popover-trigger-width] p-0"
          align="start"
        >
          <ReasoningEffortSelect
            v-model="reasoningFormValue"
            :efforts="availableReasoningEfforts"
            @update:model-value="reasoningPopoverOpen = false"
          />
        </PopoverContent>
      </Popover>
    </div>

    <!-- Save -->
    <div class="flex justify-end">
      <Button
        :disabled="!hasChanges || isLoading"
        @click="handleSave"
      >
        <Spinner v-if="isLoading" />
        {{ $t('bots.settings.save') }}
      </Button>
    </div>

    <Separator />

    <!-- Danger Zone -->
    <div class="rounded-lg border border-destructive/50 bg-destructive/5 p-4 space-y-3">
      <h3 class="text-xs font-semibold text-destructive">
        {{ $t('bots.settings.dangerZone') }}
      </h3>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.settings.deleteBotDescription') }}
      </p>
      <div class="flex items-center justify-end">
        <ConfirmPopover
          :message="$t('bots.deleteConfirm')"
          :loading="deleteLoading"
          :confirm-text="$t('common.delete')"
          @confirm="handleDeleteBot"
        >
          <template #trigger>
            <Button
              variant="destructive"
              :disabled="deleteLoading"
            >
              <Spinner
                v-if="deleteLoading"
                class="mr-1.5"
              />
              {{ $t('bots.settings.deleteBot') }}
            </Button>
          </template>
        </ConfirmPopover>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Input,
  Button,
  Separator,
  Spinner,
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@memohai/ui'
import { Lightbulb, ChevronDown } from 'lucide-vue-next'
import { reactive, computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ModelSelect from './model-select.vue'
import ReasoningEffortSelect from './reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY } from './reasoning-effort'
import SearchProviderSelect from './search-provider-select.vue'
import MemoryProviderSelect from './memory-provider-select.vue'
import TtsModelSelect from './tts-model-select.vue'
import BrowserContextSelect from './browser-context-select.vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings, deleteBotsById, getModels, getProviders, getSearchProviders, getMemoryProviders, getTtsProviders, getBrowserContexts, getBotsByBotIdMemoryStatus, postBotsByBotIdMemoryRebuild } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import type { Ref } from 'vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const router = useRouter()

const botIdRef = computed(() => props.botId) as Ref<string>

// ---- Data ----
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

const { data: searchProviderData } = useQuery({
  key: ['all-search-providers'],
  query: async () => {
    const { data } = await getSearchProviders({ throwOnError: true })
    return data
  },
})

const { data: memoryProviderData } = useQuery({
  key: ['all-memory-providers'],
  query: async () => {
    const { data } = await getMemoryProviders({ throwOnError: true })
    return data
  },
})

const { data: ttsProviderData } = useQuery({
  key: ['tts-providers'],
  query: async () => {
    const { data } = await getTtsProviders({ throwOnError: true })
    return data
  },
})

const { data: ttsModelData } = useQuery({
  key: ['tts-models'],
  query: async () => {
    const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'
    const token = localStorage.getItem('token')
    const resp = await fetch(`${apiBase}/tts-models`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!resp.ok) throw new Error('Failed to fetch TTS models')
    return resp.json()
  },
})

const { data: browserContextData } = useQuery({
  key: ['all-browser-contexts'],
  query: async () => {
    const { data } = await getBrowserContexts({ throwOnError: true })
    return data
  },
})

const { mutateAsync: updateSettings, isLoading } = useMutation({
  mutation: async (body: Partial<SettingsSettings>) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] }),
})

const { mutateAsync: deleteBot, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    await deleteBotsById({ path: { id: botIdRef.value }, throwOnError: true })
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bots'] })
    queryCache.invalidateQueries({ key: ['bot'] })
  },
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])
const imageCapableModels = computed(() =>
  models.value.filter((m) => m.config?.compatibilities?.includes('image-output')),
)
const searchProviders = computed(() => (searchProviderData.value ?? []).filter((p) => p.enable !== false))
const memoryProviders = computed(() => memoryProviderData.value ?? [])
const ttsProviders = computed(() => (ttsProviderData.value ?? []).filter((p) => p.enable !== false))
const enabledTtsProviderIds = computed(() => new Set(ttsProviders.value.map((p) => p.id)))
const ttsModels = computed(() => (ttsModelData.value ?? []).filter((m: Record<string, unknown>) => enabledTtsProviderIds.value.has(m.tts_provider_id as string)))
const browserContexts = computed(() => browserContextData.value ?? [])

// ---- Form ----
const form = reactive({
  chat_model_id: '',
  title_model_id: '',
  image_model_id: '',
  search_provider_id: '',
  memory_provider_id: '',
  tts_model_id: '',
  browser_context_id: '',
  language: '',
  reasoning_enabled: false,
  reasoning_effort: 'medium',
})

const selectedMemoryProvider = computed(() =>
  memoryProviders.value.find((provider) => provider.id === form.memory_provider_id),
)
const selectedMemoryProviderType = computed(() =>
  selectedMemoryProvider.value?.provider ?? '',
)
const selectedBuiltinMemoryProvider = computed(() =>
  selectedMemoryProvider.value?.provider === 'builtin' ? selectedMemoryProvider.value : null,
)
const selectedMem0MemoryProvider = computed(() =>
  selectedMemoryProvider.value?.provider === 'mem0' ? selectedMemoryProvider.value : null,
)
const selectedBuiltinMemoryMode = computed(() =>
  (selectedBuiltinMemoryProvider.value?.config as Record<string, string> | undefined)?.memory_mode || 'off',
)
const persistedMemoryProviderID = computed(() => settings.value?.memory_provider_id ?? '')
const isSelectedMemoryProviderPersisted = computed(() =>
  !!form.memory_provider_id && form.memory_provider_id === persistedMemoryProviderID.value,
)
const showBuiltinIndexedMemoryStatus = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showMem0MemoryStatus = computed(() =>
  !!selectedMem0MemoryProvider.value,
)
const showMemoryProviderStatusCard = computed(() =>
  showBuiltinIndexedMemoryStatus.value || showMem0MemoryStatus.value,
)
const shouldLoadMemoryStatus = computed(() =>
  !!botIdRef.value
  && showMemoryProviderStatusCard.value
  && isSelectedMemoryProviderPersisted.value,
)
const indexedMemoryStatusTitle = computed(() =>
  selectedMemoryProviderType.value === 'mem0'
    ? t('bots.settings.mem0StatusTitle')
    : selectedBuiltinMemoryMode.value === 'dense'
    ? t('bots.settings.denseStatusTitle')
    : t('bots.settings.sparseStatusTitle'),
)
const indexedMemoryStatusHint = computed(() =>
  selectedMemoryProviderType.value === 'mem0'
    ? t('bots.settings.mem0StatusHint')
    : selectedBuiltinMemoryMode.value === 'dense'
    ? t('bots.settings.denseStatusHint')
    : t('bots.settings.sparseStatusHint'),
)

const chatModelSupportsReasoning = computed(() => {
  if (!form.chat_model_id) return false
  const m = models.value.find((m) => m.id === form.chat_model_id)
  return !!m?.config?.compatibilities?.includes('reasoning')
})

const availableReasoningEfforts = computed(() => {
  if (!form.chat_model_id) return ['low', 'medium', 'high']
  const model = models.value.find((m) => m.id === form.chat_model_id)
  const efforts = ((model?.config as { reasoning_efforts?: string[] } | undefined)?.reasoning_efforts ?? [])
    .filter((effort) => ['none', 'low', 'medium', 'high', 'xhigh'].includes(effort))
  return efforts.length > 0 ? efforts : ['low', 'medium', 'high']
})

watch(availableReasoningEfforts, (efforts) => {
  if (!efforts.includes(form.reasoning_effort)) {
    form.reasoning_effort = efforts.includes('medium') ? 'medium' : efforts[0] ?? 'medium'
  }
}, { immediate: true })

const reasoningPopoverOpen = ref(false)

const reasoningFormValue = computed({
  get: () => form.reasoning_enabled ? form.reasoning_effort : 'off',
  set: (v: string) => {
    if (v === 'off') {
      form.reasoning_enabled = false
    } else {
      form.reasoning_enabled = true
      form.reasoning_effort = v
    }
  },
})

const { data: memoryStatusData, isLoading: isMemoryStatusLoading } = useQuery({
  key: () => ['bot-memory-status', botIdRef.value, persistedMemoryProviderID.value],
  query: async () => {
    const { data } = await getBotsByBotIdMemoryStatus({
      path: { bot_id: botIdRef.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => shouldLoadMemoryStatus.value,
})

const { mutateAsync: rebuildMemory, isLoading: isRebuilding } = useMutation({
  mutation: async () => {
    const { data } = await postBotsByBotIdMemoryRebuild({
      path: { bot_id: botIdRef.value },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-memory-status', botIdRef.value, persistedMemoryProviderID.value] })
  },
})

const memoryStatus = computed(() => memoryStatusData.value ?? null)
const statusCardData = computed(() => memoryStatus.value)
const showQdrantDetails = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showEncoderHealth = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const showQdrantHealth = computed(() =>
  selectedBuiltinMemoryMode.value === 'sparse' || selectedBuiltinMemoryMode.value === 'dense',
)
const encoderHealthLabel = computed(() =>
  selectedBuiltinMemoryMode.value === 'dense'
    ? t('bots.settings.memoryDenseEmbeddingHealth')
    : t('bots.settings.memoryEncoderHealth'),
)

watch(settings, (val) => {
  if (val) {
    form.chat_model_id = val.chat_model_id ?? ''
    form.title_model_id = val.title_model_id ?? ''
    form.image_model_id = val.image_model_id ?? ''
    form.search_provider_id = val.search_provider_id ?? ''
    form.memory_provider_id = val.memory_provider_id ?? ''
    form.tts_model_id = val.tts_model_id ?? ''
    form.browser_context_id = val.browser_context_id ?? ''
    form.language = val.language ?? ''
    form.reasoning_enabled = val.reasoning_enabled ?? false
    form.reasoning_effort = val.reasoning_effort || 'medium'
  }
}, { immediate: true })

const hasChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  let changed =
    form.chat_model_id !== (s.chat_model_id ?? '')
    || form.title_model_id !== (s.title_model_id ?? '')
    || form.image_model_id !== (s.image_model_id ?? '')
    || form.search_provider_id !== (s.search_provider_id ?? '')
    || form.memory_provider_id !== (s.memory_provider_id ?? '')
    || form.tts_model_id !== (s.tts_model_id ?? '')
    || form.browser_context_id !== (s.browser_context_id ?? '')
    || form.language !== (s.language ?? '')
    || form.reasoning_enabled !== (s.reasoning_enabled ?? false)
    || form.reasoning_effort !== (s.reasoning_effort || 'medium')
  return changed
})

async function handleSave() {
  try {
    await updateSettings({ ...form })
    toast.success(t('bots.settings.saveSuccess'))
  } catch {
    return
  }
}

function healthTextClass(ok: boolean | undefined) {
  return ok ? 'text-foreground' : 'text-destructive'
}

function healthLabel(ok: boolean | undefined, error?: string) {
  if (ok) return t('bots.settings.memoryHealthOk')
  if (error) return error
  return t('bots.settings.memoryHealthUnavailable')
}

async function handleMemorySync() {
  if (!isSelectedMemoryProviderPersisted.value) {
    toast.error(t('bots.settings.indexedMemoryStatusPendingSave'))
    return
  }
  try {
    const result = await rebuildMemory()
    toast.success(t('bots.settings.memorySyncSuccess', {
      fsCount: result?.fs_count ?? 0,
      restoredCount: result?.restored_count ?? 0,
      storageCount: result?.storage_count ?? 0,
    }))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.memorySyncFailed')))
  }
}

async function handleDeleteBot() {
  try {
    await deleteBot()
    await router.push({ name: 'bots' })
    toast.success(t('bots.deleteSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.lifecycle.deleteFailed')))
  }
}
</script>
