<template>
  <div class="rounded-lg border bg-muted/30 text-xs overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <Check
        v-if="block.done"
        class="size-3 text-green-600 dark:text-green-400"
      />
      <LoaderCircle
        v-else
        class="size-3 animate-spin text-muted-foreground"
      />
      <GitBranch class="size-3 text-violet-400" />
      <span class="font-mono font-medium text-xs text-foreground">
        spawn
      </span>
      <Badge
        v-if="block.done && taskCount !== null"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolSpawnCount', { count: taskCount }) }}
      </Badge>
      <Badge
        v-else-if="block.done"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolDone') }}
      </Badge>
      <Badge
        v-else
        variant="outline"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolRunning') }}
      </Badge>
    </div>

    <!-- Live subagent status (shown during execution when progress available) -->
    <div
      v-if="subagentStatuses.length && !results.length"
      class="px-3 py-2 space-y-1"
    >
      <div
        v-for="status in subagentStatuses"
        :key="status.index"
        class="flex items-center gap-1.5 text-xs truncate"
        :title="status.task"
      >
        <LoaderCircle
          v-if="status.status === 'running'"
          class="size-2.5 animate-spin text-muted-foreground shrink-0"
        />
        <CircleCheck
          v-else-if="status.status === 'completed'"
          class="size-2.5 text-green-500 shrink-0"
        />
        <CircleX
          v-else-if="status.status === 'failed'"
          class="size-2.5 text-red-500 shrink-0"
        />
        <span class="font-mono text-foreground shrink-0">#{{ status.index + 1 }}</span>
        <span class="truncate text-muted-foreground">{{ status.task }}</span>
      </div>
    </div>

    <!-- Task list fallback (shown while running without progress) -->
    <div
      v-else-if="tasks.length && !results.length"
      class="px-3 py-2 space-y-1"
    >
      <div
        v-for="(task, idx) in tasks"
        :key="idx"
        class="text-xs text-muted-foreground truncate"
        :title="task"
      >
        <span class="text-foreground font-mono mr-1.5">#{{ idx + 1 }}</span>
        {{ task }}
      </div>
    </div>

    <!-- Results (clickable to navigate to subagent session) -->
    <div
      v-if="block.done && results.length"
      class="px-3 py-2 space-y-1"
    >
      <component
        :is="result.session_id ? 'button' : 'div'"
        v-for="(result, idx) in results"
        :key="idx"
        class="flex items-center gap-1.5 text-xs w-full text-left rounded-md px-1.5 py-1 -mx-1.5 transition-colors"
        :class="result.session_id
          ? 'cursor-pointer hover:bg-accent'
          : ''"
        @click="result.session_id ? navigateToSession(result.session_id) : undefined"
      >
        <CircleCheck
          v-if="result.success"
          class="size-2.5 text-green-500 shrink-0"
        />
        <CircleX
          v-else
          class="size-2.5 text-red-500 shrink-0"
        />
        <span class="font-mono text-foreground shrink-0">#{{ idx + 1 }}</span>
        <span
          v-if="result.task"
          class="truncate text-muted-foreground"
          :title="result.task"
        >
          {{ result.task }}
        </span>
        <button
          v-if="!result.success"
          class="ml-auto text-[10px] text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded border border-border hover:bg-accent transition-colors shrink-0"
          @click.stop="retryTask(result.task || '')"
        >
          {{ $t('chat.toolRetry') }}
        </button>
        <ExternalLink
          v-if="result.session_id"
          class="size-2.5 text-muted-foreground/50 shrink-0 ml-auto"
        />
      </component>
    </div>

    <!-- Detailed results (collapsible) -->
    <Collapsible
      v-if="block.done && hasDetailedResults"
      v-model:open="resultOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <ChevronRight
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': resultOpen }"
        />
        {{ $t('chat.toolResult') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div class="px-3 pb-2 space-y-2">
          <div
            v-for="(result, idx) in results"
            :key="idx"
            class="text-xs"
          >
            <pre
              v-if="result.text"
              class="text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-32 overflow-y-auto pl-4"
            >{{ result.text }}</pre>
            <p
              v-if="result.error"
              class="text-red-500 pl-4"
            >
              {{ result.error }}
            </p>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Check, LoaderCircle, GitBranch, ChevronRight, CircleCheck, CircleX, ExternalLink } from 'lucide-vue-next'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memohai/ui'
import { useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import type { ToolCallBlock } from '@/store/chat-list'

interface SpawnTaskResult {
  task?: string
  session_id?: string
  text?: string
  success?: boolean
  error?: string
}

interface SubagentStatus {
  index: number
  task: string
  status: 'running' | 'completed' | 'failed'
  attempt?: number
}

const props = defineProps<{ block: ToolCallBlock }>()

const router = useRouter()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const resultOpen = ref(false)

const tasks = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  const t = input?.tasks
  return Array.isArray(t) ? (t as string[]) : []
})

const taskCount = computed(() => {
  return tasks.value.length || null
})

// Extract the latest subagent status from progress events.
// The backend heartbeat sends SubagentStatus[] arrays as progress updates.
const subagentStatuses = computed<SubagentStatus[]>(() => {
  const progress = props.block.progress
  if (!Array.isArray(progress) || progress.length === 0) return []
  // The last progress entry is the most recent heartbeat payload.
  const latest = progress[progress.length - 1]
  if (!latest || typeof latest !== 'object' || !Array.isArray(latest)) return []
  return (latest as SubagentStatus[]).filter(
    s => s && typeof s === 'object' && 'status' in s
  ) as SubagentStatus[]
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
  const botId = currentBotId.value
  if (!botId || !sessionId) return
  chatStore.selectSession(sessionId)
  router.push({
    name: 'chat',
    params: { botId, sessionId },
  })
}

function retryTask(task: string) {
  if (!task) return
  chatStore.sendMessage(`请重试 spawn 中失败的任务：${task}`)
}
</script>
