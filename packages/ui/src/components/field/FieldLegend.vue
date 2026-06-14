<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Title for a <FieldSet>. Renders a real <legend>. `variant="label"` for a quiet
// inline heading, `variant="title"` (default) for a section header.
const props = withDefaults(defineProps<{
  class?: HTMLAttributes['class']
  variant?: 'title' | 'label'
}>(), {
  variant: 'title',
})
</script>

<template>
  <legend
    data-slot="field-legend"
    :data-variant="variant"
    :class="cn(
      'p-0 text-foreground',
      variant === 'title' ? 'text-[14px] font-medium' : 'text-[13px] font-medium',
      props.class,
    )"
  >
    <slot />
  </legend>
</template>
