<template>
  <div
    v-if="line"
    class="h-4 overflow-hidden"
  >
    <div
      :key="animKey"
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

// Show the latest line, paced so bursty output can't thrash reactivity. The
// entrance slide replays only when the line is a *new* line — not when the
// current line grows token-by-token — so a streaming reasoning line updates its
// text in place (calm) instead of re-animating on every token (the flicker).
const THROTTLE_MS = 160
const line = ref(latest.value)
const animKey = ref(0)
let throttleTimer: ReturnType<typeof setTimeout> | null = null
let lastFlushAt = 0

function apply(next: string) {
  if (next === line.value) return
  // A growing line is a prefix-extension of the current one (or vice versa);
  // anything else is a genuinely different line worth re-animating.
  const isGrowth = next.startsWith(line.value) || line.value.startsWith(next)
  if (!isGrowth) animKey.value++
  line.value = next
}

watch(latest, () => {
  const elapsed = Date.now() - lastFlushAt
  if (elapsed >= THROTTLE_MS) {
    lastFlushAt = Date.now()
    apply(latest.value)
  } else if (throttleTimer === null) {
    throttleTimer = setTimeout(() => {
      throttleTimer = null
      lastFlushAt = Date.now()
      apply(latest.value)
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
