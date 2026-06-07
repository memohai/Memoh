<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-slot="table-container"
    class="relative w-full overflow-auto rounded-lg border border-border"
  >
    <table
      data-slot="table"
      :class="cn('w-full caption-top text-label', props.class)"
    >
      <slot />
    </table>
  </div>
</template>
