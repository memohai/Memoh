<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select browser context'"
    :search-placeholder="$t('browserContext.searchPlaceholder')"
    search-aria-label="Search browser contexts"
    :empty-text="$t('browserContext.emptyTitle')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select browser context'"
        class="w-full justify-between font-normal"
      >
        <span class="flex items-center gap-2 truncate">
          <FontAwesomeIcon
            v-if="selected"
            :icon="['fas', 'window-maximize']"
            class="size-3.5 text-muted-foreground"
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
        :icon="['fas', 'window-maximize']"
        class="size-3.5 text-muted-foreground"
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
import type { BrowsercontextsBrowserContext } from '@memoh/sdk'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'

const props = defineProps<{
  contexts: BrowsercontextsBrowserContext[]
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
  const contextOptions = props.contexts.map((ctx) => ({
    value: ctx.id || '',
    label: ctx.name || ctx.id || '',
    keywords: [ctx.name ?? ''],
  }))
  return [noneOption, ...contextOptions]
})
</script>
