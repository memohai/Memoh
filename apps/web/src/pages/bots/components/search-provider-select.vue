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
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select search provider'"
        class="w-full justify-between font-normal text-xs shadow-none h-9"
      >
        <span class="flex min-w-0 items-center gap-2 truncate">
          <SearchProviderLogo
            v-if="selectedProvider"
            :provider="selectedProvider.provider || ''"
            size="xs"
          />
          <span
            class="truncate"
            :title="displayLabel || placeholder"
          >{{ displayLabel || placeholder }}</span>
        </span>
        <Search
          class="ml-2 size-3.5 shrink-0 text-muted-foreground"
        />
      </Button>
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
import { Search } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
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
