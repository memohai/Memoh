<script setup lang="ts">
import type { PaginationFirstProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '#/components/button'
import { reactiveOmit } from '@vueuse/core'
import { ChevronsLeftIcon } from 'lucide-vue-next'
import { PaginationFirst, useForwardProps } from 'reka-ui'
import { Button } from '#/components/button'

// as-child renders the real <Button>, so the page control inherits the full
// [data-button] chrome (hover fill / press-scale / focus ring / disabled fade).
// Applying buttonVariants() classes alone (the old approach) only carried layout
// — the interaction lives on [data-button] in style.css and was never reached.
const props = withDefaults(defineProps<PaginationFirstProps & {
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
}>(), {
  size: 'icon',
})

const delegatedProps = reactiveOmit(props, 'class', 'size')
const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <PaginationFirst
    data-slot="pagination-first"
    as-child
    v-bind="forwarded"
  >
    <Button
      variant="ghost"
      :size="size"
      :class="props.class"
      aria-label="Go to first page"
    >
      <slot>
        <ChevronsLeftIcon />
      </slot>
    </Button>
  </PaginationFirst>
</template>
