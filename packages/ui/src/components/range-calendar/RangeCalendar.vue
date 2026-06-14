<script setup lang="ts">
import type { RangeCalendarRootEmits, RangeCalendarRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import {
  RangeCalendarCell,
  RangeCalendarCellTrigger,
  RangeCalendarGrid,
  RangeCalendarGridBody,
  RangeCalendarGridHead,
  RangeCalendarGridRow,
  RangeCalendarHeadCell,
  RangeCalendarHeader,
  RangeCalendarHeading,
  RangeCalendarNext,
  RangeCalendarPrev,
  RangeCalendarRoot,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '#/lib/utils'

const props = withDefaults(
  defineProps<RangeCalendarRootProps & { class?: HTMLAttributes['class'] }>(),
  {
    weekdayFormat: 'short',
  },
)
const emits = defineEmits<RangeCalendarRootEmits>()

const delegatedProps = reactiveOmit(props, 'class')
const forwarded = useForwardPropsEmits(delegatedProps, emits)

const navButtonClass = cn(
  'inline-flex size-7 items-center justify-center rounded-md text-muted-foreground outline-none transition-colors',
  'hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/30',
  'disabled:pointer-events-none disabled:opacity-40',
)

// Per-cell wrapper paints the continuous range band (accent) and rounds only the
// two ends, so a selected range reads as one connected pill rather than separate days.
const cellClass = cn(
  'relative p-0 text-center text-sm',
  '[&:has([data-selected])]:bg-accent',
  '[&:has([data-selection-start])]:rounded-l-md [&:has([data-selection-end])]:rounded-r-md',
  'first:[&:has([data-selected])]:rounded-l-md last:[&:has([data-selected])]:rounded-r-md',
)

// The day button itself: transparent at rest (band shows through), charcoal fill on
// the two endpoints, accent on the hover-preview span, muted for out-of-month/disabled.
const cellTriggerClass = cn(
  'relative inline-flex size-9 select-none items-center justify-center rounded-md text-sm font-normal text-foreground outline-none transition-colors',
  'hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/30',
  'data-[highlighted]:bg-accent data-[highlighted]:text-foreground',
  'data-[selection-start]:bg-foreground data-[selection-start]:text-background data-[selection-start]:hover:bg-foreground',
  'data-[selection-end]:bg-foreground data-[selection-end]:text-background data-[selection-end]:hover:bg-foreground',
  '[&[data-today]:not([data-selection-start]):not([data-selection-end])]:font-semibold',
  'data-[outside-view]:text-muted-foreground/40',
  'data-[disabled]:pointer-events-none data-[disabled]:text-muted-foreground/40',
  'data-[unavailable]:pointer-events-none data-[unavailable]:text-muted-foreground/40 data-[unavailable]:line-through',
)
</script>

<template>
  <RangeCalendarRoot
    v-slot="{ grid, weekDays }"
    data-slot="range-calendar"
    :class="cn('w-fit', props.class)"
    v-bind="forwarded"
  >
    <RangeCalendarHeader class="relative mb-2 flex items-center justify-center">
      <RangeCalendarPrev :class="cn(navButtonClass, 'absolute left-0')">
        <ChevronLeft class="size-4" />
      </RangeCalendarPrev>
      <RangeCalendarHeading class="text-sm font-medium" />
      <RangeCalendarNext :class="cn(navButtonClass, 'absolute right-0')">
        <ChevronRight class="size-4" />
      </RangeCalendarNext>
    </RangeCalendarHeader>

    <div class="flex gap-4">
      <RangeCalendarGrid
        v-for="month in grid"
        :key="month.value.toString()"
        class="border-collapse"
      >
        <RangeCalendarGridHead>
          <RangeCalendarGridRow class="flex">
            <RangeCalendarHeadCell
              v-for="day in weekDays"
              :key="day"
              class="w-9 pb-1 text-[0.72rem] font-normal text-muted-foreground"
            >
              {{ day }}
            </RangeCalendarHeadCell>
          </RangeCalendarGridRow>
        </RangeCalendarGridHead>
        <RangeCalendarGridBody>
          <RangeCalendarGridRow
            v-for="(weekDates, idx) in month.rows"
            :key="`week-${idx}`"
            class="mt-1 flex w-full"
          >
            <RangeCalendarCell
              v-for="weekDate in weekDates"
              :key="weekDate.toString()"
              :date="weekDate"
              :class="cellClass"
            >
              <RangeCalendarCellTrigger
                :day="weekDate"
                :month="month.value"
                :class="cellTriggerClass"
              />
            </RangeCalendarCell>
          </RangeCalendarGridRow>
        </RangeCalendarGridBody>
      </RangeCalendarGrid>
    </div>
  </RangeCalendarRoot>
</template>
