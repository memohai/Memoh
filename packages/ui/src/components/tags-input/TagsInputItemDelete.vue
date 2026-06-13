<script setup lang="ts">
import type { TagsInputItemDeleteProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { X } from 'lucide-vue-next'
import { TagsInputItemDelete, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputItemDeleteProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TagsInputItemDelete
    v-bind="forwardedProps"
    :class="cn(
      // A real interactive affordance, not a bare glyph: a small icon-button that
      // lights up on hover. Two rules it must obey, both from existing system parts:
      //   1. PURE CIRCLE — an in-pill dismiss is round, not a rounded square; the
      //      hover chip must echo the capsule it lives in (rounded-full).
      //   2. NO hand-rolled colour. The chip is built from the gray accent ramp at
      //      its -soft-active stop, so the X hover deepens to the SAME ramp's next
      //      stop (-border). The ghost button's --btn-ghost-hover is tuned for a
      //      white page and would vanish on this already-gray chip, so the ramp step
      //      is the correct reuse here. Icon is muted at rest, foreground on hover.
      'inline-flex size-4 shrink-0 items-center justify-center rounded-full text-muted-foreground',
      'transition-colors hover:bg-[var(--accent-gray-border)] hover:text-foreground',
      'focus-visible:outline-none focus-visible:bg-[var(--accent-gray-border)] focus-visible:text-foreground',
      props.class,
    )"
  >
    <slot>
      <X class="size-3" />
    </slot>
  </TagsInputItemDelete>
</template>
