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
import { emptyTimezoneValue, timezoneOptions } from '@/utils/timezones'

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

// timezoneOptions is precomputed at module scope; only prepend the optional
// "inherit" row, so mounting/opening this component does no per-zone work.
const options = computed<ComboboxOption[]>(() => {
  if (!props.allowEmpty) return timezoneOptions
  return [
    { value: emptyTimezoneValue, label: props.emptyLabel || t('bots.timezoneInherited') },
    ...timezoneOptions,
  ]
})
</script>
