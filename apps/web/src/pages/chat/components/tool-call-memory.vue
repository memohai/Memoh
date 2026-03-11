<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'brain']"
        class="size-3 text-muted-foreground"
      />
      <span class="text-xs truncate text-foreground">
        {{ query }}
      </span>
      <Badge
        v-if="block.done && results.length"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolMemoryResults', { count: results.length }) }}
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

    <Collapsible
      v-if="block.done && results.length"
      v-model:open="resultsOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': resultsOpen }"
        />
        {{ $t('chat.toolSearchResultsLabel') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div class="px-3 pb-2 space-y-1.5">
          <div
            v-for="(item, i) in results"
            :key="i"
            class="flex items-start gap-2"
          >
            <span class="text-xs text-muted-foreground whitespace-pre-wrap break-all flex-1">{{ item.memory }}</span>
            <Badge
              variant="outline"
              class="text-[10px] shrink-0"
            >
              {{ item.score.toFixed(2) }}
            </Badge>
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

interface MemoryResult {
  id: string
  memory: string
  score: number
}

const props = defineProps<{ block: ToolCallBlock }>()

const resultsOpen = ref(false)

const query = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.query as string) ?? ''
})

const results = computed<MemoryResult[]>(() => {
  if (!props.block.done || !props.block.result) return []
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc ?? result).results as MemoryResult[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
