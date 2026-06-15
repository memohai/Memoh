<template>
  <aside
    class="relative flex shrink-0 flex-col border-l border-sidebar-border bg-sidebar"
    :style="asideStyle"
    :inert="!branchSidebarOpen || undefined"
  >
    <nav class="flex h-11 shrink-0 items-center gap-1.5 pl-3 pr-2 py-1.5">
      <button
        type="button"
        class="inline-flex h-8 w-8 shrink-0 cursor-default items-center justify-center rounded-full bg-sidebar-accent text-foreground outline-none"
        :title="t('chat.branchHistory.title')"
        :aria-label="t('chat.branchHistory.title')"
        aria-pressed="true"
      >
        <GitBranch
          :stroke-width="1.75"
          class="size-[18px] shrink-0"
        />
      </button>
      <div class="ml-auto flex items-center gap-0.5">
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
          :title="t('common.zoomOut', 'Zoom out')"
          :aria-label="t('common.zoomOut', 'Zoom out')"
          :disabled="!branchSidebarOpen"
          @click="zoomOut"
        >
          <ZoomOut class="size-3.5" />
        </button>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
          :title="t('common.reset', 'Reset')"
          :aria-label="t('common.reset', 'Reset')"
          :disabled="!branchSidebarOpen"
          @click="resetZoom"
        >
          <RefreshCcw class="size-3.5" />
        </button>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
          :title="t('common.zoomIn', 'Zoom in')"
          :aria-label="t('common.zoomIn', 'Zoom in')"
          :disabled="!branchSidebarOpen"
          @click="zoomIn"
        >
          <ZoomIn class="size-3.5" />
        </button>
      </div>
    </nav>

    <div
      v-if="branchSidebarOpen"
      class="relative min-h-0 flex-1"
    >
      <div
        v-if="branchLoading"
        class="flex h-full items-center justify-center gap-2 px-4 text-center text-xs text-muted-foreground"
      >
        <LoaderCircle class="size-3.5 animate-spin" />
        {{ t('chat.branchHistory.loading') }}
      </div>

      <div
        v-else-if="branchItems.length === 0"
        class="flex h-full items-center justify-center px-6 text-center text-xs text-muted-foreground"
      >
        {{ t('chat.branchHistory.empty') }}
      </div>

      <VueFlow
        v-else
        :id="flowId"
        :nodes="flowNodes"
        :edges="flowEdges"
        class="branch-flow h-full w-full"
        :min-zoom="0.5"
        :max-zoom="1.15"
        :nodes-draggable="false"
        :nodes-connectable="false"
        :elements-selectable="false"
        :zoom-on-double-click="true"
        :zoom-on-scroll="true"
        :zoom-on-pinch="true"
        :pan-on-scroll="true"
        :pan-on-drag="true"
        :prevent-scrolling="false"
        :fit-view-on-init="true"
        :default-viewport="{ x: 0, y: 0, zoom: 1 }"
        @init="handleFlowInit"
      >
        <template #node-branch="{ data }">
          <button
            type="button"
            class="branch-card nodrag group relative flex flex-col overflow-hidden rounded-lg border px-3 py-2.5 text-left shadow-sm transition-[background-color,border-color,box-shadow,transform] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            :class="data.active
              ? 'border-primary/45 bg-sidebar-accent text-foreground shadow-md'
              : 'border-sidebar-border/70 bg-background/88 text-sidebar-foreground hover:border-sidebar-border hover:bg-sidebar-accent/55 hover:shadow'"
            :style="{ width: `${data.width}px`, height: `${data.height}px` }"
            :disabled="data.active || branchActionLoading"
            :aria-current="data.active ? 'true' : undefined"
            :title="data.title"
            @click.stop="chatStore.switchBranch(data.branchId)"
          >
            <div class="flex min-h-0 flex-1 flex-col gap-1.5">
              <p class="line-clamp-2 text-[13px] font-medium leading-snug tracking-normal">
                {{ data.title }}
              </p>
              <p
                v-if="data.preview"
                class="text-[11px] leading-snug text-muted-foreground"
                :class="data.previewLines > 2 ? 'line-clamp-3' : 'line-clamp-2'"
              >
                {{ data.preview }}
              </p>
            </div>
          </button>
        </template>
      </VueFlow>
    </div>

    <div
      class="group absolute left-0 top-0 z-10 h-full w-1 cursor-col-resize"
      @mousedown="onResizeStart"
    >
      <div
        class="h-full w-full transition-colors group-hover:bg-border"
        :class="{ 'bg-ring': isResizing }"
      />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { GitBranch, LoaderCircle, RefreshCcw, ZoomIn, ZoomOut } from 'lucide-vue-next'
import { VueFlow, MarkerType, Position, type Edge, type Node, type VueFlowStore } from '@vue-flow/core'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

interface BranchHistoryItem {
  id: string
  branchId: string
  parentTurnId: string
  title: string
  preview: string
  active: boolean
  depth: number
  row: number
  width: number
  height: number
  previewLines: number
}

interface BranchNodeData {
  id: string
  branchId: string
  title: string
  preview: string
  active: boolean
  width: number
  height: number
  previewLines: number
}

const { t } = useI18n()
const chatStore = useChatStore()
const { branchGraph, branchLoading, branchActionLoading, currentBotId, sessionId } = storeToRefs(chatStore)
const workspaceTabs = useWorkspaceTabsStore()
const { branchSidebarOpen, branchSidebarWidth } = storeToRefs(workspaceTabs)
const MIN_WIDTH = 220
const MAX_WIDTH = 520
const NODE_GAP_X = 44
const NODE_GAP_Y = 24
const MIN_ZOOM = 0.5
const MAX_ZOOM = 1.55
const flowId = 'chat-branch-history-flow'
const isResizing = ref(false)
const flowInstance = ref<VueFlowStore | null>(null)

const cardMetrics = computed(() => {
  const width = Math.round(Math.min(320, Math.max(196, branchSidebarWidth.value - 72)))
  const previewLines = width >= 288 ? 3 : 2
  return {
    width,
    height: previewLines > 2 ? 112 : 92,
    titleChars: Math.round(width / 7.2) * 2,
    previewChars: Math.round(width / 5.8) * previewLines,
    previewLines,
  }
})

const asideStyle = computed<Record<string, string>>(() => ({
  width: `${branchSidebarWidth.value}px`,
  marginRight: branchSidebarOpen.value ? '0px' : `-${branchSidebarWidth.value}px`,
  transition: 'margin-right 300ms cubic-bezier(0.32, 0.72, 0, 1)',
  '--btn-ghost-hover': 'var(--sidebar-hover)',
}))

const branchItems = computed<BranchHistoryItem[]>(() => {
  const graph = branchGraph.value
  const turns = graph?.turns ?? []
  const titleByBranch = new Map((graph?.branches ?? []).map(branch => [branch.id, branch.title?.trim() ?? '']))
  const metrics = cardMetrics.value
  return turns.map((turn, index) => {
    const preview = turn.preview ?? {}
    const assistantText = preview.assistant_text?.trim() ?? ''
    const title = turn.title?.trim() || titleByBranch.get(turn.branch_id)?.trim() || ''
    const fallbackTitle = firstPreviewLine(assistantText, metrics.titleChars) || t('chat.branchHistory.turnLabel', { index: index + 1 })
    const previewText = title ? firstPreviewLine(assistantText, metrics.previewChars) : ''
    return {
      id: turn.id,
      branchId: turn.branch_id,
      parentTurnId: turn.parent_turn_id?.trim() ?? '',
      title: clampPreview(title || fallbackTitle, metrics.titleChars),
      preview: previewText && previewText !== title ? previewText : '',
      active: turn.active === true,
      depth: turn.depth ?? 0,
      row: index,
      width: metrics.width,
      height: metrics.height,
      previewLines: metrics.previewLines,
    }
  })
})

const flowNodes = computed<Node<BranchNodeData>[]>(() =>
  branchItems.value.map((item) => ({
    id: item.id,
    type: 'branch',
    position: {
      x: 12 + item.depth * (item.width + NODE_GAP_X),
      y: 12 + item.row * (item.height + NODE_GAP_Y),
    },
    style: {
      width: `${item.width}px`,
      height: `${item.height}px`,
    },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: {
      id: item.id,
      branchId: item.branchId,
      title: item.title,
      preview: item.preview,
      active: item.active,
      width: item.width,
      height: item.height,
      previewLines: item.previewLines,
    },
  })),
)

const flowEdges = computed<Edge[]>(() =>
  branchItems.value
    .filter(item => item.parentTurnId)
    .map((item) => ({
      id: `${item.parentTurnId}-${item.id}`,
      source: item.parentTurnId,
      target: item.id,
      type: 'smoothstep',
      animated: item.active,
      style: {
        stroke: item.active ? 'var(--primary)' : 'var(--sidebar-border)',
        strokeWidth: item.active ? 1.8 : 1.2,
      },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: item.active ? 'var(--primary)' : 'var(--sidebar-border)',
        width: 12,
        height: 12,
      },
    })),
)

function firstPreviewLine(text: string, maxChars: number): string {
  const line = text.split(/\r?\n/).map(item => item.trim()).find(Boolean) ?? ''
  return clampPreview(line, maxChars)
}

function clampPreview(text: string, maxChars: number): string {
  const normalized = text.replace(/\s+/g, ' ').trim()
  if (normalized.length <= maxChars) return normalized
  return `${normalized.slice(0, Math.max(0, maxChars - 1)).trimEnd()}…`
}

function refreshBranches() {
  if (!currentBotId.value || !sessionId.value) return
  void chatStore.loadBranches(currentBotId.value, sessionId.value)
}

async function zoomIn() {
  await setFlowZoom((flowInstance.value?.viewport.zoom ?? 1) + 0.15)
}

async function zoomOut() {
  await setFlowZoom((flowInstance.value?.viewport.zoom ?? 1) - 0.15)
}

async function resetZoom() {
  await focusActiveBranch()
}

async function setFlowZoom(zoom: number) {
  if (!flowInstance.value) return
  const viewport = flowInstance.value.viewport
  await flowInstance.value.setViewport({
    x: viewport.x,
    y: viewport.y,
    zoom: Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom)),
  }, { duration: 140 })
}

function handleFlowInit(instance: VueFlowStore) {
  flowInstance.value = instance
  void focusActiveBranch()
}

async function focusActiveBranch() {
  const active = [...branchItems.value].reverse().find(item => item.active) ?? latestVisibleParentTurn()
  if (!active || !flowInstance.value) return
  await nextTick()
  await flowInstance.value.fitView({
    nodes: [active.id],
    padding: 0.45,
    minZoom: 0.75,
    maxZoom: 1.05,
    duration: 220,
  })
}

function latestVisibleParentTurn(): BranchHistoryItem | undefined {
  const graph = branchGraph.value
  const activeBranchId = graph?.active_branch_id?.trim()
  if (!activeBranchId) return undefined
  const activeBranch = graph?.branches?.find(branch => branch.id === activeBranchId)
  const parentBranchId = activeBranch?.parent_branch_id?.trim()
  const forkFromTurnSeq = activeBranch?.fork_from_turn_seq ?? activeBranch?.fork_from_seq ?? 0
  if (!parentBranchId || forkFromTurnSeq <= 0) return undefined
  return [...branchItems.value]
    .reverse()
    .find((item) => {
      const turn = graph?.turns?.find(candidate => candidate.id === item.id)
      return item.branchId === parentBranchId && ((turn?.turn_seq ?? turn?.branch_seq) === forkFromTurnSeq)
    })
}

watch([currentBotId, sessionId, branchSidebarOpen], ([botId, sid, open]) => {
  if (open && botId && sid) refreshBranches()
}, { immediate: true })

watch(
  () => [branchGraph.value?.active_branch_id, branchItems.value.length, branchSidebarOpen.value],
  () => {
    if (branchSidebarOpen.value) void focusActiveBranch()
  },
)

onMounted(refreshBranches)

function onResizeStart(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  const startX = e.clientX
  const startWidth = branchSidebarWidth.value

  function onMouseMove(ev: MouseEvent) {
    const delta = startX - ev.clientX
    branchSidebarWidth.value = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth + delta))
  }

  function onMouseUp() {
    isResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }

  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}
</script>

<style>
@import '@vue-flow/core/dist/style.css';
</style>

<style scoped>
.branch-flow {
  --vf-node-bg: transparent;
  --vf-node-text: var(--foreground);
  --vf-connection-path: var(--sidebar-border);
  background: transparent;
}

.branch-flow :deep(.vue-flow__pane) {
  cursor: grab;
}

.branch-flow :deep(.vue-flow__pane.dragging) {
  cursor: grabbing;
}

.branch-flow :deep(.vue-flow__node) {
  cursor: default;
  border: 0;
  background: transparent;
  box-shadow: none;
}

.branch-flow :deep(.vue-flow__node.selected) {
  box-shadow: none;
}

.branch-flow :deep(.vue-flow__handle) {
  opacity: 0;
  pointer-events: none;
}

.branch-flow :deep(.vue-flow__edge-path) {
  stroke-linecap: round;
}

.branch-flow :deep(.vue-flow__edge.animated path) {
  stroke-dasharray: 6;
}
</style>
