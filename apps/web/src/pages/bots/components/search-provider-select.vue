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
    <template #trigger="{ open, displayLabel }">
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="!selected ? '' : undefined"
        type="button"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select search provider'"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
          <SearchProviderLogo
            v-if="selectedProvider"
            :provider="selectedProvider.provider || ''"
            size="xs"
          />
          <span class="line-clamp-1">{{ displayLabel || placeholder }}</span>
        </span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </template>

    <template #option-icon="{ option }">
      <SearchProviderLogo
        v-if="option.value"
        :provider="getProviderName(option.value)"
        size="xs"
      />
    </template>

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
import { ChevronsUpDown } from 'lucide-vue-next'
import { selectTriggerClass } from '@memohai/ui'
import { computed } from 'vue'
import type { SearchprovidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

const props = defineProps<{
  providers: SearchprovidersGetResponse[]
  placeholder?: string
}>()
const { t } = useI18n()

const selected = defineModel<string>({ default: '' })

const selectedProvider = computed(() => {
  if (!selected.value) return undefined
  return props.providers.find((p) => p.id === selected.value)
})

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

function getProviderName(id: string) {
  return props.providers.find((provider) => provider.id === id)?.provider || ''
}
</script>
