<template>
  <div class="rounded-lg border bg-muted/30 text-sm overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-2 bg-muted/50">
      <FontAwesomeIcon
        :icon="['fas', block.done ? 'check' : 'spinner']"
        class="size-3"
        :class="block.done ? 'text-green-600 dark:text-green-400' : 'animate-spin text-muted-foreground'"
      />
      <FontAwesomeIcon
        :icon="['fas', 'file-lines']"
        class="size-3 text-muted-foreground"
      />
      <button
        class="font-mono text-xs truncate hover:underline text-foreground cursor-pointer"
        :title="filePath"
        @click="handleOpenFile"
      >
        {{ filePath }}
      </button>
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
import { computed, inject } from 'vue'
import { Badge } from '@memoh/ui'
import type { ToolCallBlock } from '@/store/chat-list'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'

const props = defineProps<{ block: ToolCallBlock }>()

const openInFileManager = inject(openInFileManagerKey, undefined)

const filePath = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})

function handleOpenFile() {
  if (filePath.value && openInFileManager) {
    openInFileManager(filePath.value, false)
  }
}
</script>
