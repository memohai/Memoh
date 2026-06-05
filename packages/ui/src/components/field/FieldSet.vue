<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Semantic grouping of related fields. Renders a real <fieldset> so a
// <FieldLegend> labels the whole group for assistive tech.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <fieldset
    data-slot="field-set"
    :class="cn('flex min-w-0 flex-col gap-4 border-0 p-0', props.class)"
  >
    <slot />
  </fieldset>
</template>
