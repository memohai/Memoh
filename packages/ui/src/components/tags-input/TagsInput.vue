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
      // inset --field-edge hairline (driven by style.css), invalid turns it
      // destructive. Focus-within only nudges the edge to ENGAGED (not the near-black
      // solid we reserve for single-line fields) so a big multi-row tag box never
      // reads as a heavy black-boxed card.
      // min-h-9 matches a single-line input so an empty field has real presence;
      // py-1.5 + gap-1.5 give the chips + typing line room to breathe and wrap.
      'flex min-h-9 flex-wrap items-center gap-1.5 rounded-md px-2 py-1.5 text-body outline-none',
      props.class)"
  >
    <slot v-bind="slotProps" />
  </TagsInputRoot>
</template>
