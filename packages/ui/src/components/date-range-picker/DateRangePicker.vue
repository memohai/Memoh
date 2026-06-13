<script setup lang="ts">
import type { DateRange } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { DateFormatter, getLocalTimeZone } from '@internationalized/date'
import { CalendarDays } from 'lucide-vue-next'
import { computed } from 'vue'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/popover'
import RangeCalendar from '#/components/range-calendar/RangeCalendar.vue'
import { selectTriggerClass } from '#/lib/trigger'
import { cn } from '#/lib/utils'

const props = withDefaults(
  defineProps<{
    modelValue?: DateRange
    placeholder?: string
    locale?: string
    size?: 'sm' | 'default' | 'lg'
    class?: HTMLAttributes['class']
  }>(),
  {
    size: 'default',
    locale: 'en',
  },
)
const emits = defineEmits<{
  'update:modelValue': [value: DateRange | undefined]
}>()

const range = computed<DateRange | undefined>({
  get: () => props.modelValue,
  set: value => emits('update:modelValue', value ?? undefined),
})

const formatter = computed(() => new DateFormatter(props.locale, { dateStyle: 'medium' }))

const hasValue = computed(() => Boolean(props.modelValue?.start))

const label = computed(() => {
  const value = props.modelValue
  if (!value?.start) {
    return props.placeholder ?? ''
  }
  const start = formatter.value.format(value.start.toDate(getLocalTimeZone()))
  if (!value.end) {
    return start
  }
  return `${start} – ${formatter.value.format(value.end.toDate(getLocalTimeZone()))}`
})
</script>

<template>
  <Popover>
    <PopoverTrigger
      type="button"
      data-slot="select-trigger"
      :data-size="size"
      :data-placeholder="hasValue ? undefined : ''"
      :class="cn(selectTriggerClass, 'w-full', props.class)"
    >
      <span class="flex min-w-0 items-center gap-2">
        <CalendarDays class="size-4" />
        <span class="truncate">{{ label }}</span>
      </span>
    </PopoverTrigger>
    <PopoverContent
      class="w-auto p-3"
      align="start"
    >
      <RangeCalendar
        v-model="range"
        :locale="locale"
      />
    </PopoverContent>
  </Popover>
</template>
