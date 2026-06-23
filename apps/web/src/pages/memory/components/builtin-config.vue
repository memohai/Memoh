<template>
  <div class="space-y-5">
    <div class="space-y-1">
      <h2 class="text-sm font-medium text-foreground">
        {{ $t('memory.graphTitle') }}
      </h2>
      <p class="text-xs text-muted-foreground">
        {{ $t('memory.graphDescription') }}
      </p>
    </div>

    <div
      v-if="graphStatus"
      class="grid grid-cols-2 gap-3 sm:grid-cols-4"
    >
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.source_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphNodes') }}
        </p>
      </div>
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.edge_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphEdges') }}
        </p>
      </div>
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.markdown_file_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphFiles') }}
        </p>
      </div>
      <div class="space-y-0.5 rounded-md border border-border bg-background px-3 py-2">
        <p class="text-xl font-semibold text-foreground">
          {{ graphStatus.indexed_count ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.semanticIndexEntries') }}
        </p>
      </div>
    </div>

    <div class="space-y-4 rounded-md border border-border bg-card p-4">
      <div class="space-y-1">
        <h3 class="text-sm font-medium text-foreground">
          {{ $t('memory.semanticIndexTitle') }}
        </h3>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.semanticIndexDescription') }}
        </p>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ $t('memory.semanticEmbeddingModel') }}</Label>
          <ModelSelect
            v-model="embeddingModelId"
            :models="models"
            :providers="providers"
            model-type="embedding"
            :placeholder="$t('memory.semanticEmbeddingModelPlaceholder')"
          />
        </div>
      </div>

      <div
        v-if="graphStatus?.vector_index"
        class="flex items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-2 text-xs"
      >
        <span class="break-all text-muted-foreground">
          {{ graphStatus.vector_index }}
        </span>
        <span :class="graphStatus.pgvector?.ok ? 'text-foreground' : 'text-destructive'">
          {{ graphStatus.pgvector?.ok ? $t('memory.semanticIndexHealthy') : (graphStatus.pgvector?.error || $t('memory.semanticIndexUnavailable')) }}
        </span>
      </div>
    </div>

    <div class="flex justify-end">
      <LoadingButton
        :loading="saveLoading"
        @click="handleSave"
      >
        {{ $t('common.save') }}
      </LoadingButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Label, toast } from '@memohai/ui'
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
import LoadingButton from '@/components/loading-button/index.vue'
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
  }
  catch (error) {
    console.error('Failed to save memory provider:', error)
    toast.error(t('common.saveFailed'))
  }
  finally {
    saveLoading.value = false
  }
}
</script>
