<template>
  <span
    class="inline-block size-2 rounded-full"
    :class="colorClass"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  status?: 'success' | 'error' | 'warning' | 'idle'
}>(), {
  status: 'idle',
})

const colorClass = computed(() => {
  switch (props.status) {
    case 'success': return 'bg-success'
    case 'error': return 'bg-destructive'
    case 'warning': return 'bg-warning'
    default: return 'bg-muted-foreground'
  }
})
</script>
