<script setup lang="ts">
import { PopoverAnchor } from 'reka-ui'
import { injectHoverCardContext } from './context'

// The trigger is just the Popper anchor (no click-to-toggle). Hover bridge lives
// in the shared context: enter → open, leave → arm close. focus/blur mirror it so
// keyboard users can reveal the card too (Popover content never traps focus).
defineProps<{ asChild?: boolean }>()

const ctx = injectHoverCardContext()
</script>

<template>
  <PopoverAnchor
    :as-child="asChild"
    data-slot="hover-card-trigger"
    @pointerenter="ctx.scheduleOpen"
    @pointerleave="ctx.scheduleClose"
    @focus="ctx.scheduleOpen"
    @blur="ctx.scheduleClose"
  >
    <slot />
  </PopoverAnchor>
</template>
