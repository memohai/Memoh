<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    v-bind="$attrs"
  ><path
    d="M4 18.5V5.5h3.2l4.8 6.4 4.8-6.4H20v13h-3.4V11l-4.6 6-4.6-6v7.5H4z"
    fill="currentColor"
  /></svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
