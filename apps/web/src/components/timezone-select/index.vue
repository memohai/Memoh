<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select timezone'"
    :search-placeholder="$t('common.searchTimezone')"
    search-aria-label="Search timezones"
    :empty-text="$t('common.noTimezoneFound')"
    :show-group-headers="false"
    :width-ratio="0.55"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import type { SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
import { timezones, emptyTimezoneValue, getUtcOffsetLabel } from '@/utils/timezones'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  placeholder?: string
  allowEmpty?: boolean
  emptyLabel?: string
}>(), {
  placeholder: '',
  allowEmpty: false,
  emptyLabel: '',
})

const selected = defineModel<string>({ default: '' })

const offsetMap = computed(() => {
  const map = new Map<string, string>()
  for (const tz of timezones) {
    map.set(tz, getUtcOffsetLabel(tz))
  }
  return map
})

const options = computed<SearchableSelectOption[]>(() => {
  const items: SearchableSelectOption[] = []
  if (props.allowEmpty) {
    items.push({
      value: emptyTimezoneValue,
      label: props.emptyLabel || t('bots.timezoneInherited'),
    })
  }
  for (const tz of timezones) {
    const parts = tz.split('/')
    const offset = offsetMap.value.get(tz) ?? ''
    items.push({
      value: tz,
      label: tz,
      description: offset,
      keywords: [...parts, offset],
    })
  }
  return items
})
</script>
