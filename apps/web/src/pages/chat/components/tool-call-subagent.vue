<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'code-branch']"
        class="size-3 text-violet-400"
      />
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

    <!-- Task list -->
    <div
      v-if="tasks.length"
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

    <!-- Results -->
    <Collapsible
      v-if="block.done && results.length"
      v-model:open="resultOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
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
            <div class="flex items-center gap-1.5 mb-0.5">
              <FontAwesomeIcon
                :icon="['fas', result.success ? 'circle-check' : 'circle-xmark']"
                class="size-2.5"
                :class="result.success ? 'text-green-500' : 'text-red-500'"
              />
              <span class="font-mono text-foreground">#{{ idx + 1 }}</span>
              <span
                v-if="result.task"
                class="truncate text-muted-foreground"
                :title="result.task"
              >
                {{ result.task }}
              </span>
            </div>
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
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

interface SpawnTaskResult {
  task?: string
  session_id?: string
  text?: string
  success?: boolean
  error?: string
}

const props = defineProps<{ block: ToolCallBlock }>()

const resultOpen = ref(false)

const tasks = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  const t = input?.tasks
  return Array.isArray(t) ? (t as string[]) : []
})

const taskCount = computed(() => {
  return tasks.value.length || null
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
</script>
