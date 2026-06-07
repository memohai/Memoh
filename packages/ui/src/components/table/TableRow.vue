<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '#/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
  // Opt-in: only a row that is genuinely clickable (navigates to a detail,
  // toggles selection) should carry an interaction color. A plain display
  // row stays quiet — same contract as <Item>, where hover/press fills only
  // arm on real controls. data-[state=selected] is a separate opt-in marker.
  interactive?: boolean
}>()
</script>

<template>
  <tr
    data-slot="table-row"
    :class="
      cn(
        'border-b transition-colors data-[state=selected]:bg-[color:var(--ui-selected)]',
        props.interactive
          && 'cursor-pointer hover:bg-[color:var(--ui-hover)] active:bg-[color:var(--ui-pressed)]',
        props.class,
      )
    "
  >
    <slot />
  </tr>
</template>
