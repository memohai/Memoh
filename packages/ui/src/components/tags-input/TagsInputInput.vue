<script setup lang="ts">
import type { TagsInputInputProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TagsInputInput, useForwardProps } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputInputProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <TagsInputInput
    v-bind="forwardedProps"
    :class="cn(
      // The actual typing area. flex-1 + min-w-24 so it stays a usably wide target
      // instead of collapsing to a sliver once a couple of tags wrap; min-h-6 makes
      // the caret line tall enough to click into comfortably.
      'min-h-6 min-w-24 flex-1 bg-transparent px-1 text-body focus:outline-none',
      props.class,
    )"
  />
</template>
