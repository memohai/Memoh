<script setup lang="ts">
import type { SelectTriggerProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { ChevronDown } from 'lucide-vue-next'
import { SelectIcon, SelectTrigger, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = withDefaults(
  defineProps<SelectTriggerProps & { class?: HTMLAttributes['class'], size?: 'sm' | 'default' }>(),
  { size: 'default' },
)

const delegatedProps = reactiveOmit(props, 'class', 'size')
const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <SelectTrigger
    data-slot="select-trigger"
    :data-size="size"
    v-bind="forwardedProps"
    :class="cn(
      'flex w-fit items-center justify-between gap-2 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2 text-sm whitespace-nowrap shadow-sm transition-all outline-none',
      'data-[placeholder]:text-gray-500 dark:data-[placeholder]:text-gray-400',
      '[&_svg:not([class*=\'text-\'])]:text-gray-500 dark:[&_svg:not([class*=\'text-\'])]:text-gray-400',
      'focus:border-primary focus:ring-2 focus:ring-primary/20',
      'hover:bg-gray-50 dark:hover:bg-gray-700',
      'disabled:cursor-not-allowed disabled:opacity-50',
      'data-[size=default]:h-9 data-[size=sm]:h-8',
      '*:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2',
      '[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=\'size-\'])]:size-4',
      props.class,
    )"
  >
    <slot />
    <SelectIcon as-child>
      <ChevronDown class="size-4 opacity-50" />
    </SelectIcon>
  </SelectTrigger>
</template>
