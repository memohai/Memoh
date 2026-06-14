<template>
  <div class="space-y-1.5">
    <p
      v-if="contentTruncated"
      class="rounded-sm border border-border bg-muted/30 px-2 py-1 text-xs text-muted-foreground"
    >
      {{ t('chat.tools.detail.contentTruncated', { bytes: contentBytes }) }}
    </p>
    <CodeBlock
      v-if="content"
      :code="content"
      :filename="filePath"
      class="overflow-x-auto overflow-y-auto max-h-96 text-xs leading-relaxed"
    />
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noContent') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import CodeBlock from './code-block.vue'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const filePath = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.path as string) ?? ''
})

const content = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.content as string) ?? ''
})

const contentTruncated = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return input?.content_truncated === true
})

const contentBytes = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  const bytes = input?.content_bytes
  return typeof bytes === 'number' && Number.isFinite(bytes) ? bytes : 0
})
</script>
