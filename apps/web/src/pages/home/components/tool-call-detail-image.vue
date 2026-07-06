<template>
  <div class="space-y-1.5">
    <img
      v-if="src"
      :src="src"
      :alt="prompt"
      class="rounded-md border border-border max-w-xs max-h-64 object-contain"
    >
    <p
      v-if="path"
      class="text-xs text-muted-foreground font-mono truncate"
      :title="path"
    >
      {{ path }}
    </p>
    <EmptyRow v-if="!src && !path">
      {{ t('chat.tools.detail.noImage') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const prompt = computed(() => {
  const input = props.block.input as Record<string, unknown> | undefined
  return (input?.prompt as string) ?? ''
})

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  return props.block.result as Record<string, unknown>
}

const path = computed(() => {
  const r = resolveResult()
  if (!r) return ''
  const sc = r.structuredContent as Record<string, unknown> | undefined
  return ((sc ?? r).path as string) ?? ''
})

const src = computed(() => {
  const r = resolveResult()
  if (!r) return ''
  const content = r.content as Array<Record<string, unknown>> | undefined
  if (!Array.isArray(content)) return ''
  const img = content.find(c => c.type === 'image')
  if (!img) return ''
  const data = img.data as string | undefined
  const mime = (img.mimeType as string) || 'image/png'
  return data ? `data:${mime};base64,${data}` : ''
})
</script>
