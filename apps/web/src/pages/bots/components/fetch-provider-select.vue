<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select fetch provider'"
    :search-placeholder="$t('webSearch.searchPlaceholder')"
    search-aria-label="Search fetch providers"
    :empty-text="$t('webSearch.emptyFetch')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select fetch provider'"
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
            :title="displayLabel || selectedProvider?.name || placeholder"
          >{{ displayLabel || selectedProvider?.name || placeholder }}</span>
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
import type { FetchprovidersGetResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import SearchProviderLogo from '@/components/search-provider-logo/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

const props = defineProps<{
  providers: FetchprovidersGetResponse[]
  placeholder?: string
}>()
const { t } = useI18n()

const selected = defineModel<string>({ default: '' })

const nativeProvider = computed(() => props.providers.find((p) => p.provider === 'native'))

const selectedProvider = computed(() => {
  if (!selected.value) return nativeProvider.value
  return props.providers.find((p) => p.id === selected.value) ?? nativeProvider.value
})

const options = computed<SearchableSelectOption[]>(() => {
  return props.providers.map((provider) => ({
    value: provider.id || '',
    label: provider.name || provider.id || '',
    description: provider.enable === false ? t('common.disabled') : provider.provider,
    keywords: [provider.name ?? '', provider.provider ?? '', provider.enable === false ? t('common.disabled') : '', t(`webSearch.providerNames.${provider.provider}`, '')],
  }))
})

function getProviderName(id: string) {
  return props.providers.find((provider) => provider.id === id)?.provider || ''
}
</script>
