<script setup lang="ts">
import type { TagsInputRootEmits, TagsInputRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { TagsInputRoot, useForwardPropsEmits } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<TagsInputRootProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<TagsInputRootEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <TagsInputRoot
    v-slot="slotProps"
    v-bind="forwarded"
    data-slot="tags-input"
    :class="cn(
      // Same field language as Input/Textarea/InputGroup — transparent fill + one
      // inset --field-edge hairline (driven by style.css), focus-within deepens the
      // edge near-black IN PLACE, invalid turns it destructive. No outer ring, no
      // bg-background, no rounded-lg: was the old border-input + focus ring-2 card.
      'flex flex-wrap items-center gap-2 rounded-md px-2 py-1 text-body outline-none',
      props.class)"
  >
    <slot v-bind="slotProps" />
  </TagsInputRoot>
</template>
