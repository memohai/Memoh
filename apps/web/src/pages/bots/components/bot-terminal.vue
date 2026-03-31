<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import Terminal from '@/components/terminal/index.vue'

const route = useRoute()
const botId = computed(() => route.params.botId as string)
const activeTab = useSyncedQueryParam('tab', 'overview')
</script>

<template>
  <Terminal
    :bot-id="botId"
    :visible="activeTab === 'terminal'"
  />
</template>
