<script setup lang="ts">
import FileManager from '@/components/file-manager/index.vue'

defineProps<{
  botId: string
  botType?: string
}>()
</script>

<template>
  <FileManager
    :bot-id="botId"
    class="h-full"
  />
</template>
