<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Handle, Position, type NodeProps } from '@vue-flow/core'
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  Clock3,
  Database,
  FileOutput,
  GitMerge,
  LoaderCircle,
  PlayCircle,
  ScanSearch,
  Search,
  ShieldCheck,
  Sparkles,
  RefreshCw,
  Wrench,
  type LucideIcon,
} from 'lucide-vue-next'
import { compactTaskTitle } from '../model'
import type { TaskFlowNodeData, TaskNodeKind } from '../composables/use-dag-graph'

const props = defineProps<NodeProps<TaskFlowNodeData>>()

const { t } = useI18n()

const task = computed(() => props.data.task)
const taskID = computed(() => String(task.value.id ?? ''))
const status = computed(() => String(task.value.status ?? ''))
const goal = computed(() => String(task.value.goal ?? ''))

const title = computed(() => compactTaskTitle(goal.value, taskID.value))

const isSpinning = computed(() => ['running', 'dispatching', 'verifying'].includes(status.value))

interface KindStyle {
  label: string
  icon: LucideIcon
  color: string
}

function kindStyle(kind: TaskNodeKind): KindStyle {
  switch (kind) {
    case 'trigger':
      return { label: t('orchestration.nodeKindTrigger'), icon: PlayCircle, color: 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20' }
    case 'planner':
      return { label: t('orchestration.nodeKindPlanner'), icon: Sparkles, color: 'border-border bg-muted/45 text-foreground' }
    case 'search':
      return { label: t('orchestration.nodeKindSearch'), icon: Search, color: 'text-sky-600 bg-sky-500/10 border-sky-500/20' }
    case 'memory':
      return { label: t('orchestration.nodeKindMemory'), icon: Database, color: 'text-blue-600 bg-blue-500/10 border-blue-500/20' }
    case 'merge':
      return { label: t('orchestration.nodeKindMerge'), icon: GitMerge, color: 'text-teal-600 bg-teal-500/10 border-teal-500/20' }
    case 'verify':
      return { label: t('orchestration.nodeKindValidation'), icon: ShieldCheck, color: 'text-indigo-600 bg-indigo-500/10 border-indigo-500/20' }
    case 'output':
      return { label: t('orchestration.nodeKindOutput'), icon: FileOutput, color: 'text-orange-600 bg-orange-500/10 border-orange-500/20' }
    case 'tool':
      return { label: t('orchestration.nodeKindTool'), icon: Wrench, color: 'text-amber-600 bg-amber-500/10 border-amber-500/20' }
    default:
      return { label: t('orchestration.nodeKindLlm'), icon: Bot, color: 'border-border bg-background text-foreground' }
  }
}

interface StatusStyle {
  label: string
  icon: LucideIcon
  dot: string
  glyph: string
}

function statusStyle(value: string): StatusStyle {
  switch (value) {
    case 'completed':
      return { label: t('orchestration.statusSuccess'), icon: CheckCircle2, dot: 'bg-emerald-500', glyph: 'text-emerald-500' }
    case 'running':
      return { label: t('orchestration.statusRunning'), icon: LoaderCircle, dot: 'bg-sky-500', glyph: 'text-sky-500' }
    case 'dispatching':
      return { label: t('orchestration.statusDispatching'), icon: LoaderCircle, dot: 'bg-sky-500', glyph: 'text-sky-500' }
    case 'verifying':
      return { label: t('orchestration.statusVerifying'), icon: LoaderCircle, dot: 'bg-sky-500', glyph: 'text-sky-500' }
    case 'waiting_human':
      return { label: t('orchestration.statusWaitingHuman'), icon: ScanSearch, dot: 'bg-amber-500', glyph: 'text-amber-500' }
    case 'failed':
      return { label: t('orchestration.statusFailed'), icon: AlertCircle, dot: 'bg-rose-500', glyph: 'text-rose-500' }
    case 'blocked':
      return { label: t('orchestration.statusBlocked'), icon: AlertCircle, dot: 'bg-rose-500', glyph: 'text-rose-500' }
    case 'cancelled':
      return { label: t('orchestration.statusCancelled'), icon: AlertCircle, dot: 'bg-rose-500', glyph: 'text-rose-500' }
    case 'active':
      return { label: t('orchestration.statusActive'), icon: LoaderCircle, dot: 'bg-sky-500', glyph: 'text-sky-500' }
    case 'idle':
      return { label: t('orchestration.statusIdle'), icon: Clock3, dot: 'bg-muted-foreground', glyph: 'text-muted-foreground' }
    default:
      return { label: value ? value.replaceAll('_', ' ') : t('orchestration.statusPending'), icon: Clock3, dot: 'bg-muted-foreground', glyph: 'text-muted-foreground' }
  }
}

const kind = computed(() => kindStyle(props.data.kind))
const statusInfo = computed(() => statusStyle(status.value))
const showLevelLabel = computed(() => `L${props.data.level}`)
const isDimmed = computed(() => props.data.hasSelection && !props.data.isRelated)

function retryTask() {
  if (!props.data.canRetry || props.data.isRetrying || !taskID.value) return
  props.data.onRetryTask?.(taskID.value)
}
</script>

<template>
  <div
    class="memoh-task-node group relative w-[208px] rounded-lg border bg-card px-2.5 py-2 text-left shadow-[0_0.7px_0.8px_hsl(var(--foreground)/0.05),0_2.2px_2.8px_-0.5px_hsl(var(--foreground)/0.06),0_6px_10px_-1px_hsl(var(--foreground)/0.07),0_16px_28px_-2px_hsl(var(--foreground)/0.09)] transition-all duration-150 hover:-translate-y-0.5 hover:shadow-[0_0.9px_1px_hsl(var(--foreground)/0.06),0_3px_4px_-0.5px_hsl(var(--foreground)/0.07),0_9px_14px_-1px_hsl(var(--foreground)/0.09),0_24px_40px_-2.5px_hsl(var(--foreground)/0.11)]"
    :class="[
      data.isSelected
        ? 'border-foreground/25 ring-2 ring-foreground/10 shadow-[0_0.8px_1px_hsl(var(--foreground)/0.05),0_3px_5px_-0.5px_hsl(var(--foreground)/0.06),0_10px_18px_-1px_hsl(var(--foreground)/0.08),0_24px_44px_-3px_hsl(var(--foreground)/0.10)]'
        : 'border-border/70',
      isDimmed ? 'opacity-45' : '',
    ]"
  >
    <button
      v-if="data.canRetry"
      type="button"
      class="nodrag nopan absolute right-1.5 top-1.5 flex size-6 items-center justify-center rounded-md text-muted-foreground opacity-0 transition hover:bg-muted hover:text-foreground group-hover:opacity-100"
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

    <Handle
      type="target"
      :position="Position.Left"
      :connectable="false"
      class="memoh-task-handle"
    />

    <div class="flex items-start gap-2">
      <span
        class="flex size-6 shrink-0 items-center justify-center rounded-md border"
        :class="kind.color"
      >
        <component
          :is="kind.icon"
          class="size-3.5"
        />
      </span>
      <div class="min-w-0 flex-1">
        <p class="line-clamp-2 text-xs font-semibold leading-snug">
          {{ title }}
        </p>
        <p class="mt-0.5 truncate text-[11px] text-muted-foreground">
          {{ kind.label }}
        </p>
      </div>
    </div>

    <div class="mt-2 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
      <span class="inline-flex items-center gap-1.5">
        <span
          class="size-1.5 rounded-full"
          :class="statusInfo.dot"
        />
        <component
          :is="statusInfo.icon"
          class="size-3 shrink-0"
          :class="[statusInfo.glyph, isSpinning ? 'animate-spin' : '']"
        />
        {{ statusInfo.label }}
      </span>
      <span class="tabular-nums text-[10px]">{{ showLevelLabel }}</span>
    </div>

    <Handle
      type="source"
      :position="Position.Right"
      :connectable="false"
      class="memoh-task-handle"
    />
  </div>
</template>
