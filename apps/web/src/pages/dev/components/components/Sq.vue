<script setup lang="ts">
// Thin squircle wrapper. When `on`, applies smooth corner rounding to the single
// slotted element via `SmoothCorners as-child` — clip-path + autoEffects converts the
// element's CSS border/box-shadow into SVG overlays that follow the squircle path.
// When off, passes the child through untouched (plain CSS border-radius).
//
// Known clip-path tradeoffs (lisse): border/shadow are extracted ONCE on mount, so
// :hover transitions on border/box-shadow don't animate (background-color hover still
// does); `outline` (focus ring) is not clipped and stays rectangular.
import { SmoothCorners } from '@lisse/vue'

defineProps<{ on: boolean; radius: number; smoothing?: number }>()
</script>

<template>
  <SmoothCorners
    v-if="on"
    as-child
    :corners="{ radius, smoothing: smoothing ?? 0.6 }"
  >
    <slot />
  </SmoothCorners>
  <slot v-else />
</template>
