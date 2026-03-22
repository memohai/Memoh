<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <div class="flex items-center gap-2">
        <FontAwesomeIcon
          :icon="['fas', 'volume-high']"
          class="size-5"
        />
        <div>
          <h2 class="text-base font-semibold">
            {{ curProvider?.name }}
          </h2>
          <p class="text-xs text-muted-foreground">
            {{ currentMeta?.display_name ?? curProvider?.provider }}
          </p>
        </div>
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit="handleSave">
      <div class="space-y-5">
        <section>
          <FormField
            v-slot="{ componentField }"
            name="name"
          >
            <FormItem>
              <Label :for="componentField.id || 'tts-provider-name'">
                {{ $t('common.name') }}
              </Label>
              <FormControl>
                <Input
                  :id="componentField.id || 'tts-provider-name'"
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </section>

        <Separator class="my-4" />

        <!-- Models -->
        <section>
          <div class="flex justify-between items-center mb-4">
            <h3 class="text-sm font-medium">
              {{ $t('ttsProvider.models') }}
            </h3>
            <div
              v-if="curProviderId"
              class="flex items-center gap-2 ml-auto"
            >
              <LoadingButton
                type="button"
                variant="outline"
                class="flex items-center gap-2"
                :loading="importLoading"
                @click="handleImportModels"
              >
                <FontAwesomeIcon :icon="['fas', 'file-import']" />
                {{ $t('ttsProvider.importModels') }}
              </LoadingButton>
              <AddTtsModel
                :provider-id="curProviderId"
                @created="refreshModels"
              />
            </div>
          </div>

          <div
            v-if="providerModels.length === 0"
            class="text-sm text-muted-foreground py-4 text-center"
          >
            {{ $t('ttsProvider.noModels') }}
          </div>

          <div
            v-for="model in providerModels"
            :key="model.id"
            class="border border-border rounded-lg mb-4"
          >
            <button
              type="button"
              class="w-full flex items-center justify-between p-3 text-left hover:bg-accent/50 rounded-t-lg transition-colors"
              @click="toggleModel(model.id)"
            >
              <div>
                <span class="text-sm font-medium">{{ model.name || model.model_id }}</span>
                <span
                  v-if="model.name"
                  class="text-xs text-muted-foreground ml-2"
                >{{ model.model_id }}</span>
              </div>
              <FontAwesomeIcon
                :icon="['fas', expandedModelId === model.id ? 'chevron-up' : 'chevron-down']"
                class="size-3 text-muted-foreground"
              />
            </button>

            <div
              v-if="expandedModelId === model.id"
              class="px-3 pb-3 space-y-4 border-t border-border pt-3"
            >
              <ModelConfigEditor
                :model-id="model.id"
                :model-name="model.model_id"
                :config="model.config || {}"
                :capabilities="getModelCapabilities(model.model_id)"
                @save="(cfg) => handleSaveModelConfig(model.id, cfg)"
                @test="(text, cfg) => handleTestModel(model.id, text, cfg)"
              />
            </div>
          </div>
        </section>
      </div>

      <section class="flex justify-end mt-6 gap-4">
        <ConfirmPopover
          :message="$t('ttsProvider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="handleDelete"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>
        <LoadingButton
          type="submit"
          :loading="editLoading"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </section>
    </form>
  </div>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Separator,
  Label,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import ModelConfigEditor from './model-config-editor.vue'
import AddTtsModel from './add-tts-model.vue'
import { computed, inject, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { putTtsProvidersById, deleteTtsProvidersById, getTtsProvidersMeta } from '@memoh/sdk'
import type { TtsProviderResponse, TtsProviderMetaResponse, TtsModelInfo } from '@memoh/sdk'

const { t } = useI18n()
const curProvider = inject('curTtsProvider', ref<TtsProviderResponse>())
const curProviderId = computed(() => curProvider.value?.id)

const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'
function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

const { data: metaList } = useQuery({
  key: () => ['tts-providers-meta'],
  query: async () => {
    const { data } = await getTtsProvidersMeta({ throwOnError: true })
    return data
  },
})

const currentMeta = computed<TtsProviderMetaResponse | null>(() => {
  if (!metaList.value || !curProvider.value?.provider) return null
  return (metaList.value as TtsProviderMetaResponse[]).find((m) => m.provider === curProvider.value?.provider) ?? null
})

function getModelCapabilities(modelId: string) {
  const meta = currentMeta.value
  if (!meta?.models) return null
  return meta.models.find((m: TtsModelInfo) => m.id === modelId)?.capabilities ?? null
}

// Provider models
const { data: providerModelsData, refresh: refreshModels } = useQuery({
  key: () => ['tts-provider-models', curProviderId.value],
  query: async () => {
    if (!curProviderId.value) return []
    const resp = await fetch(`${apiBase}/tts-providers/${curProviderId.value}/models`, {
      headers: authHeaders(),
    })
    if (!resp.ok) throw new Error('Failed to fetch models')
    return resp.json()
  },
  enabled: () => !!curProviderId.value,
})

const providerModels = computed(() => providerModelsData.value ?? [])

const expandedModelId = ref('')
function toggleModel(id: string) {
  expandedModelId.value = expandedModelId.value === id ? '' : id
}

const queryCache = useQueryCache()

const schema = toTypedSchema(z.object({
  name: z.string().min(1),
}))

const form = useForm({ validationSchema: schema })

let loadedProviderId = ''
watch(() => curProvider.value?.id, (id) => {
  if (!id || id === loadedProviderId) return
  loadedProviderId = id
  expandedModelId.value = ''
  const p = curProvider.value
  if (p) {
    form.setValues({ name: p.name ?? '' })
  }
}, { immediate: true })

const { mutateAsync: submitUpdate, isLoading: editLoading } = useMutation({
  mutation: async (data: { name: string }) => {
    if (!curProviderId.value) return
    const { data: result } = await putTtsProvidersById({
      path: { id: curProviderId.value },
      body: { name: data.name },
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['tts-providers'] }),
})

const { mutateAsync: doDelete, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return
    await deleteTtsProvidersById({ path: { id: curProviderId.value }, throwOnError: true })
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['tts-providers'] })
    queryCache.invalidateQueries({ key: ['tts-models'] })
  },
})

const handleSave = form.handleSubmit(async (values) => {
  try {
    await submitUpdate({ name: values.name })
    toast.success(t('provider.saveChanges'))
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  }
})

async function handleDelete() {
  try {
    await doDelete()
    toast.success(t('common.deleteSuccess'))
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  }
}

// Import models
const importLoading = ref(false)
async function handleImportModels() {
  if (!curProviderId.value) return
  importLoading.value = true
  try {
    const resp = await fetch(`${apiBase}/tts-providers/${curProviderId.value}/import-models`, {
      method: 'POST',
      headers: authHeaders(),
    })
    if (!resp.ok) throw new Error('Import failed')
    toast.success(t('ttsProvider.importSuccess'))
    refreshModels()
    queryCache.invalidateQueries({ key: ['tts-models'] })
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('ttsProvider.importFailed'))
  } finally {
    importLoading.value = false
  }
}

// Save model config
async function handleSaveModelConfig(modelId: string, config: Record<string, unknown>) {
  try {
    const resp = await fetch(`${apiBase}/tts-models/${modelId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({ config }),
    })
    if (!resp.ok) throw new Error('Save failed')
    toast.success(t('provider.saveChanges'))
    refreshModels()
    queryCache.invalidateQueries({ key: ['tts-models'] })
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t('common.saveFailed'))
  }
}

// Test model synthesis
async function handleTestModel(modelId: string, text: string, config: Record<string, unknown>) {
  const resp = await fetch(`${apiBase}/tts-models/${modelId}/test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify({ text, config }),
  })
  if (!resp.ok) {
    const errBody = await resp.text()
    let msg: string
    try {
      msg = JSON.parse(errBody)?.message ?? errBody
    } catch {
      msg = errBody
    }
    throw new Error(msg)
  }
  return resp.blob()
}
</script>
