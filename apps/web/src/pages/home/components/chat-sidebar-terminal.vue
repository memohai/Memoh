<template>
  <div class="flex flex-col h-full min-w-0">
    <TerminalComponent
      v-if="botId"
      :bot-id="botId"
      :visible="visible"
    />
  </div>
</template>

<script setup lang="ts">
import TerminalComponent from '@/components/terminal/index.vue'

defineProps<{
  botId: string
  visible: boolean
}>()
</script>
