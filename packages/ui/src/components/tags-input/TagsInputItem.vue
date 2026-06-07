<script setup lang="ts">
import type { TagsInputItemProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'

import { reactiveOmit } from '@vueuse/core'
import { TagsInputItem, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputItemProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TagsInputItem
    v-bind="forwardedProps"
    :class="cn(
      'flex h-5 items-center rounded-md bg-secondary text-secondary-foreground',
      // Active (selected, about-to-delete) marks itself with ONE inset near-black
      // hairline — no offset ring — to match the field edge language.
      'data-[state=active]:shadow-[inset_0_0_0_1px_var(--field-edge-solid)]',
      props.class,
    )"
  >
    <slot />
  </TagsInputItem>
</template>
