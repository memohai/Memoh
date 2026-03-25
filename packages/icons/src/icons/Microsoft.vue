<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="currentColor"
    v-bind="$attrs"
  ><path d="M11.49 2H2v9.492h9.492V2h-.002z"></path><path d="M22 2h-9.492v9.492H22V2z"></path><path d="M11.49 12.508H2V22h9.492v-9.492h-.002z"></path><path d="M22 12.508h-9.492V22H22v-9.492z"></path></svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
