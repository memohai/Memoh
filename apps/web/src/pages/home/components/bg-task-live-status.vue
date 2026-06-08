<template>
  <LivePeekLine
    v-if="isActive"
    :text="task.outputTail"
    mono
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { BackgroundTask } from '@/store/chat-list'
import LivePeekLine from './live-peek-line.vue'

const props = defineProps<{ task: BackgroundTask }>()

const isActive = computed(() => {
  const status = (props.task.status || '').trim().toLowerCase()
  return status === 'running' || status === 'stalled'
})
</script>
