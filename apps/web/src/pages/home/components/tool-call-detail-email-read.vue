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
    <pre
      v-if="body"
      class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-60 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ body }}</pre>
    <p
      v-if="!subject && !from && !body"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noEmail') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

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
