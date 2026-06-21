<template>
  <div v-if="subagents.length || isLoading">
    <p class="mb-1.5 text-caption font-medium uppercase tracking-wider text-muted-foreground">
      {{ $t('chat.infoSubagents') }}
    </p>
    <div
      v-if="isLoading && !subagents.length"
      class="flex items-center gap-2 py-1 text-body text-muted-foreground"
    >
      <Loader2 class="size-3 animate-spin" />
      {{ $t('common.loading') }}
    </div>
    <div
      v-else
      class="space-y-0.5"
    >
      <button
        v-for="agent in subagents"
        :key="agent.id"
        type="button"
        class="flex w-full items-center gap-1.5 rounded-sm px-2 py-1 text-left text-body transition-colors hover:bg-accent"
        @click="navigateToSession(agent.id)"
      >
        <GitBranch class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="min-w-0 flex-1 truncate text-foreground">
          {{ agent.agent_id || agent.title }}
        </span>
        <ExternalLink class="size-3 shrink-0 text-muted-foreground/50" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { GitBranch, ExternalLink, Loader2 } from 'lucide-vue-next'
import { useSubagentList } from '../composables/useSubagentList'

const props = defineProps<{
  visible: boolean
}>()

const visibleRef = toRef(props, 'visible')
const { subagents, isLoading, navigateToSession } = useSubagentList(visibleRef)
</script>
