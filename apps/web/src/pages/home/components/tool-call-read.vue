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
      <FileText class="size-3 text-muted-foreground" />
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
import { Check, LoaderCircle, FileText } from 'lucide-vue-next'
import { Badge } from '@memohai/ui'
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
