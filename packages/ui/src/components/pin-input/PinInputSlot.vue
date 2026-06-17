<script setup lang="ts">
import type { PinInputInputProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { PinInputInput } from 'reka-ui'
import { cn } from '#/lib/utils'

const props = defineProps<PinInputInputProps & {
  class?: HTMLAttributes['class']
}>()

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <PinInputInput
    v-bind="delegatedProps"
    data-slot="pin-input-slot"
    :class="cn(
      // Each cell is its OWN mini field box: the edge is the shared --field-edge
      // inset hairline from style.css (pixel-identical to <Input> — same mechanism,
      // so no border-miter corner darkening, no doubled dividers). Cells are spaced
      // by the group's gap, so every corner is rounded like a small input and focus
      // deepens the whole edge in place (no ring, no :has neighbour trick).
      'size-8 rounded-md bg-transparent text-center text-body text-foreground',
      'outline-none',
      'disabled:pointer-events-none disabled:opacity-40',
      'placeholder:text-muted-foreground',
      props.class,
    )"
  />
</template>
