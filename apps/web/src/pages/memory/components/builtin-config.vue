<template>
  <div class="space-y-5">
    <div class="space-y-2">
      <div>
        <h2 class="text-sm font-medium text-foreground">
          {{ $t('memory.modeLabel') }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.modeHint') }}
        </p>
      </div>

      <SegmentedControl
        :model-value="mode"
        :items="modeItems"
        :aria-label="$t('memory.modeLabel')"
        class="w-fit"
        @update:model-value="(value) => (mode = value as MemoryMode)"
      />
    </div>

    <p class="text-xs text-muted-foreground">
      {{ $t(`memory.modeDescriptions.${mode}`) }}
    </p>

    <div
      v-if="mode === 'dense'"
      class="space-y-3 rounded-lg border border-border bg-card p-4"
    >
      <div class="space-y-2">
        <Label>{{ $t('memory.denseEmbeddingModel') }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.denseEmbeddingModelDescription') }}
        </p>
        <ModelSelect
          v-model="embeddingModelId"
          :models="models"
          :providers="providers"
          model-type="embedding"
          :placeholder="$t('memory.denseEmbeddingModel')"
        />
      </div>
      <div class="rounded-md border border-border bg-background px-3 py-2 text-xs text-muted-foreground">
        {{ $t('memory.denseQdrantHint') }}
      </div>
    </div>

    <div
      v-if="collections.length > 0"
      class="grid gap-3 sm:grid-cols-2"
    >
      <div
        v-for="collection in collections"
        :key="collection.name"
        class="space-y-1 rounded-lg border border-border bg-background/70 p-4"
      >
        <div class="flex items-center justify-between gap-3">
          <p class="break-all text-xs font-medium text-foreground">
            {{ collection.name }}
          </p>
          <span
            class="text-xs"
            :class="collection.qdrant?.ok ? 'text-foreground' : 'text-destructive'"
          >
            {{ collection.qdrant?.ok ? $t('memory.collectionHealthy') : $t('memory.collectionUnavailable') }}
          </span>
        </div>
        <p class="text-2xl font-semibold text-foreground">
          {{ collection.points ?? 0 }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.collectionPoints') }}
        </p>
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
import { Label, SegmentedControl, type SegmentedItem, toast } from '@memohai/ui'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  getModels,
  getProviders,
  getMemoryProvidersByIdStatus,
  postMemoryProviders,
  putMemoryProvidersById,
} from '@memohai/sdk'
import type { AdaptersProviderGetResponse, AdaptersProviderStatusResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import LoadingButton from '@/components/loading-button/index.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'

type MemoryMode = 'off' | 'dense'

const props = defineProps<{
  provider?: AdaptersProviderGetResponse | null
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

const mode = ref<MemoryMode>('off')
const embeddingModelId = ref('')
const saveLoading = ref(false)

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
const collections = computed(() => (statusData.value as AdaptersProviderStatusResponse | null)?.collections ?? [])

const modeItems = computed<SegmentedItem<MemoryMode>[]>(() => [
  { value: 'off', label: t('memory.modeNames.off') },
  { value: 'dense', label: t('memory.modeNames.dense') },
])

watch(() => props.provider, (val) => {
  const config = (val?.config ?? {}) as Record<string, unknown>
  const nextMode = config.memory_mode
  mode.value = nextMode === 'dense' ? 'dense' : 'off'
  embeddingModelId.value = typeof config.embedding_model_id === 'string' ? config.embedding_model_id : ''
}, { immediate: true })

async function handleSave() {
  saveLoading.value = true
  try {
    const config: Record<string, unknown> = { memory_mode: mode.value }
    if (mode.value === 'dense' && embeddingModelId.value) {
      config.embedding_model_id = embeddingModelId.value
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
    console.error('Failed to save memory mode:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    saveLoading.value = false
  }
}
</script>
