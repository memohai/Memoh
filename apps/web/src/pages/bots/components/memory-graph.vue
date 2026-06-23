<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
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
        class="flex gap-4 text-xs text-muted-foreground"
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
        style="height: 100%; width: 100%;"
        @click="handleNodeClick"
      />
    </div>

    <div
      v-else
      class="flex h-80 items-center justify-center text-sm text-muted-foreground"
    >
      {{ $t('memory.graphEmpty') }}
    </div>

    <!-- Node detail popover -->
    <div
      v-if="selectedNode"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      @click.self="selectedNode = null"
    >
      <div class="w-full max-w-lg space-y-3 rounded-lg border border-border bg-popover p-5 shadow-lg">
        <div class="flex items-start justify-between gap-3">
          <div class="space-y-1">
            <span
              v-if="selectedNode.subject"
              class="inline-block rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary"
            >
              {{ selectedNode.subject }}
            </span>
            <p class="text-sm leading-relaxed text-foreground">
              {{ selectedNode.memory }}
            </p>
          </div>
          <button
            class="text-muted-foreground transition-colors hover:text-foreground"
            @click="selectedNode = null"
          >
            ✕
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import type { ECElementEvent } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { sdkApiUrl } from '@/lib/api-client'

use([CanvasRenderer, GraphChart, TooltipComponent])

interface GraphNode {
  id: string
  label: string
  memory: string
  subject?: string
  topic?: string
  metadata?: Record<string, unknown>
}
interface GraphEdge {
  source: string
  target: string
  rel: string
}
interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}
interface ChartNodeData extends GraphNode {
  name: string
  displayName: string
  symbolSize: number
  itemStyle: { color: string }
}

const props = defineProps<{ botId: string }>()

const loading = ref(true)
const graphData = ref<GraphData | null>(null)
const selectedNode = ref<GraphNode | null>(null)
let fetchSeq = 0

// Topic → color palette is replaced by subjectColor() for deterministic hashing.

const chartOption = computed(() => {
  if (!graphData.value || graphData.value.nodes.length === 0) return {}
  return {
    tooltip: {
      trigger: 'item',
      formatter: (params: { dataType?: string; data?: ChartNodeData }) => {
        if (params.dataType !== 'node' || !params.data) return ''
        const text = params.data.memory || params.data.label || ''
        return escapeTooltip([
          params.data.displayName,
          text.length > 100 ? text.slice(0, 97) + '...' : text,
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
        color: '#888',
        formatter: (params: { data?: ChartNodeData }) => params.data?.displayName ?? '',
      },
      force: {
        repulsion: 200,
        edgeLength: [60, 160],
        gravity: 0.08,
      },
      lineStyle: {
        color: '#aaa',
        width: 1,
        curveness: 0.1,
      },
      emphasis: {
        focus: 'adjacency',
        lineStyle: { width: 3 },
      },
      data: graphData.value.nodes.map((n): ChartNodeData => {
        const displayName = n.subject || n.label || n.id
        return {
          ...n,
          name: n.id,
          displayName,
          symbolSize: 30,
          itemStyle: {
            color: subjectColor(n.subject),
          },
        }
      }),
      links: graphData.value.edges.map((e) => ({
        source: e.source,
        target: e.target,
      })),
    }],
  }
})

function subjectColor(subject?: string): string {
  if (!subject) return '#888'
  let hash = 0
  for (let i = 0; i < subject.length; i++) {
    hash = ((hash << 5) - hash + subject.charCodeAt(i)) | 0
  }
  const palette = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316']
  return palette[Math.abs(hash) % palette.length] ?? '#888'
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
    && 'memory' in data
}

function handleNodeClick(params: ECElementEvent) {
  if (params.dataType === 'node' && isChartNodeData(params.data)) {
    selectedNode.value = params.data
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
    const url = sdkApiUrl({ url: `/bots/${encodeURIComponent(botId)}/memory/graph` })
    const token = localStorage.getItem('token')?.trim() ?? ''
    const resp = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!resp.ok) throw new Error(`graph fetch failed: ${resp.status}`)
    const data = await resp.json() as GraphData
    if (seq === fetchSeq) {
      graphData.value = data
    }
  } catch (e) {
    if (seq === fetchSeq) {
      console.error('failed to load memory graph', e)
      graphData.value = { nodes: [], edges: [] }
    }
  } finally {
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
