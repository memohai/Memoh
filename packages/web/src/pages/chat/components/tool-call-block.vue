<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <!-- Header -->
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <span class="font-mono font-medium text-xs">
        {{ block.toolName }}
      </span>
      <Badge
        v-if="block.done"
        variant="secondary"
        class="text-[10px] ml-auto"
      >
        {{ $t('chat.toolDone') }}
      </Badge>
      <Badge
        v-else
        variant="outline"
        class="text-[10px] ml-auto"
      >
        {{ $t('chat.toolRunning') }}
      </Badge>
    </div>

    <!-- Input (collapsible) -->
    <Collapsible v-if="block.input" v-model:open="inputOpen">
        <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': inputOpen }"
        />
        {{ $t('chat.toolInput') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all">{{ formatJson(block.input) }}</pre>
      </CollapsibleContent>
    </Collapsible>

    <!-- Result (collapsible) -->
    <Collapsible v-if="block.done && block.result != null" v-model:open="resultOpen">
        <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full border-t border-muted">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': resultOpen }"
        />
        {{ $t('chat.toolResult') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all">{{ formatJson(block.result) }}</pre>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

defineProps<{
  block: ToolCallBlock
}>()

const inputOpen = ref(false)
const resultOpen = ref(false)

function formatJson(val: unknown): string {
  if (typeof val === 'string') return val
  try {
    return JSON.stringify(val, null, 2)
  } catch {
    return String(val)
  }
}
</script>
