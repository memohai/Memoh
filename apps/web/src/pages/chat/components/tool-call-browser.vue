<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'window-maximize']"
        class="size-3 text-muted-foreground"
      />
      <span class="font-mono font-medium text-xs text-muted-foreground">
        {{ actionLabel }}
      </span>
      <span
        v-if="detail"
        class="text-xs truncate text-foreground"
        :title="detail"
      >
        {{ detail }}
      </span>
      <Badge
        v-if="block.done"
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
      v-if="block.done && resultText"
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
        <pre class="px-3 pb-2 text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-40 overflow-y-auto">{{ resultText }}</pre>
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

const input = computed(() => props.block.input as Record<string, unknown> | undefined)

const actionLabel = computed(() => {
  if (block.toolName === 'browser_action') {
    return (input.value?.action as string) ?? 'browser_action'
  }
  return (input.value?.observe as string) ?? 'browser_observe'
})

const { block } = props

const detail = computed(() => {
  const i = input.value
  if (!i) return ''
  return (i.url as string) ?? (i.selector as string) ?? ''
})

function resolveResult() {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const resultText = computed(() => {
  const r = resolveResult()
  if (!r) return ''
  // Skip displaying base64 image data
  if (r.content && Array.isArray(r.content)) {
    const texts = (r.content as Array<Record<string, unknown>>)
      .filter((c) => c.type === 'text')
      .map((c) => c.text as string)
    if (texts.length) return texts.join('\n')
  }
  const { content: _c, ...rest } = r
  const display = Object.keys(rest).length ? rest : r
  try {
    return JSON.stringify(display, null, 2)
  }
  catch {
    return String(r)
  }
})
</script>
