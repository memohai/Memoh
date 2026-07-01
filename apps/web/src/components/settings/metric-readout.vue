<template>
  <!-- One metric tile. The caller owns the grid (grid-cols-3 / sm:grid-cols-4);
       this is a single cell, never the grid. `framed` draws the tile box; unframed
       is the same content bare, for a stat that sits directly on a surface
       (bot-overview usage). Shared min-h so a cold-load tile doesn't jump. -->
  <div :class="framed ? 'flex min-h-[4.375rem] flex-col rounded-[var(--radius-menu-shell)] border border-border bg-card p-3' : 'flex flex-col'">
    <p class="text-caption text-muted-foreground">
      {{ label }}
    </p>

    <!-- Status renders a signal dot + label in place of a bare value; the dot
         color is a RATIONED signal token (success/warning/destructive), never a
         surface tint. Otherwise the value line is a tabular figure so digits stay
         column-aligned across tiles, with a `value` slot for custom markup
         (mono paths/counts) that falls back to the value prop. -->
    <div
      v-if="status"
      class="mt-1 flex items-center gap-1.5"
    >
      <span
        class="size-1.5 rounded-full"
        :class="dotClass"
      />
      <span
        class="text-sm font-medium leading-none"
        :class="statusTextClass"
      >
        <slot name="value">{{ value }}</slot>
      </span>
    </div>
    <p
      v-else
      class="mt-1 text-lg font-semibold tabular-nums text-foreground"
    >
      <slot name="value">
        {{ value }}
      </slot>
    </p>

    <p
      v-if="sub"
      class="mt-1 text-caption text-muted-foreground"
    >
      {{ sub }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  value?: string
  sub?: string
  framed?: boolean
  // A rationed signal state. Present = the value line is a status dot + label
  // instead of a bare figure; absent = a plain metric readout.
  status?: 'ok' | 'warn' | 'error'
}>(), {
  value: '',
  sub: '',
  framed: true,
})

const dotClass = computed(() => {
  switch (props.status) {
    case 'ok': return 'bg-success'
    case 'warn': return 'bg-warning'
    case 'error': return 'bg-destructive'
    default: return ''
  }
})

// The label text tracks the signal, but only the error case shades the text —
// an ok/warn readout keeps foreground text so the dot alone carries the signal
// and the tile doesn't read as tinted.
const statusTextClass = computed(() =>
  props.status === 'error' ? 'text-destructive' : 'text-foreground',
)
</script>
