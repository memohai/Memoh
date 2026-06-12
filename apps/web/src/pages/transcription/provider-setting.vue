<template>
  <SettingsShell width="narrow">
    <section class="flex items-center gap-3">
      <span class="flex size-10 shrink-0 items-center justify-center rounded-full bg-muted">
        <ProviderIcon
          v-if="curProvider?.icon"
          :icon="curProvider.icon"
          size="1.5em"
        />
        <span
          v-else
          class="text-xs font-medium text-muted-foreground"
        >
          {{ getInitials(curProvider?.name) }}
        </span>
      </span>
      <div class="min-w-0">
        <h2 class="text-sm font-semibold truncate">
          {{ curProvider?.name }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ currentMeta?.display_name ?? curProvider?.client_type }}
        </p>
      </div>
      <div class="ml-auto flex items-center gap-2">
        <span class="text-xs text-muted-foreground">
          {{ $t('common.enable') }}
        </span>
        <Switch
          :model-value="curProvider?.enable ?? false"
          :disabled="!curProvider?.id || enableLoading"
          @update:model-value="handleToggleEnable"
        />
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form
      @submit.prevent="handleSaveProvider"
    >
      <div class="grid gap-4 md:grid-cols-2">
        <section class="space-y-2 md:col-span-2">
          <Label for="transcription-provider-name">{{ $t('common.name') }}</Label>
          <Input
            id="transcription-provider-name"
            v-model="providerName"
            type="text"
            :placeholder="$t('common.namePlaceholder')"
          />
        </section>

        <section
          v-for="field in orderedProviderFields"
          :key="field.key"
          class="space-y-2"
          :class="isWideField(field) ? 'md:col-span-2' : ''"
        >
          <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `transcription-provider-${field.key}`">
            {{ field.title || field.key }}
          </Label>
          <p
            v-if="field.description"
            class="text-xs text-muted-foreground"
          >
            {{ field.description }}
          </p>
          <div
            v-if="field.type === 'secret'"
            class="relative"
          >
            <Input
              :id="`transcription-provider-${field.key}`"
              v-model="providerConfig[field.key] as string"
              :type="visibleSecrets[field.key] ? 'text' : 'password'"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
            >
              <component
                :is="visibleSecrets[field.key] ? EyeOff : Eye"
                class="size-3.5"
              />
            </button>
          </div>
          <Switch
            v-else-if="field.type === 'bool'"
            :model-value="!!providerConfig[field.key]"
            @update:model-value="(val) => providerConfig[field.key] = !!val"
          />
          <Input
            v-else-if="field.type === 'number'"
            :id="`transcription-provider-${field.key}`"
            v-model.number="providerConfig[field.key] as number"
            type="number"
          />
          <Select
            v-else-if="field.type === 'enum' && field.enum"
            :model-value="String(providerConfig[field.key] ?? '')"
            @update:model-value="(val) => providerConfig[field.key] = val"
          >
            <SelectTrigger>
              <SelectValue :placeholder="field.title || field.key" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="opt in field.enum"
                :key="opt"
                :value="opt"
              >
                {{ opt }}
              </SelectItem>
            </SelectContent>
          </Select>
          <Input
            v-else
            :id="`transcription-provider-${field.key}`"
            v-model="providerConfig[field.key] as string"
            type="text"
          />
        </section>
      </div>

      <div class="flex justify-end mt-4">
        <LoadingButton
          type="submit"
          :loading="saveLoading"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </div>
    </form>

    <Separator class="mt-6 mb-6" />

    <section>
      <div class="flex justify-between items-center mb-4">
        <h3 class="text-xs font-medium">
          {{ $t('transcription.models') }}
        </h3>
        <div
          v-if="curProviderId"
          class="flex items-center gap-2"
        >
          <LoadingButton
            type="button"
            variant="outline"
            size="sm"
            :loading="importLoading"
            @click="handleImportModels"
          >
            {{ $t('transcription.importModels') }}
          </LoadingButton>
          <CreateModel
            :id="curProviderId"
            default-type="transcription"
            hide-type
            :type-options="transcriptionTypeOptions"
            :invalidate-keys="['transcription-provider-models', 'transcription-models']"
          />
        </div>
      </div>

      <div
        v-if="providerModels.length === 0"
        class="text-xs text-muted-foreground py-4 text-center"
      >
        {{ $t('transcription.noModels') }}
      </div>

      <div
        v-for="model in providerModels"
        :key="model.id"
        class="border border-border rounded-lg mb-4"
      >
        <button
          type="button"
          class="w-full flex items-center justify-between p-3 text-left hover:bg-accent/50 rounded-t-lg transition-colors"
          @click="toggleModel(model.id ?? '')"
        >
          <div>
            <span class="text-xs font-medium">{{ model.name || model.model_id }}</span>
            <span
              v-if="model.name"
              class="text-xs text-muted-foreground ml-2"
            >
              {{ model.model_id }}
            </span>
          </div>
          <component
            :is="expandedModelId === model.id ? ChevronUp : ChevronDown"
            class="size-3 text-muted-foreground"
          />
        </button>
        <div
          v-if="expandedModelId === model.id"
          class="px-3 pb-3 space-y-4 border-t border-border pt-3"
        >
          <ModelConfigEditor
            :model-id="model.id ?? ''"
            :model-name="model.model_id ?? ''"
            :config="model.config || {}"
            :schema="getModelSchema(model.model_id ?? '')"
            mode="transcription"
            :on-test="(file, cfg) => handleTestModel(model.id ?? '', file as File, cfg)"
            @save="(cfg) => handleSaveModel(model.id ?? '', cfg)"
          />
        </div>
      </div>
    </section>
  </SettingsShell>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { toast } from '@memohai/ui'
import { useI18n } from 'vue-i18n'
import {
  getTranscriptionProvidersById,
  getTranscriptionProvidersMeta,
  getTranscriptionProvidersByIdModels,
  postTranscriptionProvidersByIdImportModels,
  postTranscriptionModelsByIdTest,
  putProvidersById,
  putTranscriptionModelsById,
} from '@memohai/sdk'
import type {
  AudioProviderMetaResponse,
  AudioSpeechProviderResponse,
  AudioTestTranscriptionResponse,
  AudioTranscriptionModelResponse,
} from '@memohai/sdk'
import { ChevronDown, ChevronUp, Eye, EyeOff } from 'lucide-vue-next'
import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Separator, Switch } from '@memohai/ui'
import ProviderIcon from '@/components/provider-icon/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import ModelConfigEditor from '@/pages/speech/components/model-config-editor.vue'
import CreateModel from '@/components/create-model/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'

interface FieldSchema { key: string, type: string, title?: string, description?: string, enum?: string[], order?: number }
interface ConfigSchema { fields?: FieldSchema[] }
interface ModelMeta { id: string, name: string, config_schema?: ConfigSchema, capabilities?: { config_schema?: ConfigSchema } }
interface ProviderMeta {
  provider: string
  display_name?: string
  config_schema?: ConfigSchema
  default_transcription_model?: string
  transcription_models?: ModelMeta[]
  models?: ModelMeta[]
}

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? ''
  return label ? label.slice(0, 2).toUpperCase() : '?'
}

function normalizeConfigSchema(schema?: AudioProviderMetaResponse['config_schema']): ConfigSchema | undefined {
  if (!schema) return undefined
  const fields: FieldSchema[] = []
  for (const field of schema.fields ?? []) {
    if (!field?.key || !field.type) continue
    fields.push({
      key: field.key,
      type: field.type,
      title: field.title,
      description: field.description,
      enum: field.enum,
      order: field.order,
    })
  }
  return { fields }
}

function normalizeModelMeta(model: NonNullable<AudioProviderMetaResponse['models']>[number]): ModelMeta | null {
  if (!model?.id) return null
  return {
    id: model.id,
    name: model.name ?? model.id,
    config_schema: normalizeConfigSchema(model.config_schema),
    capabilities: model.capabilities
      ? { config_schema: normalizeConfigSchema(model.capabilities.config_schema) }
      : undefined,
  }
}

function normalizeProviderMeta(meta: AudioProviderMetaResponse): ProviderMeta {
  return {
    provider: meta.provider ?? '',
    display_name: meta.display_name,
    config_schema: normalizeConfigSchema(meta.config_schema),
    default_transcription_model: meta.default_transcription_model,
    transcription_models: (meta.transcription_models ?? [])
      .map(normalizeModelMeta)
      .filter((model): model is ModelMeta => model !== null),
    models: (meta.models ?? [])
      .map(normalizeModelMeta)
      .filter((model): model is ModelMeta => model !== null),
  }
}

const { t } = useI18n()
const curProvider = inject('curTranscriptionProvider', ref<AudioSpeechProviderResponse>())
const curProviderId = computed(() => curProvider.value?.id)
const providerName = ref('')
const providerConfig = reactive<Record<string, unknown>>({})
const visibleSecrets = reactive<Record<string, boolean>>({})
const expandedModelId = ref('')
const enableLoading = ref(false)
const saveLoading = ref(false)
const importLoading = ref(false)
const queryCache = useQueryCache()
const transcriptionTypeOptions = [
  { value: 'transcription', label: 'Transcription' },
]

const { data: providerDetail } = useQuery({
  key: () => ['transcription-provider-detail', curProviderId.value ?? ''],
  query: async () => {
    if (!curProviderId.value) return null
    const { data } = await getTranscriptionProvidersById({
      path: { id: curProviderId.value },
      throwOnError: true,
    })
    return (data ?? null) as AudioSpeechProviderResponse | null
  },
})

const { data: metaList } = useQuery({
  key: () => ['transcription-providers-meta'],
  query: async () => {
    const { data } = await getTranscriptionProvidersMeta({ throwOnError: true })
    return (data ?? []).map(normalizeProviderMeta)
  },
})

const currentMeta = computed(() => (metaList.value ?? []).find(m => m.provider === curProvider.value?.client_type) ?? null)
const orderedProviderFields = computed(() => [...(currentMeta.value?.config_schema?.fields ?? [])].sort((a, b) => (a.order ?? 0) - (b.order ?? 0)))

function isWideField(field: FieldSchema) {
  if (field.type === 'secret') return true
  const key = field.key.toLowerCase()
  if (key.includes('url') || key.includes('endpoint') || key.includes('key') || key.includes('token') || key.includes('path') || key.includes('uri')) return true
  if ((field.description ?? '').length > 80) return true
  return false
}

const { data: providerModelData } = useQuery({
  key: () => ['transcription-provider-models', curProviderId.value ?? ''],
  query: async () => {
    if (!curProviderId.value) return []
    const { data } = await getTranscriptionProvidersByIdModels({
      path: { id: curProviderId.value },
      throwOnError: true,
    })
    return (data ?? []) as AudioTranscriptionModelResponse[]
  },
})

const providerModels = computed(() => providerModelData.value ?? [])

watch(() => providerDetail.value, (provider) => {
  providerName.value = provider?.name ?? curProvider.value?.name ?? ''
  Object.keys(providerConfig).forEach((key) => delete providerConfig[key])
  Object.assign(providerConfig, { ...(provider?.config ?? {}) })
}, { immediate: true, deep: true })

function getModelSchema(modelID: string): ConfigSchema | null {
  const models = currentMeta.value?.transcription_models ?? currentMeta.value?.models ?? []
  const exact = models.find(m => m.id === modelID)
  const fallback = exact ?? models.find(m => m.id === currentMeta.value?.default_transcription_model) ?? models[0]
  return fallback?.config_schema ?? fallback?.capabilities?.config_schema ?? null
}

function toggleModel(id: string) {
  expandedModelId.value = expandedModelId.value === id ? '' : id
}

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value?.client_type) return
  const prev = curProvider.value.enable ?? false
  curProvider.value = { ...curProvider.value, enable: value }
  enableLoading.value = true
  try {
    await putProvidersById({
      path: { id: curProviderId.value },
      body: {
        name: providerName.value.trim() || curProvider.value.name || '',
        client_type: curProvider.value.client_type,
        enable: value,
        config: sanitizeConfig(providerConfig),
      },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['transcription-providers'] })
    queryCache.invalidateQueries({ key: ['transcription-provider-detail', curProviderId.value ?? ''] })
  } catch {
    curProvider.value = { ...curProvider.value, enable: prev }
    toast.error(t('common.saveFailed'))
  } finally {
    enableLoading.value = false
  }
}

async function handleSaveProvider() {
  if (!curProviderId.value || !curProvider.value?.client_type) return
  saveLoading.value = true
  try {
    await putProvidersById({
      path: { id: curProviderId.value },
      body: {
        name: providerName.value.trim() || curProvider.value.name || '',
        client_type: curProvider.value.client_type,
        enable: curProvider.value.enable,
        config: sanitizeConfig(providerConfig),
      },
      throwOnError: true,
    })
    toast.success(t('transcription.saveSuccess'))
    queryCache.invalidateQueries({ key: ['transcription-providers'] })
    queryCache.invalidateQueries({ key: ['transcription-provider-detail', curProviderId.value ?? ''] })
  } catch {
    toast.error(t('common.saveFailed'))
  } finally {
    saveLoading.value = false
  }
}

async function handleSaveModel(modelId: string, config: Record<string, unknown>) {
  const model = providerModels.value.find(item => item.id === modelId)
  if (!model) return
  try {
    await putTranscriptionModelsById({
      path: { id: modelId },
      body: { name: model.name ?? model.model_id ?? modelId, config },
      throwOnError: true,
    })
    toast.success(t('transcription.saveSuccess'))
    queryCache.invalidateQueries({ key: ['transcription-provider-models', curProviderId.value ?? ''] })
    queryCache.invalidateQueries({ key: ['transcription-models'] })
  } catch {
    toast.error(t('common.saveFailed'))
  }
}

async function handleImportModels() {
  if (!curProviderId.value) return
  importLoading.value = true
  try {
    const { data } = await postTranscriptionProvidersByIdImportModels({
      path: { id: curProviderId.value },
      throwOnError: true,
    })
    const payload = (data ?? {}) as { created?: number, skipped?: number }
    toast.success(t('transcription.importSuccess', {
      created: payload.created ?? 0,
      skipped: payload.skipped ?? 0,
    }))
    queryCache.invalidateQueries({ key: ['transcription-provider-models', curProviderId.value ?? ''] })
    queryCache.invalidateQueries({ key: ['transcription-models'] })
    queryCache.invalidateQueries({ key: ['transcription-providers-meta'] })
  } catch {
    toast.error(t('transcription.importFailed'))
  } finally {
    importLoading.value = false
  }
}

async function handleTestModel(modelId: string, file: File, config: Record<string, unknown>) {
  const { data } = await postTranscriptionModelsByIdTest({
    path: { id: modelId },
    body: {
      file,
      config: JSON.stringify(config),
    },
    throwOnError: true,
  })
  return (data ?? {}) as AudioTestTranscriptionResponse
}

function sanitizeConfig(input: Record<string, unknown>) {
  const result: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(input)) {
    if (value === '' || value == null) continue
    result[key] = value
  }
  return result
}
</script>
