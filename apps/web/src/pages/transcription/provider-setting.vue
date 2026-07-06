<template>
  <SettingsShell width="narrow">
    <div class="space-y-6">
      <!-- Identity card: same header shape as the provider detail. -->
      <SettingsSection>
        <SettingsRow>
          <template #leading>
            <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted">
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
          </template>
          <template #content>
            <h2 class="truncate text-sm font-semibold">
              {{ curProvider?.name }}
            </h2>
          </template>
          <div class="flex shrink-0 items-center gap-2">
            <span class="text-xs text-muted-foreground">
              {{ $t('common.enable') }}
            </span>
            <Switch
              :model-value="curProvider?.enable ?? false"
              :disabled="!curProvider?.id || enableLoading"
              :aria-label="$t('common.enable')"
              @update:model-value="handleToggleEnable"
            />
          </div>
        </SettingsRow>
      </SettingsSection>

      <!-- Provider configuration card -->
      <form @submit.prevent="handleSaveProvider">
        <SettingsSection :title="$t('provider.configurationTitle')">
          <div>
            <SettingsRow :label="$t('common.name')">
              <Input
                id="transcription-provider-name"
                v-model="providerName"
                type="text"
                class="w-80"
                :placeholder="$t('common.namePlaceholder')"
              />
            </SettingsRow>

            <SettingsRow
              v-for="field in orderedProviderFields"
              :key="field.key"
              :label="field.title || field.key"
              :description="field.description"
            >
              <div
                v-if="field.type === 'secret'"
                class="relative w-80"
              >
                <Input
                  :id="`transcription-provider-${field.key}`"
                  v-model="providerConfig[field.key] as string"
                  :type="visibleSecrets[field.key] ? 'text' : 'password'"
                  class="pr-9"
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
                class="w-80"
              />
              <Select
                v-else-if="field.type === 'enum' && field.enum"
                :model-value="String(providerConfig[field.key] ?? '')"
                @update:model-value="(val) => providerConfig[field.key] = val"
              >
                <SelectTrigger class="w-80">
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
                class="w-80"
              />
            </SettingsRow>
          </div>

          <template #footer>
            <LoadingButton
              type="submit"
              size="sm"
              :loading="saveLoading"
            >
              {{ $t('provider.saveChanges') }}
            </LoadingButton>
          </template>
        </SettingsSection>
      </form>

      <!-- Models: a list; editing a model opens a dialog (the same shape as the
           provider's model management), not an inline accordion. -->
      <SettingsSection :title="$t('transcription.models')">
        <template
          v-if="curProviderId"
          #actions
        >
          <div class="flex items-center gap-2">
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
        </template>

        <div
          v-if="providerModels.length === 0"
          class="px-4 py-10 text-center text-xs text-muted-foreground"
        >
          {{ $t('transcription.noModels') }}
        </div>

        <template v-else>
          <ModelListRow
            v-for="(model, index) in providerModels"
            :key="model.id"
            :label="model.name || model.model_id || ''"
            :meta="model.name && model.name !== model.model_id ? model.model_id : ''"
            :last="index === providerModels.length - 1"
            @click="openModelEditor(model)"
          />
        </template>
      </SettingsSection>

      <!-- Editing opens the config editor in a dialog rather than expanding the
           row in place. -->
      <Dialog v-model:open="modelDialogOpen">
        <DialogContent class="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {{ editingModel?.name || editingModel?.model_id }}
            </DialogTitle>
          </DialogHeader>
          <ModelConfigEditor
            v-if="editingModel"
            :key="editingModel.id"
            :model-id="editingModel.id ?? ''"
            :model-name="editingModel.model_id ?? ''"
            :config="editingModel.config || {}"
            :schema="getModelSchema(editingModel.model_id ?? '')"
            mode="transcription"
            :on-test="(file, cfg) => handleTestModel(editingModel?.id ?? '', file as File, cfg)"
            @save="(cfg) => handleSaveModel(editingModel?.id ?? '', cfg)"
          />
        </DialogContent>
      </Dialog>
    </div>
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
import { Eye, EyeOff } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogHeader, DialogTitle, Input, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Switch } from '@memohai/ui'
import ProviderIcon from '@/components/provider-icon/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import ModelConfigEditor from '@/pages/speech/components/model-config-editor.vue'
import CreateModel from '@/components/create-model/index.vue'
import SettingsShell from '@/components/settings-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import ModelListRow from '@/components/settings/model-list-row.vue'

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
const modelDialogOpen = ref(false)
const editingModel = ref<AudioTranscriptionModelResponse | null>(null)
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

function openModelEditor(model: AudioTranscriptionModelResponse) {
  editingModel.value = model
  modelDialogOpen.value = true
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
