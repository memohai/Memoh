<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Groups label + description on one side of a horizontal field so the control can
// sit opposite (e.g. a Switch/Checkbox row).
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="field-content"
    :class="cn('flex min-w-0 flex-1 flex-col gap-1', props.class)"
  >
    <slot />
  </div>
</template>
