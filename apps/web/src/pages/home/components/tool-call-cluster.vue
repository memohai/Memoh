<template>
  <div class="leading-relaxed">
    <HeaderRow
      :open="open"
      tone="muted"
      @toggle="open = !open"
    >
      <RailIconStack :icons="icons" />
      <span class="ml-1 shrink-0">{{ summaryLabel }}</span>
      <ExpandChevron
        :open="open"
        class="ml-auto"
      />
    </HeaderRow>

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
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock as ToolCallBlockType } from '@/store/chat-list'
import { distinctToolNames } from '@/store/chat-list.utils'
import { getToolDisplay } from './tool-call-registry'
import RailIconStack from './rail-icon-stack.vue'
import ToolCallBlock from './tool-call-block.vue'
import HeaderRow from './tool-detail/header-row.vue'
import ExpandChevron from './tool-detail/expand-chevron.vue'

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
