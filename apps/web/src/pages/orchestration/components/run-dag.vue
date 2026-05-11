<script setup lang="ts">
import { computed, markRaw, nextTick, onActivated, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueFlow, useVueFlow, type NodeMouseEvent } from '@vue-flow/core'
import { Button, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@memohai/ui'
import { Maximize, Minus, Plus, ScanSearch } from 'lucide-vue-next'
import TaskFlowNode from './task-flow-node.vue'
import LaneNode from './lane-node.vue'
import { useDagGraph } from '../composables/use-dag-graph'
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

const { nodes, edges } = useDagGraph(
  computed(() => props.inspector),
  computed(() => props.selectedTaskId),
)

const retryableTaskIdSet = computed(() => new Set(props.retryableTaskIds ?? []))
const displayNodes = computed(() =>
  nodes.value.map((node) => {
    if (node.type !== 'taskFlow') return node
    const canRetry = retryableTaskIdSet.value.has(node.id)
    return {
      ...node,
      data: {
        ...node.data,
        canRetry,
        isRetrying: canRetry && props.retryingTaskId === node.id,
        onRetryTask: (taskID: string) => emit('retry-task', taskID),
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
  updateNodeData,
} = useVueFlow()

const zoomPercent = computed(() => `${Math.round((viewport.value?.zoom ?? 1) * 100)}%`)

const nodeTypes = {
  taskFlow: markRaw(TaskFlowNode),
  lane: markRaw(LaneNode),
}

const fitViewPadding = 0.18

async function refit() {
  await nextTick()
  if (getNodes.value.length === 0) return
  await fitView({ padding: fitViewPadding, duration: 0 })
}

function topologySignature(): string {
  return [
    props.inspector?.run.id ?? '',
    displayNodes.value.map((n) => `${n.id}:${n.type}:${Math.round(n.position.x)},${Math.round(n.position.y)}`).join('|'),
    edges.value.map((e) => `${e.id}:${e.source}->${e.target}`).join('|'),
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
    for (const node of next) {
      if (!node.data) continue
      updateNodeData(node.id, node.data)
    }
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
  if (event.node.type === 'lane') return
  emit('select-task', event.node.id)
}
</script>

<template>
  <div class="memoh-run-dag relative size-full bg-background">
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
  </div>
</template>

<style>
.memoh-vue-flow {
  background: transparent;
}

.memoh-vue-flow .vue-flow__edge-path {
  stroke: #9ca3af;
  transition: stroke 150ms ease, stroke-width 150ms ease;
}

.memoh-vue-flow .vue-flow__edge.memoh-edge-active .vue-flow__edge-path {
  stroke: #6366f1;
}

.memoh-vue-flow .vue-flow__edge.memoh-edge-structural .vue-flow__edge-path {
  stroke-dasharray: 5 4;
}

.memoh-vue-flow .vue-flow__node-taskFlow {
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
}

.memoh-vue-flow .vue-flow__node-taskFlow.selected {
  box-shadow: none;
}

.memoh-vue-flow .vue-flow__node-taskFlow {
  cursor: pointer;
}

.memoh-vue-flow .vue-flow__node-lane {
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
  pointer-events: none;
  cursor: default;
  overflow: visible;
}

.memoh-vue-flow .vue-flow__handle {
  width: 6px !important;
  height: 6px !important;
  min-width: 0 !important;
  min-height: 0 !important;
  border: 0 !important;
  background: hsl(var(--muted-foreground) / 0.4) !important;
}

.memoh-vue-flow .vue-flow__handle:hover {
  background: hsl(var(--foreground) / 0.7) !important;
}

.memoh-task-node:hover .vue-flow__handle,
.vue-flow__node-taskFlow:hover .vue-flow__handle {
  background: hsl(var(--foreground) / 0.55) !important;
}
</style>
