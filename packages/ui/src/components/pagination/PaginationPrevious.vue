<script setup lang="ts">
import type { PaginationPrevProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '#/components/button'
import { reactiveOmit } from '@vueuse/core'
import { ChevronLeftIcon } from 'lucide-vue-next'
import { PaginationPrev, useForwardProps } from 'reka-ui'
import { Button } from '#/components/button'

const props = withDefaults(defineProps<PaginationPrevProps & {
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
}>(), {
  size: 'icon',
})

const delegatedProps = reactiveOmit(props, 'class', 'size')
const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <PaginationPrev
    data-slot="pagination-previous"
    as-child
    v-bind="forwarded"
  >
    <Button
      variant="ghost"
      :size="size"
      :class="props.class"
      aria-label="Go to previous page"
    >
      <slot>
        <ChevronLeftIcon />
      </slot>
    </Button>
  </PaginationPrev>
</template>
