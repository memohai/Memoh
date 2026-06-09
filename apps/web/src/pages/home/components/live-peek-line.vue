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

const props = defineProps<{ text?: string, mono?: boolean, intervalMs?: number }>()

const latest = computed(() => latestOutputLine(props.text))

// A live peek that chases the raw token stream is exhausting to watch, so we
// sample it at a calm cadence (intervalMs) rather than following every change —
// the reasoning stream can fire many lines a second, but the peek refreshes at
// most ~once per interval. The gentle opacity settle replays only when the line
// is a *new* line, not when the current one grows, so there's no per-token
// flashing. Callers that want a snappier feel (bg output) pass a smaller value.
const intervalMs = props.intervalMs ?? 240
const line = ref(latest.value)
const animKey = ref(0)
let throttleTimer: ReturnType<typeof setTimeout> | null = null
let lastFlushAt = 0

function apply(next: string) {
  if (next === line.value) return
  // A growing line is a prefix-extension of the current one (or vice versa);
  // anything else is a genuinely different line worth re-settling.
  const isGrowth = next.startsWith(line.value) || line.value.startsWith(next)
  if (!isGrowth) animKey.value++
  line.value = next
}

watch(latest, () => {
  const elapsed = Date.now() - lastFlushAt
  if (elapsed >= intervalMs) {
    lastFlushAt = Date.now()
    apply(latest.value)
  } else if (throttleTimer === null) {
    throttleTimer = setTimeout(() => {
      throttleTimer = null
      lastFlushAt = Date.now()
      apply(latest.value)
    }, intervalMs - elapsed)
  }
})

onUnmounted(() => {
  if (throttleTimer !== null) clearTimeout(throttleTimer)
})
</script>

<style scoped>
/* A gentle brighten, never a flash from zero — calmest at a low cadence. */
.live-peek-line {
  animation: live-peek-in 260ms ease-out;
}

@keyframes live-peek-in {
  from {
    opacity: 0.5;
  }

  to {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .live-peek-line {
    animation: none;
  }
}
</style>
