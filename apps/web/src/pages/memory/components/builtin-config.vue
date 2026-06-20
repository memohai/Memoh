<template>
  <!-- Titleless card: the mode row's own label carries the name, so a section
       title would just echo it one rung up. The whole built-in memory config
       lives in one continuous white card — mode, the model it needs, and the
       live index status — instead of floating bare on the page background. -->
  <SettingsSection>
    <!-- Memory mode is the hero. The row description flips with the selected
         mode, so switching gives immediate in-place feedback about what that
         mode does — no separate floating explainer line under the control. -->
    <SettingsRow
      :label="$t('memory.modeLabel')"
      :description="$t(`memory.modeDescriptions.${mode}`)"
      stack="sm"
      align="start"
    >
      <SegmentedControl
        :model-value="mode"
        :items="modeItems"
        :aria-label="$t('memory.modeLabel')"
        @update:model-value="(value) => (mode = value as MemoryMode)"
      />
    </SettingsRow>

    <!-- Semantic mode vectorizes memories, so it needs an embedding model.
         Shown only for that mode; the storage backend it writes to is
         implementation trivia and stays out of the copy. -->
    <SettingsRow
      v-if="mode === 'dense'"
      :label="$t('memory.denseEmbeddingModel')"
      :description="$t('memory.denseEmbeddingModelDescription')"
      stack="sm"
    >
      <div class="w-full sm:w-64">
        <ModelSelect
          v-model="embeddingModelId"
          :models="models"
          :providers="providers"
          model-type="embedding"
          :placeholder="$t('memory.denseEmbeddingModel')"
        />
      </div>
    </SettingsRow>

    <!-- Live index status: a distilled read of what's actually provisioned —
         entries stored + one health badge — keyed to the SAVED mode, not the
         draft, so it reflects reality. When memory is off there is no index to
         report, so the row disappears entirely rather than showing empty tiles. -->
    <SettingsRow
      v-for="collection in activeCollections"
      :key="collection.name"
      :label="collectionLabel(collection)"
      :description="$t('memory.entriesStored', { count: collection.points ?? 0 })"
    >
      <Badge :variant="collection.qdrant?.ok ? 'success' : 'destructive'">
        {{ collection.qdrant?.ok ? $t('memory.collectionHealthy') : $t('memory.collectionUnavailable') }}
      </Badge>
    </SettingsRow>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Badge, SegmentedControl, type SegmentedItem, toast } from '@memohai/ui'
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
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import ModelSelect from '@/pages/bots/components/model-select.vue'

type MemoryMode = 'off' | 'dense'
type Collection = NonNullable<AdaptersProviderStatusResponse['collections']>[number]

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

// The persisted state, used both to gate Save (draft vs saved) and to decide
// which index status is worth showing — the status query describes what's
// actually provisioned, which tracks the saved mode, not the in-flight draft.
const savedMode = computed<MemoryMode>(() => {
  const config = (props.provider?.config ?? {}) as Record<string, unknown>
  const m = config.memory_mode
  return m === 'dense' ? m : 'off'
})
const savedEmbeddingModelId = computed(() => {
  const config = (props.provider?.config ?? {}) as Record<string, unknown>
  return typeof config.embedding_model_id === 'string' ? config.embedding_model_id : ''
})

const hasChanges = computed(() => {
  if (mode.value !== savedMode.value) return true
  // Embedding choice only matters (and only counts as a change) in dense mode.
  if (mode.value === 'dense' && embeddingModelId.value !== savedEmbeddingModelId.value) return true
  return false
})

// The status endpoint can return every provisioned collection at once (both the
// keyword and semantic indexes), so narrow it to the one the saved mode uses;
// showing the inactive index's "Unavailable" is pure noise. Fall back to
// whatever came back if the name heuristic misses, so status never silently
// vanishes on an unexpected collection name.
const activeCollections = computed<Collection[]>(() => {
  if (savedMode.value === 'off') return []
  const matched = collections.value.filter((c) => (c.name ?? '').toLowerCase().includes('dense'))
  return matched.length > 0 ? matched : collections.value
})

function collectionLabel(collection: Collection): string {
  const name = (collection.name ?? '').toLowerCase()
  if (name.includes('dense')) return t('memory.semanticIndex')
  return collection.name ?? ''
}

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

// The Save control lives in the page header (PageShell #actions), the house
// pattern for a root page's manual save (mirrors bot-tool-approval) — not a
// footer band inside this card, which would leave an empty strip below a single
// row. The parent drives that button from this exposed state.
defineExpose({ hasChanges, saveLoading, save: handleSave })
</script>
