<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 512 512"
    v-bind="$attrs"
  ><path
    d="M256 0C114.6 0 0 114.6 0 256s114.6 256 256 256 256-114.6 256-256S397.4 0 256 0m116.6 269.8-47.2 47.2c-6.2 6.2-16.4 6.2-22.6 0l-46.8-46.8-46.8 46.8c-6.2 6.2-16.4 6.2-22.6 0l-47.2-47.2c-6.2-6.2-6.2-16.4 0-22.6l47.2-47.2c6.2-6.2 16.4-6.2 22.6 0l46.8 46.8 46.8-46.8c6.2-6.2 16.4-6.2 22.6 0l47.2 47.2c6.2 6.2 6.2 16.4 0 22.6"
    style="fill:#96d04a"
  /></svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
