<template>
  <div
    v-if="liveStatus"
    class="h-4 overflow-hidden"
  >
    <div
      :key="liveStatus"
      class="bg-task-live font-mono text-xs truncate text-muted-foreground/70 leading-4"
      :title="task.outputTail"
    >
      {{ liveStatus }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { BackgroundTask } from '@/store/chat-list'
import { latestOutputLine } from '@/store/chat-list.utils'

const props = defineProps<{ task: BackgroundTask }>()

const isActive = computed(() => {
  const status = (props.task.status || '').trim().toLowerCase()
  return status === 'running' || status === 'stalled'
})
const latestLine = computed(() => latestOutputLine(props.task.outputTail))

// Pace the displayed line so bursty output can't re-trigger the fade fast
// enough to stall; the latest line always wins (leading + trailing).
const THROTTLE_MS = 140
const displayLine = ref(latestLine.value)
let throttleTimer: ReturnType<typeof setTimeout> | null = null
let lastFlushAt = 0

watch(latestLine, () => {
  const elapsed = Date.now() - lastFlushAt
  if (elapsed >= THROTTLE_MS) {
    displayLine.value = latestLine.value
    lastFlushAt = Date.now()
  } else if (throttleTimer === null) {
    throttleTimer = setTimeout(() => {
      throttleTimer = null
      lastFlushAt = Date.now()
      displayLine.value = latestLine.value
    }, THROTTLE_MS - elapsed)
  }
})

onUnmounted(() => {
  if (throttleTimer !== null) clearTimeout(throttleTimer)
})

const liveStatus = computed(() => (isActive.value ? displayLine.value : ''))
</script>

<style scoped>
.bg-task-live {
  animation: bg-task-live-in 200ms ease-out;
}

@keyframes bg-task-live-in {
  from {
    opacity: 0;
    transform: translateY(3px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .bg-task-live {
    animation: none;
  }
}
</style>
