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
        :zoom-on-double-click="false"
        :zoom-on-scroll="false"
        :zoom-on-pinch="false"
        :pan-on-scroll="true"
        :pan-on-drag="true"
        :prevent-scrolling="false"
        :fit-view-on-init="true"
        @init="handleFlowInit"
      >
        <template #node-branch="{ data }">
          <button
            type="button"
            class="branch-card nowheel nodrag flex h-[68px] w-[200px] flex-col justify-center rounded-lg px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            :class="data.active
              ? 'bg-primary/12 text-foreground ring-1 ring-primary/35'
              : 'bg-sidebar-accent/70 text-foreground hover:bg-sidebar-accent'"
            :disabled="data.active || branchActionLoading"
            :aria-current="data.active ? 'true' : undefined"
            :title="data.label"
            @click.stop="chatStore.switchBranch(data.branchId)"
          >
            <div class="min-w-0">
              <span class="line-clamp-2 text-xs font-semibold leading-snug">{{ data.label }}</span>
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
import { GitBranch, LoaderCircle } from 'lucide-vue-next'
import { VueFlow, MarkerType, Position, type Edge, type Node, type VueFlowStore } from '@vue-flow/core'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

interface BranchHistoryItem {
  id: string
  branchId: string
  parentTurnId: string
  label: string
  active: boolean
  depth: number
  row: number
}

interface BranchNodeData {
  id: string
  branchId: string
  label: string
  active: boolean
}

const { t } = useI18n()
const chatStore = useChatStore()
const { branchGraph, branchLoading, branchActionLoading, currentBotId, sessionId } = storeToRefs(chatStore)
const workspaceTabs = useWorkspaceTabsStore()
const { branchSidebarOpen, branchSidebarWidth } = storeToRefs(workspaceTabs)
const MIN_WIDTH = 220
const MAX_WIDTH = 520
const NODE_WIDTH = 200
const NODE_HEIGHT = 68
const NODE_GAP_X = 38
const NODE_GAP_Y = 22
const flowId = 'chat-branch-history-flow'
const isResizing = ref(false)
const flowInstance = ref<VueFlowStore | null>(null)

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
  return turns.map((turn, index) => {
    const preview = turn.preview ?? {}
    const userText = preview.user_text?.trim() ?? ''
    const title = turn.title?.trim() || titleByBranch.get(turn.branch_id)?.trim() || ''
    return {
      id: turn.id,
      branchId: turn.branch_id,
      parentTurnId: turn.parent_turn_id?.trim() ?? '',
      label: title || firstPreviewLine(userText) || t('chat.branchHistory.turnLabel', { index: index + 1 }),
      active: turn.active === true,
      depth: turn.depth ?? 0,
      row: index,
    }
  })
})

const flowNodes = computed<Node<BranchNodeData>[]>(() =>
  branchItems.value.map((item) => ({
    id: item.id,
    type: 'branch',
    position: {
      x: 12 + item.depth * (NODE_WIDTH + NODE_GAP_X),
      y: 12 + item.row * (NODE_HEIGHT + NODE_GAP_Y),
    },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: {
      id: item.id,
      branchId: item.branchId,
      label: item.label,
      active: item.active,
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

function firstPreviewLine(text: string): string {
  return text.split(/\r?\n/).map(line => line.trim()).find(Boolean)?.slice(0, 48) ?? ''
}

function refreshBranches() {
  if (!currentBotId.value || !sessionId.value) return
  void chatStore.loadBranches(currentBotId.value, sessionId.value)
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
    minZoom: 0.8,
    maxZoom: 1,
    duration: 220,
  })
}

function latestVisibleParentTurn(): BranchHistoryItem | undefined {
  const graph = branchGraph.value
  const activeBranchId = graph?.active_branch_id?.trim()
  if (!activeBranchId) return undefined
  const activeBranch = graph?.branches?.find(branch => branch.id === activeBranchId)
  const forkFromMessageId = activeBranch?.fork_from_message_id?.trim()
  const parentBranchId = activeBranch?.parent_branch_id?.trim()
  const forkFromSeq = activeBranch?.fork_from_seq ?? 0
  if (!forkFromMessageId || !parentBranchId || forkFromSeq <= 0) return undefined
  return [...branchItems.value]
    .reverse()
    .find((item) => {
      const turn = graph?.turns?.find(candidate => candidate.id === item.id)
      return item.branchId === parentBranchId && turn?.branch_seq === forkFromSeq
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
}

.branch-flow :deep(.vue-flow__edge-path) {
  stroke-linecap: round;
}
</style>
