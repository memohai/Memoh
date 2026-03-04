<template>
  <ToolCallRead
    v-if="block.toolName === 'read'"
    :block="block"
  />
  <ToolCallWrite
    v-else-if="block.toolName === 'write'"
    :block="block"
  />
  <ToolCallEdit
    v-else-if="block.toolName === 'edit'"
    :block="block"
  />
  <ToolCallList
    v-else-if="block.toolName === 'list'"
    :block="block"
  />
  <ToolCallExec
    v-else-if="block.toolName === 'exec'"
    :block="block"
  />
  <ToolCallWebSearch
    v-else-if="block.toolName === 'web_search'"
    :block="block"
  />
  <ToolCallSchedule
    v-else-if="scheduleTools.has(block.toolName)"
    :block="block"
  />
  <ToolCallGeneric
    v-else
    :block="block"
  />
</template>

<script setup lang="ts">
import type { ToolCallBlock } from '@/store/chat-list'
import ToolCallRead from './tool-call-read.vue'
import ToolCallWrite from './tool-call-write.vue'
import ToolCallEdit from './tool-call-edit.vue'
import ToolCallList from './tool-call-list.vue'
import ToolCallExec from './tool-call-exec.vue'
import ToolCallWebSearch from './tool-call-web-search.vue'
import ToolCallSchedule from './tool-call-schedule.vue'
import ToolCallGeneric from './tool-call-generic.vue'

defineProps<{
  block: ToolCallBlock
}>()

const scheduleTools = new Set([
  'list_schedule',
  'get_schedule',
  'create_schedule',
  'update_schedule',
  'delete_schedule',
])
</script>
