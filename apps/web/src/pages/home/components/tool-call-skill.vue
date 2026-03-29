<template>
  <div class="rounded-lg border bg-muted/30 text-xs overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <Check
        v-if="block.done"
        class="size-3 text-green-600 dark:text-green-400"
      />
      <LoaderCircle
        v-else
        class="size-3 animate-spin text-muted-foreground"
      />
      <Sparkles class="size-3 text-muted-foreground" />
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
import { Check, LoaderCircle, Sparkles } from 'lucide-vue-next'
import { Badge } from '@memohai/ui'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()

const skillName = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.skillName as string) ?? ''
})
</script>
