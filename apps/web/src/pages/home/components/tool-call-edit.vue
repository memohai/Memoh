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
      <SquarePen class="size-3 text-muted-foreground" />
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

    <Collapsible
      v-if="hasChanges"
      v-model:open="diffOpen"
    >
      <CollapsibleTrigger class="flex items-center gap-1.5 px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer w-full">
        <ChevronRight
          class="size-2.5 transition-transform"
          :class="{ 'rotate-90': diffOpen }"
        />
        {{ $t('chat.toolEditChanges') }}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div
          v-if="shiki.loading.value"
          class="px-3 pb-2 text-xs text-muted-foreground"
        >
          <LoaderCircle class="size-3 animate-spin" />
        </div>
        <!-- eslint-disable vue/no-v-html -->
        <div
          v-else
          class="shiki-diff-container overflow-x-auto text-xs [&_pre]:bg-transparent! [&_pre]:p-3 [&_pre]:m-0 [&_code]:text-xs"
          v-html="shiki.html.value"
        />
        <!-- eslint-enable vue/no-v-html -->
      </CollapsibleContent>
    </Collapsible>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, watch } from 'vue'
import { Check, LoaderCircle, SquarePen, ChevronRight } from 'lucide-vue-next'
import { Badge, Collapsible, CollapsibleTrigger, CollapsibleContent } from '@memohai/ui'
import type { ToolCallBlock } from '@/store/chat-list'
import { openInFileManagerKey } from '../composables/useFileManagerProvider'
import { useShikiHighlighter, extractFilename } from '@/composables/useShikiHighlighter'

const props = defineProps<{ block: ToolCallBlock }>()

const openInFileManager = inject(openInFileManagerKey, undefined)
const shiki = useShikiHighlighter()
const diffOpen = ref(false)

const filePath = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})

const oldText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.old_text as string) ?? ''
})

const newText = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.new_text as string) ?? ''
})

const hasChanges = computed(() => oldText.value || newText.value)

watch(diffOpen, (open) => {
  if (open && hasChanges.value && !shiki.html.value) {
    void shiki.highlightDiff(oldText.value, newText.value, extractFilename(filePath.value))
  }
})

function handleOpenFile() {
  if (filePath.value && openInFileManager) {
    openInFileManager(filePath.value, false)
  }
}
</script>

<style>
.shiki-diff-container .diff-block pre {
  margin: 0 !important;
  padding: 0.5rem 0.75rem !important;
  background: transparent !important;
}
.shiki-diff-container .diff-remove {
  background-color: oklch(0.55 0.12 25 / 0.12);
  border-left: 3px solid oklch(0.55 0.12 25 / 0.5);
}
.shiki-diff-container .diff-add {
  background-color: oklch(0.55 0.12 145 / 0.12);
  border-left: 3px solid oklch(0.55 0.12 145 / 0.5);
}
</style>
