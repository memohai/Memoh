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
      'relative -mb-px inline-flex h-9 items-center justify-center gap-1.5 border-b-2 border-transparent px-1 text-body font-medium whitespace-nowrap transition-colors',
      'text-muted-foreground hover:text-foreground cursor-pointer',
      'data-[state=active]:border-foreground data-[state=active]:text-foreground',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20 focus-visible:rounded-xs',
      'disabled:pointer-events-none disabled:opacity-40',
      '[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=\'size-\'])]:size-4',
      props.class,
    )"
    v-bind="forwardedProps"
  >
    <slot />
  </TabsTrigger>
</template>
