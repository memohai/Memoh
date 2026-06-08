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
    <circle
      cx="12"
      cy="12"
      r="9"
    />
    <line
      x1="12"
      y1="3"
      x2="12"
      y2="21"
    />
    <line
      x1="12"
      y1="8.5"
      x2="19.4"
      y2="8.5"
    />
    <line
      x1="12"
      y1="12"
      x2="20.5"
      y2="12"
    />
    <line
      x1="12"
      y1="15.5"
      x2="19.4"
      y2="15.5"
    />
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  strokeWidth?: number | string
}>(), {
  strokeWidth: 2,
})
</script>
