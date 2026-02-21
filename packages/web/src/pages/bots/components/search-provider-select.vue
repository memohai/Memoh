<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select search provider'"
    :search-placeholder="$t('searchProvider.searchPlaceholder')"
    search-aria-label="Search providers"
    :empty-text="$t('searchProvider.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select search provider'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <SearchProviderLogo
            v-if="selectedProvider"
            :provider="selectedProvider.provider || ''"
            size="xs"
          />
          <span class="truncate">{{ displayLabel || placeholder }}</span>
        </span>
        <FontAwesomeIcon
          :icon="['fas', 'magnifying-glass']"
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
        class="truncate"
        :class="{ 'text-muted-foreground': !option.value }"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { Button } from '@memoh/ui'
import { computed } from 'vue'
import type { SearchprovidersGetResponse } from '@memoh/sdk'
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
