<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

// Hairline divider between field groups. Optional centered label via slot.
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="field-separator"
    :class="cn('relative flex items-center', $slots.default ? 'py-1' : '', props.class)"
  >
    <span class="h-px flex-1 bg-border" />
    <span
      v-if="$slots.default"
      class="text-muted-foreground text-caption px-2"
    >
      <slot />
    </span>
    <span
      v-if="$slots.default"
      class="h-px flex-1 bg-border"
    />
  </div>
</template>
