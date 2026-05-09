<template>
  <div class="space-y-1.5">
    <div
      v-if="hasChanges && shiki.loading.value"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
    </div>
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-else-if="hasChanges"
      class="shiki-diff-container overflow-x-auto overflow-y-auto max-h-96 text-xs rounded-sm [&_pre]:bg-transparent! [&_pre]:p-2 [&_pre]:m-0 [&_code]:text-xs"
      v-html="shiki.html.value"
    />
    <!-- eslint-enable vue/no-v-html -->
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noChanges') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { LoaderCircle } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import { extractFilename, useShikiHighlighter } from '@/composables/useShikiHighlighter'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()
const shiki = useShikiHighlighter()

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

const hasChanges = computed(() => Boolean(oldText.value || newText.value))

onMounted(() => {
  if (hasChanges.value) {
    void shiki.highlightDiff(oldText.value, newText.value, extractFilename(filePath.value))
  }
})
</script>

<style>
.shiki-diff-container .diff-block pre {
  margin: 0 !important;
  padding: 0.5rem 0.75rem !important;
  background: transparent !important;
}
.shiki-diff-container .diff-remove {
  background-color: var(--diff-remove);
  border-left: 3px solid var(--diff-remove-border);
}
.shiki-diff-container .diff-add {
  background-color: var(--diff-add);
  border-left: 3px solid var(--diff-add-border);
}
</style>
