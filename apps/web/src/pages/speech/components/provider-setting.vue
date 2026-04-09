<template>
  <div class="p-4">
    <section class="flex items-center gap-3">
      <Volume2
        class="size-5"
      />
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

    <!-- Models -->
    <section>
      <div class="flex justify-between items-center mb-4">
        <h3 class="text-xs font-medium">
          {{ $t('speech.models') }}
        </h3>
      </div>

      <div
        v-if="providerModels.length === 0"
        class="text-xs text-muted-foreground py-4 text-center"
      >
        {{ $t('speech.noModels') }}
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
            >{{ model.model_id }}</span>
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
            :capabilities="getModelCapabilities(model.model_id ?? '')"
            @test="(text, cfg) => handleTestModel(model.id ?? '', text, cfg)"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import {
  Separator,
  Switch,
} from '@memohai/ui'
import ModelConfigEditor from './model-config-editor.vue'
import { Volume2, ChevronUp, ChevronDown } from 'lucide-vue-next'
import { computed, inject, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getSpeechProvidersMeta, getSpeechModels, putProvidersById } from '@memohai/sdk'
import type { TtsSpeechProviderResponse, TtsProviderMetaResponse, TtsModelInfo } from '@memohai/sdk'

const { t } = useI18n()
const curProvider = inject('curTtsProvider', ref<TtsSpeechProviderResponse>())
const curProviderId = computed(() => curProvider.value?.id)
const enableLoading = ref(false)

const { data: metaList } = useQuery({
  key: () => ['speech-providers-meta'],
  query: async () => {
    const { data } = await getSpeechProvidersMeta({ throwOnError: true })
    return data
  },
})

const currentMeta = computed<TtsProviderMetaResponse | null>(() => {
  if (!metaList.value || !curProvider.value?.client_type) return null
  return (metaList.value as TtsProviderMetaResponse[]).find((m) => m.provider === curProvider.value?.client_type) ?? null
})

function getModelCapabilities(modelId: string) {
  const meta = currentMeta.value
  if (!meta?.models) return null
  return meta.models.find((m: TtsModelInfo) => m.id === modelId)?.capabilities ?? null
}

const { data: allSpeechModels } = useQuery({
  key: () => ['speech-models'],
  query: async () => {
    const { data } = await getSpeechModels({ throwOnError: true })
    return data
  },
})

const providerModels = computed(() => {
  if (!allSpeechModels.value || !curProviderId.value) return []
  return allSpeechModels.value.filter((m) => m.provider_id === curProviderId.value)
})

const expandedModelId = ref('')
function toggleModel(id: string) {
  expandedModelId.value = expandedModelId.value === id ? '' : id
}

const queryCache = useQueryCache()

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value) return

  const prev = curProvider.value.enable ?? false
  curProvider.value = { ...curProvider.value, enable: value }

  enableLoading.value = true
  try {
    await putProvidersById({
      path: { id: curProviderId.value },
      body: { enable: value },
      throwOnError: true,
    })
    queryCache.invalidateQueries({ key: ['speech-providers'] })
  } catch {
    curProvider.value = { ...curProvider.value, enable: prev }
    toast.error(t('common.saveFailed'))
  } finally {
    enableLoading.value = false
  }
}

async function handleTestModel(modelId: string, text: string, config: Record<string, unknown>) {
  const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'
  const token = localStorage.getItem('token')
  const resp = await fetch(`${apiBase}/speech-models/${modelId}/test`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
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
