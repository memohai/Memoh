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

// A quick-pick preset: a label plus the inclusive range it sets.
export interface DateRangePreset {
  label: string
  range: DateRange
}

const props = withDefaults(
  defineProps<{
    modelValue?: DateRange
    placeholder?: string
    locale?: string
    size?: 'sm' | 'default' | 'lg'
    // Optional quick-pick presets shown as a rail beside the calendar.
    presets?: DateRangePreset[]
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

function sameRange(a?: DateRange, b?: DateRange) {
  return a?.start?.toString() === b?.start?.toString()
    && a?.end?.toString() === b?.end?.toString()
}

const activePreset = computed(() =>
  props.presets?.find(preset => sameRange(preset.range, props.modelValue)),
)

const label = computed(() => {
  // A matched preset reads as its name ("Last 7 Days"); otherwise the dates.
  if (activePreset.value) return activePreset.value.label
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

function applyPreset(preset: DateRangePreset) {
  range.value = preset.range
}
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
      class="w-auto p-0"
      align="start"
    >
      <div class="flex">
        <!-- Quick presets sit in a rail to the LEFT of the calendar, so they
             never have to line up with the day grid; each row uses the shared
             menu interaction (overlay hover + selected fill), so the picker
             reads like every other menu surface. -->
        <div
          v-if="presets?.length"
          class="flex w-36 flex-col gap-0.5 border-r border-border p-2"
        >
          <button
            v-for="preset in presets"
            :key="preset.label"
            type="button"
            class="rounded-menu px-2.5 py-1.5 text-left text-control transition-colors hover:bg-[color:var(--ui-hover)] data-[active=true]:bg-[color:var(--ui-selected)] data-[active=true]:font-medium"
            :data-active="activePreset?.label === preset.label ? 'true' : undefined"
            @click="applyPreset(preset)"
          >
            {{ preset.label }}
          </button>
        </div>
        <div class="p-3">
          <RangeCalendar
            v-model="range"
            :locale="locale"
          />
        </div>
      </div>
    </PopoverContent>
  </Popover>
</template>
