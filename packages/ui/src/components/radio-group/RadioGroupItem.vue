<script setup lang="ts">
import type { RadioGroupItemProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import {
  RadioGroupItem,
  useForwardProps,
} from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<RadioGroupItemProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <RadioGroupItem
    data-slot="radio-group-item"
    v-bind="forwardedProps"
    :class="
      cn(
        // No `border` utility on purpose: the edge width/color is owned by
        // style.css so it can animate thin↔thick on select (utilities would
        // win over the component layer and pin width to 1px).
        'relative size-3.5 shrink-0 rounded-full bg-transparent outline-none cursor-pointer',
        // hit-slop: clickable area extends well beyond the visual circle
        `before:absolute before:-inset-2 before:content-['']`,
        'focus-visible:ring-2 focus-visible:ring-ring/20',
        'disabled:cursor-not-allowed disabled:opacity-40',
        props.class,
      )
    "
  >
    <slot />
  </RadioGroupItem>
</template>
