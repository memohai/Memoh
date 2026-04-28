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

    <!-- Transcription Model -->
    <div class="space-y-2">
      <Label>{{ $t('bots.settings.transcriptionModel') }}</Label>
      <TtsModelSelect
        v-model="form.transcription_model_id"
        :models="transcriptionModels"
        :providers="ttsProviders"
        :placeholder="$t('bots.settings.transcriptionModelPlaceholder')"
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

    <!-- Timezone -->
    <div class="space-y-2">
      <Label>{{ $t('bots.timezone') }}</Label>
      <TimezoneSelect
        :model-value="form.timezone || emptyTimezoneValue"
        :placeholder="$t('bots.timezonePlaceholder')"
        allow-empty
        :empty-label="$t('bots.timezoneInherited')"
        @update:model-value="(val: string) => form.timezone = val === emptyTimezoneValue ? '' : val"
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

    <!-- Timezone -->
    <div class="space-y-2">
      <Label>{{ $t('bots.timezone') }}</Label>
      <TimezoneSelect
        :model-value="form.timezone || emptyTimezoneValue"
        :placeholder="$t('bots.timezonePlaceholder')"
        allow-empty
        :empty-label="$t('bots.timezoneInherited')"
        @update:model-value="(val: string) => form.timezone = val === emptyTimezoneValue ? '' : val"
      />
    </div>

    <Separator />

    <!-- Reasoning -->
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

    <!-- Tool Approval -->
    <div class="space-y-3 rounded-lg border border-border p-4">
      <div class="flex items-center justify-between">
        <div class="space-y-1">
          <Label>{{ $t('bots.settings.toolApproval') }}</Label>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.settings.toolApprovalDescription') }}
          </p>
        </div>
        <Switch
          :model-value="form.tool_approval_config.enabled"
          @update:model-value="(val) => form.tool_approval_config.enabled = !!val"
        />
      </div>

      <div class="grid gap-3 md:grid-cols-3">
        <div
          v-for="tool in approvalTools"
          :key="tool"
          class="space-y-2 rounded-md border border-border p-3"
        >
          <div class="flex items-center justify-between gap-2">
            <Label class="font-mono">{{ tool }}</Label>
            <Switch
              :model-value="toolApprovalPolicy(tool).require_approval"
              @update:model-value="(val) => toolApprovalPolicy(tool).require_approval = !!val"
            />
          </div>
          <div class="space-y-1">
            <Label class="text-xs text-muted-foreground">{{ $t('bots.settings.toolApprovalBypass') }}</Label>
            <Textarea
              :model-value="approvalBypassText(tool)"
              class="min-h-20 font-mono text-xs"
              @update:model-value="(val) => updateApprovalBypass(tool, String(val))"
            />
          </div>
          <div class="space-y-1">
            <Label class="text-xs text-muted-foreground">{{ $t('bots.settings.toolApprovalMustReview') }}</Label>
            <Textarea
              :model-value="approvalForceReviewText(tool)"
              class="min-h-20 font-mono text-xs"
              @update:model-value="(val) => updateApprovalForceReview(tool, String(val))"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Show Tool Calls in IM -->
    <div class="flex items-center justify-between">
      <div class="space-y-1">
        <Label>{{ $t('bots.settings.showToolCallsInIM') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.settings.showToolCallsInIMDescription') }}
        </p>
      </div>
      <Switch
        :model-value="form.show_tool_calls_in_im"
        @update:model-value="(val) => form.show_tool_calls_in_im = !!val"
      />
    </div>

    <!-- Save -->
    <div class="flex justify-end">
      <Button
        :disabled="!hasChanges || saveLoading"
        @click="handleSave"
      >
        <Spinner v-if="saveLoading" />
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
  Textarea,
  Button,
  Separator,
  Spinner,
  Popover,
  PopoverTrigger,
  PopoverContent,
  Switch,
} from '@memohai/ui'
import { Lightbulb, ChevronDown } from 'lucide-vue-next'
import { reactive, computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import TimezoneSelect from '@/components/timezone-select/index.vue'
import ModelSelect from './model-select.vue'
import ReasoningEffortSelect from './reasoning-effort-select.vue'
import { EFFORT_LABELS, EFFORT_OPACITY } from './reasoning-effort'
import SearchProviderSelect from './search-provider-select.vue'
import MemoryProviderSelect from './memory-provider-select.vue'
import TtsModelSelect from './tts-model-select.vue'
import BrowserContextSelect from './browser-context-select.vue'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import { getBotsById, putBotsById, getBotsByBotIdSettings, putBotsByBotIdSettings, deleteBotsById, getModels, getProviders, getSearchProviders, getMemoryProviders, getSpeechProviders, getSpeechModels, getTranscriptionProviders, getTranscriptionModels, getBrowserContexts, getBotsByBotIdMemoryStatus, postBotsByBotIdMemoryRebuild } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import type { Ref } from 'vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { emptyTimezoneValue } from '@/utils/timezones'

const props = defineProps<{
  botId: string
}>()

type ApprovalTool = 'write' | 'edit' | 'exec'

interface ToolApprovalFilePolicy {
  require_approval: boolean
  bypass_globs: string[]
  force_review_globs: string[]
}

interface ToolApprovalExecPolicy {
  require_approval: boolean
  bypass_commands: string[]
  force_review_commands: string[]
}

interface ToolApprovalConfig {
  enabled: boolean
  write: ToolApprovalFilePolicy
  edit: ToolApprovalFilePolicy
  exec: ToolApprovalExecPolicy
}

const defaultToolApprovalConfig = (): ToolApprovalConfig => ({
  enabled: false,
  write: {
    require_approval: true,
    bypass_globs: ['/data/**', '/tmp/**'],
    force_review_globs: [],
  },
  edit: {
    require_approval: true,
    bypass_globs: ['/data/**', '/tmp/**'],
    force_review_globs: [],
  },
  exec: {
    require_approval: false,
    bypass_commands: [],
    force_review_commands: [],
  },
})

const approvalTools: ApprovalTool[] = ['write', 'edit', 'exec']

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

const { data: browserContextData } = useQuery({
  key: ['all-browser-contexts'],
  query: async () => {
    const { data } = await getBrowserContexts({ throwOnError: true })
    return data
  },
})

const { mutateAsync: updateSettings, isLoading } = useMutation({
  mutation: async (body: Partial<SettingsSettings> & { tool_approval_config?: ToolApprovalConfig }) => {
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
  mutation: async (timezone: string) => {
    const { data } = await putBotsById({
      path: { id: botIdRef.value },
      body: { timezone },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot', botIdRef.value] })
    queryCache.invalidateQueries({ key: ['bots'] })
  },
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
const transcriptionProviders = computed(() => (transcriptionProviderData.value ?? []).filter((p: Record<string, unknown>) => p.enable !== false))
const enabledTranscriptionProviderIds = computed(() => new Set(transcriptionProviders.value.map((p: Record<string, unknown>) => p.id as string)))
const ttsModels = computed(() => (ttsModelData.value ?? []).filter((m: Record<string, unknown>) => enabledTtsProviderIds.value.has(m.provider_id as string)))
const transcriptionModels = computed(() => (transcriptionModelData.value ?? []).filter((m: Record<string, unknown>) => enabledTranscriptionProviderIds.value.has(m.provider_id as string)))
const browserContexts = computed(() => browserContextData.value ?? [])

// ---- Form ----
const form = reactive({
  chat_model_id: '',
  title_model_id: '',
  image_model_id: '',
  search_provider_id: '',
  memory_provider_id: '',
  tts_model_id: '',
  transcription_model_id: '',
  browser_context_id: '',
  timezone: '',
  language: '',
  reasoning_enabled: false,
  reasoning_effort: 'medium',
  show_tool_calls_in_im: false,
  tool_approval_config: defaultToolApprovalConfig(),
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

function normalizeToolApprovalConfig(raw: unknown): ToolApprovalConfig {
  const defaults = defaultToolApprovalConfig()
  if (!raw || typeof raw !== 'object') return defaults
  const value = raw as Partial<ToolApprovalConfig>
  return {
    enabled: value.enabled ?? defaults.enabled,
    write: {
      require_approval: value.write?.require_approval ?? defaults.write.require_approval,
      bypass_globs: value.write?.bypass_globs ?? defaults.write.bypass_globs,
      force_review_globs: value.write?.force_review_globs ?? defaults.write.force_review_globs,
    },
    edit: {
      require_approval: value.edit?.require_approval ?? defaults.edit.require_approval,
      bypass_globs: value.edit?.bypass_globs ?? defaults.edit.bypass_globs,
      force_review_globs: value.edit?.force_review_globs ?? defaults.edit.force_review_globs,
    },
    exec: {
      require_approval: value.exec?.require_approval ?? defaults.exec.require_approval,
      bypass_commands: value.exec?.bypass_commands ?? defaults.exec.bypass_commands,
      force_review_commands: value.exec?.force_review_commands ?? defaults.exec.force_review_commands,
    },
  }
}

function toolApprovalPolicy(tool: ApprovalTool) {
  return form.tool_approval_config[tool]
}

function approvalBypassText(tool: ApprovalTool): string {
  const policy = toolApprovalPolicy(tool)
  return (tool === 'exec'
    ? (policy as ToolApprovalExecPolicy).bypass_commands
    : (policy as ToolApprovalFilePolicy).bypass_globs
  ).join('\n')
}

function updateApprovalBypass(tool: ApprovalTool, raw: string) {
  const values = raw.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
  if (tool === 'exec') {
    form.tool_approval_config.exec.bypass_commands = values
  } else {
    form.tool_approval_config[tool].bypass_globs = values
  }
}

function approvalForceReviewText(tool: ApprovalTool): string {
  const policy = toolApprovalPolicy(tool)
  return (tool === 'exec'
    ? (policy as ToolApprovalExecPolicy).force_review_commands
    : (policy as ToolApprovalFilePolicy).force_review_globs
  ).join('\n')
}

function updateApprovalForceReview(tool: ApprovalTool, raw: string) {
  const values = raw.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
  if (tool === 'exec') {
    form.tool_approval_config.exec.force_review_commands = values
  } else {
    form.tool_approval_config[tool].force_review_globs = values
  }
}

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
    form.transcription_model_id = val.transcription_model_id ?? ''
    form.browser_context_id = val.browser_context_id ?? ''
    form.language = val.language ?? ''
    form.timezone = val.timezone ?? ''
    form.reasoning_enabled = val.reasoning_enabled ?? false
    form.reasoning_effort = val.reasoning_effort || 'medium'
    form.show_tool_calls_in_im = val.show_tool_calls_in_im ?? false
    form.tool_approval_config = normalizeToolApprovalConfig((val as SettingsSettings & { tool_approval_config?: unknown }).tool_approval_config)
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
    || form.browser_context_id !== (s.browser_context_id ?? '')
    || form.language !== (s.language ?? '')
    || form.timezone !== (s.timezone ?? '')
    || form.reasoning_enabled !== (s.reasoning_enabled ?? false)
    || form.reasoning_effort !== (s.reasoning_effort || 'medium')
    || form.show_tool_calls_in_im !== (s.show_tool_calls_in_im ?? false)
    || JSON.stringify(form.tool_approval_config) !== JSON.stringify(normalizeToolApprovalConfig((s as SettingsSettings & { tool_approval_config?: unknown }).tool_approval_config))
  )
})

const hasTimezoneChanges = computed(() => form.timezone !== (bot.value?.timezone ?? ''))
const hasChanges = computed(() => hasSettingsChanges.value || hasTimezoneChanges.value)
const saveLoading = computed(() => isLoading.value || isUpdatingBot.value)

async function handleSave() {
  try {
    if (hasSettingsChanges.value) {
      const { timezone: _timezone, ...settingsPayload } = form
      await updateSettings(settingsPayload)
    }
    if (hasTimezoneChanges.value) {
      await updateBot(form.timezone)
    }
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
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
