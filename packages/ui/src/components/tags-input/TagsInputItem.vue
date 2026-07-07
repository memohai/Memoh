<script setup lang="ts">
import type { TagsInputItemProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'

import { reactiveOmit } from '@vueuse/core'
import { TagsInputItem, useForwardProps } from 'reka-ui'
import { badgeVariants } from '#/components/badge'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputItemProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TagsInputItem
    v-bind="forwardedProps"
    :class="cn(
      // A tag IS a Badge chip living inside the field — reuse the badge surface
      // (flat soft pill, no border) as the single source of truth. Override the box
      // metrics for the compact chip: the text sits flush-left, a 2px gap separates
      // it from the delete button which owns the right inset.
      badgeVariants({ variant: 'default' }),
      'h-5 gap-0.5 py-0 pl-2 pr-1',
      // Active = selected and about-to-delete (Backspace). Instead of a black ring
      // (a strong signal we reserve for decided, important controls), it shifts to
      // the destructive soft tint — same flat-pill language, now reading as remove.
      'data-[state=active]:bg-accent-red-soft-active data-[state=active]:text-accent-red-deep',
      props.class,
    )"
  >
    <slot />
  </TagsInputItem>
</template>
