<script setup lang="ts">
import type { PaginationListItemProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import type { ButtonVariants } from '#/components/button'
import { reactiveOmit } from '@vueuse/core'
import { PaginationListItem } from 'reka-ui'
import { Button } from '#/components/button'

// Active page reads as a bordered chip (outline = the inset hairline contract);
// inactive pages are ghost so they hover/press like any other control. Both go
// through the real <Button> via as-child so [data-button] chrome applies.
const props = withDefaults(defineProps<PaginationListItemProps & {
  size?: ButtonVariants['size']
  class?: HTMLAttributes['class']
  isActive?: boolean
}>(), {
  size: 'icon',
})

const delegatedProps = reactiveOmit(props, 'class', 'size', 'isActive')
</script>

<template>
  <PaginationListItem
    data-slot="pagination-item"
    as-child
    v-bind="delegatedProps"
  >
    <Button
      :variant="isActive ? 'outline' : 'ghost'"
      :size="size"
      :aria-current="isActive ? 'page' : undefined"
      :class="props.class"
    >
      <slot>
        {{ value }}
      </slot>
    </Button>
  </PaginationListItem>
</template>
