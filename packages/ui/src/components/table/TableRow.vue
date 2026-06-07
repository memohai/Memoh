<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <tr
    data-slot="table-row"
    :class="cn('border-b transition-colors hover:bg-[color:var(--ui-hover)] data-[state=selected]:bg-[color:var(--ui-selected)]', props.class)"
  >
    <slot />
  </tr>
</template>
