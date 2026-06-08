<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import type { DemoTurn } from './demo-types'

defineProps<{ turn: DemoTurn }>()
const emit = defineEmits<{ (e: 'life', kind: 'mount' | 'unmount'): void }>()

// Every (re)mount triggers the flash animation below, so a turn that remounts
// on refetch is impossible to miss. Stable turns flash exactly once.
onMounted(() => emit('life', 'mount'))
onUnmounted(() => emit('life', 'unmount'))
</script>

<template>
  <div
    class="demo-turn"
    :class="turn.role"
  >
    <div class="who">
      {{ turn.role === 'user' ? 'You' : 'Assistant' }}
    </div>

    <div
      v-if="turn.role === 'user'"
      class="bubble"
    >
      {{ turn.text }}
    </div>

    <template v-else>
      <template
        v-for="block in turn.blocks"
        :key="block.id"
      >
        <div
          v-if="block.kind === 'thinking'"
          class="thinking"
        >
          💭 {{ block.content }}
        </div>
        <div
          v-else-if="block.kind === 'text'"
          class="text"
        >
          {{ block.content }}
        </div>
        <div
          v-else-if="block.kind === 'tool'"
          class="bgtask"
          :data-status="block.status"
        >
          <div class="bgtask-head">
            <span
              class="dot"
              :class="block.status"
            />
            <span class="tool">{{ block.toolName }}</span>
            <span class="status">{{ block.status }}</span>
          </div>
          <pre class="bgtask-out">{{ block.output }}</pre>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
@keyframes demoFlash {
  0% { background: rgba(244, 63, 94, 0.22); }
  100% { background: transparent; }
}
.demo-turn { padding: 8px 10px; border-radius: 10px; margin-bottom: 8px; animation: demoFlash 700ms ease-out; }
.who { font-size: 11px; opacity: 0.55; margin-bottom: 4px; }
.bubble { display: inline-block; background: rgba(99, 102, 241, 0.16); padding: 6px 10px; border-radius: 8px; }
.thinking { font-size: 12px; opacity: 0.6; font-style: italic; margin: 2px 0; white-space: pre-wrap; }
.text { white-space: pre-wrap; line-height: 1.55; margin: 2px 0; }
.bgtask { border: 1px solid rgba(127, 127, 127, 0.25); border-radius: 10px; margin: 8px 0; overflow: hidden; box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1); }
.bgtask-head { display: flex; align-items: center; gap: 8px; padding: 6px 10px; background: rgba(127, 127, 127, 0.08); font-size: 12px; }
.bgtask[data-status='completed'] .bgtask-head { background: rgba(34, 197, 94, 0.14); }
.tool { font-weight: 600; }
.status { margin-left: auto; opacity: 0.7; text-transform: capitalize; }
.bgtask-out { margin: 0; padding: 8px 10px; font-family: ui-monospace, monospace; font-size: 11.5px; line-height: 1.5; white-space: pre-wrap; max-height: 170px; overflow: auto; }
.dot { width: 9px; height: 9px; border-radius: 50%; background: #f59e0b; }
.dot.running { animation: demoPulse 1s infinite; }
.dot.completed { background: #22c55e; }
@keyframes demoPulse { 50% { opacity: 0.25; } }
</style>
