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
const shownReady = computed(() => (shown.value ? isReady(shown.value) : false))
let timer: ReturnType<typeof setTimeout> | null = null
// The dwell clock starts when a real command first becomes VISIBLE — not at
// mount. A placeholder shown at startup has no clock yet (0); it starts the
// instant the command resolves (in place, see the watcher) or when we switch
// to an already-ready tool. That way the first command gets a full dwell too.
let commandShownAt = shownReady.value ? Date.now() : 0

function show(next: ToolCallBlockType | undefined) {
  shown.value = next
  commandShownAt = next && isReady(next) ? Date.now() : 0
}

// A placeholder resolving its command in place is the moment that command
// becomes visible — begin its dwell here.
watch(shownReady, (ready) => {
  if (ready && commandShownAt === 0) commandShownAt = Date.now()
})

watch(candidate, (next) => {
  if (!next || next.id === shown.value?.id) return
  // Never make an unresolved placeholder linger; otherwise hold the shown
  // command for its full dwell, measured from when it became visible.
  if (!shownReady.value) {
    show(next)
    return
  }
  const elapsed = Date.now() - commandShownAt
  if (elapsed >= DWELL_MS) {
    show(next)
  } else if (timer === null) {
    timer = setTimeout(() => {
      timer = null
      show(candidate.value)
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
