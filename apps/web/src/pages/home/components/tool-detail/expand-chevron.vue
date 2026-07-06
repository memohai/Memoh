<template>
  <!--
    Owner for the "this row expands/collapses a body" chevron. There were
    three independent implementations: tool-call-cluster rotated one icon;
    tool-call-group and tool-call-inline instead SWAPPED between ChevronDown
    and ChevronRight with their v-if/v-else branches in FLIPPED order between
    the two files (copy one as a template for the other and the open/closed
    direction silently inverts). They also disagreed on rest opacity
    (45/50/60). This owns the single-icon rotate (fewer DOM nodes, no v-if
    ordering to get backwards) at the middle rest opacity (50), hovering to
    100 (2 of the 3 sites already agreed on the hover target). Placement
    (ml-auto vs ml-0.5) stays with the caller via class passthrough — the two
    header shapes intentionally place it differently (cluster right-aligns a
    compact summary row; group/inline sit it right after inline text).
  -->
  <ChevronRight
    class="size-3.5 shrink-0 opacity-50 transition-transform group-hover:opacity-100"
    :class="open ? 'rotate-90' : ''"
  />
</template>

<script setup lang="ts">
import { ChevronRight } from 'lucide-vue-next'

defineProps<{ open: boolean }>()
</script>
