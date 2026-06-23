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
      class="relative h-[480px] overflow-hidden rounded-lg border border-border bg-card"
    >
      <VChart
        :option="chartOption"
        autoresize
        class="h-full w-full"
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
              v-if="selectedNode.topic"
              class="inline-block rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary"
            >
              {{ selectedNode.topic }}
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
import { computed, onMounted, ref } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { sdkApiUrl } from '@/lib/api-client'

use([CanvasRenderer, GraphChart, TooltipComponent])

interface GraphNode {
  id: string
  label: string
  memory: string
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

const props = defineProps<{ botId: string }>()

const loading = ref(true)
const graphData = ref<GraphData | null>(null)
const selectedNode = ref<GraphNode | null>(null)

// Topic → color palette (categorical, distinguishable).
const topicColors = [
  '#3b82f6', '#ef4444', '#10b981', '#f59e0b',
  '#8b5cf6', '#ec4899', '#14b8a6', '#f97316',
  '#6366f1', '#84cc16',
]

const topicColorMap = computed(() => {
  const map = new Map<string, string>()
  if (!graphData.value) return map
  let idx = 0
  for (const node of graphData.value.nodes) {
    const topic = node.topic || 'other'
    if (!map.has(topic)) {
      map.set(topic, topicColors[idx % topicColors.length])
      idx++
    }
  }
  return map
})

const chartOption = computed(() => {
  if (!graphData.value) return {}
  const colorMap = topicColorMap.value
  return {
    tooltip: {
      trigger: 'item',
      formatter: (params: { dataType?: string; data?: GraphNode }) => {
        if (params.dataType === 'node' && params.data) {
          const text = params.data.memory || params.data.label || ''
          return text.length > 80 ? text.slice(0, 77) + '...' : text
        }
        return ''
      },
    },
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      label: {
        show: true,
        position: 'right',
        fontSize: 11,
        color: '#888',
        width: 120,
        overflow: 'truncate',
        formatter: (p: { data?: GraphNode }) => p.data?.label ?? '',
      },
      force: {
        repulsion: 200,
        edgeLength: [60, 160],
        gravity: 0.08,
      },
      edgeSymbol: ['none', 'none'],
      lineStyle: {
        color: '#ccc',
        width: 1,
        curveness: 0.1,
      },
      emphasis: {
        focus: 'adjacency',
        lineStyle: { width: 3 },
        label: { fontWeight: 'bold' },
      },
      data: graphData.value.nodes.map((n) => ({
        id: n.id,
        name: n.id,
        label: n.label,
        memory: n.memory,
        topic: n.topic,
        symbolSize: 22,
        itemStyle: { color: colorMap.get(n.topic || 'other') ?? '#888' },
      })),
      links: graphData.value.edges.map((e) => ({
        source: e.source,
        target: e.target,
      })),
    }],
  }
})

function handleNodeClick(params: { dataType?: string; data?: GraphNode }) {
  if (params.dataType === 'node' && params.data) {
    selectedNode.value = params.data
  }
}

async function fetchGraph() {
  loading.value = true
  try {
    const url = sdkApiUrl({ url: `/bots/${props.botId}/memory/graph` })
    const token = localStorage.getItem('token')?.trim() ?? ''
    const resp = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!resp.ok) throw new Error(`graph fetch failed: ${resp.status}`)
    graphData.value = await resp.json() as GraphData
  } catch (e) {
    console.error('failed to load memory graph', e)
    graphData.value = { nodes: [], edges: [] }
  } finally {
    loading.value = false
  }
}

onMounted(fetchGraph)

defineExpose({ refresh: fetchGraph })
</script>
