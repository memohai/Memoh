<template>
  <FontAwesomeIcon
    :icon="['fas', 'spinner']"
    spin
    :style="{ fontSize: `${size}px` }"
  />
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  size?: number
}>(), {
  size: 16,
})
</script>
