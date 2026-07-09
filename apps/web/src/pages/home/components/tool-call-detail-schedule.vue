<template>
  <div class="space-y-1.5">
    <div
      v-if="items.length"
      class="space-y-1"
    >
      <div
        v-for="(item, i) in items"
        :key="item.id ?? i"
        class="flex flex-col gap-0.5 text-xs"
      >
        <div class="flex items-center gap-2">
          <span class="text-foreground truncate flex-1">{{ item.name || item.id || t('chat.tools.detail.unnamedSchedule') }}</span>
          <Badge
            v-if="item.pattern"
            variant="secondary"
            size="sm"
            font="mono"
            class="shrink-0"
          >
            {{ item.pattern }}
          </Badge>
        </div>
        <span
          v-if="item.prompt"
          class="text-caption text-muted-foreground line-clamp-2"
        >{{ item.prompt }}</span>
      </div>
    </div>
    <EmptyRow v-else>
      {{ t('chat.tools.detail.noSchedules') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@felinic/ui'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'

interface ScheduleItem {
  id?: string
  name?: string
  pattern?: string
  prompt?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const items = computed<ScheduleItem[]>(() => {
  if (!props.block.done) return []
  const r = resolveResult()
  if (!r) return []
  const arr = r.items as ScheduleItem[] | undefined
  return Array.isArray(arr) ? arr : []
})
</script>
