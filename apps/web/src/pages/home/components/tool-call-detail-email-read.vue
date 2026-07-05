<template>
  <div class="space-y-1.5">
    <div
      v-if="subject"
      class="text-xs font-medium text-foreground"
    >
      {{ subject }}
    </div>
    <div
      v-if="from"
      class="text-caption text-muted-foreground"
    >
      {{ from }}
    </div>
    <PreviewBox v-if="body">
      {{ body }}
    </PreviewBox>
    <EmptyRow v-if="!subject && !from && !body">
      {{ t('chat.tools.detail.noEmail') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'
import PreviewBox from './tool-detail/preview-box.vue'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const subject = computed(() => {
  const r = resolveResult()
  return (r?.subject as string) ?? ''
})

const from = computed(() => {
  const r = resolveResult()
  return (r?.from as string) ?? ''
})

const body = computed(() => {
  const r = resolveResult()
  return (r?.body as string) ?? ''
})
</script>
