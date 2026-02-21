<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select model'"
    :search-placeholder="$t('bots.settings.searchModel')"
    search-aria-label="Search models"
    :empty-text="$t('bots.settings.noModel')"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ModelsGetResponse, ProvidersGetResponse } from '@memoh/sdk'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

const props = defineProps<{
  models: ModelsGetResponse[]
  providers: ProvidersGetResponse[]
  modelType: 'chat' | 'embedding'
  placeholder?: string
}>()

const selected = defineModel<string>({ default: '' })

const typeFilteredModels = computed(() =>
  props.models.filter((m) => m.type === props.modelType),
)

const providerMap = computed(() => {
  const map = new Map<string, string>()
  for (const p of props.providers) {
    map.set(p.id, p.name ?? p.id)
  }
  return map
})

const options = computed<SearchableSelectOption[]>(() =>
  typeFilteredModels.value.map((model) => {
    const providerId = model.llm_provider_id
    return {
      value: model.model_id,
      label: model.name || model.model_id,
      description: model.name ? model.model_id : undefined,
      group: providerId,
      groupLabel: providerMap.value.get(providerId) ?? providerId,
      keywords: [model.model_id, model.name ?? ''],
    }
  }),
)
</script>
