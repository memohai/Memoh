<script setup lang="ts">
defineProps<{
  size?: number | string
  class?: string
}>()
</script>

<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    :width="size"
    :height="size"
    :class="$props.class"
    fill="currentColor"
  >
    <!-- TODO: paste SVG path here -->
  </svg>
</template>
