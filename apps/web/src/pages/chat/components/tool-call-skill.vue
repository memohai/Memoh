<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'wand-magic-sparkles']"
        class="size-3 text-muted-foreground"
      />
      <span
        v-if="skillName"
        class="text-xs truncate text-foreground"
      >
        {{ skillName }}
      </span>
      <span
        v-else
        class="font-mono font-medium text-xs text-muted-foreground"
      >
        use_skill
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Badge } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const skillName = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.skillName as string) ?? ''
})
</script>
