<template>
  <div class="space-y-2.5">
    <!-- Section header: title + description sit ABOVE the card (not as a row
         inside it), so there is no in-card hairline slicing under the title. -->
    <div class="flex min-h-7 items-center gap-4 px-2">
      <h2 class="text-label font-medium text-muted-foreground">
        {{ $t('memory.graphTitle') }}
      </h2>
    </div>
    <p class="px-2 text-body text-muted-foreground">
      {{ $t('memory.graphDescription') }}
    </p>

    <SettingsSection>
      <!-- Cold-load skeleton: bare tiles (no frame) so they sit on the card
           surface without card-in-card nesting. -->
      <div
        v-if="!graphStatus"
        class="grid grid-cols-1 gap-4 px-4 py-4 sm:grid-cols-2"
      >
        <Skeleton
          v-for="n in 2"
          :key="n"
          class="h-8 w-full rounded-[var(--radius-control)]"
        />
      </div>

      <!-- Configuration overview: unframed tiles live directly on the card
         surface (the card is the container; the tiles don't redraw a border).
         This is a *provider-level* settings page, so it shows the provider's
         configured state, NOT per-bot counts — those live on each bot's memory
         tab. The embedding model itself is chosen in the row below, so it is
         intentionally NOT a tile (no duplicated info). -->
      <div
        v-else
        class="grid grid-cols-1 gap-4 px-4 py-4 sm:grid-cols-2"
      >
        <MetricReadout
          :framed="false"
          :label="$t('memory.modeLabel')"
          :value="modeLabel"
        />
        <MetricReadout
          :framed="false"
          :label="$t('memory.semanticIndexTitle')"
          :status="semanticReadiness"
          :value="semanticReadinessLabel"
        />
      </div>

      <SettingsRow
        :label="$t('memory.semanticEmbeddingModel')"
        :description="$t('memory.semanticIndexDescription')"
        stack="sm"
        align="start"
      >
        <div class="w-full sm:w-64">
          <ModelSelect
            v-model="embeddingModelId"
            :models="models"
            :providers="providers"
            model-type="embedding"
            :placeholder="$t('memory.semanticEmbeddingModelPlaceholder')"
          />
        </div>
      </SettingsRow>
    </SettingsSection>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Skeleton, toast } from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  getMemoryProvidersByIdStatus,
  getModels,
  getProviders,
  postMemoryProviders,
  putMemoryProvidersById,
} from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderStatusResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import MetricReadout from '@/components/settings/metric-readout.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'

const props = defineProps<{
  provider?: AdaptersProviderGetResponse | null
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const saveLoading = ref(false)
const embeddingModelId = ref('')

const { data: modelData } = useQuery({
  key: () => ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: providerData } = useQuery({
  key: () => ['providers'],
  query: async () => {
    const { data } = await getProviders({ throwOnError: true })
    return data
  },
})

const { data: statusData } = useQuery({
  key: () => ['memory-provider-status', props.provider?.id ?? ''],
  query: async () => {
    const id = props.provider?.id
    if (!id) return null
    const { data } = await getMemoryProvidersByIdStatus({ path: { id }, throwOnError: true })
    return data
  },
  enabled: () => !!props.provider?.id,
})

const models = computed(() => modelData.value ?? [])
const providers = computed(() => providerData.value ?? [])
const graphStatus = computed(() => statusData.value as AdaptersProviderStatusResponse | null)
// Mode tile: the configured memory mode, falling back to the built-in default
// ('graph') when the provider config hasn't been saved yet.
const modeLabel = computed(() => {
  const mode = graphStatus.value?.memory_mode || 'graph'
  return t(`memory.modeNames.${mode}`, mode)
})
// Semantic-index tile reflects *readiness* (an embedding model has been chosen),
// not a live pgvector health probe — this is a provider-config page, not a
// per-bot status. 'ok' once a model is set; neutral (no dot) otherwise. The
// model itself is picked in the Embedding Model row below, so it is not a tile.
const hasEmbeddingModel = computed(() => Boolean(graphStatus.value?.embedding_model_id?.trim()))
const semanticReadiness = computed<'ok' | undefined>(() => hasEmbeddingModel.value ? 'ok' : undefined)
const semanticReadinessLabel = computed(() => hasEmbeddingModel.value ? t('memory.semanticIndexHealthy') : t('memory.notConfigured'))
const savedEmbeddingModelId = computed(() => {
  const config = (props.provider?.config ?? {}) as Record<string, unknown>
  return typeof config.embedding_model_id === 'string' ? config.embedding_model_id : ''
})
const hasChanges = computed(() => {
  const config = (props.provider?.config ?? {}) as Record<string, unknown>
  if (!props.provider?.id || config.memory_mode !== 'graph') return true
  return embeddingModelId.value.trim() !== savedEmbeddingModelId.value
})

watch(() => props.provider, (provider) => {
  const config = (provider?.config ?? {}) as Record<string, unknown>
  embeddingModelId.value = typeof config.embedding_model_id === 'string' ? config.embedding_model_id : ''
}, { immediate: true })

async function handleSave() {
  saveLoading.value = true
  try {
    const config: Record<string, unknown> = { memory_mode: 'graph' }
    if (embeddingModelId.value.trim()) {
      config.embedding_model_id = embeddingModelId.value.trim()
    }
    if (props.provider?.id) {
      await putMemoryProvidersById({
        path: { id: props.provider.id },
        body: { name: props.provider.name ?? 'Built-in', config },
        throwOnError: true,
      })
    } else {
      await postMemoryProviders({
        body: { name: 'Built-in', provider: 'builtin', config },
        throwOnError: true,
      })
    }
    toast.success(t('memory.saveSuccess'))
    queryCache.invalidateQueries({ key: ['memory-providers'] })
    if (props.provider?.id) {
      queryCache.invalidateQueries({ key: ['memory-provider-status', props.provider.id] })
    }
  } catch (error) {
    console.error('Failed to save memory provider:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    saveLoading.value = false
  }
}

defineExpose({ hasChanges, saveLoading, save: handleSave })
</script>
