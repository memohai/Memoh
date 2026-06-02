<template>
  <div class="space-y-1.5">
    <div
      v-if="content && shiki.loading.value"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
    </div>
    <!-- eslint-disable vue/no-v-html -->
    <div
      v-else-if="content"
      class="shiki-container overflow-x-auto overflow-y-auto max-h-96 text-xs rounded-sm bg-muted/30 [&_pre]:bg-transparent! [&_pre]:p-2 [&_pre]:m-0 [&_code]:text-xs"
      v-html="shiki.html.value"
    />
    <!-- eslint-enable vue/no-v-html -->
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noContent') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
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

const content = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.content as string) ?? ''
})

// Re-highlight whenever content arrives. Input now streams in after the tool
// block first renders (tool_call_input_start), so an onMounted-only highlight
// would miss the content that lands later.
watch(
  [content, filePath],
  ([text, path]) => {
    if (text) {
      void shiki.highlight(text, extractFilename(path))
    }
  },
  { immediate: true },
)
</script>
