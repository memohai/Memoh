<script setup lang="ts">
import type { TabsTriggerProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TabsTrigger, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TabsTriggerProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TabsTrigger
    data-slot="tabs-trigger"
    :class="cn(
      // shared shell — the active indicator is the sliding bar / thumb owned by
      // TabsList, so the trigger itself only carries text + layout.
      'inline-flex cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap transition-colors',
      'disabled:pointer-events-none disabled:opacity-40',
      '[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
      // underline skin (keyed off the list root) — transparent border keeps the
      // baseline stable; the foreground color is the only state change. the pill
      // skin lives in TabsList scoped CSS (full SegmentedControl parity).
      '[[data-variant=underline]_&]:-mb-px [[data-variant=underline]_&]:h-9 [[data-variant=underline]_&]:border-b-2 [[data-variant=underline]_&]:border-transparent [[data-variant=underline]_&]:px-1',
      '[[data-variant=underline]_&]:text-body [[data-variant=underline]_&]:font-medium [[data-variant=underline]_&]:text-muted-foreground',
      '[[data-variant=underline]_&]:hover:text-foreground [[data-variant=underline]_&]:data-[state=active]:text-foreground',
      '[[data-variant=underline]_&]:focus-visible:outline-none [[data-variant=underline]_&]:focus-visible:ring-2 [[data-variant=underline]_&]:focus-visible:ring-ring/20 [[data-variant=underline]_&]:focus-visible:rounded-xs',
      props.class,
    )"
    v-bind="forwardedProps"
  >
    <slot />
  </TabsTrigger>
</template>
