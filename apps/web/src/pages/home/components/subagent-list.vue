<template>
  <div
    v-if="subagents.length"
    class="space-y-0.5"
  >
    <p class="mb-1.5 text-caption font-medium uppercase tracking-wider text-muted-foreground">
      {{ $t('chat.infoSubagents') }}
    </p>
    <Button
      v-for="agent in subagents"
      :key="agent.id"
      variant="ghost"
      size="sm"
      class="h-auto min-h-8 w-full justify-start gap-1.5 px-2 py-1.5 text-body font-normal"
      @click="navigateToSession(agent.id)"
    >
      <GitBranch class="size-3.5 shrink-0 text-muted-foreground" />
      <span class="min-w-0 flex-1 truncate text-left text-foreground">
        {{ agent.agentId }}
      </span>
      <ExternalLink class="size-3.5 shrink-0 text-muted-foreground" />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { GitBranch, ExternalLink } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useSubagentList } from '../composables/useSubagentList'

const { subagents, navigateToSession } = useSubagentList()
</script>
