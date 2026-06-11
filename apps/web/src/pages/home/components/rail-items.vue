<template>
  <template
    v-for="item in items"
    :key="item.key"
  >
    <ToolCallCluster
      v-if="item.kind === 'cluster'"
      :tools="(item.tools as ToolCallBlockType[])"
    />
    <ThinkingBlock
      v-else-if="item.block.type === 'reasoning'"
      :block="(item.block as ThinkingBlockType)"
      :streaming="item.block.id === streamingBlockId"
    />
    <ToolCallBlock
      v-else-if="item.block.type === 'tool'"
      :block="(item.block as ToolCallBlockType)"
    />
  </template>
</template>

<script setup lang="ts">
import type {
  ContentBlock,
  ThinkingBlock as ThinkingBlockType,
  ToolCallBlock as ToolCallBlockType,
} from '@/store/chat-list'
import type { RailItem } from '@/store/chat-list.utils'
import ThinkingBlock from './thinking-block.vue'
import ToolCallBlock from './tool-call-block.vue'
import ToolCallCluster from './tool-call-cluster.vue'

defineProps<{
  items: RailItem<ContentBlock>[]
  streamingBlockId?: number | null
}>()
</script>
