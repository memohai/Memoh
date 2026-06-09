<template>
  <div class="rail-settle">
    <button
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-0.5 select-none text-muted-foreground hover:text-foreground"
      :aria-expanded="open"
      @click="open = !open"
    >
      <RailIconStack :icons="icons" />
      <span class="ml-1 shrink-0 font-medium">{{ label }}</span>
      <ChevronRight
        class="size-3.5 shrink-0 ml-auto opacity-45 transition-transform group-hover:opacity-90"
        :class="open ? 'rotate-90' : ''"
      />
    </button>
    <div
      v-if="open"
      class="ml-0.5"
    >
      <RailItems :items="clusteredItems" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronRight, Lightbulb } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { Component } from 'vue'
import type { ContentBlock, ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import { clusterRailBlocks, summarizeRailSegment } from '@/store/chat-list.utils'
import { getToolDisplay } from './tool-call-registry'
import RailIconStack from './rail-icon-stack.vue'
import RailItems from './rail-items.vue'

const props = defineProps<{ blocks: ContentBlock[] }>()
const { t } = useI18n()

const open = ref(false)

const summary = computed(() => summarizeRailSegment(props.blocks))

const MAX_ICONS = 4
const icons = computed<Component[]>(() => {
  const out: Component[] = []
  if (summary.value.thinkingCount > 0) out.push(Lightbulb)
  const byName = new Map(
    props.blocks
      .filter((block): block is ToolCallBlockType => block.type === 'tool')
      .map(block => [block.toolName, block]),
  )
  for (const name of summary.value.toolNames) {
    if (out.length >= MAX_ICONS) break
    const block = byName.get(name)
    if (block) out.push(getToolDisplay(block).icon)
  }
  return out
})

const label = computed(() => {
  const parts: string[] = []
  if (summary.value.thinkingCount > 0) parts.push(t('chat.rail.thinking', { count: summary.value.thinkingCount }))
  if (summary.value.toolCount > 0) parts.push(t('chat.tools.clustered', { count: summary.value.toolCount }))
  return parts.join(' · ')
})

const clusteredItems = computed(() => clusterRailBlocks(props.blocks, false))
</script>

<style scoped>
/* When a turn settles, its rail collapses to this one line — settle it in
   gently (a small downward drop + fade) so the fold reads as a collapse
   rather than a hard cut. */
.rail-settle {
  animation: rail-settle-in 260ms ease-out;
}

@keyframes rail-settle-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .rail-settle {
    animation: none;
  }
}
</style>
