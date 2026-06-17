<template>
  <div class="space-y-1.5">
    <div
      v-if="provider"
      class="text-caption uppercase tracking-wide text-muted-foreground/70"
    >
      {{ provider }}
    </div>
    <div
      v-if="format"
      class="text-caption uppercase tracking-wide text-muted-foreground/70"
    >
      {{ format }}
    </div>
    <div
      v-if="title"
      class="text-xs font-medium text-foreground"
    >
      {{ title }}
    </div>
    <div
      v-if="excerpt"
      class="text-caption text-muted-foreground italic"
    >
      {{ excerpt }}
    </div>
    <pre
      v-if="contentPreview"
      class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ contentPreview }}</pre>
    <p
      v-if="!format && !title && !excerpt && !contentPreview"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noPreview') }}
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

const format = computed(() => {
  const r = resolveResult()
  return (r?.format as string) ?? ''
})

const provider = computed(() => {
  const r = resolveResult()
  return (r?.providerName as string) ?? (r?.provider as string) ?? ''
})

const title = computed(() => {
  const r = resolveResult()
  return (r?.title as string) ?? ''
})

const excerpt = computed(() => {
  const r = resolveResult()
  return (r?.excerpt as string) ?? ''
})

const contentPreview = computed(() => {
  const r = resolveResult()
  if (!r) return ''
  const content = (r.content as string) ?? (r.textContent as string) ?? ''
  if (typeof content !== 'string') return ''
  return content.length > 800 ? `${content.slice(0, 800)}…` : content
})
</script>
