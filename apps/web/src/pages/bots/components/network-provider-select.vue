<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select network provider'"
    :search-placeholder="$t('network.searchPlaceholder')"
    search-aria-label="Search network providers"
    :empty-text="$t('network.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="!selected ? '' : undefined"
        type="button"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select network provider'"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="line-clamp-1">{{ displayLabel || placeholder }}</span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </template>

    <template #option-label="{ option }">
      <span
        class="flex-1 truncate text-left"
        :class="{ 'text-muted-foreground': !option.value }"
        :title="option.label"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { ChevronsUpDown } from 'lucide-vue-next'
import { selectTriggerClass } from '@memohai/ui'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

interface OverlayProviderItem {
  kind: string
  display_name: string
  description?: string
}

const props = defineProps<{
  providers: OverlayProviderItem[]
  placeholder?: string
}>()

const selected = defineModel<string>({ default: '' })
const { t } = useI18n()

const options = computed<SearchableSelectOption[]>(() => {
  const noneOption: SearchableSelectOption = {
    value: '',
    label: t('common.none'),
    keywords: [t('common.none')],
  }
  const providerOptions = props.providers.map(provider => ({
    value: provider.kind || '',
    label: provider.display_name || provider.kind || '',
    description: provider.description,
    keywords: [provider.display_name ?? '', provider.kind ?? '', provider.description ?? ''],
  }))
  return [noneOption, ...providerOptions]
})
</script>
