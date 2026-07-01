<template>
  <button
    type="button"
    class="group/tile relative flex w-52 flex-col items-center rounded-[var(--radius-menu-shell)] border border-border p-5 text-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
    :class="surfaceClass"
  >
    <!-- Corner status: entry-only, and present only when the tile has something
         to flag. A healthy tile shows nothing here. -->
    <div
      v-if="variant === 'entity' && $slots.status"
      class="absolute right-3 top-3 flex h-5 items-center"
    >
      <slot name="status" />
    </div>

    <div class="flex size-14 shrink-0 items-center justify-center">
      <slot name="media" />
    </div>

    <div class="mt-3 w-full min-w-0">
      <!-- Slot fallback lets a loading tile drop a Skeleton bar in the name's
           place while keeping the exact shell, so nothing reflows on load. -->
      <slot name="name">
        <div class="truncate text-sm font-medium">
          {{ name }}
        </div>
      </slot>
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  name: string
  variant?: 'entity' | 'add'
}>(), {
  variant: 'entity',
})

// Surface + hover chrome per variant. `add` rests at canvas level (bg-background)
// and lifts to the card surface on hover — an elevation change, not a tint.
// `entity` rests on the card and deepens with the neutral overlay (bg-accent →
// --ui-hover), scheme-agnostic with no dark: override. Both are the tile's own
// chrome, so they carry the sanctioned escape hatch.
const surfaceClass = computed(() =>
  props.variant === 'add'
    ? 'bg-background text-muted-foreground hover:bg-card' /* ui-allow-style */
    : 'bg-card hover:bg-accent', /* ui-allow-style */
)
</script>
