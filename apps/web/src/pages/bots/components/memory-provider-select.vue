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
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="!selected ? '' : undefined"
        type="button"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select memory provider'"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
          <Brain
            v-if="selected"
            class="size-3.5 shrink-0 text-primary"
          />
          <span class="line-clamp-1">{{ displayLabel || placeholder }}</span>
        </span>
        <ChevronsUpDown class="opacity-50" />
      </button>
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
import { Brain, ChevronsUpDown } from 'lucide-vue-next'
import { selectTriggerClass } from '@memohai/ui'
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
