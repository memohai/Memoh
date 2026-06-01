<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Top Action Bar -->
    <div class="flex items-center justify-between pb-4 border-b border-border/50">
      <div class="space-y-1">
        <h3 class="text-sm font-semibold text-foreground">
          {{ $t('bots.tabs.settings') }}
        </h3>
        <p class="text-[11px] text-muted-foreground">
          Manage system behaviors and global parameters.
        </p>
      </div>

      <div class="flex items-center gap-3 shrink-0">
        <Transition name="fade">
          <div
            v-if="hasChanges"
            class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
          >
            <div class="size-1 rounded-full bg-muted-foreground/40" />
            <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">Unsaved</span>
          </div>
        </Transition>

        <Button
          size="sm"
          :disabled="!hasChanges || saveLoading || !nameValid"
          class="h-8 text-xs font-medium min-w-24 shadow-none"
          @click="handleSave"
        >
          <Spinner
            v-if="saveLoading"
            class="mr-1.5 size-3"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <!-- Standardized Card Container -->
    <div class="space-y-4">
      <div class="space-y-3 rounded-md border border-border bg-background p-4 shadow-none">
        <div class="space-y-0.5">
          <h4 class="text-xs font-medium text-foreground">
            {{ $t('bots.name') }}
          </h4>
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.nameHint') }}
          </p>
        </div>
        <div class="relative">
          <Input
            v-model="nameInput"
            type="text"
            autocapitalize="off"
            autocomplete="off"
            spellcheck="false"
            class="h-8 pr-9 text-xs"
            :placeholder="$t('bots.namePlaceholder')"
          />
          <span class="absolute right-3 top-1/2 -translate-y-1/2">
            <LoaderCircle
              v-if="nameStatus === 'checking'"
              class="size-4 animate-spin text-muted-foreground"
            />
            <Check
              v-else-if="nameStatus === 'available'"
              class="size-4 text-success-foreground"
            />
            <X
              v-else-if="nameStatus === 'taken' || nameStatus === 'invalid' || nameStatus === 'reserved'"
              class="size-4 text-destructive"
            />
          </span>
        </div>
        <p
          v-if="nameStatusMessage"
          class="text-xs"
          :class="nameStatus === 'available'
            ? 'text-success-foreground'
            : 'text-destructive'"
        >
          {{ nameStatusMessage }}
        </p>
      </div>

      <SettingsGlobalCard :form="form" />
      
      <SettingsInteractionCard
        :form="form"
        :models="models"
        :providers="providers"
      />

      <SettingsContextCard
        :form="form"
        :search-providers="searchProviders"
        :memory-providers="memoryProviders"
        :persisted-memory-provider-i-d="persistedMemoryProviderID"
        :memory-status="memoryStatus"
        :is-memory-status-loading="isMemoryStatusLoading"
        :is-rebuilding="isRebuilding"
        @sync-memory="handleMemorySync"
      />
      
      <SettingsMultimediaCard
        :form="form"
        :tts-models="ttsModels"
        :tts-providers="ttsProviders"
        :transcription-models="transcriptionModels"
        :image-capable-models="imageCapableModels"
        :providers="providers"
      />
    </div>

    <!-- Bot Backup -->
    <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div class="space-y-0.5">
          <h4 class="text-xs font-medium text-foreground">
            {{ $t('bots.backup.cardTitle') }}
          </h4>
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.backup.cardSubtitle') }}
          </p>
        </div>
        <BotBackupActions
          :bot-id="botId"
          :bot-name="bot?.display_name"
          @imported="handleBackupImported"
        />
      </div>
    </div>

    <!-- Danger Zone: Isolated with top margin -->
    <div class="pt-4">
      <SettingsDangerZone
        :delete-loading="deleteLoading"
        @delete="handleDeleteBot"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Spinner,
} from '@memohai/ui'
import { Check, X, LoaderCircle } from 'lucide-vue-next'
import { reactive, ref, computed, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import SettingsGlobalCard from './settings-global-card.vue'
import SettingsInteractionCard from './settings-interaction-card.vue'
import SettingsContextCard from './settings-context-card.vue'
import SettingsMultimediaCard from './settings-multimedia-card.vue'
import SettingsDangerZone from './settings-danger-zone.vue'
import BotBackupActions from './bot-backup-actions.vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsById, putBotsById, getBotsByBotIdSettings, putBotsByBotIdSettings, deleteBotsById, getModels, getProviders, getSearchProviders, getMemoryProviders, getSpeechProviders, getSpeechModels, getTranscriptionProviders, getTranscriptionModels, getBotsByBotIdMemoryStatus, postBotsByBotIdMemoryRebuild, getBotsNameAvailability } from '@memohai/sdk'
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

const { data: bot } = useQuery({
  key: () => ['bot', botIdRef.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botIdRef.value }, throwOnError: true })
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
  key: ['speech-providers'],
  query: async () => {
    const { data } = await getSpeechProviders({ throwOnError: true })
    return data
  },
})

const { data: ttsModelData } = useQuery({
  key: ['speech-models'],
  query: async () => {
    const { data } = await getSpeechModels({ throwOnError: true })
    return data
  },
})

const { data: transcriptionModelData } = useQuery({
  key: ['transcription-models'],
  query: async () => {
    const { data } = await getTranscriptionModels({ throwOnError: true })
    return data
  },
})

const { data: transcriptionProviderData } = useQuery({
  key: ['transcription-providers'],
  query: async () => {
    const { data } = await getTranscriptionProviders({ throwOnError: true })
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

const { mutateAsync: updateBot, isLoading: isUpdatingBot } = useMutation({
  mutation: async (body: { timezone?: string, name?: string }) => {
    const { data } = await putBotsById({
      path: { id: botIdRef.value },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot', botIdRef.value] })
    queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] })
    queryCache.invalidateQueries({ key: ['bots'] })
  },
})

// ---- URL name (slug) editing ----
type NameStatus = 'idle' | 'checking' | 'available' | 'taken' | 'invalid' | 'reserved'
const nameInput = ref('')
const nameStatus = ref<NameStatus>('idle')

watch(bot, (val) => {
  nameInput.value = val?.name ?? ''
  nameStatus.value = 'idle'
}, { immediate: true })

const hasNameChange = computed(() => nameInput.value.trim() !== (bot.value?.name ?? ''))

const checkNameAvailability = useDebounceFn(async (candidate: string) => {
  const normalized = candidate.trim()
  if (!normalized || !hasNameChange.value) {
    nameStatus.value = 'idle'
    return
  }
  try {
    const { data } = await getBotsNameAvailability({
      query: { name: normalized, exclude_bot_id: botIdRef.value },
      throwOnError: true,
    })
    nameStatus.value = data?.available ? 'available' : ((data?.reason as NameStatus) || 'taken')
  } catch {
    nameStatus.value = 'idle'
  }
}, 400)

watch(nameInput, (candidate) => {
  if (!hasNameChange.value) {
    nameStatus.value = 'idle'
    return
  }
  nameStatus.value = 'checking'
  void checkNameAvailability(candidate)
})

const nameStatusMessage = computed(() => {
  switch (nameStatus.value) {
    case 'checking':
      return t('bots.nameStatus.checking')
    case 'available':
      return t('bots.nameStatus.available')
    case 'taken':
      return t('bots.nameStatus.taken')
    case 'invalid':
      return t('bots.nameStatus.invalid')
    case 'reserved':
      return t('bots.nameStatus.reserved')
    default:
      return ''
  }
})

const nameValid = computed(() => !hasNameChange.value || nameStatus.value === 'available')

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
const transcriptionProviders = computed(() => (transcriptionProviderData.value ?? []).filter((p: Record<string, unknown>) => p.enable !== false))
const enabledTranscriptionProviderIds = computed(() => new Set(transcriptionProviders.value.map((p: Record<string, unknown>) => p.id as string)))
const ttsModels = computed(() => (ttsModelData.value ?? []).filter((m: Record<string, unknown>) => enabledTtsProviderIds.value.has(m.provider_id as string)))
const transcriptionModels = computed(() => (transcriptionModelData.value ?? []).filter((m: Record<string, unknown>) => enabledTranscriptionProviderIds.value.has(m.provider_id as string)))

// ---- Form ----
const form = reactive({
  chat_model_id: '',
  title_model_id: '',
  image_model_id: '',
  search_provider_id: '',
  memory_provider_id: '',
  tts_model_id: '',
  transcription_model_id: '',
  timezone: '',
  language: '',
  reasoning_enabled: false,
  reasoning_effort: 'medium',
  show_tool_calls_in_im: false,
})

const persistedMemoryProviderID = computed(() => settings.value?.memory_provider_id ?? '')

const { data: memoryStatusData, isLoading: isMemoryStatusLoading } = useQuery({
  key: () => ['bot-memory-status', botIdRef.value, persistedMemoryProviderID.value],
  query: async () => {
    const { data } = await getBotsByBotIdMemoryStatus({
      path: { bot_id: botIdRef.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!botIdRef.value,
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

watch(settings, (val) => {
  if (val) {
    form.chat_model_id = val.chat_model_id ?? ''
    form.title_model_id = val.title_model_id ?? ''
    form.image_model_id = val.image_model_id ?? ''
    form.search_provider_id = val.search_provider_id ?? ''
    form.memory_provider_id = val.memory_provider_id ?? ''
    form.tts_model_id = val.tts_model_id ?? ''
    form.transcription_model_id = val.transcription_model_id ?? ''
    form.language = val.language ?? ''
    form.reasoning_enabled = val.reasoning_enabled ?? false
    form.reasoning_effort = val.reasoning_effort || 'medium'
    form.show_tool_calls_in_im = val.show_tool_calls_in_im ?? false
  }
}, { immediate: true })

watch(bot, (val) => {
  form.timezone = val?.timezone ?? ''
}, { immediate: true })

const hasSettingsChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  return (
    form.chat_model_id !== (s.chat_model_id ?? '')
    || form.title_model_id !== (s.title_model_id ?? '')
    || form.image_model_id !== (s.image_model_id ?? '')
    || form.search_provider_id !== (s.search_provider_id ?? '')
    || form.memory_provider_id !== (s.memory_provider_id ?? '')
    || form.tts_model_id !== (s.tts_model_id ?? '')
    || form.transcription_model_id !== (s.transcription_model_id ?? '')
    || form.language !== (s.language ?? '')
    || form.reasoning_enabled !== (s.reasoning_enabled ?? false)
    || form.reasoning_effort !== (s.reasoning_effort || 'medium')
    || form.show_tool_calls_in_im !== (s.show_tool_calls_in_im ?? false)
  )
})

const hasTimezoneChanges = computed(() => form.timezone !== (bot.value?.timezone ?? ''))
const hasChanges = computed(() => hasSettingsChanges.value || hasTimezoneChanges.value || hasNameChange.value)
const saveLoading = computed(() => isLoading.value || isUpdatingBot.value)

function isNameConflict(error: unknown): boolean {
  const e = error as { status?: number, response?: { status?: number } } | null
  if (e?.status === 409 || e?.response?.status === 409) return true
  return resolveApiErrorMessage(error, '').toLowerCase().includes('already taken')
}

async function handleSave() {
  if (!nameValid.value) {
    toast.error(nameStatusMessage.value || t('bots.nameStatus.invalid'))
    return
  }
  try {
    if (hasSettingsChanges.value) {
      const { timezone: _timezone, ...settingsPayload } = form
      await updateSettings(settingsPayload)
    }
    if (hasTimezoneChanges.value || hasNameChange.value) {
      await updateBot({
        ...(hasTimezoneChanges.value ? { timezone: form.timezone } : {}),
        ...(hasNameChange.value ? { name: nameInput.value.trim() } : {}),
      })
    }
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    if (isNameConflict(error)) {
      nameStatus.value = 'taken'
      toast.error(t('bots.nameStatus.taken'))
      return
    }
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}

async function handleMemorySync() {
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

function handleBackupImported(botId: string) {
  queryCache.invalidateQueries({ key: ['bots'] })
  if (botId && botId !== props.botId) {
    router.push({ name: 'bot-detail', params: { botId } })
    return
  }
  queryCache.invalidateQueries({ key: ['bot', botIdRef.value] })
  queryCache.invalidateQueries({ key: ['bot-settings', botIdRef.value] })
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
