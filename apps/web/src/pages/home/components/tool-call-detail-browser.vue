<template>
  <div class="space-y-1.5">
    <div
      v-if="targetRef || selector || url"
      class="flex flex-col gap-0.5 text-xs"
    >
      <div
        v-if="url"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.url') }}
        </span>
        <span
          class="font-mono truncate"
          :title="url"
        >{{ url }}</span>
      </div>
      <div
        v-if="targetRef"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.ref') }}
        </span>
        <span class="font-mono">{{ targetRef }}</span>
      </div>
      <div
        v-if="selector"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.selector') }}
        </span>
        <span
          class="font-mono truncate"
          :title="selector"
        >{{ selector }}</span>
      </div>
    </div>

    <div
      v-if="title"
      class="text-xs font-medium text-foreground"
    >
      {{ title }}
    </div>

    <pre
      v-if="textPreview"
      class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ textPreview }}</pre>

    <div
      v-if="screenshotPath"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <span class="text-[10px] uppercase tracking-wide text-muted-foreground/70 shrink-0">
        {{ t('chat.tools.detail.screenshotPath') }}
      </span>
      <span
        class="font-mono truncate"
        :title="screenshotPath"
      >{{ screenshotPath }}</span>
    </div>

    <p
      v-if="!targetRef && !selector && !url && !title && !textPreview && !screenshotPath"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noData') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

const input = computed(() => asObject(props.block.input))

function resolveResult(): Record<string, unknown> {
  const result = asObject(props.block.result)
  const sc = asObject(result.structuredContent)
  return Object.keys(sc).length > 0 ? sc : result
}

const url = computed(() => {
  const fromInput = input.value.url
  if (typeof fromInput === 'string' && fromInput) return fromInput
  const r = resolveResult()
  return typeof r.url === 'string' ? r.url : ''
})

const targetRef = computed(() => {
  const v = input.value.ref
  return typeof v === 'string' ? v : ''
})

const selector = computed(() => {
  const v = input.value.selector
  return typeof v === 'string' ? v : ''
})

const title = computed(() => {
  const r = resolveResult()
  return typeof r.title === 'string' ? r.title : ''
})

const screenshotPath = computed(() => {
  const r = resolveResult()
  return typeof r.path === 'string' ? r.path : ''
})

function extractTextContent(result: Record<string, unknown>): string {
  const content = result.content
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .filter(c => c && typeof c === 'object' && (c as Record<string, unknown>).type === 'text')
      .map(c => String((c as Record<string, unknown>).text ?? ''))
      .filter(Boolean)
      .join('\n')
  }
  return ''
}

const textPreview = computed(() => {
  const r = resolveResult()
  const candidate
    = (typeof r.snapshot === 'string' && r.snapshot)
      || (Array.isArray(r.snapshot) ? (r.snapshot as unknown[]).join('\n') : '')
      || (typeof r.text === 'string' ? r.text : '')
      || (typeof r.html === 'string' ? r.html : '')
      || extractTextContent(r)
  const text = typeof candidate === 'string' ? candidate : ''
  if (!text) return ''
  return text.length > 1200 ? `${text.slice(0, 1200)}…` : text
})
</script>
