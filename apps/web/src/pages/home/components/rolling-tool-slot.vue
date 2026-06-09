<template>
  <div class="relative">
    <Transition name="roll">
      <div
        v-if="shown"
        :key="shown.id"
      >
        <ToolCallBlock :block="shown" />
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import ToolCallBlock from './tool-call-block.vue'

// Codex-style single active command. Two refinements over "just show the last
// tool": an exec hasn't surfaced its command until its args stream in, so
// sitting on that bare "$ …" frontier made the slot read as perpetually
// "Working…". So (1) prefer the most recent *showable* tool — an exec with a
// command, or any non-exec — falling back to the latest only during genuine
// startup; and (2) hold each command on screen long enough to read, skipping
// ones that come and go faster than the dwell rather than flashing past.
const props = defineProps<{ tools: ToolCallBlockType[] }>()

function isReady(tool: ToolCallBlockType): boolean {
  if (tool.toolName !== 'exec') return true
  const input = tool.input as Record<string, unknown> | undefined
  return typeof input?.command === 'string' && input.command.length > 0
}

const candidate = computed<ToolCallBlockType | undefined>(() => {
  const tools = props.tools
  for (let i = tools.length - 1; i >= 0; i--) {
    if (isReady(tools[i]!)) return tools[i]
  }
  return tools[tools.length - 1]
})

const DWELL_MS = 650
const shown = ref<ToolCallBlockType | undefined>(candidate.value)
let timer: ReturnType<typeof setTimeout> | null = null
// Seed at mount so the first shown command gets a full dwell too (not an
// instant elapsed from epoch that would let it flash away).
let lastSwitch = Date.now()

watch(candidate, (next) => {
  if (!next || next.id === shown.value?.id) return
  // Hold a shown command for its dwell — including the first one — so it never
  // flashes past; but never make a not-yet-resolved placeholder linger: if what
  // is shown isn't a real command yet, switch to the resolved candidate at once.
  const shownReady = shown.value ? isReady(shown.value) : false
  const elapsed = Date.now() - lastSwitch
  if (!shownReady || elapsed >= DWELL_MS) {
    lastSwitch = Date.now()
    shown.value = next
  } else if (timer === null) {
    timer = setTimeout(() => {
      timer = null
      lastSwitch = Date.now()
      shown.value = candidate.value
    }, DWELL_MS - elapsed)
  }
})

onUnmounted(() => {
  if (timer !== null) clearTimeout(timer)
})
</script>

<style scoped>
.roll-enter-active {
  animation: roll-in 260ms ease-out;
}

.roll-leave-active {
  position: absolute;
  inset-inline: 0;
  top: 0;
  transition: opacity 120ms ease-in;
}

.roll-leave-to {
  opacity: 0;
}

@keyframes roll-in {
  from {
    opacity: 0;
    transform: translateY(9px);
  }

  to {
    opacity: 1;
    transform: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .roll-enter-active,
  .roll-leave-active {
    animation: none;
    transition: none;
  }
}
</style>
