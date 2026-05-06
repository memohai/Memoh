<script setup lang="ts">
import { computed, markRaw } from 'vue'
import {
  MarkerType,
  Position,
  VueFlow,
  type Edge,
  type Node,
} from '@vue-flow/core'
import TaskNode from './task-node.vue'

import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

interface TaskGraphNodeData {
  title: string
  label: string
  worker: string
  status: string
  goal: string
  resultSummary: string
  attemptLabel: string
  isRoot: boolean
  checkpointLabel: string
}

const props = defineProps<{
  nodes: Node<TaskGraphNodeData>[]
  edges: Edge[]
  maxLevel: number
}>()

const selectedTaskId = defineModel<string>('selectedTaskId', { required: true })

const nodeTypes = {
  orchestrationTask: markRaw(TaskNode),
}

const graphNodes = computed(() =>
  props.nodes.map((node) => ({
    ...node,
    selected: node.id === selectedTaskId.value,
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
  })),
)

const graphEdges = computed(() =>
  props.edges.map((edge) => {
    const isSelected = edge.source === selectedTaskId.value || edge.target === selectedTaskId.value
    return {
      ...edge,
      type: 'smoothstep',
      animated: isSelected,
      markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
      style: {
        stroke: isSelected ? 'color-mix(in oklab, var(--color-foreground) 72%, transparent)' : 'color-mix(in oklab, var(--color-border) 90%, transparent)',
        strokeWidth: isSelected ? 1.5 : 1.1,
      },
    }
  }),
)

function onNodeClick(event: { node: Node<TaskGraphNodeData> }) {
  selectedTaskId.value = event.node.id
}
</script>

<template>
  <div class="relative h-full w-full min-h-[300px] overflow-hidden rounded-lg border border-border/60 bg-linear-to-br from-[#f9f5ee] to-background dark:from-[#141414] dark:to-background">
    <!-- Background Columns (L1, L2, etc.) -->
    <div
      class="pointer-events-none absolute inset-y-0 left-0 flex h-full min-w-max z-0"
      aria-hidden="true"
    >
      <div
        v-for="i in Math.max(1, props.maxLevel + 1)"
        :key="i"
        class="w-[250px] shrink-0 border-r border-border/40 last:border-r-0"
      >
        <div class="flex h-10 items-center justify-center border-b border-border/40">
          <span class="text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
            L{{ i }}
          </span>
        </div>
      </div>
    </div>

    <VueFlow
      :nodes="graphNodes"
      :edges="graphEdges"
      :node-types="nodeTypes"
      fit-view-on-init
      :min-zoom="0.4"
      :max-zoom="1.5"
      :nodes-draggable="false"
      :nodes-connectable="false"
      :elements-selectable="true"
      :select-nodes-on-drag="false"
      :pan-on-drag="true"
      :zoom-on-double-click="false"
      :default-viewport="{ zoom: 0.9 }"
      class="orchestration-flow h-full w-full relative z-10"
      @node-click="onNodeClick"
    />
  </div>
</template>

<style scoped>
:deep(.orchestration-flow .vue-flow__pane) {
  background:
    radial-gradient(circle at 1px 1px, color-mix(in oklab, var(--color-border) 48%, transparent) 1px, transparent 0);
  background-size: 20px 20px;
}

:deep(.orchestration-flow .vue-flow__node) {
  border-radius: 1rem;
  background: transparent;
  box-shadow: none;
}

:deep(.orchestration-flow .vue-flow__node.selected) {
  box-shadow: none;
}

:deep(.orchestration-flow .vue-flow__edge-path) {
  transition: stroke 140ms ease, stroke-width 140ms ease;
}
</style>
