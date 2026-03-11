<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />

      <!-- send -->
      <template v-if="block.toolName === 'send'">
        <FontAwesomeIcon
          :icon="['fas', 'paper-plane']"
          class="size-3 text-muted-foreground"
        />
        <span
          v-if="platform || target"
          class="text-xs truncate text-foreground"
          :title="`${platform} → ${target}`"
        >
          <span
            v-if="platform"
            class="text-muted-foreground"
          >{{ platform }}</span>
          <span v-if="platform && target"> → </span>
          <span v-if="target">{{ target }}</span>
        </span>
        <span
          v-if="text"
          class="text-xs truncate text-muted-foreground"
          :title="text"
        >
          {{ text }}
        </span>
      </template>

      <!-- react -->
      <template v-else>
        <FontAwesomeIcon
          :icon="['fas', 'face-smile']"
          class="size-3 text-muted-foreground"
        />
        <span
          v-if="emoji"
          class="text-sm"
        >
          {{ emoji }}
        </span>
        <span
          v-if="block.done && action"
          class="text-xs text-muted-foreground"
        >
          {{ action }}
        </span>
      </template>

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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const platform = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.platform as string) ?? ''
})

const target = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.target as string) ?? ''
})

const text = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.text as string) ?? ''
})

const emoji = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.emoji as string) ?? ''
})

const action = computed(() => {
  if (!props.block.done || !props.block.result) return ''
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  return ((sc ?? result).action as string) ?? ''
})
</script>
