<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <kbd
    :class="cn(
      'bg-muted text-muted-foreground pointer-events-none inline-flex h-5 w-fit min-w-5 items-center justify-center gap-1 rounded-sm px-1 font-sans text-body font-medium select-none',
      '[&_svg:not([class*=\'size-\'])]:size-3',
      // Inside a tooltip: a smaller, less-rounded chip (h-4 / rounded-xs) so it stays
      // proportional to the compact bubble, with a translucent light fill on the ink.
      '[[data-slot=tooltip-content]_&]:h-4 [[data-slot=tooltip-content]_&]:min-w-4 [[data-slot=tooltip-content]_&]:rounded-xs [[data-slot=tooltip-content]_&]:bg-background/20 [[data-slot=tooltip-content]_&]:text-background dark:[[data-slot=tooltip-content]_&]:bg-background/10',
      props.class,
    )"
  >
    <slot />
  </kbd>
</template>
