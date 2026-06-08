<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :search-placeholder="$t('common.searchTimezone')"
    :search-aria-label="$t('common.searchTimezone')"
    :empty-text="$t('common.noTimezoneFound')"
    :search-icon="false"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel, placeholder: triggerPlaceholder }">
      <button
        data-slot="select-trigger"
        data-size="default"
        :data-placeholder="displayLabel ? undefined : ''"
        type="button"
        :aria-expanded="open"
        :class="[selectTriggerClass, 'w-full']"
      >
        <span class="line-clamp-1">{{ displayLabel || triggerPlaceholder }}</span>
        <ChevronsUpDown class="opacity-50" />
      </button>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { ChevronsUpDown } from 'lucide-vue-next'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { selectTriggerClass } from '@memohai/ui'
import SearchableSelectPopover, { type SearchableSelectOption } from '@/components/searchable-select-popover/index.vue'
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
    items.push({
      value: tz,
      label: tz,
      description: offsetMap.value.get(tz) ?? '',
    })
  }
  return items
})
</script>
