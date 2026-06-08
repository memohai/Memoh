<template>
  <Combobox
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :search-placeholder="$t('common.searchTimezone')"
    :empty-text="$t('common.noTimezoneFound')"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Combobox, { type ComboboxOption } from '@/components/combobox/index.vue'
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

const options = computed<ComboboxOption[]>(() => {
  const items: ComboboxOption[] = []
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
