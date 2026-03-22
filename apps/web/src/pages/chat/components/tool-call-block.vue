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
  <ToolCallMemory
    v-else-if="block.toolName === 'search_memory'"
    :block="block"
  />
  <ToolCallWebFetch
    v-else-if="block.toolName === 'web_fetch'"
    :block="block"
  />
  <ToolCallMessage
    v-else-if="messageTools.has(block.toolName)"
    :block="block"
  />
  <ToolCallContacts
    v-else-if="block.toolName === 'get_contacts'"
    :block="block"
  />
  <ToolCallEmail
    v-else-if="emailTools.has(block.toolName)"
    :block="block"
  />
  <ToolCallBrowser
    v-else-if="browserTools.has(block.toolName)"
    :block="block"
  />
  <ToolCallSubagent
    v-else-if="subagentTools.has(block.toolName)"
    :block="block"
  />
  <ToolCallSkill
    v-else-if="block.toolName === 'use_skill'"
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
import ToolCallMemory from './tool-call-memory.vue'
import ToolCallWebFetch from './tool-call-web-fetch.vue'
import ToolCallMessage from './tool-call-message.vue'
import ToolCallContacts from './tool-call-contacts.vue'
import ToolCallEmail from './tool-call-email.vue'
import ToolCallBrowser from './tool-call-browser.vue'
import ToolCallSubagent from './tool-call-subagent.vue'
import ToolCallSkill from './tool-call-skill.vue'
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

const messageTools = new Set(['send', 'react'])

const emailTools = new Set([
  'send_email',
  'list_email',
  'read_email',
  'list_email_accounts',
])

const browserTools = new Set(['browser_action', 'browser_observe'])

const subagentTools = new Set(['spawn'])
</script>
