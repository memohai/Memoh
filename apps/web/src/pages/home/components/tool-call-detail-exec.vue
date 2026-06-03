<template>
  <div class="space-y-1.5">
    <div
      v-if="backgroundMeta.length"
      class="flex flex-wrap gap-x-2 gap-y-0.5 text-[11px] text-foreground"
    >
      <span
        v-for="item in backgroundMeta"
        :key="item"
        class="font-mono"
      >{{ item }}</span>
    </div>
    <pre
      v-if="progressText"
      class="text-xs text-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ progressText }}</pre>
    <pre
      v-if="backgroundOutput"
      class="text-xs text-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ backgroundOutput }}</pre>
    <pre
      v-if="stdout"
      class="text-xs text-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ stdout }}</pre>
    <pre
      v-if="stderr"
      class="text-xs text-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ stderr }}</pre>
    <pre
      v-if="errorText"
      class="text-xs text-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
    >{{ errorText }}</pre>
    <p
      v-if="!progressText && !backgroundOutput && !stdout && !stderr && !errorText"
      class="text-xs text-muted-foreground italic"
    >
      {{ isBackgroundActive ? t('chat.tools.detail.waitingOutput') : t('chat.tools.detail.noOutput') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const backgroundTask = computed(() => props.block.backgroundTask)

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  return sc ?? result
}

const stdout = computed(() => {
  const r = resolveResult()
  return (r?.stdout as string) ?? ''
})

const stderr = computed(() => {
  const r = resolveResult()
  return (r?.stderr as string) ?? ''
})

const backgroundOutput = computed(() =>
  backgroundTask.value?.outputTail
  || backgroundTask.value?.chunk
  || '',
)

const isBackgroundActive = computed(() => {
  const status = (backgroundTask.value?.status ?? '').trim().toLowerCase()
  return status === 'running' || status === 'stalled'
})

const backgroundMeta = computed(() => {
  const task = backgroundTask.value
  if (!task?.taskId) return []
  const items = [task.taskId]
  if (task.status) items.push(task.status)
  if (task.duration) items.push(task.duration)
  if (task.outputFile) items.push(task.outputFile)
  return items
})

const errorText = computed(() => {
  if (!props.block.result) return ''
  const result = props.block.result as Record<string, unknown>
  if (result.isError !== true) return ''
  const content = result.content as Array<Record<string, unknown>> | undefined
  if (!Array.isArray(content)) return ''
  return content
    .filter(c => c.type === 'text')
    .map(c => c.text as string)
    .join('\n')
})

const progressText = computed(() =>
  (props.block.progress ?? [])
    .map(item => formatProgress(item))
    .filter(Boolean)
    .join('\n'),
)

function formatProgress(val: unknown): string {
  if (typeof val === 'string') return val
  try {
    return JSON.stringify(val, null, 2)
  }
  catch {
    return String(val)
  }
}
</script>
