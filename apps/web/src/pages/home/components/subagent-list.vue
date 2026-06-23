<template>
  <div v-if="subagents.length || isLoading || error">
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
      v-else-if="error && !subagents.length"
      class="flex items-center justify-between gap-2 py-1 text-body text-muted-foreground"
      role="alert"
      aria-live="polite"
    >
      <span class="inline-flex min-w-0 items-center gap-1.5">
        <CircleAlert class="size-3.5 shrink-0 text-destructive" />
        <span class="truncate">{{ $t('chat.infoSubagentsLoadFailed') }}</span>
      </span>
      <Button
        variant="ghost"
        size="text"
        class="shrink-0"
        @click="refetch()"
      >
        <RefreshCw class="size-3" />
        {{ $t('common.actions.retry') }}
      </Button>
    </div>
    <div
      v-else
      class="space-y-0.5"
    >
      <Button
        v-for="agent in subagents"
        :key="agent.id"
        variant="ghost"
        size="sm"
        block
        class="h-auto min-h-8 w-full justify-start gap-1.5 px-2 py-1.5 text-body font-normal"
        @click="navigateToSession(agent.id)"
      >
        <GitBranch class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="min-w-0 flex-1 text-left">
          <span class="block truncate text-foreground">{{ agent.title }}</span>
          <span
            v-if="agent.agent_id && agent.agent_id !== agent.title"
            class="block truncate text-caption text-muted-foreground"
          >
            {{ agent.agent_id }}
          </span>
        </span>
        <ExternalLink class="size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { CircleAlert, ExternalLink, GitBranch, Loader2, RefreshCw } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { useSubagentList } from '../composables/useSubagentList'

const props = defineProps<{
  visible: boolean
}>()

const visibleRef = toRef(props, 'visible')
const { subagents, error, isLoading, refetch, navigateToSession } = useSubagentList(visibleRef)
</script>
