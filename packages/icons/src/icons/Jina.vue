<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="currentColor"
    v-bind="$attrs"
  ><path d="M6.608 21.416a4.608 4.608 0 100-9.217 4.608 4.608 0 000 9.217zM20.894 2.015c.614 0 1.106.492 1.106 1.106v9.002c0 5.13-4.148 9.309-9.217 9.37v-9.355l-.03-9.032c0-.614.491-1.106 1.106-1.106h7.158l-.123.015z"></path></svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: string | number }>(), { size: '1em' })
defineOptions({ inheritAttrs: false })
</script>
