<template>
  <div class="space-y-1.5">
    <div
      v-if="tasks.length && !results.length"
      class="space-y-1"
    >
      <div
        v-for="(task, idx) in tasks"
        :key="idx"
        class="flex items-start gap-1.5 text-xs"
      >
        <span class="font-mono text-foreground shrink-0">#{{ idx + 1 }}</span>
        <span
          class="truncate text-muted-foreground"
          :title="task"
        >{{ task }}</span>
      </div>
    </div>

    <div
      v-if="results.length"
      class="space-y-1"
    >
      <component
        :is="result.session_id ? 'button' : 'div'"
        v-for="(result, idx) in results"
        :key="idx"
        class="flex items-center gap-1.5 text-xs w-full text-left rounded-sm px-1 py-0.5 transition-colors"
        :class="result.session_id ? 'cursor-pointer hover:bg-accent' : ''"
        @click="result.session_id ? navigateToSession(result.session_id) : undefined"
      >
        <CircleCheck
          v-if="result.success"
          class="size-3 text-success shrink-0"
        />
        <CircleX
          v-else
          class="size-3 text-destructive shrink-0"
        />
        <span class="font-mono text-foreground shrink-0">#{{ idx + 1 }}</span>
        <span
          v-if="result.task"
          class="truncate text-muted-foreground"
          :title="result.task"
        >{{ result.task }}</span>
        <ExternalLink
          v-if="result.session_id"
          class="size-3 text-muted-foreground/50 shrink-0 ml-auto"
        />
      </component>
    </div>

    <div
      v-if="hasDetailedResults"
      class="space-y-1 pt-1 border-t border-border/50"
    >
      <div
        v-for="(result, idx) in results"
        :key="idx"
      >
        <pre
          v-if="result.text"
          class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-32 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
        >{{ result.text }}</pre>
        <p
          v-if="result.error"
          class="text-xs text-destructive"
        >
          {{ result.error }}
        </p>
      </div>
    </div>

    <p
      v-if="!tasks.length && !results.length"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noTasks') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, CircleX, ExternalLink } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import type { ToolCallBlock } from '@/store/chat-list'

interface SpawnTaskResult {
  task?: string
  session_id?: string
  text?: string
  success?: boolean
  error?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()

const tasks = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  const t = input?.tasks
  return Array.isArray(t) ? (t as string[]) : []
})

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const results = computed<SpawnTaskResult[]>(() => {
  const r = resolveResult()
  if (!r) return []
  const items = r.results
  return Array.isArray(items) ? (items as SpawnTaskResult[]) : []
})

const hasDetailedResults = computed(() =>
  results.value.some(r => r.text || r.error),
)

function navigateToSession(sessionId: string) {
  if (!sessionId || !chatStore.currentBotId) return
  workspaceTabs.openChat(sessionId)
}
</script>
