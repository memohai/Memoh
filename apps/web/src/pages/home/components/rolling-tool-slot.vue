<template>
  <div class="relative">
    <Transition name="roll">
      <div
        v-if="current"
        :key="current.id"
      >
        <ToolCallBlock :block="current" />
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import ToolCallBlock from './tool-call-block.vue'

// Codex-style single active command: only the latest tool of a live run is
// shown; when a new command arrives it slides up into the slot (roll-in) and
// the previous is absorbed (it has already settled into the prior-steps chip).
const props = defineProps<{ tools: ToolCallBlockType[] }>()

const current = computed(() => props.tools[props.tools.length - 1])
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
