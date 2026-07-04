<script setup lang="ts">
import type { PrimitiveProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { Primitive } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<PrimitiveProps & {
  class?: HTMLAttributes['class']
  /** Density of the label row. `compact` is the settings-sidebar tier: a shorter
   *  h-6 row, the pl-3.5 (14px) hanging indent that aligns the label with the nav
   *  items below it, and a lighter muted/475 treatment. Owned here so the two
   *  settings call sites stop each hand-writing the same `h-6! pl-[14px]! …`
   *  !important overrides (which only existed to out-specificity the base string). */
  size?: 'default' | 'compact'
}>()
</script>

<template>
  <Primitive
    data-slot="sidebar-group-label"
    data-sidebar="group-label"
    :as="as"
    :as-child="asChild"
    :class="cn(
      'text-sidebar-foreground/70 ring-sidebar-ring flex h-8 shrink-0 items-center rounded-md px-2 text-body font-medium outline-hidden transition-[margin,opacity] duration-200 ease-linear focus-visible:ring-2 [&>svg]:size-4 [&>svg]:shrink-0',
      'group-data-[collapsible=icon]:-mt-8 group-data-[collapsible=icon]:opacity-0',
      size === 'compact' && 'h-6 pl-3.5 pr-3 font-[475] text-muted-foreground',
      props.class)"
  >
    <slot />
  </Primitive>
</template>
