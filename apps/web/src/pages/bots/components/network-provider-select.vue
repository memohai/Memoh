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
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select network provider'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <Network
            v-if="selected"
            class="size-3.5 text-primary"
          />
          <span class="truncate">{{ displayLabel || placeholder }}</span>
        </span>
        <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </template>

    <template #option-icon="{ option }">
      <Network
        v-if="option.value"
        class="size-3.5 text-primary"
      />
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { Network, Search } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
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
