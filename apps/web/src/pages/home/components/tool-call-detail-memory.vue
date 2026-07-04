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
        <Badge
          v-if="typeof item.score === 'number'"
          variant="secondary"
          size="sm"
          font="mono"
          class="shrink-0"
        >
          {{ item.score.toFixed(2) }}
        </Badge>
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
import { Badge } from '@memohai/ui'
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
