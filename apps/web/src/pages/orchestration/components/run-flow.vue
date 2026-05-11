<script setup lang="ts">
import { computed, markRaw, nextTick, onActivated, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueFlow, useVueFlow, type NodeMouseEvent } from '@vue-flow/core'
import { Button, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@memohai/ui'
import { Maximize, Minus, Plus, ScanSearch, Workflow } from 'lucide-vue-next'
import FlowSpanNode from './flow-span-node.vue'
import FlowLaneNode from './flow-lane-node.vue'
import FlowGapNode from './flow-gap-node.vue'
import { useFlowGraph } from '../composables/use-flow-graph'
import type { RunInspectorPayload } from '../model'

import '@vue-flow/core/dist/style.css'

const props = defineProps<{
  inspector: RunInspectorPayload | null | undefined
  selectedTaskId: string
  inspectorOpen: boolean
  retryableTaskIds?: string[]
  retryingTaskId?: string
}>()

const emit = defineEmits<{
  'select-task': [string]
  'open-inspector': []
  'retry-task': [string]
}>()

const { t } = useI18n()

const { nodes, edges, summary } = useFlowGraph(
  computed(() => props.inspector),
  computed(() => props.selectedTaskId),
)

const retryableTaskIdSet = computed(() => new Set(props.retryableTaskIds ?? []))
const displayNodes = computed(() =>
  nodes.value.map((node) => {
    if (node.type !== 'flowSpan') return node
    const taskID = String(node.data.span?.task_id ?? '').trim()
    const canRetry = !!taskID && retryableTaskIdSet.value.has(taskID)
    return {
      ...node,
      data: {
        ...node.data,
        canRetry,
        isRetrying: canRetry && props.retryingTaskId === taskID,
        onRetryTask: (id: string) => emit('retry-task', id),
      },
    }
  }),
)

const {
  fitView,
  zoomIn,
  zoomOut,
  viewport,
  getNodes,
  setNodes,
  setEdges,
} = useVueFlow()

const zoomPercent = computed(() => `${Math.round((viewport.value?.zoom ?? 1) * 100)}%`)

const nodeTypes = {
  flowSpan: markRaw(FlowSpanNode),
  flowLane: markRaw(FlowLaneNode),
  flowGap: markRaw(FlowGapNode),
}

const fitViewPadding = 0.22

const isEmpty = computed(() => nodes.value.length === 0)

const summaryItems = computed(() => [
  { key: 'steps', label: t('orchestration.flowSteps'), value: summary.value.steps },
  { key: 'planning', label: t('orchestration.flowPlanning'), value: summary.value.planning },
  { key: 'attempts', label: t('orchestration.flowAttempt'), value: summary.value.attempts },
  { key: 'actions', label: t('orchestration.flowActions'), value: summary.value.actions },
])

async function refit() {
  await nextTick()
  if (getNodes.value.length === 0) return
  await fitView({ padding: fitViewPadding, duration: 0 })
}

function topologySignature(): string {
  return [
    props.inspector?.run.id ?? '',
    displayNodes.value.map((n) => n.id).join('|'),
  ].join('#')
}

let lastTopology = ''

watch(
  displayNodes,
  (next) => {
    const sig = topologySignature()
    if (sig !== lastTopology) {
      lastTopology = sig
      setNodes(next)
      void refit()
      return
    }
    setNodes(next)
  },
  { immediate: true, deep: false },
)

watch(
  edges,
  (next) => {
    setEdges(next)
  },
  { immediate: true, deep: false },
)

onMounted(() => {
  void refit()
})

onActivated(() => {
  void refit()
})

function onNodeClick(event: NodeMouseEvent) {
  if (event.node.type !== 'flowSpan') return
  const taskID = String(event.node.data?.span?.task_id ?? '').trim()
  if (!taskID) return
  emit('select-task', taskID)
}
</script>

<template>
  <div class="memoh-run-flow relative size-full bg-background">
    <div class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,hsl(var(--border)/0.8)_1px,transparent_0)] bg-[length:24px_24px] opacity-30" />

    <VueFlow
      :node-types="nodeTypes"
      :min-zoom="0.3"
      :max-zoom="2"
      :nodes-draggable="false"
      :nodes-connectable="false"
      :edges-updatable="false"
      :elevate-edges-on-select="false"
      :only-render-visible-elements="true"
      :select-nodes-on-drag="false"
      :prevent-scrolling="true"
      pan-on-drag
      zoom-on-scroll
      zoom-on-pinch
      class="memoh-vue-flow"
      @node-click="onNodeClick"
    />

    <div
      v-if="!isEmpty"
      class="absolute left-4 top-4 z-30 flex w-fit items-stretch overflow-hidden rounded-md border border-border bg-background shadow-sm"
    >
      <div
        v-for="item in summaryItems"
        :key="item.key"
        class="flex min-w-[64px] flex-col items-center justify-center border-r border-border px-3 py-1.5 text-center last:border-r-0"
      >
        <p class="text-[13px] font-semibold leading-none tabular-nums text-foreground">
          {{ item.value }}
        </p>
        <p class="mt-1 text-[10px] uppercase leading-none tracking-wide text-muted-foreground">
          {{ item.label }}
        </p>
      </div>
    </div>

    <TooltipProvider :delay-duration="200">
      <div class="absolute right-4 top-4 z-30 flex w-fit items-center rounded-md border border-border bg-background shadow-sm">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="ghost"
              size="icon"
              class="size-8 rounded-r-none"
              @click="refit()"
            >
              <Maximize class="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('orchestration.fitView') }}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="ghost"
              size="icon"
              class="size-8 rounded-none border-l border-border"
              @click="() => zoomOut()"
            >
              <Minus class="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('orchestration.zoomOut') }}</TooltipContent>
        </Tooltip>
        <span class="h-8 select-none border-x border-border px-2 text-[11px] leading-8 tabular-nums text-muted-foreground">
          {{ zoomPercent }}
        </span>
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="ghost"
              size="icon"
              class="size-8 rounded-l-none"
              @click="() => zoomIn()"
            >
              <Plus class="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('orchestration.zoomIn') }}</TooltipContent>
        </Tooltip>
      </div>
    </TooltipProvider>

    <Button
      v-if="!inspectorOpen"
      variant="outline"
      size="icon"
      class="absolute right-4 top-16 z-30 size-8 bg-background shadow-sm"
      :title="t('orchestration.nodeInspector')"
      @click="emit('open-inspector')"
    >
      <ScanSearch class="size-3.5" />
    </Button>

    <div
      v-if="isEmpty"
      class="pointer-events-none absolute inset-0 z-20 flex items-center justify-center px-6"
    >
      <div class="pointer-events-auto flex max-w-md flex-col items-center gap-3 rounded-xl border border-dashed border-border/70 bg-background/80 px-8 py-10 text-center shadow-sm backdrop-blur">
        <span class="flex size-10 items-center justify-center rounded-full border border-border/60 bg-muted/40 text-muted-foreground">
          <Workflow class="size-5" />
        </span>
        <p class="text-sm font-semibold text-foreground">
          {{ t('orchestration.noFlowSpans') }}
        </p>
        <p class="max-w-xs text-xs text-muted-foreground">
          {{ t('orchestration.runFlowDescription') }}
        </p>
      </div>
    </div>
  </div>
</template>

<style>
.memoh-run-flow .memoh-vue-flow {
  background: transparent;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__edge-path {
  stroke: #9ca3af;
  transition: stroke 150ms ease, stroke-width 150ms ease;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__edge.memoh-edge-active .vue-flow__edge-path {
  stroke: #6366f1;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__node-flowSpan {
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
  cursor: pointer;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__node-flowSpan.selected {
  box-shadow: none;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__node-flowLane {
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
  pointer-events: none;
  cursor: default;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__node-flowGap {
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
  pointer-events: none;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__handle {
  width: 6px !important;
  height: 6px !important;
  min-width: 0 !important;
  min-height: 0 !important;
  border: 0 !important;
  background: hsl(var(--muted-foreground) / 0.4) !important;
}

.memoh-run-flow .memoh-vue-flow .vue-flow__handle:hover {
  background: hsl(var(--foreground) / 0.7) !important;
}

.memoh-flow-span-node:hover .vue-flow__handle,
.memoh-run-flow .memoh-vue-flow .vue-flow__node-flowSpan:hover .vue-flow__handle {
  background: hsl(var(--foreground) / 0.55) !important;
}
</style>
