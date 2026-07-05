<template>
  <div class="space-y-1.5">
    <div
      v-if="targetRef || coordinates"
      class="flex flex-col gap-0.5 text-xs"
    >
      <div
        v-if="targetRef"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-caption uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.ref') }}
        </span>
        <span class="font-mono">{{ targetRef }}</span>
      </div>
      <div
        v-if="coordinates"
        class="flex items-center gap-1.5 text-muted-foreground"
      >
        <span class="text-caption uppercase tracking-wide text-muted-foreground/70 shrink-0">
          {{ t('chat.tools.detail.coordinates') }}
        </span>
        <span class="font-mono">{{ coordinates }}</span>
      </div>
    </div>

    <div
      v-if="refCount"
      class="text-caption text-muted-foreground"
    >
      {{ t('chat.tools.detail.refCount', { count: refCount }) }}
    </div>

    <PreviewBox v-if="snapshotText">
      {{ snapshotText }}
    </PreviewBox>

    <div
      v-if="screenshotPath"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <span class="text-caption uppercase tracking-wide text-muted-foreground/70 shrink-0">
        {{ t('chat.tools.detail.screenshotPath') }}
      </span>
      <span
        class="font-mono truncate"
        :title="screenshotPath"
      >{{ screenshotPath }}</span>
    </div>

    <EmptyRow v-if="!targetRef && !coordinates && !refCount && !snapshotText && !screenshotPath">
      {{ t('chat.tools.detail.noData') }}
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

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

const input = computed(() => asObject(props.block.input))

function resolveResult(): Record<string, unknown> {
  const result = asObject(props.block.result)
  const sc = asObject(result.structuredContent)
  return Object.keys(sc).length > 0 ? sc : result
}

const targetRef = computed(() => {
  const v = input.value.ref
  return typeof v === 'string' ? v : ''
})

const coordinates = computed(() => {
  const x = input.value.x
  const y = input.value.y
  if (typeof x === 'number' && typeof y === 'number') return `${x}, ${y}`
  return ''
})

const refCount = computed(() => {
  const r = resolveResult()
  const v = r.ref_count
  return typeof v === 'number' && v > 0 ? v : 0
})

const snapshotText = computed(() => {
  const r = resolveResult()
  const snapshot = r.snapshot
  let text = ''
  if (typeof snapshot === 'string') text = snapshot
  else if (Array.isArray(snapshot)) text = snapshot.map(line => String(line)).join('\n')
  if (!text) return ''
  return text.length > 1200 ? `${text.slice(0, 1200)}…` : text
})

const screenshotPath = computed(() => {
  const r = resolveResult()
  return typeof r.path === 'string' ? r.path : ''
})
</script>
