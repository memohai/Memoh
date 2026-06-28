<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-3">
      <div>
        <h2 class="text-sm font-medium text-foreground">
          {{ $t('memory.graphTitle') }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ $t('memory.graphViewHint') }}
        </p>
      </div>
      <div
        v-if="graphData"
        class="flex shrink-0 gap-4 text-xs text-muted-foreground"
      >
        <span>{{ graphData.nodes.length }} {{ $t('memory.graphNodes') }}</span>
        <span>{{ graphData.edges.length }} {{ $t('memory.graphEdges') }}</span>
      </div>
    </div>

    <div
      v-if="loading"
      class="flex h-80 items-center justify-center text-sm text-muted-foreground"
    >
      {{ $t('common.loading') }}
    </div>

    <div
      v-else-if="graphData && graphData.nodes.length > 0"
      class="relative h-[30rem] overflow-hidden rounded-lg border border-border bg-card"
    >
      <VChart
        :option="chartOption"
        autoresize
        class="size-full"
        @click="handleNodeClick"
      />
    </div>

    <div
      v-else
      class="flex h-80 items-center justify-center text-sm text-muted-foreground"
    >
      {{ $t('memory.graphEmpty') }}
    </div>

    <Dialog
      :open="!!selectedNode"
      @update:open="handleDialogOpen"
    >
      <DialogScrollContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {{ selectedNodeTitle }}
          </DialogTitle>
        </DialogHeader>
        <div
          v-if="selectedNode"
          class="space-y-3"
        >
          <div class="flex flex-wrap gap-2">
            <span
              v-if="selectedNode.slug"
              class="rounded-full bg-[var(--accent-blue-soft-active)] px-2 py-0.5 text-xs font-medium text-[var(--accent-blue-deep)]"
            >
              {{ selectedNode.slug }}
            </span>
            <span
              v-if="selectedNode.topic"
              class="rounded-full bg-[var(--accent-green-soft-active)] px-2 py-0.5 text-xs font-medium text-[var(--accent-green-deep)]"
            >
              {{ selectedNode.topic }}
            </span>
          </div>
          <p class="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
            {{ selectedNode.memory }}
          </p>
        </div>
      </DialogScrollContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDark } from '@vueuse/core'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import type { ECElementEvent } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { Dialog, DialogHeader, DialogScrollContent, DialogTitle } from '@memohai/ui'
import {
  getBotsByBotIdMemoryGraph,
  type HandlersGraphEdge,
  type HandlersGraphNode,
  type HandlersGraphResponse,
} from '@memohai/sdk'

use([CanvasRenderer, GraphChart, TooltipComponent])

type GraphNode = HandlersGraphNode
type GraphEdge = HandlersGraphEdge

interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

interface ChartNodeData extends GraphNode {
  id: string
  name: string
  displayName: string
  symbolSize: number
  itemStyle: { color: string }
}

interface ChartTheme {
  label: string
  line: string
  fallback: string
  fontFamily: string
  palette: string[]
}

const props = defineProps<{ botId: string }>()

const isDark = useDark()
const loading = ref(true)
const graphData = ref<GraphData | null>(null)
const selectedNode = ref<GraphNode | null>(null)
let fetchSeq = 0

const colorCanvas = typeof document !== 'undefined'
  ? document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  : null

function readColor(token: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const probe = document.createElement('span')
  probe.style.color = `var(${token})`
  probe.style.display = 'none'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()
  if (!resolved) return fallback
  if (!colorCanvas) return resolved
  try {
    colorCanvas.clearRect(0, 0, 1, 1)
    colorCanvas.fillStyle = '#000'
    colorCanvas.fillStyle = resolved
    colorCanvas.fillRect(0, 0, 1, 1)
    const [r = 0, g = 0, b = 0, a = 255] = colorCanvas.getImageData(0, 0, 1, 1).data
    return a === 255 ? `rgb(${r}, ${g}, ${b})` : `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`
  }
  catch {
    return fallback
  }
}

const chartTheme = computed<ChartTheme>(() => {
  void isDark.value
  return {
    label: readColor('--muted-foreground', '#71717a'),
    line: readColor('--border', '#d4d4d8'),
    fallback: readColor('--muted-foreground', '#71717a'),
    fontFamily: typeof document !== 'undefined' ? getComputedStyle(document.body).fontFamily : 'inherit',
    palette: [
      readColor('--accent-blue', '#2383e2'),
      readColor('--accent-green', '#448361'),
      readColor('--accent-teal', '#2c8b9e'),
      readColor('--accent-orange', '#d9730d'),
      readColor('--accent-pink', '#c14c8a'),
      readColor('--accent-red', '#cd3c3a'),
      readColor('--accent-yellow', '#cb912f'),
      readColor('--accent-purple', '#9065b0'),
    ],
  }
})

const selectedNodeTitle = computed(() => selectedNode.value ? displayName(selectedNode.value) : '')

const chartOption = computed(() => {
  if (!graphData.value || graphData.value.nodes.length === 0) return {}
  const theme = chartTheme.value
  const nodes = graphData.value.nodes.filter((node): node is GraphNode & { id: string } => !!node.id)
  const edges = graphData.value.edges.filter((edge): edge is GraphEdge & { source: string; target: string } => !!edge.source && !!edge.target)
  return {
    tooltip: {
      trigger: 'item',
      formatter: (params: { dataType?: string; data?: ChartNodeData }) => {
        if (params.dataType !== 'node' || !params.data) return ''
        const text = params.data.memory || params.data.label || ''
        return escapeTooltip([
          params.data.displayName,
          text.length > 100 ? `${text.slice(0, 97)}...` : text,
        ].filter(Boolean).join('\n'))
      },
    },
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      label: {
        show: true,
        fontSize: 11,
        color: theme.label,
        fontFamily: theme.fontFamily,
        formatter: (params: { data?: ChartNodeData }) => params.data?.displayName ?? '',
      },
      force: {
        repulsion: 200,
        edgeLength: [60, 160],
        gravity: 0.08,
      },
      lineStyle: {
        color: theme.line,
        width: 1,
        curveness: 0.1,
      },
      emphasis: {
        focus: 'adjacency',
        lineStyle: { width: 3 },
      },
      data: nodes.map((node): ChartNodeData => ({
        ...node,
        name: node.id,
        displayName: displayName(node),
        symbolSize: 30,
        itemStyle: {
          color: subjectColor(node.subject || node.slug || node.topic, theme),
        },
      })),
      links: edges.map((edge) => ({
        source: edge.source,
        target: edge.target,
      })),
    }],
  }
})

function displayName(node: GraphNode): string {
  return node.slug || node.subject || node.label || node.id || ''
}

function subjectColor(subject: string | undefined, theme: ChartTheme): string {
  if (!subject || theme.palette.length === 0) return theme.fallback
  let hash = 0
  for (let i = 0; i < subject.length; i++) {
    hash = ((hash << 5) - hash + subject.charCodeAt(i)) | 0
  }
  return theme.palette[Math.abs(hash) % theme.palette.length] ?? theme.fallback
}

function escapeTooltip(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll('\n', '<br>')
}

function isChartNodeData(data: unknown): data is ChartNodeData {
  return typeof data === 'object'
    && data !== null
    && 'id' in data
    && 'name' in data
}

function handleNodeClick(params: ECElementEvent) {
  if (params.dataType === 'node' && isChartNodeData(params.data)) {
    selectedNode.value = params.data
  }
}

function handleDialogOpen(open: boolean) {
  if (!open) {
    selectedNode.value = null
  }
}

function normalizeGraph(data: HandlersGraphResponse | undefined): GraphData {
  return {
    nodes: data?.nodes ?? [],
    edges: data?.edges ?? [],
  }
}

async function fetchGraph() {
  const botId = props.botId.trim()
  const seq = ++fetchSeq
  if (!botId) {
    graphData.value = null
    loading.value = false
    return
  }

  loading.value = true
  try {
    const { data } = await getBotsByBotIdMemoryGraph({
      path: { bot_id: botId },
      throwOnError: true,
    })
    if (seq === fetchSeq) {
      graphData.value = normalizeGraph(data)
    }
  }
  catch (error) {
    if (seq === fetchSeq) {
      console.error('failed to load memory graph', error)
      graphData.value = { nodes: [], edges: [] }
    }
  }
  finally {
    if (seq === fetchSeq) {
      loading.value = false
    }
  }
}

watch(() => props.botId, () => {
  selectedNode.value = null
  void fetchGraph()
}, { immediate: true })

defineExpose({ refresh: fetchGraph })
</script>
