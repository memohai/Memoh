<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { NodeProps } from '@vue-flow/core'
import { LoaderCircle, RefreshCw } from 'lucide-vue-next'
import { formatRelativeDate } from '../model'
import { useOrchestrationMeta } from '../composables/use-orchestration-meta'
import type { FlowSpanNodeData } from '../composables/use-flow-graph'

const props = defineProps<NodeProps<FlowSpanNodeData>>()

const { t, locale } = useI18n()
const { statusMeta, flowKindMeta } = useOrchestrationMeta()

const span = computed(() => props.data.span)
const status = computed(() => props.data.taskStatus || String(span.value.status ?? ''))
const kind = computed(() => flowKindMeta(span.value.kind))
const statusInfo = computed(() => statusMeta(status.value))
const title = computed(() => String(span.value.title ?? '').trim() || kind.value.label)
const startedLabel = computed(() => formatRelativeDate(span.value.started_at || span.value.finished_at, locale.value))
const isSpinning = computed(() => ['running', 'dispatching', 'verifying', 'active'].includes(status.value))
const kindLabel = computed(() => {
  const label = kind.value.label.trim()
  const titleText = title.value.trim()
  if (!label || label.toLowerCase() === titleText.toLowerCase()) return ''
  if (['planning', 'replanning', 'attempt', 'attempt_finalize', 'verification', 'checkpoint', 'checkpoint_resume'].includes(String(span.value.kind ?? ''))) return ''
  return label
})

const seqLabel = computed(() => {
  const start = Number(span.value.start_seq ?? 0)
  const end = Number(span.value.end_seq ?? 0)
  if (start > 0 && end > 0 && start !== end) return `#${start}-${end}`
  if (start > 0) return `#${start}`
  if (end > 0) return `#${end}`
  return ''
})

const indexLabel = computed(() => `${props.data.index}/${props.data.total}`)
const widthStyle = computed(() => ({ width: `${props.data.width}px` }))

const toolSummary = computed(() => {
  const names = Array.isArray(span.value.tool_names) ? span.value.tool_names.filter(Boolean) : []
  if (names.length === 0) {
    const count = Number(span.value.action_count ?? 0)
    return count > 0 ? t('orchestration.flowActionCount', { count }) : ''
  }
  const visible = names.slice(0, 3).join(', ')
  const suffix = names.length > 3 ? ` +${names.length - 3}` : ''
  return `${visible}${suffix}`
})

const isDimmed = computed(() => props.data.hasSelection && !props.data.isRelated)

function retryTask() {
  const taskID = String(span.value.task_id ?? '').trim()
  if (!props.data.canRetry || props.data.isRetrying || !taskID) return
  props.data.onRetryTask?.(taskID)
}
</script>

<template>
  <div
    class="memoh-flow-span-node group relative rounded-lg border bg-card px-3 py-2 text-left shadow-[0_0.7px_0.8px_hsl(var(--foreground)/0.05),0_2.2px_2.8px_-0.5px_hsl(var(--foreground)/0.06),0_6px_10px_-1px_hsl(var(--foreground)/0.07),0_16px_28px_-2px_hsl(var(--foreground)/0.09)] transition-all duration-150 hover:-translate-y-0.5 hover:shadow-[0_0.9px_1px_hsl(var(--foreground)/0.06),0_3px_4px_-0.5px_hsl(var(--foreground)/0.07),0_9px_14px_-1px_hsl(var(--foreground)/0.09),0_24px_40px_-2.5px_hsl(var(--foreground)/0.11)]"
    :style="widthStyle"
    :class="[
      data.isSelected
        ? 'border-foreground/25 ring-2 ring-foreground/10 shadow-[0_0.8px_1px_hsl(var(--foreground)/0.05),0_3px_5px_-0.5px_hsl(var(--foreground)/0.06),0_10px_18px_-1px_hsl(var(--foreground)/0.08),0_24px_44px_-3px_hsl(var(--foreground)/0.10)]'
        : 'border-border/70',
      isDimmed ? 'opacity-45' : '',
    ]"
  >
    <div class="flex items-start gap-2">
      <span
        class="flex size-6 shrink-0 items-center justify-center rounded-md border"
        :class="kind.color"
      >
        <component
          :is="kind.icon"
          class="size-3"
        />
      </span>
      <div class="min-w-0 flex-1">
        <p class="truncate text-xs font-semibold leading-snug">
          {{ title }}
        </p>
        <p class="mt-0.5 truncate text-[11px] text-muted-foreground">
          <template v-if="kindLabel">
            {{ kindLabel }}
          </template>
          <template v-if="startedLabel && startedLabel !== '--'">
            <template v-if="kindLabel">
              ·
            </template>{{ startedLabel }}
          </template>
        </p>
      </div>
      <span
        v-if="seqLabel"
        class="shrink-0 self-start rounded-sm bg-muted/50 px-1.5 py-0.5 text-[10px] tabular-nums text-muted-foreground"
        :title="`step ${indexLabel}`"
      >
        {{ seqLabel }}
      </span>
      <button
        v-if="data.canRetry"
        type="button"
        class="nodrag nopan -mr-1 -mt-1 flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
        :disabled="data.isRetrying"
        :title="t('orchestration.retryTask')"
        @pointerdown.stop
        @mousedown.stop
        @click.stop="retryTask"
      >
        <LoaderCircle
          v-if="data.isRetrying"
          class="size-3.5 animate-spin"
        />
        <RefreshCw
          v-else
          class="size-3.5"
        />
      </button>
    </div>

    <div class="mt-1.5 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
      <span class="inline-flex min-w-0 items-center gap-1.5">
        <span
          class="size-1.5 shrink-0 rounded-full"
          :class="statusInfo.dot"
        />
        <component
          :is="statusInfo.icon"
          class="size-3 shrink-0"
          :class="[isSpinning ? 'animate-spin' : '']"
        />
        <span class="truncate">{{ statusInfo.label }}</span>
      </span>
      <span class="shrink-0 tabular-nums text-[10px]">
        {{ data.durationLabel || indexLabel }}
      </span>
    </div>

    <div class="mt-1.5 flex items-center gap-1.5 text-[10px]">
      <p
        v-if="data.taskTitle"
        class="min-w-0 truncate rounded-md border border-border bg-muted/35 px-2 py-0.5 text-foreground"
        :title="data.taskTitle"
      >
        {{ data.taskTitle }}
      </p>
      <p
        v-if="toolSummary"
        class="min-w-0 truncate rounded-md bg-muted/40 px-2 py-0.5 text-muted-foreground"
        :title="toolSummary"
      >
        {{ toolSummary }}
      </p>
    </div>
  </div>
</template>
