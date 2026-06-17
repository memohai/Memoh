<!-- Custom copy glyph. lucide's `Copy` draws the back square as a bracket that
     stops ~4px short of the front square, leaving a visible gap that reads as two
     disconnected shapes. This redraws the same two-square copy mark in lucide's
     exact stroke language (24px box, 2px round stroke, currentColor) but runs the
     bracket's two open ends onto the front square's left and top edges, so the
     back square reads as continuously joined to the front one. Single <svg> root,
     so it inherits the button's [&_svg] sizing and any passed size-* class. -->
<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="strokeWidth"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <rect
      width="14"
      height="14"
      x="8"
      y="8"
      rx="3"
    />
    <path d="M8 16H5a3 3 0 0 1-3-3V5a3 3 0 0 1 3-3h8a3 3 0 0 1 3 3v3" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ strokeWidth?: number }>(), { strokeWidth: 2 })
</script>
