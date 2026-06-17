<template>
  <div class="space-y-1.5">
    <div
      v-if="results.length"
      class="space-y-1.5"
    >
      <div
        v-for="(item, i) in results"
        :key="item.id ?? i"
        class="flex items-start gap-2"
      >
        <span class="text-xs text-foreground whitespace-pre-wrap wrap-break-word flex-1">
          {{ item.memory }}
        </span>
        <span
          v-if="typeof item.score === 'number'"
          class="text-caption text-muted-foreground font-mono shrink-0 rounded bg-muted/30 px-1 py-0.5"
        >
          {{ item.score.toFixed(2) }}
        </span>
      </div>
    </div>
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noResults') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

interface MemoryResult {
  id?: string
  memory: string
  score?: number
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const results = computed<MemoryResult[]>(() => {
  if (!props.block.done || !props.block.result) return []
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc ?? result).results as MemoryResult[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
