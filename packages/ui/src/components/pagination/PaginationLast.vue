<script setup lang="ts">
import type { PaginationLastProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '#/components/button'
import { reactiveOmit } from '@vueuse/core'
import { ChevronsRightIcon } from 'lucide-vue-next'
import { PaginationLast, useForwardProps } from 'reka-ui'
import { Button } from '#/components/button'

const props = withDefaults(defineProps<PaginationLastProps & {
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
}>(), {
  size: 'icon',
})

const delegatedProps = reactiveOmit(props, 'class', 'size')
const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <PaginationLast
    data-slot="pagination-last"
    as-child
    v-bind="forwarded"
  >
    <Button
      variant="ghost"
      :size="size"
      :class="props.class"
      aria-label="Go to last page"
    >
      <slot>
        <ChevronsRightIcon />
      </slot>
    </Button>
  </PaginationLast>
</template>
