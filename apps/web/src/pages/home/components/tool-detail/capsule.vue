<template>
  <!--
    Owner for the "muted capsule surface" shell shared by the process body
    (tool-call-group, housing multiple nested tool rows) and a non-grouped
    tool's own detail card (tool-call-inline). Both are `rounded-md bg-muted`,
    but the padding had drifted independently (px-2.5/py-1.5 vs px-3/py-2).
    That drift wasn't arbitrary — tool-call-group's own comment says the
    tighter padding deliberately establishes a denser, one-notch-smaller type
    scale so nested steps read as a distinct layer — so instead of forcing one
    padding number onto both (which would erase that intentional density
    difference) or leaving two hand-copied numbers free to drift further
    apart, the difference is named as an explicit variant on one owner.
    tool-call-inline's OTHER detail variant (bg-card, used only `inGroup` —
    a card nested inside the group's own capsule) stays hand-written: it is a
    genuinely different surface, not a padding tweak of this shell.
  -->
  <div :class="shellClass">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  // 'compact' = the process body housing multiple nested tool rows.
  // 'detail'  = a single tool's own (non-grouped) detail card.
  density?: 'compact' | 'detail'
}>(), {
  density: 'detail',
})

// Tailwind scans literal source text — every combination spelled out in full
// so neither variant's classes are dropped by the JIT scanner.
const shellClass = computed(() =>
  props.density === 'compact'
    ? 'rounded-md bg-muted px-2.5 py-1.5'
    : 'rounded-md bg-muted px-3 py-2',
)
</script>
