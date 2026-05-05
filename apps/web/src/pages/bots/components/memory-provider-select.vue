<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select memory provider'"
    :search-placeholder="$t('memory.searchPlaceholder')"
    search-aria-label="Search memory providers"
    :empty-text="$t('memory.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select memory provider'"
        class="w-full justify-between font-normal text-xs shadow-none h-9"
      >
        <span class="flex min-w-0 items-center gap-2 truncate">
          <Brain
            v-if="selected"
            class="size-3.5 shrink-0 text-primary"
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
      <Brain
        v-if="option.value"
        class="size-3.5 shrink-0 text-primary"
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
import { Brain, Search } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

interface MemoryProviderItem {
  id: string
  name: string
  provider: string
  config?: Record<string, string>
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
    description: provider.provider === 'builtin'
      ? t(`memory.modeNames.${provider.config?.memory_mode || 'off'}`)
      : provider.provider,
    keywords: [
      provider.name ?? '',
      provider.provider ?? '',
      provider.config?.memory_mode ?? '',
    ],
  }))
  return [noneOption, ...providerOptions]
})
</script>
