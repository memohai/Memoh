<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'clock']"
        class="size-3 text-muted-foreground"
      />
      <span class="font-mono font-medium text-xs text-muted-foreground">
        {{ block.toolName }}
      </span>
      <span
        v-if="label"
        class="text-xs truncate text-foreground"
        :title="label"
      >
        {{ label }}
      </span>
      <Badge
        v-if="block.done && isList && itemCount !== null"
        variant="secondary"
        class="text-[10px] ml-auto shrink-0"
      >
        {{ $t('chat.toolScheduleItems', { count: itemCount }) }}
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const isList = computed(() => props.block.toolName === 'list_schedule')

const label = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  if (!input) return ''

  const name = input.name as string | undefined
  const id = input.id as string | undefined
  const pattern = input.pattern as string | undefined

  switch (props.block.toolName) {
    case 'create_schedule': {
      const parts = [name, pattern].filter(Boolean)
      return parts.join('  ')
    }
    case 'update_schedule':
      return name ?? id ?? ''
    case 'get_schedule':
    case 'delete_schedule':
      return id ?? ''
    default:
      return ''
  }
})

const itemCount = computed(() => {
  if (!props.block.done || !props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc ?? result).items
  if (Array.isArray(items)) return items.length
  return null
})
</script>
