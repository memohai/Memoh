<template>
  <div
    v-if="line"
    class="h-4 overflow-hidden"
  >
    <div
      :key="line"
      class="live-peek-line truncate text-xs text-muted-foreground/70 leading-4"
      :class="mono ? 'font-mono' : ''"
      :title="text"
    >
      {{ line }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { latestOutputLine } from '@/store/chat-list.utils'

const props = defineProps<{ text?: string, mono?: boolean }>()

const latest = computed(() => latestOutputLine(props.text))

// Pace the displayed line so bursty updates can't re-trigger the fade fast
// enough to stall; the latest line always wins (leading + trailing).
const THROTTLE_MS = 140
const line = ref(latest.value)
let throttleTimer: ReturnType<typeof setTimeout> | null = null
let lastFlushAt = 0

watch(latest, () => {
  const elapsed = Date.now() - lastFlushAt
  if (elapsed >= THROTTLE_MS) {
    line.value = latest.value
    lastFlushAt = Date.now()
  } else if (throttleTimer === null) {
    throttleTimer = setTimeout(() => {
      throttleTimer = null
      lastFlushAt = Date.now()
      line.value = latest.value
    }, THROTTLE_MS - elapsed)
  }
})

onUnmounted(() => {
  if (throttleTimer !== null) clearTimeout(throttleTimer)
})
</script>

<style scoped>
.live-peek-line {
  animation: live-peek-in 200ms ease-out;
}

@keyframes live-peek-in {
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
  .live-peek-line {
    animation: none;
  }
}
</style>
