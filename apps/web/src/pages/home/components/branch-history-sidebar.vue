<template>
  <aside
    v-if="branchSidebarOpen"
    class="relative flex shrink-0 flex-col border-l border-sidebar-border bg-sidebar"
    role="complementary"
    :aria-label="t('chat.branchHistory.title')"
    :style="asideStyle"
  >
    <nav
      class="flex h-11 shrink-0 items-center gap-1.5 pl-3 pr-2 py-1.5"
    >
      <button
        type="button"
        class="inline-flex h-8 w-8 shrink-0 cursor-default items-center justify-center rounded-full bg-sidebar-accent text-foreground outline-none"
        :title="t('chat.branchHistory.title')"
        :aria-label="t('chat.branchHistory.title')"
        aria-pressed="true"
      >
        <GitBranch
          :stroke-width="1.75"
          class="size-4 shrink-0"
        />
      </button>
    </nav>

    <div
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

      <div
        v-else
        class="relative h-full w-full"
      >
        <VueFlow
          :id="flowId"
          :nodes="flowNodes"
          :edges="flowEdges"
          class="branch-flow h-full w-full"
          :min-zoom="MIN_ZOOM"
          :max-zoom="MAX_ZOOM"
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
            <div class="relative">
              <Handle
                id="target-left"
                type="target"
                :position="Position.Left"
                class="branch-handle branch-handle-target"
              />
              <div
                role="button"
                tabindex="0"
                class="branch-card nodrag nopan group relative flex cursor-pointer flex-col overflow-hidden rounded-lg px-2.5 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
                :class="branchCardClass(data)"
                :style="{ width: `${data.width}px`, height: `${data.height}px` }"
                :aria-disabled="branchActionLoading"
                :aria-current="data.active ? 'true' : undefined"
                :title="data.title"
                @click.stop="switchToBranch(data.branchId)"
                @keydown.enter.prevent="switchToBranch(data.branchId)"
                @keydown.space.prevent="switchToBranch(data.branchId)"
              >
                <div class="flex min-h-0 flex-1 flex-col gap-1">
                  <p class="line-clamp-2 text-body font-medium">
                    {{ data.title }}
                  </p>
                  <p
                    v-if="data.preview"
                    class="text-caption text-muted-foreground"
                    :class="data.previewLines > 2 ? 'line-clamp-3' : 'line-clamp-2'"
                  >
                    {{ data.preview }}
                  </p>
                </div>
              </div>
              <Handle
                id="source-right"
                type="source"
                :position="Position.Right"
                class="branch-handle branch-handle-source"
              />
            </div>
          </template>
        </VueFlow>

        <div
          class="absolute bottom-3 left-3 z-10 flex items-center gap-0.5 rounded-md bg-sidebar/90 p-1 shadow-sm backdrop-blur"
        >
          <button
            type="button"
            class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
            :title="t('common.zoomOut')"
            :aria-label="t('common.zoomOut')"
            @click="zoomOut"
          >
            <ZoomOut class="size-3.5" />
          </button>
          <button
            type="button"
            class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
            :title="t('common.reset')"
            :aria-label="t('common.reset')"
            @click="resetZoom"
          >
            <RefreshCcw class="size-3.5" />
          </button>
          <button
            type="button"
            class="inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
            :title="t('common.zoomIn')"
            :aria-label="t('common.zoomIn')"
            @click="zoomIn"
          >
            <ZoomIn class="size-3.5" />
          </button>
        </div>
      </div>
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
import { computed, nextTick, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { GitBranch, LoaderCircle, RefreshCcw, ZoomIn, ZoomOut } from 'lucide-vue-next'
import { VueFlow, Handle, MarkerType, Position, type Edge, type Node, type VueFlowStore } from '@vue-flow/core'
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
  activePath: boolean
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
const NODE_GAP_X = 36
const NODE_GAP_Y = 18
const MIN_ZOOM = 0.5
const MAX_ZOOM = 1.55
const flowId = 'chat-branch-history-flow'
const isResizing = ref(false)
const flowInstance = ref<VueFlowStore | null>(null)

const cardMetrics = computed(() => {
  const width = Math.round(Math.min(288, Math.max(176, branchSidebarWidth.value - 88)))
  const previewLines = width >= 260 ? 3 : 2
  return {
    width,
    height: previewLines > 2 ? 98 : 80,
    titleChars: Math.round(width / 7.2) * 2,
    previewChars: Math.round(width / 5.8) * previewLines,
    previewLines,
  }
})

const asideStyle = computed<Record<string, string>>(() => ({
  width: `${branchSidebarWidth.value}px`,
  '--btn-ghost-hover': 'var(--sidebar-hover)',
}))

const branchItems = computed<BranchHistoryItem[]>(() => {
  const graph = branchGraph.value
  const turns = graph?.turns ?? []
  const titleByBranch = new Map((graph?.branches ?? []).map(branch => [branch.id, branch.title?.trim() ?? '']))
  const metrics = cardMetrics.value
  const items = turns.map((turn, index) => {
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
  return layoutBranchTree(items)
})

function layoutBranchTree(items: BranchHistoryItem[]): BranchHistoryItem[] {
  if (items.length <= 1) return items

  const itemById = new Map(items.map(item => [item.id, item]))
  const childrenByParent = new Map<string, BranchHistoryItem[]>()
  const roots: BranchHistoryItem[] = []

  for (const item of items) {
    const parentId = item.parentTurnId
    if (parentId && itemById.has(parentId)) {
      const children = childrenByParent.get(parentId) ?? []
      children.push(item)
      childrenByParent.set(parentId, children)
    } else {
      roots.push(item)
    }
  }

  const rowById = new Map<string, number>()
  const depthById = new Map<string, number>()
  let nextLeafRow = 0

  function place(item: BranchHistoryItem, depth: number, visiting = new Set<string>()) {
    if (rowById.has(item.id)) return
    if (visiting.has(item.id)) {
      depthById.set(item.id, depth)
      rowById.set(item.id, nextLeafRow++)
      return
    }

    visiting.add(item.id)
    depthById.set(item.id, depth)
    const children = childrenByParent.get(item.id) ?? []

    if (!children.length) {
      rowById.set(item.id, nextLeafRow++)
    } else {
      for (const child of children) {
        place(child, depth + 1, visiting)
      }
      const firstChild = children[0]
      const lastChild = children[children.length - 1]
      rowById.set(item.id, ((rowById.get(firstChild.id) ?? nextLeafRow) + (rowById.get(lastChild.id) ?? nextLeafRow)) / 2)
    }
    visiting.delete(item.id)
  }

  for (const root of roots) place(root, 0)
  for (const item of items) {
    if (!rowById.has(item.id)) place(item, item.depth)
  }

  return items.map(item => ({
    ...item,
    depth: depthById.get(item.id) ?? item.depth,
    row: rowById.get(item.id) ?? item.row,
  }))
}

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
      activePath: activePathNodeIds.value.has(item.id),
      width: item.width,
      height: item.height,
      previewLines: item.previewLines,
    },
  })),
)

const activePathState = computed(() => {
  const items = branchItems.value
  const itemById = new Map(items.map(item => [item.id, item]))
  const active = [...items].reverse().find(item => item.active) ?? latestVisibleParentTurn()
  const nodeIds = new Set<string>()
  const edgeIds = new Set<string>()

  let current = active
  const visited = new Set<string>()
  while (current && !visited.has(current.id)) {
    visited.add(current.id)
    nodeIds.add(current.id)
    if (!current.parentTurnId) break
    const parent = itemById.get(current.parentTurnId)
    if (!parent) break
    edgeIds.add(`${parent.id}-${current.id}`)
    current = parent
  }

  return { nodeIds, edgeIds }
})

const activePathNodeIds = computed(() => activePathState.value.nodeIds)
const activePathEdgeIds = computed(() => activePathState.value.edgeIds)

const flowEdges = computed<Edge[]>(() =>
  branchItems.value
    .filter(item => item.parentTurnId)
    .map((item) => {
      const id = `${item.parentTurnId}-${item.id}`
      const activePath = activePathEdgeIds.value.has(id)
      return {
        id,
        source: item.parentTurnId,
        target: item.id,
        sourceHandle: 'source-right',
        targetHandle: 'target-left',
        type: 'straight',
        style: {
          stroke: activePath ? 'var(--accent-blue)' : 'var(--sidebar-border)',
          strokeOpacity: activePath ? 0.62 : 0.36,
          strokeWidth: activePath ? 1.6 : 1,
        },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: activePath ? 'var(--accent-blue)' : 'var(--sidebar-border)',
          width: 12,
          height: 12,
        },
      }
    }),
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

function branchCardClass(data: BranchNodeData): string {
  if (data.active) return 'bg-[color:var(--accent-blue-soft-active)] text-foreground'
  if (data.activePath) return 'bg-sidebar-accent text-foreground'
  return 'text-sidebar-foreground hover:bg-[color:var(--sidebar-hover)]'
}

function switchToBranch(branchId: string) {
  const targetBranchId = branchId.trim()
  if (!targetBranchId || branchActionLoading.value) return
  void chatStore.switchBranch(targetBranchId).then(() => {
    if (branchSidebarOpen.value) void focusActiveBranch()
  })
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
  const sourceMessageId = activeBranch?.fork_from_message_id?.trim()
  if (sourceMessageId) {
    const sourceTurn = graph?.turns?.find(turn =>
      turn.user_message_id?.trim() === sourceMessageId
      || turn.assistant_message_id?.trim() === sourceMessageId,
    )
    if (sourceTurn?.user_message_id?.trim() === sourceMessageId) {
      const parentItem = branchItems.value.find(item => item.id === sourceTurn.parent_turn_id?.trim())
      if (parentItem) return parentItem
      return undefined
    }
    if (sourceTurn) {
      return branchItems.value.find(item => item.id === sourceTurn.id)
    }
  }
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
    void focusActiveBranch()
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
  pointer-events: auto;
  cursor: default;
  border: 0;
  background: transparent;
  box-shadow: none;
}

.branch-flow :deep(.vue-flow__node.selected) {
  box-shadow: none;
}

.branch-flow :deep(.branch-card) {
  pointer-events: auto;
}

.branch-flow :deep(.branch-handle) {
  height: 0;
  width: 0;
  border: 0;
  min-height: 0;
  min-width: 0;
  background: transparent;
  box-shadow: none;
  opacity: 0;
  pointer-events: none;
  visibility: hidden;
}

.branch-flow :deep(.branch-handle-source) {
  right: 0;
}

.branch-flow :deep(.branch-handle-target) {
  left: 0;
}

.branch-flow :deep(.vue-flow__edge-path) {
  stroke-linecap: round;
}

</style>
