<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :search-placeholder="$t('common.searchTimezone')"
    :empty-text="$t('common.noTimezoneFound')"
    :show-group-headers="false"
    :popover-class="popoverClass"
    :popover-align="popoverAlign"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { emptyTimezoneValue, timezoneOptions } from '@/utils/timezones'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  placeholder?: string
  allowEmpty?: boolean
  emptyLabel?: string
  popoverClass?: string
  popoverAlign?: 'start' | 'center' | 'end'
}>(), {
  placeholder: '',
  allowEmpty: false,
  emptyLabel: '',
  popoverClass: 'w-[var(--reka-popover-trigger-width)]',
  popoverAlign: 'start',
})

const selected = defineModel<string>({ default: '' })

const options = computed<SearchableSelectOption[]>(() => {
  if (!props.allowEmpty) return timezoneOptions
  return [
    { value: emptyTimezoneValue, label: props.emptyLabel || t('bots.timezoneInherited') },
    ...timezoneOptions,
  ]
})
</script>
