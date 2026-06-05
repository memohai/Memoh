<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Field helper text — soft, secondary guidance under a control.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <p
    data-slot="field-description"
    :class="cn('text-muted-foreground text-body leading-snug', props.class)"
  >
    <slot />
  </p>
</template>
