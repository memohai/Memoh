<template>
  <div
    class="border-l border-border/60 pl-3.5 space-y-1.5 transition-opacity duration-300"
    :class="settled ? 'opacity-70' : 'opacity-100'"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ settled?: boolean }>()
</script>
