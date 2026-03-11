<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'robot']"
        class="size-3 text-muted-foreground"
      />

      <!-- query_subagent -->
      <template v-if="block.toolName === 'query_subagent'">
        <span
          v-if="name"
          class="font-mono font-medium text-xs text-foreground"
        >
          {{ name }}
        </span>
        <span
          v-if="query"
          class="text-xs truncate text-muted-foreground"
          :title="query"
        >
          {{ query }}
        </span>
      </template>

      <!-- list_subagents / delete_subagent -->
      <template v-else>
        <span class="font-mono font-medium text-xs text-muted-foreground">
          {{ block.toolName }}
        </span>
        <span
          v-if="block.toolName === 'delete_subagent' && deleteId"
          class="text-xs truncate text-foreground"
        >
          {{ deleteId }}
        </span>
      </template>

      <Badge
        v-if="block.done && block.toolName === 'list_subagents' && subagentCount !== null"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolSubagentCount', { count: subagentCount }) }}
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

    <!-- query_subagent result -->
    <Collapsible
      v-if="block.done && block.toolName === 'query_subagent' && subagentResult"
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
        <pre class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-40 overflow-y-auto">{{ subagentResult }}</pre>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const resultOpen = ref(false)

const name = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.name as string) ?? ''
})

const query = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.query as string) ?? ''
})

const deleteId = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.id as string) ?? ''
})

function resolveResult() {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const subagentCount = computed(() => {
  const r = resolveResult()
  if (!r) return null
  const items = r.items as unknown[] | undefined
  return Array.isArray(items) ? items.length : null
})

const subagentResult = computed(() => {
  const r = resolveResult()
  return (r?.result as string) ?? ''
})
</script>
