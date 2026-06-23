<template>
  <SettingsSection>
    <SettingsRow
      :label="$t('memory.graphTitle')"
      :description="$t('memory.graphDescription')"
      stack="sm"
      align="start"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphNodes')"
      :description="String(graphStatus.source_count ?? 0)"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphEdges')"
      :description="String(graphStatus.edge_count ?? 0)"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.graphFiles')"
      :description="String(graphStatus.markdown_file_count ?? 0)"
    />

    <SettingsRow
      v-if="graphStatus"
      :label="$t('memory.semanticIndexEntries')"
      :description="String(graphStatus.indexed_count ?? 0)"
    />

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

    <SettingsRow
      v-if="graphStatus?.vector_index"
      :label="$t('memory.semanticIndexTitle')"
      :description="graphStatus.vector_index"
      stack="sm"
      align="start"
    >
      <Badge :variant="graphStatus.pgvector?.ok ? 'success' : 'destructive'">
        {{ graphStatus.pgvector?.ok ? $t('memory.semanticIndexHealthy') : (graphStatus.pgvector?.error || $t('memory.semanticIndexUnavailable')) }}
      </Badge>
    </SettingsRow>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Badge, toast } from '@memohai/ui'
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
