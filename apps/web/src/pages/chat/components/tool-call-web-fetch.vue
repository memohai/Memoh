<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'globe']"
        class="size-3 text-muted-foreground"
      />
      <a
        v-if="url"
        :href="url"
        target="_blank"
        rel="noopener noreferrer"
        class="text-xs truncate text-primary hover:underline"
        :title="url"
      >
        {{ url }}
      </a>
      <Badge
        v-if="block.done && format"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ format }}
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
      v-if="block.done && preview"
      v-model:open="previewOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <FontAwesomeIcon
          :icon="['fas', 'chevron-right']"
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': previewOpen }"
        />
        {{ $t('chat.toolWebFetchPreview') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div class="px-3 pb-2 space-y-1">
          <div
            v-if="title"
            class="text-xs font-medium text-foreground"
          >
            {{ title }}
          </div>
          <div
            v-if="excerpt"
            class="text-[10px] text-muted-foreground italic"
          >
            {{ excerpt }}
          </div>
          <pre
            v-if="contentPreview"
            class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-40 overflow-y-auto"
          >{{ contentPreview }}</pre>
        </div>
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const previewOpen = ref(false)

const url = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.url as string) ?? ''
})

function resolveResult() {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const format = computed(() => {
  const r = resolveResult()
  return (r?.format as string) ?? ''
})

const title = computed(() => {
  const r = resolveResult()
  return (r?.title as string) ?? ''
})

const excerpt = computed(() => {
  const r = resolveResult()
  return (r?.excerpt as string) ?? ''
})

const contentPreview = computed(() => {
  const r = resolveResult()
  if (!r) return ''
  const content = (r.content as string) ?? (r.textContent as string) ?? ''
  return content.length > 500 ? `${content.slice(0, 500)}…` : content
})

const preview = computed(() => title.value || excerpt.value || contentPreview.value)
</script>
