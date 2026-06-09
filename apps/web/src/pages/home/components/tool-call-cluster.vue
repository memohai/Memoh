<template>
  <div class="leading-relaxed">
    <button
      :aria-expanded="open"
      class="group flex items-center gap-1.5 w-full text-left transition-colors cursor-pointer py-0.5 select-none text-muted-foreground hover:text-foreground"
      @click="open = !open"
    >
      <RailIconStack :icons="icons" />
      <span class="ml-1 shrink-0">{{ summaryLabel }}</span>
      <ChevronRight
        class="size-3.5 shrink-0 ml-auto opacity-45 transition-transform group-hover:opacity-90"
        :class="open ? 'rotate-90' : ''"
      />
    </button>

    <div
      v-if="open"
      class="mt-1 space-y-1.5"
    >
      <ToolCallBlock
        v-for="tool in tools"
        :key="tool.id"
        :block="tool"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import { distinctToolNames } from '@/store/chat-list.utils'
import { getToolDisplay } from './tool-call-registry'
import RailIconStack from './rail-icon-stack.vue'
import ToolCallBlock from './tool-call-block.vue'

const props = defineProps<{ tools: ToolCallBlockType[] }>()
const { t } = useI18n()

const open = ref(false)

const summaryLabel = computed(() => t('chat.tools.clustered', { count: props.tools.length }))

const MAX_ICONS = 4
const icons = computed(() => {
  const byName = new Map(props.tools.map(tool => [tool.toolName, tool]))
  return distinctToolNames(props.tools)
    .slice(0, MAX_ICONS)
    .map(name => byName.get(name))
    .filter((tool): tool is ToolCallBlockType => tool !== undefined)
    .map(tool => getToolDisplay(tool).icon)
})
</script>
