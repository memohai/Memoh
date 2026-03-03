<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select memory provider'"
    :search-placeholder="$t('memoryProvider.searchPlaceholder')"
    search-aria-label="Search memory providers"
    :empty-text="$t('memoryProvider.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select memory provider'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <FontAwesomeIcon
            v-if="selected"
            :icon="['fas', 'brain']"
            class="size-3.5 text-primary"
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
      <FontAwesomeIcon
        v-if="option.value"
        :icon="['fas', 'brain']"
        class="size-3.5 text-primary"
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
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

interface MemoryProviderItem {
  id: string
  name: string
  provider: string
}

const props = defineProps<{
  providers: MemoryProviderItem[]
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
