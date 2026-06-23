<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select search provider'"
    :search-placeholder="$t('webSearch.searchPlaceholder')"
    search-aria-label="Search providers"
    :empty-text="$t('webSearch.empty')"
    :show-group-headers="false"
  >
    <template #option-label="{ option }">
      <span
        class="truncate flex-1 text-left"
        :class="{ 'text-muted-foreground': !option.value }"
        :title="option.label"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SearchprovidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

const props = defineProps<{
  providers: SearchprovidersGetResponse[]
  placeholder?: string
}>()
const { t } = useI18n()

const selected = defineModel<string>({ default: '' })

const options = computed<SearchableSelectOption[]>(() => {
  const noneOption: SearchableSelectOption = {
    value: '',
    label: t('common.none'),
    keywords: [t('common.none')],
  }
  const providerOptions = props.providers.map((provider) => ({
    value: provider.id || '',
    label: provider.name || provider.id || '',
    description: provider.provider,
    keywords: [provider.name ?? '', provider.provider ?? ''],
  }))
  return [noneOption, ...providerOptions]
})
</script>
