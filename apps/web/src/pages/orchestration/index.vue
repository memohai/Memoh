<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery, useQueryCache } from '@pinia/colada'
import {
  Button,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Input,
  NativeSelect,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
} from '@memohai/ui'
import { toast } from 'vue-sonner'
import {
  AlertCircle,
  Bot,
  Boxes,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Copy,
  Database,
  FileOutput,
  GitMerge,
  Hammer,
  Library,
  LoaderCircle,
  Maximize2,
  PlayCircle,
  RefreshCw,
  ScanSearch,
  Search,
  Settings2,
  ShieldCheck,
  Sparkles,
  Square,
  Workflow,
  Wrench,
  X,
  ZoomIn,
  ZoomOut,
  type LucideIcon,
} from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import {
  getOrchestrationBotsByBotIdRuns,
  postOrchestrationCheckpointsByCheckpointIdResolve,
  getOrchestrationRunsByRunIdInspector,
  postOrchestrationRunsByRunIdCancel,
} from '@memohai/sdk'
import { useClipboard } from '@/composables/useClipboard'
import { fetchBots } from '@/composables/api/useChat.chat-api'
import { useChatSelectionStore } from '@/store/chat-selection'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  compactResultSummary,
  compactTaskLabel,
  compactTaskTitle,
  compactWorker,
  formatDate,
  formatJsonValue,
  shortId,
  type BotItem,
  type RunInspectorDependency,
  type RunInspectorExecutionSpan,
  type RunInspectorTask,
  type RunListItem,
} from './model'

type NodeKind = 'trigger' | 'llm' | 'planner' | 'search' | 'tool' | 'memory' | 'merge' | 'verify' | 'output'
type InspectorTab = 'thinking' | 'config' | 'task' | 'inputs' | 'outputs' | 'logs'

interface CanvasNode {
  task: RunInspectorTask
  id: string
  title: string
  subtitle: string
  kind: NodeKind
  status: string
  level: number
  x: number
  y: number
}

interface ThinkingItem {
  id: string
  kind: 'reasoning' | 'tool' | 'output'
  title: string
  detail: string
  status: string
}

const { t } = useI18n()
const { copyText, isSupported: clipboardSupported } = useClipboard()
const router = useRouter()
const route = useRoute()
const queryCache = useQueryCache()
const selectionStore = useChatSelectionStore()
const { currentBotId } = storeToRefs(selectionStore)

const selectedBotId = ref('')
const selectedRunId = ref('')
const selectedTaskId = ref('')
const inspectedTaskId = ref('')
const runSearchQuery = ref('')
const taskSearchQuery = ref('')
const selectedInspectorTab = ref<InspectorTab>('thinking')
const expandedThinkingItemIds = ref<string[]>([])
const outputRawOpen = ref(false)
const taskTechnicalOpen = ref(false)
const expandedLogItemIds = ref<string[]>([])
const runSelectOpen = ref(false)
const inspectorOpen = ref(true)
const inspectorWidth = ref(340)
const stoppingRun = ref(false)
const resolvingCheckpointId = ref('')
const canvasZoom = ref(1)
const canvasViewportRef = ref<HTMLElement | null>(null)
const canvasWorldRef = ref<HTMLElement | null>(null)
const minimapViewportRef = ref<HTMLElement | null>(null)
const viewX = ref(0)
const viewY = ref(0)
const viewportWidth = ref(0)
const viewportHeight = ref(0)
const isMinimapDragging = ref(false)
let viewportResizeObserver: ResizeObserver | null = null
let observedCanvasViewport: HTMLElement | null = null
let panStart: { clientX: number, clientY: number, viewX: number, viewY: number } | null = null
let panFrame = 0
let pendingView: { x: number, y: number } | null = null
let previousUserSelect = ''
let bodySelectionLocked = false
let currentViewX = 0
let currentViewY = 0
let currentZoom = 1
let inspectorSelectionFrame = 0
let taskScrollFrame = 0
let zoomFrame = 0
let zoomCommitTimer = 0
let inspectorRefreshTimer = 0
let inspectorResizeStart: { clientX: number, width: number } | null = null

const minCanvasZoom = 0.35
const maxCanvasZoom = 1.5
const fitViewPadding = 64
const minInspectorWidth = 280
const maxInspectorWidth = 560

const laneWidth = 180
const laneHeaderHeight = 52
const nodeWidth = 144
const nodeHeight = 76
const rowGap = 118
const graphPaddingX = 280
const graphPaddingY = 300
const minimapMaxWidth = 132
const minimapMaxHeight = 80
const canvasPanOverscroll = 180

const { data: bots } = useQuery({
  key: () => ['orchestration-bots'],
  query: fetchBots,
})

const { data: runPage } = useQuery({
  key: () => ['orchestration-runs', selectedBotId.value],
  query: async () => {
    const { data } = await getOrchestrationBotsByBotIdRuns({
      path: { bot_id: selectedBotId.value },
      query: { limit: 100 },
      throwOnError: true,
    })
    return data as { items?: RunListItem[] }
  },
  enabled: () => selectedBotId.value.length > 0,
})

const {
  data: inspector,
  asyncStatus: inspectorStatus,
  error: inspectorError,
  refetch: refetchInspector,
} = useQuery({
  key: () => ['orchestration-inspector', selectedRunId.value],
  query: async () => {
    const { data } = await getOrchestrationRunsByRunIdInspector({
      path: { run_id: selectedRunId.value },
      throwOnError: true,
    })
    return data
  },
  enabled: () => selectedRunId.value.length > 0,
})

const inspectorErrorMessage = computed(() => {
  if (inspectorStatus.value !== 'error') return ''
  return resolveApiErrorMessage(inspectorError.value, t('orchestration.inspectorLoadFailed'))
})

const botItems = computed(() => (bots.value ?? []) as BotItem[])
const runs = computed(() => runPage.value?.items ?? [])
const selectedRunFallback = computed<RunListItem | null>(() => {
  const run = inspector.value?.run
  if (!run || !selectedRunId.value || run.id !== selectedRunId.value) return null
  return {
    id: run.id,
    goal: run.goal,
    lifecycle_status: run.lifecycle_status,
    planning_status: run.planning_status,
    root_task_id: run.root_task_id,
    terminal_reason: run.terminal_reason || '',
    created_at: run.created_at,
    updated_at: run.updated_at,
    finished_at: run.finished_at,
  } as RunListItem
})
const runsWithSelected = computed(() => {
  const items = runs.value.slice()
  const fallback = selectedRunFallback.value
  if (!fallback || items.some((item) => item.id === fallback.id)) return items
  return [fallback, ...items]
})
const filteredRuns = computed(() => {
  const q = runSearchQuery.value.trim().toLowerCase()
  if (!q) return runsWithSelected.value
  return runsWithSelected.value.filter((run) => {
    const goal = (run.goal ?? '').toLowerCase()
    const id = (run.id ?? '').toLowerCase()
    return goal.includes(q) || id.includes(q)
  })
})

const tasks = computed(() => inspector.value?.tasks ?? [])
const dependencies = computed(() => inspector.value?.dependencies ?? [])
const results = computed(() => inspector.value?.results ?? [])
const verifications = computed(() => inspector.value?.verifications ?? [])
const checkpoints = computed(() => inspector.value?.checkpoints ?? [])
const artifacts = computed(() => inspector.value?.artifacts ?? [])
const inputManifests = computed(() => inspector.value?.input_manifests ?? [])
const executionSpans = computed(() => inspector.value?.execution_spans ?? [])
const actionRecords = computed(() => inspector.value?.action_records ?? [])
const workers = computed(() => inspector.value?.workers ?? [])
const rootTaskId = computed(() => inspector.value?.run.root_task_id ?? '')
const rootHasChildren = computed(() =>
  rootTaskId.value.length > 0 && dependencies.value.some((edge) => edge.predecessor_task_id === rootTaskId.value),
)
const canvasTasks = computed(() => tasks.value)
const canvasDependencies = computed(() => dependencies.value)

watch(botItems, (items) => {
  const routeBotID = typeof route.query.bot_id === 'string' ? route.query.bot_id.trim() : ''
  const preferredBotID = routeBotID || (currentBotId.value ?? '').trim()
  if (preferredBotID && items.some((item) => item.id === preferredBotID)) {
    selectedBotId.value = preferredBotID
    return
  }
  if (selectedBotId.value && items.some((item) => item.id === selectedBotId.value)) return
  selectedBotId.value = items[0]?.id ?? ''
}, { immediate: true })

watch(runs, (items) => {
  const routeRunID = typeof route.query.run_id === 'string' ? route.query.run_id.trim() : ''
  if (routeRunID) {
    selectedRunId.value = routeRunID
    return
  }
  if (selectedRunId.value && items.some((item) => item.id === selectedRunId.value)) return
  selectedRunId.value = items[0]?.id ?? ''
}, { immediate: true })

watch(selectedBotId, (botID, previousBotID) => {
  if (!previousBotID || botID === previousBotID) return
  const routeBotID = typeof route.query.bot_id === 'string' ? route.query.bot_id.trim() : ''
  if (routeBotID === botID) return
  selectedRunId.value = ''
  runSearchQuery.value = ''
})

watch(inspector, (value, previous) => {
  if (!value) return
  const shouldInitializeCanvas =
    !previous ||
    previous.run.id !== value.run.id ||
    viewportWidth.value === 0 ||
    viewportHeight.value === 0
  const initializeCanvas = () => {
    if (!shouldInitializeCanvas) return
    nextTick(() => {
      setupCanvasViewport()
      resetCanvasView()
    })
  }
  const routeTaskID = typeof route.query.task_id === 'string' ? route.query.task_id.trim() : ''
  const visibleTaskIDs = new Set(canvasTasks.value.map((task) => task.id))
  if (routeTaskID && visibleTaskIDs.has(routeTaskID)) {
    setSelectedTask(routeTaskID, { immediateInspector: true })
    initializeCanvas()
    return
  }
  if (selectedTaskId.value && visibleTaskIDs.has(selectedTaskId.value)) {
    if (!inspectedTaskId.value || !visibleTaskIDs.has(inspectedTaskId.value)) {
      inspectedTaskId.value = selectedTaskId.value
    }
    initializeCanvas()
    return
  }
  setSelectedTask(value.run.root_task_id || canvasTasks.value[0]?.id || value.tasks[0]?.id || '', { immediateInspector: true })
  initializeCanvas()
}, { immediate: true })

watch([selectedBotId, selectedRunId], ([botID, runID]) => {
  const nextQuery: Record<string, string> = {}
  if (botID) nextQuery.bot_id = botID
  if (runID) nextQuery.run_id = runID

  const currentBotQuery = typeof route.query.bot_id === 'string' ? route.query.bot_id : ''
  const currentRunQuery = typeof route.query.run_id === 'string' ? route.query.run_id : ''
  if (
    currentBotQuery === (nextQuery.bot_id ?? '') &&
    currentRunQuery === (nextQuery.run_id ?? '')
  ) {
    return
  }
  void router.replace({ query: nextQuery })
}, { immediate: true })

watch(selectedTaskId, (id) => {
  if (!id) return
  scheduleTaskListScroll(id)
})

watch(inspectedTaskId, () => {
  expandedThinkingItemIds.value = []
  expandedLogItemIds.value = []
  outputRawOpen.value = false
  taskTechnicalOpen.value = false
})

watch(canvasViewportRef, () => {
  setupCanvasViewport()
  nextTick(resetCanvasView)
})

watch(selectedRunId, async () => {
  selectedTaskId.value = ''
  inspectedTaskId.value = ''
  selectedInspectorTab.value = 'thinking'
  await nextTick()
  resetCanvasView()
})

onMounted(() => {
  setupCanvasViewport()
  inspectorRefreshTimer = window.setInterval(() => {
    if (!selectedRunId.value || !isInspectorRunActive()) return
    void refetchInspector()
  }, 2500)
  nextTick(resetCanvasView)
})

onBeforeUnmount(() => {
  viewportResizeObserver?.disconnect()
  panStart = null
  inspectorResizeStart = null
  isMinimapDragging.value = false
  restoreBodySelectionIfDragging()
  removePanListeners()
  removeMinimapListeners()
  removeInspectorResizeListeners()
  if (panFrame) window.cancelAnimationFrame(panFrame)
  if (zoomFrame) window.cancelAnimationFrame(zoomFrame)
  if (zoomCommitTimer) window.clearTimeout(zoomCommitTimer)
  if (inspectorRefreshTimer) window.clearInterval(inspectorRefreshTimer)
  if (inspectorSelectionFrame) window.cancelAnimationFrame(inspectorSelectionFrame)
  if (taskScrollFrame) window.cancelAnimationFrame(taskScrollFrame)
})

const selectedTask = computed(() => tasks.value.find((task) => task.id === inspectedTaskId.value) ?? null)
const selectedCanvasNode = computed(() => canvasNodes.value.find((node) => node.id === inspectedTaskId.value) ?? null)
const inspectorSelectionPending = computed(() => selectedTaskId.value !== inspectedTaskId.value)
const orchestrationGridStyle = computed(() => ({
  gridTemplateColumns: inspectorOpen.value
    ? `250px minmax(0, 1fr) ${inspectorWidth.value}px`
    : '250px minmax(0, 1fr)',
}))
const selectedRunLabel = computed(() => {
  const run = runsWithSelected.value.find((item) => item.id === selectedRunId.value)
  return run ? runLabel(run) : t('orchestration.noRunsTitle')
})
const canStopRun = computed(() =>
  !!selectedRunId.value &&
  ['created', 'running', 'cancelling', 'waiting_human'].includes(String(inspector.value?.run.lifecycle_status ?? '')) &&
  inspector.value?.run.lifecycle_status !== 'cancelling',
)

const selectedTaskResults = computed(() =>
  results.value.filter((item) => String(item.task_id ?? '') === inspectedTaskId.value),
)
const selectedTaskVerifications = computed(() =>
  verifications.value.filter((item) => String(item.task_id ?? '') === inspectedTaskId.value),
)
const selectedTaskCheckpoints = computed(() =>
  checkpoints.value.filter((item) => String(item.task_id ?? '') === inspectedTaskId.value),
)
const selectedOpenCheckpoint = computed(() =>
  selectedTaskCheckpoints.value.find((item) => String(item.status ?? '') === 'open') ?? null,
)
const selectedTaskArtifacts = computed(() =>
  artifacts.value.filter((item) => String(item.task_id ?? '') === inspectedTaskId.value),
)
const selectedTaskInputManifests = computed(() =>
  inputManifests.value.filter((item) => item.task_id === inspectedTaskId.value),
)
const selectedTaskExecutionSpans = computed(() =>
  executionSpans.value.filter((item) => item.task_id === inspectedTaskId.value),
)
const selectedExecutionSpan = computed(() => {
  const spans = selectedTaskExecutionSpans.value
  if (spans.length === 0) return null
  return spans.find((item) => ['created', 'claimed', 'binding', 'running'].includes(String(item.status ?? '')))
    ?? spans[spans.length - 1]
})
const selectedTaskActionRecords = computed(() => {
  const span = selectedExecutionSpan.value
  return actionRecords.value.filter((item) => {
    if (item.task_id !== inspectedTaskId.value) return false
    if (!span) return true
    if (span.kind === 'verification') {
      return (item.verification_id && item.verification_id === span.id) || (!item.attempt_id && !item.verification_id)
    }
    return (item.attempt_id && item.attempt_id === span.id) || (!item.attempt_id && !item.verification_id)
  })
})
const selectedTaskThinkingItems = computed<ThinkingItem[]>(() => {
  const items: ThinkingItem[] = []
  let pendingOutputID = ''
  let pendingOutputContent = ''

  const flushOutput = () => {
    if (!pendingOutputID) return
    items.push({
      id: pendingOutputID,
      kind: 'output',
      title: t('orchestration.thinkingGeneratedResult'),
      detail: String(selectedTaskLatestResult.value?.summary ?? '').trim() || compactResultSummary(pendingOutputContent),
      status: 'completed',
    })
    pendingOutputID = ''
    pendingOutputContent = ''
  }

  for (const action of selectedTaskActionRecords.value) {
    if (action.tool_name === 'agent.output') {
      pendingOutputID ||= action.id || `output-${items.length}`
      pendingOutputContent += formatActivityValue(actionDisplayValue(action))
      continue
    }

    flushOutput()

    if (action.tool_name === 'agent.thinking') {
      items.push({
        id: action.id || `thinking-${items.length}`,
        kind: 'reasoning',
        title: t('orchestration.thinkingReasoning'),
        detail: activityDetail(action),
        status: action.status || 'running',
      })
      continue
    }

    items.push({
      id: action.id || `tool-${items.length}`,
      kind: 'tool',
      title: activityTitle(action),
      detail: activityDetail(action),
      status: action.status || 'running',
    })
  }

  flushOutput()
  return items.slice(-30)
})
const selectedTaskActions = computed(() =>
  selectedTaskActionRecords.value
    .filter((item) =>
      item.tool_name !== 'agent.thinking' &&
      item.tool_name !== 'agent.output',
    )
    .slice(-20),
)
const selectedTaskLatestResult = computed(() => selectedTaskResults.value[0] ?? null)

const taskLevelMap = computed(() => {
  if (!rootTaskId.value || canvasTasks.value.length <= 1) {
    return buildTaskLevels(canvasTasks.value, canvasDependencies.value)
  }

  const rootTask = canvasTasks.value.find((task) => task.id === rootTaskId.value)
  if (!rootTask) return buildTaskLevels(canvasTasks.value, canvasDependencies.value)

  const childTasks = canvasTasks.value.filter((task) => task.id !== rootTaskId.value)
  const childEdges = canvasDependencies.value.filter((edge) =>
    edge.predecessor_task_id !== rootTaskId.value &&
    edge.successor_task_id !== rootTaskId.value,
  )
  const childLevels = buildTaskLevels(childTasks, childEdges)
  const levels = new Map<string, number>([[rootTaskId.value, 0]])

  for (const task of childTasks) {
    levels.set(task.id, (childLevels.get(task.id) ?? 0) + 1)
  }

  return levels
})
const maxLevel = computed(() => Math.max(0, ...Array.from(taskLevelMap.value.values())))
const stages = computed(() =>
  Array.from({ length: maxLevel.value + 1 }, (_, index) => ({
    level: index,
    label: stageLabel(index),
  })),
)

const filteredTasks = computed(() => {
  const q = taskSearchQuery.value.trim().toLowerCase()
  if (!q) return tasks.value
  return tasks.value.filter((task) => {
    const title = compactTaskTitle(task.goal, task.id).toLowerCase()
    return title.includes(q) || task.id.toLowerCase().includes(q) || task.status.toLowerCase().includes(q)
  })
})

const graphInnerWidth = computed(() => Math.max(laneWidth, stages.value.length * laneWidth))
const graphInnerHeight = computed(() => {
  const rowsByLevel = new Map<number, number>()
  for (const task of canvasTasks.value) {
    const level = taskLevelMap.value.get(task.id) ?? 0
    rowsByLevel.set(level, (rowsByLevel.get(level) ?? 0) + 1)
  }
  const maxRows = Math.max(1, ...Array.from(rowsByLevel.values()))
  return Math.max(460, laneHeaderHeight + 80 + (maxRows - 1) * rowGap + nodeHeight + 52)
})

const canvasNodes = computed<CanvasNode[]>(() => {
  const yPositions = buildNodeYPositions(canvasTasks.value, canvasDependencies.value, taskLevelMap.value)

  return canvasTasks.value.map((task) => {
    const level = taskLevelMap.value.get(task.id) ?? 0
    const kind = inferNodeKind(task, level)

    return {
      task,
      id: task.id,
      title: compactTaskLabel(task.goal, task.id),
      subtitle: kindMeta(kind).label,
      kind,
      status: task.status,
      level,
      x: graphOriginX() + level * laneWidth + Math.max(0, (laneWidth - nodeWidth) / 2),
      y: yPositions.get(task.id) ?? graphContentTop(),
    }
  })
})

const canvasWidth = computed(() => canvasWidthFor())
const canvasHeight = computed(() => canvasHeightFor())
const zoomPercent = computed(() => `${Math.round(canvasZoom.value * 100)}%`)
const canvasTransform = computed(() =>
  viewTransform(viewX.value, viewY.value),
)
const minimapScale = computed(() => Math.min(
  minimapMaxWidth / canvasWidth.value,
  minimapMaxHeight / canvasHeight.value,
))
const minimapWidth = computed(() => canvasWidth.value * minimapScale.value)
const minimapHeight = computed(() => canvasHeight.value * minimapScale.value)
const minimapViewportStyle = computed(() => ({
  left: `${viewX.value * minimapScale.value}px`,
  top: `${viewY.value * minimapScale.value}px`,
  width: `${viewportWorldWidthFor(canvasZoom.value) * minimapScale.value}px`,
  height: `${viewportWorldHeightFor(canvasZoom.value) * minimapScale.value}px`,
}))
const minimapGraphStyle = computed(() => ({
  left: `${graphOriginX() * minimapScale.value}px`,
  top: `${graphOriginY() * minimapScale.value}px`,
  width: `${graphInnerWidth.value * minimapScale.value}px`,
  height: `${graphInnerHeight.value * minimapScale.value}px`,
}))

watch([canvasWidth, canvasHeight, canvasZoom], () => {
  constrainView()
})

const libraryItems = computed(() => [
  { kind: 'trigger' as const, label: kindMeta('trigger').label, count: canvasNodes.value.filter((node) => node.kind === 'trigger').length },
  { kind: 'llm' as const, label: kindMeta('llm').label, count: canvasNodes.value.filter((node) => node.kind === 'llm' || node.kind === 'planner').length },
  { kind: 'tool' as const, label: kindMeta('tool').label, count: canvasNodes.value.filter((node) => node.kind === 'tool' || node.kind === 'search').length },
  { kind: 'memory' as const, label: kindMeta('memory').label, count: canvasNodes.value.filter((node) => node.kind === 'memory').length },
  { kind: 'merge' as const, label: kindMeta('merge').label, count: canvasNodes.value.filter((node) => node.kind === 'merge').length },
  { kind: 'verify' as const, label: kindMeta('verify').label, count: canvasNodes.value.filter((node) => node.kind === 'verify').length },
  { kind: 'output' as const, label: kindMeta('output').label, count: canvasNodes.value.filter((node) => node.kind === 'output').length },
])

const inspectorTabs = computed(() => [
  { key: 'thinking' as const, label: t('orchestration.thinking') },
  { key: 'config' as const, label: t('orchestration.config') },
  { key: 'task' as const, label: t('orchestration.taskInfo') },
  { key: 'inputs' as const, label: t('orchestration.inputs') },
  { key: 'outputs' as const, label: t('orchestration.outputs') },
  { key: 'logs' as const, label: t('orchestration.logs') },
])

const footerMetrics = computed(() => {
  const dagTasks = canvasTasks.value
  const runningCount = dagTasks.filter((task) => ['running', 'dispatching', 'verifying'].includes(task.status)).length
  const blockedCount = dagTasks.filter((task) => ['failed', 'blocked', 'cancelled'].includes(task.status)).length
  const pendingCount = dagTasks.filter((task) =>
    !['completed', 'running', 'dispatching', 'verifying', 'failed', 'blocked', 'cancelled'].includes(task.status),
  ).length
  return [
    { key: 'tasks', label: t('orchestration.tasks'), value: dagTasks.length, icon: Library, className: 'text-primary' },
    { key: 'done', label: t('orchestration.completedTasks'), value: dagTasks.filter((task) => task.status === 'completed').length, icon: CheckCircle2, className: 'text-emerald-600' },
    { key: 'running', label: t('orchestration.runningTasks'), value: runningCount, icon: LoaderCircle, className: 'text-sky-600' },
    { key: 'blocked', label: t('orchestration.blockedTasks'), value: blockedCount, icon: AlertCircle, className: 'text-rose-600' },
    { key: 'pending', label: t('orchestration.pendingTasks'), value: pendingCount, icon: Clock3, className: 'text-muted-foreground' },
    { key: 'workers', label: t('orchestration.workers'), value: workers.value.length, icon: Hammer, className: 'text-violet-600' },
  ]
})

function nodeByID(id: string): CanvasNode | null {
  return canvasNodes.value.find((node) => node.id === id) ?? null
}

function edgePath(edge: RunInspectorDependency): string {
  const source = nodeByID(edge.predecessor_task_id)
  const target = nodeByID(edge.successor_task_id)
  if (!source || !target) return ''
  const x1 = source.x + nodeWidth
  const y1 = source.y + nodeHeight / 2
  const x2 = target.x
  const y2 = target.y + nodeHeight / 2
  const mid = Math.max(x1 + 32, (x1 + x2) / 2)
  return `M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`
}

function isEdgeActive(edge: RunInspectorDependency): boolean {
  return edge.predecessor_task_id === selectedTaskId.value || edge.successor_task_id === selectedTaskId.value
}

function isTaskRelatedToSelection(taskID: string): boolean {
  if (!selectedTaskId.value) return false
  if (taskID === selectedTaskId.value) return true
  return canvasDependencies.value.some((edge) =>
    (edge.predecessor_task_id === selectedTaskId.value && edge.successor_task_id === taskID) ||
    (edge.successor_task_id === selectedTaskId.value && edge.predecessor_task_id === taskID),
  )
}

function selectTaskFromList(task: RunInspectorTask) {
  setSelectedTask(task.id)
}

function selectTask(taskID: string) {
  setSelectedTask(taskID)
}

function selectRun(runID: string) {
  selectedRunId.value = runID
  runSelectOpen.value = false
  runSearchQuery.value = ''
}

function setSelectedTask(taskID: string, options: { immediateInspector?: boolean } = {}) {
  if (!taskID) return
  if (selectedTaskId.value === taskID && inspectedTaskId.value === taskID) return
  selectedTaskId.value = taskID
  if (options.immediateInspector) {
    if (inspectorSelectionFrame) window.cancelAnimationFrame(inspectorSelectionFrame)
    inspectorSelectionFrame = 0
    inspectedTaskId.value = taskID
    return
  }
  scheduleInspectedTask(taskID)
}

function scheduleInspectedTask(taskID: string) {
  if (inspectorSelectionFrame) window.cancelAnimationFrame(inspectorSelectionFrame)
  inspectorSelectionFrame = window.requestAnimationFrame(() => {
    inspectorSelectionFrame = window.requestAnimationFrame(() => {
      inspectorSelectionFrame = 0
      inspectedTaskId.value = taskID
    })
  })
}

function scheduleTaskListScroll(taskID: string) {
  if (taskScrollFrame) window.cancelAnimationFrame(taskScrollFrame)
  taskScrollFrame = window.requestAnimationFrame(() => {
    taskScrollFrame = window.requestAnimationFrame(() => {
      taskScrollFrame = 0
      document.getElementById(`orchestration-task-${taskID}`)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    })
  })
}

function upstreamTasks(taskID: string): RunInspectorTask[] {
  const ids = canvasDependencies.value
    .filter((edge) => edge.successor_task_id === taskID)
    .map((edge) => edge.predecessor_task_id)
  return ids.map((id) => tasks.value.find((task) => task.id === id)).filter(Boolean) as RunInspectorTask[]
}

function downstreamTasks(taskID: string): RunInspectorTask[] {
  const ids = canvasDependencies.value
    .filter((edge) => edge.predecessor_task_id === taskID)
    .map((edge) => edge.successor_task_id)
  return ids.map((id) => tasks.value.find((task) => task.id === id)).filter(Boolean) as RunInspectorTask[]
}

function graphContentTop() {
  return graphOriginY() + laneHeaderHeight + 34
}

function graphContentBottom() {
  return graphOriginY() + graphInnerHeight.value - nodeHeight - 34
}

function buildNodeYPositions(
  taskList: RunInspectorTask[],
  edges: RunInspectorDependency[],
  levels: Map<string, number>,
) {
  const positions = new Map<string, number>()
  const tasksByLevel = new Map<number, RunInspectorTask[]>()
  const incoming = new Map<string, string[]>()

  for (const task of taskList) {
    const level = levels.get(task.id) ?? 0
    tasksByLevel.set(level, [...(tasksByLevel.get(level) ?? []), task])
    incoming.set(task.id, [])
  }

  for (const edge of edges) {
    incoming.get(edge.successor_task_id)?.push(edge.predecessor_task_id)
  }

  const minY = graphContentTop()
  const maxY = Math.max(minY, graphContentBottom())
  const levelList = Array.from(tasksByLevel.keys()).sort((a, b) => a - b)

  for (const level of levelList) {
    const levelTasks = tasksByLevel.get(level) ?? []
    const proposed = levelTasks.map((task, index) => {
      const predecessorCenters = (incoming.get(task.id) ?? [])
        .map((id) => positions.get(id))
        .filter((value): value is number => typeof value === 'number')
        .map((y) => y + nodeHeight / 2)

      const preferredY = predecessorCenters.length > 0
        ? average(predecessorCenters) - nodeHeight / 2
        : minY + index * rowGap

      return { task, preferredY, index }
    }).sort((a, b) => a.preferredY - b.preferredY || a.index - b.index)

    let cursorY = minY
    for (const item of proposed) {
      const y = Math.min(maxY, Math.max(item.preferredY, cursorY))
      positions.set(item.task.id, y)
      cursorY = y + rowGap
    }
  }

  return positions
}

function average(values: number[]) {
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function stageLabel(level: number): string {
  if (level === 0 && rootHasChildren.value) return t('orchestration.stageRootGoal')
  const count = canvasTasks.value.filter((task) => (taskLevelMap.value.get(task.id) ?? 0) === level).length
  return t('orchestration.stageTaskCount', { count })
}

function inferNodeKind(task: RunInspectorTask, level: number): NodeKind {
  const text = `${task.goal} ${task.worker_profile ?? ''}`.toLowerCase()
  if (rootTaskId.value === task.id || level === 0) return 'trigger'
  if (task.status === 'verifying' || verifications.value.some((item) => String(item.task_id ?? '') === task.id)) return 'verify'
  if (text.includes('search') || text.includes('web')) return 'search'
  if (text.includes('memory') || text.includes('blackboard')) return 'memory'
  if (text.includes('merge') || text.includes('aggregate') || text.includes('combine')) return 'merge'
  if (level === maxLevel.value || text.includes('output') || text.includes('deliver') || text.includes('final')) return 'output'
  if (text.includes('plan') || text.includes('decompose')) return 'planner'
  if (text.includes('tool') || text.includes('api') || text.includes('exec')) return 'tool'
  return 'llm'
}

function statusMeta(status: string): { label: string, icon: LucideIcon, dot: string, chip: string, task: string } {
  switch (status) {
    case 'completed':
      return {
        label: t('orchestration.statusSuccess'),
        icon: CheckCircle2,
        dot: 'bg-emerald-500',
        chip: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
        task: 'bg-background',
      }
    case 'running':
    case 'dispatching':
    case 'verifying':
      return {
        label: status === 'verifying' ? t('orchestration.verifyingTasks') : t('orchestration.runningTasks'),
        icon: LoaderCircle,
        dot: 'bg-sky-500',
        chip: 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300',
        task: 'border-sky-500/30 bg-sky-500/8',
      }
    case 'waiting_human':
      return {
        label: t('orchestration.statusWaiting'),
        icon: Clock3,
        dot: 'bg-amber-500',
        chip: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        task: 'border-amber-500/30 bg-amber-500/8',
      }
    case 'failed':
    case 'blocked':
    case 'cancelled':
      return {
        label: status || t('orchestration.blockedTasks'),
        icon: AlertCircle,
        dot: 'bg-rose-500',
        chip: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300',
        task: 'border-rose-500/30 bg-rose-500/8',
      }
    default:
      return {
        label: status || t('orchestration.pendingTasks'),
        icon: Clock3,
        dot: 'bg-muted-foreground',
        chip: 'border-border bg-muted/70 text-muted-foreground',
        task: 'border-border bg-muted/30',
      }
  }
}

function kindMeta(kind: NodeKind): { label: string, icon: LucideIcon, color: string } {
  switch (kind) {
    case 'trigger':
      return { label: t('orchestration.nodeKindTrigger'), icon: PlayCircle, color: 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20' }
    case 'llm':
      return { label: t('orchestration.nodeKindLlm'), icon: Bot, color: 'text-violet-600 bg-violet-500/10 border-violet-500/20' }
    case 'planner':
      return { label: t('orchestration.nodeKindPlanner'), icon: Sparkles, color: 'text-purple-600 bg-purple-500/10 border-purple-500/20' }
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
    default:
      return { label: t('orchestration.nodeKindTool'), icon: Wrench, color: 'text-amber-600 bg-amber-500/10 border-amber-500/20' }
  }
}

function runLabel(run: RunListItem): string {
  return compactTaskTitle(run.goal?.trim() || run.id, run.id).replace(/\s+/g, ' ')
}

function actionDisplayValue(action: Record<string, unknown>) {
  const output = action.output_payload as Record<string, unknown> | null | undefined
  if (output && typeof output === 'object' && typeof output.delta === 'string') {
    return output.delta
  }
  return action.output_payload ?? action.error_payload ?? action.summary
}

function activityTitle(action: Record<string, unknown>) {
  const toolName = String(action.tool_name || action.action_kind || '').trim()
  switch (toolName) {
    case 'read':
      return t('orchestration.thinkingRead')
    case 'list':
      return t('orchestration.thinkingListed')
    case 'exec':
      return t('orchestration.thinkingRanCommand')
    default:
      return t('orchestration.thinkingUsedTool')
  }
}

function activityDetail(action: Record<string, unknown>) {
  if (action.tool_name === 'agent.output') {
    return String(selectedTaskLatestResult.value?.summary ?? action.summary ?? '').trim()
  }
  const value = actionDisplayValue(action)
  const fullValue = formatActivityValue(value)
  if (fullValue) return fullValue
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    for (const key of ['path', 'command', 'cmd', 'summary', 'message', 'text']) {
      const item = record[key]
      if (typeof item === 'string' && item.trim()) return item
    }
  }
  const summary = String(action.summary || '').trim()
  if (summary) return summary
  return ''
}

function formatActivityValue(value: unknown) {
  if (value == null) return ''
  if (typeof value !== 'string') return formatJsonValue(value)

  const trimmed = value.trim()
  if (!trimmed) return ''
  if (
    (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'))
  ) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    }
    catch {
      return value
    }
  }
  return value
}

function toggleThinkingItem(id: string) {
  expandedThinkingItemIds.value = expandedThinkingItemIds.value.includes(id)
    ? expandedThinkingItemIds.value.filter((item) => item !== id)
    : [...expandedThinkingItemIds.value, id]
}

function isThinkingItemExpanded(id: string) {
  return expandedThinkingItemIds.value.includes(id)
}

function toggleLogItem(id: string) {
  expandedLogItemIds.value = expandedLogItemIds.value.includes(id)
    ? expandedLogItemIds.value.filter((item) => item !== id)
    : [...expandedLogItemIds.value, id]
}

function isLogItemExpanded(id: string) {
  return expandedLogItemIds.value.includes(id)
}

function actionDetailText(action: Record<string, unknown>) {
  return formatActivityValue(actionDisplayValue(action) ?? action.summary)
}

function logItemKey(action: Record<string, unknown>, index: number) {
  return String(action.id || action.tool_call_id || `${action.tool_name || action.action_kind || 'action'}-${index}`)
}

function spanTitle(span: RunInspectorExecutionSpan, index: number) {
  if (span.kind === 'verification') return t('orchestration.validationRun')
  return t('orchestration.executionAttempt', { count: span.attempt_no ?? index + 1 })
}

function spanSummary(span: RunInspectorExecutionSpan) {
  return compactResultSummary(span.summary || span.terminal_reason || span.failure_class || t('orchestration.noExecutionSummary'))
}

function hasObjectValue(value: unknown) {
  return !!value && typeof value === 'object' && Object.keys(value as Record<string, unknown>).length > 0
}

function newIdempotencyKey(prefix: string) {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${random}`
}

async function stopCurrentRun() {
  if (!selectedRunId.value || stoppingRun.value || !canStopRun.value) return
  stoppingRun.value = true
  try {
    await postOrchestrationRunsByRunIdCancel({
      path: { run_id: selectedRunId.value },
      body: { idempotency_key: newIdempotencyKey('cancel-run') },
      throwOnError: true,
    })
    toast.success(t('orchestration.runStopRequested'))
    await refetchInspector()
    if (selectedBotId.value) {
      queryCache.invalidateQueries({ key: ['orchestration-runs', selectedBotId.value] })
    }
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('orchestration.runStopFailed')))
  } finally {
    stoppingRun.value = false
  }
}

async function resolveCheckpointOption(checkpointId: string, optionId: string) {
  if (!checkpointId || !optionId || resolvingCheckpointId.value) return
  resolvingCheckpointId.value = checkpointId
  try {
    await postOrchestrationCheckpointsByCheckpointIdResolve({
      path: { checkpoint_id: checkpointId },
      body: {
        mode: 'select_option',
        option_id: optionId,
        idempotency_key: newIdempotencyKey('resolve-checkpoint'),
      },
      throwOnError: true,
    })
    toast.success(t('orchestration.runRefresh'))
    await refetchInspector()
    if (selectedBotId.value) {
      queryCache.invalidateQueries({ key: ['orchestration-runs', selectedBotId.value] })
    }
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('orchestration.inspectorLoadFailed')))
  } finally {
    resolvingCheckpointId.value = ''
  }
}

function refreshInspector() {
  if (selectedBotId.value) {
    queryCache.invalidateQueries({ key: ['orchestration-runs', selectedBotId.value] })
  }
  if (selectedRunId.value) {
    queryCache.invalidateQueries({ key: ['orchestration-inspector', selectedRunId.value] })
  }
}

function isInspectorRunActive() {
  const status = String(inspector.value?.run.lifecycle_status ?? '')
  return status === 'created' || status === 'running' || status === 'cancelling' || status === 'waiting_human'
}

function setCanvasZoom(value: number) {
  currentZoom = clampZoom(value)
  canvasZoom.value = currentZoom
  nextTick(constrainView)
}

function zoomCanvas(delta: number) {
  applyZoom(currentZoom + delta, { commit: true })
}

function onViewportWheel(event: WheelEvent) {
  const delta = event.deltaY > 0 ? -0.08 : 0.08
  applyZoom(currentZoom + delta, { commit: false })
}

function resetCanvasView() {
  setupCanvasViewport()
  setCanvasZoom(1)
  nextTick(centerGraphView)
}

function fitCanvasView() {
  updateViewportSize()
  const widthRatio = (viewportWidth.value - fitViewPadding) / graphInnerWidth.value
  const heightRatio = (viewportHeight.value - fitViewPadding) / graphInnerHeight.value
  setCanvasZoom(clampZoom(Math.min(widthRatio, heightRatio)))
  nextTick(centerGraphView)
}

async function toggleCanvasFullscreen() {
  const el = canvasViewportRef.value
  if (!el) return
  if (document.fullscreenElement) {
    await document.exitFullscreen()
    return
  }
  await el.requestFullscreen()
  await new Promise((resolve) => window.setTimeout(resolve, 80))
  fitCanvasView()
}

function setupCanvasViewport() {
  const el = canvasViewportRef.value
  if (!el) return
  if (observedCanvasViewport === el) {
    updateViewportSize()
    return
  }
  viewportResizeObserver?.disconnect()
  observedCanvasViewport = el
  viewportResizeObserver = new ResizeObserver(() => {
    updateViewportSize()
    constrainView()
  })
  viewportResizeObserver.observe(el)
  updateViewportSize()
}

function updateViewportSize() {
  const el = canvasViewportRef.value
  if (!el) return
  viewportWidth.value = el.clientWidth
  viewportHeight.value = el.clientHeight
}

function constrainView() {
  updateViewportSize()
  const next = clampView(viewX.value, viewY.value, currentZoom)
  setViewPosition(next.x, next.y)
}

function clampZoom(value: number) {
  return Math.min(maxCanvasZoom, Math.max(minCanvasZoom, value))
}

function viewportWorldWidthFor(zoom: number) {
  return (viewportWidth.value || canvasWidth.value) / zoom
}

function viewportWorldHeightFor(zoom: number) {
  return (viewportHeight.value || canvasHeight.value) / zoom
}

function canvasWidthFor() {
  return graphInnerWidth.value + graphWorldPaddingX() * 2
}

function canvasHeightFor() {
  return graphInnerHeight.value + graphWorldPaddingY() * 2
}

function graphWorldPaddingX() {
  return Math.max(graphPaddingX, (viewportWidth.value || 1120) / minCanvasZoom / 2 + canvasPanOverscroll)
}

function graphWorldPaddingY() {
  return Math.max(graphPaddingY, (viewportHeight.value || 700) / minCanvasZoom / 2 + canvasPanOverscroll)
}

function graphOriginX() {
  return graphWorldPaddingX()
}

function graphOriginY() {
  return graphWorldPaddingY()
}

function clampView(x: number, y: number, zoom = currentZoom) {
  const visibleWidth = viewportWorldWidthFor(zoom)
  const visibleHeight = viewportWorldHeightFor(zoom)
  const maxX = canvasWidthFor() - visibleWidth
  const maxY = canvasHeightFor() - visibleHeight
  const clampAxis = (value: number, max: number) => {
    if (max < 0) {
      return max / 2
    }
    return Math.min(max, Math.max(0, value))
  }
  const result = {
    x: clampAxis(x, maxX),
    y: clampAxis(y, maxY),
  }
  return result
}

function viewTransform(x: number, y: number, zoom = canvasZoom.value) {
  return `translate3d(${-x * zoom}px, ${-y * zoom}px, 0) scale(${zoom})`
}

function setCurrentView(x: number, y: number, zoom = currentZoom) {
  currentViewX = x
  currentViewY = y
  currentZoom = zoom
}

function renderCurrentViewToDom() {
  if (canvasWorldRef.value) {
    canvasWorldRef.value.style.transform = viewTransform(currentViewX, currentViewY, currentZoom)
  }
  if (minimapViewportRef.value) {
    minimapViewportRef.value.style.left = `${currentViewX * minimapScale.value}px`
    minimapViewportRef.value.style.top = `${currentViewY * minimapScale.value}px`
    minimapViewportRef.value.style.width = `${viewportWorldWidthFor(currentZoom) * minimapScale.value}px`
    minimapViewportRef.value.style.height = `${viewportWorldHeightFor(currentZoom) * minimapScale.value}px`
  }
}

function applyViewToDom(x: number, y: number, zoom = currentZoom) {
  setCurrentView(x, y, zoom)
  renderCurrentViewToDom()
}

function setViewPosition(x: number, y: number) {
  const next = clampView(x, y, currentZoom)
  applyViewToDom(next.x, next.y, currentZoom)
  viewX.value = next.x
  viewY.value = next.y
}

function scheduleViewPosition(x: number, y: number) {
  pendingView = clampView(x, y, currentZoom)
  if (panFrame) return
  panFrame = window.requestAnimationFrame(() => {
    panFrame = 0
    if (!pendingView) return
    applyViewToDom(pendingView.x, pendingView.y)
    pendingView = null
  })
}

function applyZoom(value: number, options: { commit: boolean }) {
  updateViewportSize()
  const nextZoom = clampZoom(Number(value.toFixed(2)))
  if (nextZoom === currentZoom) return

  const centerX = currentViewX + viewportWorldWidthFor(currentZoom) / 2
  const centerY = currentViewY + viewportWorldHeightFor(currentZoom) / 2
  const next = clampView(
    centerX - viewportWorldWidthFor(nextZoom) / 2,
    centerY - viewportWorldHeightFor(nextZoom) / 2,
    nextZoom,
  )

  if (zoomFrame) window.cancelAnimationFrame(zoomFrame)
  setCurrentView(next.x, next.y, nextZoom)
  zoomFrame = window.requestAnimationFrame(() => {
    zoomFrame = 0
    renderCurrentViewToDom()
  })

  if (options.commit) {
    commitZoomState(next.x, next.y, nextZoom)
    return
  }

  if (zoomCommitTimer) window.clearTimeout(zoomCommitTimer)
  zoomCommitTimer = window.setTimeout(() => {
    zoomCommitTimer = 0
    commitZoomState(currentViewX, currentViewY, currentZoom)
  }, 120)
}

function commitZoomState(x: number, y: number, zoom: number) {
  if (zoomFrame) {
    window.cancelAnimationFrame(zoomFrame)
    zoomFrame = 0
  }
  if (zoomCommitTimer) {
    window.clearTimeout(zoomCommitTimer)
    zoomCommitTimer = 0
  }
  const nextZoom = clampZoom(zoom)
  const next = clampView(x, y, nextZoom)
  applyViewToDom(next.x, next.y, nextZoom)
  canvasZoom.value = nextZoom
  viewX.value = next.x
  viewY.value = next.y
}

function centerGraphView() {
  setViewPosition(defaultGraphViewX(), defaultGraphViewY())
}

function flushPendingViewPosition() {
  if (panFrame) {
    window.cancelAnimationFrame(panFrame)
    panFrame = 0
  }
  if (!pendingView) return
  applyViewToDom(pendingView.x, pendingView.y)
  pendingView = null
}

function defaultGraphViewX() {
  const visibleWidth = viewportWorldWidthFor(currentZoom)
  const graphCenterX = graphOriginX() + graphInnerWidth.value / 2
  return graphCenterX - visibleWidth / 2
}

function defaultGraphViewY() {
  const visibleHeight = viewportWorldHeightFor(currentZoom)
  const graphCenterY = graphOriginY() + graphInnerHeight.value / 2
  return graphCenterY - visibleHeight / 2
}

function startViewportPan(event: PointerEvent) {
  if (event.button !== 0) return
  if ((event.target as HTMLElement).closest('button,input,select,textarea')) return
  const target = event.currentTarget as HTMLElement
  target.classList.remove('cursor-grab')
  target.classList.add('cursor-grabbing')
  target.setPointerCapture?.(event.pointerId)
  lockBodySelection()
  panStart = {
    clientX: event.clientX,
    clientY: event.clientY,
    viewX: currentViewX,
    viewY: currentViewY,
  }
  window.addEventListener('pointermove', onViewportPan)
  window.addEventListener('pointerup', stopViewportPan)
}

function onViewportPan(event: PointerEvent) {
  if (!panStart) return
  const nextX = panStart.viewX - (event.clientX - panStart.clientX) / currentZoom
  const nextY = panStart.viewY - (event.clientY - panStart.clientY) / currentZoom
  scheduleViewPosition(
    nextX,
    nextY,
  )
}

function stopViewportPan() {
  canvasViewportRef.value?.classList.remove('cursor-grabbing')
  canvasViewportRef.value?.classList.add('cursor-grab')
  panStart = null
  flushPendingViewPosition()
  setViewPosition(currentViewX, currentViewY)
  restoreBodySelectionIfDragging()
  removePanListeners()
}

function removePanListeners() {
  window.removeEventListener('pointermove', onViewportPan)
  window.removeEventListener('pointerup', stopViewportPan)
}

function startMinimapPan(event: PointerEvent) {
  if (event.button !== 0) return
  isMinimapDragging.value = true
  moveViewFromMinimap(event)
  window.addEventListener('pointermove', moveViewFromMinimap)
  window.addEventListener('pointerup', stopMinimapPan)
}

function moveViewFromMinimap(event: PointerEvent) {
  const minimap = document.getElementById('orchestration-minimap-plane')
  const rect = minimap?.getBoundingClientRect()
  if (!rect) return
  const worldX = (event.clientX - rect.left) / minimapScale.value
  const worldY = (event.clientY - rect.top) / minimapScale.value
  scheduleViewPosition(
    worldX - viewportWorldWidthFor(currentZoom) / 2,
    worldY - viewportWorldHeightFor(currentZoom) / 2,
  )
}

function stopMinimapPan() {
  isMinimapDragging.value = false
  flushPendingViewPosition()
  setViewPosition(currentViewX, currentViewY)
  removeMinimapListeners()
}

function removeMinimapListeners() {
  window.removeEventListener('pointermove', moveViewFromMinimap)
  window.removeEventListener('pointerup', stopMinimapPan)
}

function startInspectorResize(event: PointerEvent) {
  if (event.button !== 0) return
  inspectorResizeStart = {
    clientX: event.clientX,
    width: inspectorWidth.value,
  }
  lockBodySelection()
  window.addEventListener('pointermove', onInspectorResize)
  window.addEventListener('pointerup', stopInspectorResize)
}

function onInspectorResize(event: PointerEvent) {
  if (!inspectorResizeStart) return
  const nextWidth = inspectorResizeStart.width + (inspectorResizeStart.clientX - event.clientX)
  inspectorWidth.value = Math.min(maxInspectorWidth, Math.max(minInspectorWidth, nextWidth))
}

function stopInspectorResize() {
  inspectorResizeStart = null
  restoreBodySelectionIfDragging()
  removeInspectorResizeListeners()
}

function removeInspectorResizeListeners() {
  window.removeEventListener('pointermove', onInspectorResize)
  window.removeEventListener('pointerup', stopInspectorResize)
}

function restoreBodySelectionIfDragging() {
  if (bodySelectionLocked && !panStart && !inspectorResizeStart) {
    document.body.style.userSelect = previousUserSelect
    bodySelectionLocked = false
  }
}

function lockBodySelection() {
  if (!bodySelectionLocked) {
    previousUserSelect = document.body.style.userSelect
    bodySelectionLocked = true
  }
  document.body.style.userSelect = 'none'
}

async function copyTextToClipboard(value: string) {
  if (!value || !clipboardSupported) return
  const ok = await copyText(value)
  if (ok) toast.success(t('common.copied'))
  else toast.error(t('orchestration.copyFailed'))
}

function buildTaskLevels(taskList: RunInspectorTask[], edges: RunInspectorDependency[]) {
  const byID = new Map(taskList.map((task) => [task.id, task]))
  const incoming = new Map<string, number>()
  const outgoing = new Map<string, string[]>()

  for (const task of taskList) {
    incoming.set(task.id, 0)
    outgoing.set(task.id, [])
  }

  for (const edge of edges) {
    if (!byID.has(edge.predecessor_task_id) || !byID.has(edge.successor_task_id)) continue
    incoming.set(edge.successor_task_id, (incoming.get(edge.successor_task_id) ?? 0) + 1)
    outgoing.get(edge.predecessor_task_id)?.push(edge.successor_task_id)
  }

  const queue = taskList
    .filter((task) => (incoming.get(task.id) ?? 0) === 0)
    .map((task) => task.id)
  const levels = new Map<string, number>()

  for (const id of queue) levels.set(id, 0)

  while (queue.length > 0) {
    const currentID = queue.shift()!
    const currentLevel = levels.get(currentID) ?? 0

    for (const nextID of outgoing.get(currentID) ?? []) {
      levels.set(nextID, Math.max(levels.get(nextID) ?? 0, currentLevel + 1))
      incoming.set(nextID, (incoming.get(nextID) ?? 0) - 1)
      if ((incoming.get(nextID) ?? 0) <= 0) queue.push(nextID)
    }
  }

  for (const task of taskList) {
    if (!levels.has(task.id)) levels.set(task.id, 0)
  }

  return levels
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-background text-foreground">
    <Empty
      v-if="botItems.length === 0"
      class="flex h-full items-center justify-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Boxes />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t('orchestration.noBotsTitle') }}</EmptyTitle>
      <EmptyDescription>{{ $t('orchestration.noBotsDescription') }}</EmptyDescription>
    </Empty>

    <Empty
      v-else-if="selectedBotId && runs.length === 0"
      class="flex h-full items-center justify-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Workflow />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t('orchestration.noRunsTitle') }}</EmptyTitle>
      <EmptyDescription>{{ $t('orchestration.noRunsDescription') }}</EmptyDescription>
    </Empty>

    <Empty
      v-else-if="inspectorStatus === 'error'"
      class="flex h-full items-center justify-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Workflow />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t('orchestration.inspectorLoadFailed') }}</EmptyTitle>
      <EmptyDescription>{{ inspectorErrorMessage }}</EmptyDescription>
    </Empty>

    <div
      v-else-if="!inspector"
      class="flex h-full items-center justify-center text-sm text-muted-foreground"
    >
      {{ inspectorStatus === 'loading' ? $t('orchestration.loadingInspector') : '--' }}
    </div>

    <template v-else>
      <header class="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border/70 px-4">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Workflow class="size-4" />
          </div>
          <div class="min-w-0">
            <h1 class="truncate text-sm font-semibold">
              {{ $t('orchestration.title') }}
            </h1>
          </div>
          <NativeSelect
            v-model="selectedBotId"
            class="ml-2 h-8 w-48 text-xs"
          >
            <option
              v-for="bot in botItems"
              :key="bot.id"
              :value="bot.id"
            >
              {{ bot.display_name || bot.id }}
            </option>
          </NativeSelect>
          <Popover v-model:open="runSelectOpen">
            <PopoverTrigger as-child>
              <Button
                variant="outline"
                class="h-8 w-64 justify-between gap-2 px-3 text-xs"
              >
                <span class="truncate">{{ selectedRunLabel }}</span>
                <ChevronDown class="size-3.5 shrink-0 text-muted-foreground" />
              </Button>
            </PopoverTrigger>
            <PopoverContent
              class="w-72 p-2"
              align="start"
            >
              <div class="relative">
                <Search class="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  v-model="runSearchQuery"
                  :placeholder="$t('orchestration.searchRunsPlaceholder')"
                  class="h-8 pl-7 text-xs"
                />
              </div>
              <div class="mt-2 max-h-72 space-y-1 overflow-y-auto">
                <button
                  v-for="run in filteredRuns"
                  :key="run.id"
                  type="button"
                  class="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-xs hover:bg-muted/60"
                  :class="run.id === selectedRunId ? 'bg-primary/8 text-primary' : ''"
                  @click="selectRun(run.id)"
                >
                  <CheckCircle2
                    class="size-3.5 shrink-0"
                    :class="run.id === selectedRunId ? 'opacity-100' : 'opacity-0'"
                  />
                  <span class="min-w-0 flex-1 truncate">{{ runLabel(run) }}</span>
                  <span class="font-mono text-[10px] text-muted-foreground">{{ shortId(run.id, 8) }}</span>
                </button>
                <div
                  v-if="filteredRuns.length === 0"
                  class="px-2 py-6 text-center text-xs text-muted-foreground"
                >
                  {{ $t('orchestration.noRunsTitle') }}
                </div>
              </div>
            </PopoverContent>
          </Popover>
          <span
            class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px]"
            :class="statusMeta(inspector.run.lifecycle_status).chip"
          >
            <span
              class="size-1.5 rounded-full"
              :class="statusMeta(inspector.run.lifecycle_status).dot"
            />
            {{ inspector.run.lifecycle_status }}
          </span>
        </div>

        <div class="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            class="h-8 gap-1.5 border-rose-500/30 px-2.5 text-xs text-rose-600 hover:bg-rose-500/10 hover:text-rose-700 disabled:border-border disabled:text-muted-foreground"
            :disabled="!canStopRun || stoppingRun"
            :title="$t('orchestration.stopRun')"
            @click="stopCurrentRun"
          >
            <LoaderCircle
              v-if="stoppingRun"
              class="size-3.5 animate-spin"
            />
            <Square
              v-else
              class="size-3.5"
            />
            <span>{{ $t('orchestration.stopRun') }}</span>
          </Button>
          <Button
            variant="outline"
            size="icon"
            class="size-8"
            :title="$t('orchestration.refresh')"
            @click="refreshInspector"
          >
            <RefreshCw class="size-3.5" />
          </Button>
        </div>
      </header>

      <div
        class="grid min-h-0 flex-1"
        :style="orchestrationGridStyle"
      >
        <aside class="flex min-h-0 flex-col border-r border-border/70 bg-muted/15">
          <div class="flex h-11 items-center justify-between border-b border-border/60 px-3">
            <p class="text-xs font-semibold">
              {{ $t('orchestration.taskList') }}
            </p>
            <span class="text-[11px] text-muted-foreground tabular-nums">
              {{ filteredTasks.length }}/{{ tasks.length }}
            </span>
          </div>

          <ScrollArea class="min-h-0 flex-1">
            <div class="space-y-2 p-3">
              <Input
                v-model="taskSearchQuery"
                :placeholder="$t('orchestration.searchTasksPlaceholder')"
                class="h-8 text-xs"
              />

              <button
                v-for="task in filteredTasks"
                :id="`orchestration-task-${task.id}`"
                :key="task.id"
                type="button"
                class="w-full rounded-lg px-2.5 py-2 text-left shadow-[0_0.6px_0.7px_hsl(var(--foreground)/0.04),0_1.8px_2.2px_-0.5px_hsl(var(--foreground)/0.05),0_5px_8px_-1px_hsl(var(--foreground)/0.06),0_14px_24px_-2px_hsl(var(--foreground)/0.07)] transition-colors hover:bg-muted/40 hover:shadow-[0_0.7px_0.8px_hsl(var(--foreground)/0.05),0_2.4px_3px_-0.5px_hsl(var(--foreground)/0.06),0_7px_11px_-1px_hsl(var(--foreground)/0.07),0_18px_32px_-2px_hsl(var(--foreground)/0.09)]"
                :class="[
                  task.id === selectedTaskId ? 'bg-primary/6 shadow-[0_0.8px_1px_hsl(var(--foreground)/0.05),0_3px_5px_-0.5px_hsl(var(--primary)/0.08),0_10px_18px_-1px_hsl(var(--primary)/0.10),0_24px_44px_-3px_hsl(var(--primary)/0.12)]' : statusMeta(task.status).task,
                  rootHasChildren && task.id === rootTaskId ? 'bg-primary/4' : '',
                ]"
                @click="selectTaskFromList(task)"
              >
                <div class="flex items-start gap-2">
                  <component
                    :is="statusMeta(task.status).icon"
                    class="mt-0.5 size-3.5 shrink-0"
                    :class="task.status === 'running' || task.status === 'dispatching' || task.status === 'verifying' ? 'animate-spin text-sky-600' : ''"
                  />
                  <div class="min-w-0 flex-1">
                    <p class="line-clamp-2 text-[11px] font-medium leading-snug">
                      {{ compactTaskTitle(task.goal, task.id) }}
                    </p>
                    <div class="mt-1 flex items-center justify-between gap-2 text-[10px] text-muted-foreground">
                      <span class="inline-flex items-center gap-1">
                        <span
                          class="size-1.5 rounded-full"
                          :class="statusMeta(task.status).dot"
                        />
                        {{ statusMeta(task.status).label }}
                      </span>
                      <span v-if="rootHasChildren && task.id === rootTaskId">L0 / {{ $t('orchestration.stageRootGoal') }}</span>
                      <span v-else>L{{ taskLevelMap.get(task.id) ?? 0 }}</span>
                    </div>
                    <p
                      v-if="rootHasChildren && task.id === rootTaskId"
                      class="mt-1 text-[10px] text-muted-foreground"
                    >
                      {{ $t('orchestration.rootTaskEntryHint') }}
                    </p>
                  </div>
                </div>
              </button>
            </div>

            <div class="border-t border-border/60 p-3">
              <div class="mb-2 flex items-center justify-between">
                <p class="text-[11px] font-semibold">
                  {{ $t('orchestration.nodeLibrary') }}
                </p>
                <Library class="size-3.5 text-muted-foreground" />
              </div>
              <div class="space-y-1.5">
                <button
                  v-for="item in libraryItems"
                  :key="item.kind"
                  type="button"
                  class="flex w-full items-center gap-2 rounded-md border border-border/70 bg-background px-2 py-1.5 text-left text-[11px] hover:bg-muted/30"
                >
                  <span
                    class="flex size-5 items-center justify-center rounded border"
                    :class="kindMeta(item.kind).color"
                  >
                    <component
                      :is="kindMeta(item.kind).icon"
                      class="size-3"
                    />
                  </span>
                  <span class="flex-1">{{ item.label }}</span>
                  <span class="text-[10px] text-muted-foreground tabular-nums">{{ item.count }}</span>
                </button>
              </div>
            </div>
          </ScrollArea>
        </aside>

        <main class="flex min-h-0 flex-col overflow-hidden">
          <div
            ref="canvasViewportRef"
            class="relative min-h-0 flex-1 touch-none overflow-hidden bg-[#fbfaf8] cursor-grab dark:bg-background"
            @pointerdown="startViewportPan"
            @wheel.prevent="onViewportWheel"
          >
            <div
              ref="canvasWorldRef"
              class="absolute left-0 top-0 origin-top-left will-change-transform"
              :style="{ width: `${canvasWidth}px`, height: `${canvasHeight}px`, transform: canvasTransform }"
            >
              <div class="absolute inset-0 bg-[radial-gradient(circle_at_1px_1px,hsl(var(--border))_1px,transparent_0)] bg-[length:24px_24px] opacity-45" />
              <div
                class="absolute flex overflow-hidden rounded-xl border border-border/70 shadow-sm"
                :style="{
                  left: `${graphOriginX()}px`,
                  top: `${graphOriginY()}px`,
                  width: `${graphInnerWidth}px`,
                  height: `${graphInnerHeight}px`,
                }"
              >
                <div
                  v-for="stage in stages"
                  :key="stage.level"
                  class="border-r border-border/50 bg-background last:border-r-0"
                  :style="{ width: `${laneWidth}px`, height: `${graphInnerHeight}px` }"
                >
                  <div class="flex h-[52px] flex-col items-center justify-center border-b border-border/50 bg-muted/20">
                    <p class="text-[10px] font-semibold">
                      L{{ stage.level }}
                    </p>
                    <p class="text-[10px] text-muted-foreground">
                      {{ stage.label }}
                    </p>
                  </div>
                </div>
              </div>

              <svg
                class="pointer-events-none absolute inset-0 z-10"
                :width="canvasWidth"
                :height="canvasHeight"
                :viewBox="`0 0 ${canvasWidth} ${canvasHeight}`"
              >
                <defs>
                  <marker
                    id="orchestration-arrow"
                    markerWidth="8"
                    markerHeight="8"
                    refX="7"
                    refY="4"
                    orient="auto"
                  >
                    <path
                      d="M 0 0 L 8 4 L 0 8 z"
                      class="fill-border"
                    />
                  </marker>
                </defs>
                <path
                  v-for="edge in canvasDependencies"
                  :key="`${edge.predecessor_task_id}-${edge.successor_task_id}`"
                  :d="edgePath(edge)"
                  fill="none"
                  marker-end="url(#orchestration-arrow)"
                  class="transition-colors"
                  :class="isEdgeActive(edge) ? 'stroke-foreground/70' : 'stroke-border'"
                  :stroke-width="isEdgeActive(edge) ? 1.8 : 1.2"
                />
              </svg>

              <button
                v-for="node in canvasNodes"
                :key="node.id"
                type="button"
                class="absolute z-20 rounded-lg border bg-card px-2 py-2 text-left shadow-[0_0.7px_0.8px_hsl(var(--foreground)/0.05),0_2.2px_2.8px_-0.5px_hsl(var(--foreground)/0.06),0_6px_10px_-1px_hsl(var(--foreground)/0.07),0_16px_28px_-2px_hsl(var(--foreground)/0.09)] transition-all hover:-translate-y-0.5 hover:shadow-[0_0.9px_1px_hsl(var(--foreground)/0.06),0_3px_4px_-0.5px_hsl(var(--foreground)/0.07),0_9px_14px_-1px_hsl(var(--foreground)/0.09),0_24px_40px_-2.5px_hsl(var(--foreground)/0.11)]"
                :style="{ left: `${node.x}px`, top: `${node.y}px`, width: `${nodeWidth}px`, minHeight: `${nodeHeight}px` }"
                :class="[
                  selectedTaskId === node.id ? 'border-primary/50 ring-2 ring-primary/15 shadow-[0_0.8px_1px_hsl(var(--foreground)/0.05),0_3px_5px_-0.5px_hsl(var(--primary)/0.08),0_10px_18px_-1px_hsl(var(--primary)/0.10),0_24px_44px_-3px_hsl(var(--primary)/0.12)]' : 'border-border/70',
                  selectedTaskId && !isTaskRelatedToSelection(node.id) ? 'opacity-45' : '',
                ]"
                @pointerdown.stop="selectTask(node.id)"
                @click.stop
              >
                <div class="flex items-start gap-2">
                  <span
                    class="flex size-6 shrink-0 items-center justify-center rounded-md border"
                    :class="kindMeta(node.kind).color"
                  >
                    <component
                      :is="kindMeta(node.kind).icon"
                      class="size-3.5"
                    />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-[11px] font-medium">{{ node.title }}</span>
                    <span class="mt-0.5 block truncate text-[10px] text-muted-foreground">{{ node.subtitle }}</span>
                  </span>
                </div>
                <div class="mt-2 flex items-center gap-1.5 text-[10px] text-muted-foreground">
                  <span
                    class="size-1.5 rounded-full"
                    :class="statusMeta(node.status).dot"
                  />
                  {{ statusMeta(node.status).label }}
                </div>
              </button>
            </div>

            <div
              id="orchestration-minimap"
              class="absolute bottom-4 left-4 z-30 rounded border border-border/70 bg-background/90 p-2 shadow-sm backdrop-blur"
              :class="isMinimapDragging ? 'cursor-grabbing' : 'cursor-pointer'"
              :style="{ width: `${minimapWidth + 16}px`, height: `${minimapHeight + 16}px` }"
              @pointerdown.stop="startMinimapPan"
            >
              <div
                id="orchestration-minimap-plane"
                class="relative overflow-hidden rounded-sm bg-primary/8"
                :style="{ width: `${minimapWidth}px`, height: `${minimapHeight}px` }"
              >
                <span
                  class="absolute rounded-sm border border-border/70 bg-background/70"
                  :style="minimapGraphStyle"
                />
                <span
                  v-for="node in canvasNodes"
                  :key="`mini-${node.id}`"
                  class="absolute rounded-sm"
                  :class="node.id === selectedTaskId ? 'bg-primary' : 'bg-primary/25'"
                  :style="{
                    left: `${node.x * minimapScale}px`,
                    top: `${node.y * minimapScale}px`,
                    width: `${nodeWidth * minimapScale}px`,
                    height: `${nodeHeight * minimapScale}px`,
                  }"
                />
                <span
                  ref="minimapViewportRef"
                  class="absolute rounded-sm border border-primary bg-primary/10"
                  :style="minimapViewportStyle"
                />
              </div>
            </div>

            <div class="absolute right-4 top-4 z-30 flex w-fit items-center rounded-md border border-border bg-background shadow-sm">
              <Button
                variant="ghost"
                size="icon"
                class="size-8 rounded-r-none"
                :title="$t('orchestration.fitView')"
                @click="fitCanvasView"
              >
                <Settings2 class="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                class="size-8 rounded-none border-l border-border"
                @click="zoomCanvas(-0.1)"
              >
                <ZoomOut class="size-3.5" />
              </Button>
              <button
                class="h-8 border-x border-border px-2 text-[11px]"
                @click="resetCanvasView"
              >
                {{ zoomPercent }}
              </button>
              <Button
                variant="ghost"
                size="icon"
                class="size-8 rounded-none"
                @click="zoomCanvas(0.1)"
              >
                <ZoomIn class="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                class="size-8 rounded-l-none border-l border-border"
                :title="$t('orchestration.fullscreen')"
                @click="toggleCanvasFullscreen"
              >
                <Maximize2 class="size-3.5" />
              </Button>
            </div>
            <Button
              v-if="!inspectorOpen"
              variant="outline"
              size="icon"
              class="absolute right-4 top-16 z-30 size-8 bg-background shadow-sm"
              :title="$t('orchestration.nodeInspector')"
              @click="inspectorOpen = true"
            >
              <ScanSearch class="size-3.5" />
            </Button>
          </div>
        </main>

        <aside
          v-if="inspectorOpen"
          class="relative flex min-h-0 flex-col border-l border-border/70 bg-card/40"
        >
          <div
            class="absolute inset-y-0 -left-1 z-20 w-2 cursor-col-resize"
            @pointerdown.prevent="startInspectorResize"
          />
          <div class="flex h-11 items-center justify-between border-b border-border/60 px-3">
            <div class="flex items-center gap-2">
              <ScanSearch class="size-3.5 text-muted-foreground" />
              <p class="text-xs font-semibold">
                {{ $t('orchestration.nodeInspector') }}
              </p>
            </div>
            <Button
              variant="ghost"
              size="icon"
              class="size-7"
              @click="inspectorOpen = false"
            >
              <X class="size-3.5" />
            </Button>
          </div>

          <ScrollArea class="min-h-0 flex-1">
            <div
              v-if="selectedTask && selectedCanvasNode"
              class="space-y-4 p-4"
              :class="inspectorSelectionPending ? 'opacity-70' : ''"
            >
              <div class="flex items-start gap-3">
                <span
                  class="flex size-10 items-center justify-center rounded-lg border"
                  :class="kindMeta(selectedCanvasNode.kind).color"
                >
                  <component
                    :is="kindMeta(selectedCanvasNode.kind).icon"
                    class="size-5"
                  />
                </span>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold">
                    {{ compactTaskTitle(selectedTask.goal, selectedTask.id) }}
                  </p>
                  <p class="text-[11px] text-muted-foreground">
                    {{ kindMeta(selectedCanvasNode.kind).label }} {{ $t('orchestration.node') }} / L{{ selectedCanvasNode.level }}
                  </p>
                </div>
                <span
                  class="rounded-full border px-2 py-0.5 text-[10px]"
                  :class="statusMeta(selectedTask.status).chip"
                >
                  {{ statusMeta(selectedTask.status).label }}
                </span>
              </div>

              <div class="flex border-b border-border/60 text-[11px]">
                <button
                  v-for="tab in inspectorTabs"
                  :key="tab.key"
                  type="button"
                  class="px-2 py-2 transition-colors"
                  :class="selectedInspectorTab === tab.key
                    ? 'border-b border-primary font-medium text-primary'
                    : 'text-muted-foreground hover:text-foreground'"
                  @click="selectedInspectorTab = tab.key"
                >
                  {{ tab.label }}
                </button>
              </div>

              <section
                v-if="selectedInspectorTab === 'thinking'"
                class="space-y-2"
              >
                <div class="max-h-[500px] overflow-y-auto pr-1">
                  <div
                    v-if="selectedTaskThinkingItems.length === 0"
                    class="text-[11px] text-muted-foreground"
                  >
                    {{ $t('orchestration.noThinking') }}
                  </div>
                  <div
                    v-else
                    class="space-y-1"
                  >
                    <div
                      v-for="item in selectedTaskThinkingItems"
                      :key="item.id"
                      class="rounded-md text-[11px]"
                    >
                      <button
                        type="button"
                        class="flex w-full items-center gap-2 rounded-md px-1 py-1 text-left hover:bg-muted/40"
                        @click="toggleThinkingItem(item.id)"
                      >
                        <ChevronDown
                          class="size-3 shrink-0 text-muted-foreground transition-transform"
                          :class="isThinkingItemExpanded(item.id) ? 'rotate-0' : '-rotate-90'"
                        />
                        <span
                          class="size-1.5 shrink-0 rounded-full"
                          :class="item.kind === 'output' ? 'bg-emerald-500' : item.kind === 'reasoning' ? 'bg-violet-500' : 'bg-muted-foreground/50'"
                        />
                        <span class="truncate text-muted-foreground">
                          <span class="font-medium text-foreground">
                            {{ item.title }}
                          </span>
                        </span>
                      </button>
                      <pre
                        v-if="isThinkingItemExpanded(item.id) && item.detail"
                        class="ml-8 mt-1 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/25 px-2 py-1.5 text-[11px] leading-relaxed text-muted-foreground"
                      >{{ item.detail }}</pre>
                    </div>
                  </div>
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'config'"
                class="space-y-2"
              >
                <p class="text-[11px] font-semibold">
                  {{ $t('orchestration.belongsToRun') }}
                </p>
                <div class="rounded-lg border border-border/70 bg-background px-3 py-2">
                  <div class="flex items-center gap-2">
                    <Search class="size-3.5 text-primary" />
                    <div class="min-w-0">
                      <p class="truncate text-[11px] font-medium">
                        {{ compactTaskTitle(inspector.run.goal, inspector.run.id) }}
                      </p>
                      <p class="font-mono text-[10px] text-muted-foreground">
                        {{ shortId(inspector.run.id, 18) }}
                      </p>
                    </div>
                  </div>
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'config'"
                class="space-y-2"
              >
                <div class="flex items-center justify-between">
                  <p class="text-[11px] font-semibold">
                    {{ $t('orchestration.description') }}
                  </p>
                  <Button
                    v-if="clipboardSupported"
                    variant="ghost"
                    size="icon"
                    class="size-6"
                    :title="$t('common.copy')"
                    @click.stop="copyTextToClipboard(selectedTask.goal)"
                  >
                    <Copy class="size-3" />
                  </Button>
                </div>
                <div class="min-h-20 rounded-lg border border-border/70 bg-background px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">
                  {{ selectedTask.goal }}
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'config'"
                class="grid grid-cols-2 gap-2 text-[11px]"
              >
                <div class="space-y-1">
                  <p class="text-muted-foreground">
                    {{ $t('orchestration.status') }}
                  </p>
                  <div class="h-8 rounded-md border border-border/70 bg-background px-2 py-1.5 text-[10px]">
                    {{ statusMeta(selectedTask.status).label }}
                  </div>
                </div>
                <div class="space-y-1">
                  <p class="text-muted-foreground">
                    {{ $t('orchestration.outputRecords') }}
                  </p>
                  <div class="h-8 rounded-md border border-border/70 bg-background px-2 py-1.5 text-[10px]">
                    {{ selectedTaskResults.length }}
                  </div>
                </div>
                <div class="space-y-1">
                  <p class="text-muted-foreground">
                    {{ $t('orchestration.validationRecords') }}
                  </p>
                  <div class="h-8 rounded-md border border-border/70 bg-background px-2 py-1.5 text-[10px]">
                    {{ selectedTaskVerifications.length }}
                  </div>
                </div>
                <div class="space-y-1">
                  <p class="text-muted-foreground">
                    {{ $t('orchestration.pendingQuestions') }}
                  </p>
                  <div class="h-8 rounded-md border border-border/70 bg-background px-2 py-1.5 text-[10px]">
                    {{ selectedTaskCheckpoints.length }}
                  </div>
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'task'"
                class="space-y-2"
              >
                <p class="text-[11px] font-semibold">
                  {{ $t('orchestration.relatedTasks') }}
                </p>
                <div class="space-y-2 rounded-lg border border-border/70 bg-background p-3 text-[11px]">
                  <div>
                    <p class="mb-1 text-muted-foreground">
                      {{ $t('orchestration.upstreamNodes') }}
                    </p>
                    <div class="flex flex-wrap gap-1">
                      <button
                        v-for="task in upstreamTasks(selectedTask.id)"
                        :key="task.id"
                        class="rounded border border-primary/20 bg-primary/8 px-2 py-0.5 text-primary"
                        @click="selectTask(task.id)"
                      >
                        {{ compactTaskLabel(task.goal, task.id) }}
                      </button>
                      <span
                        v-if="upstreamTasks(selectedTask.id).length === 0"
                        class="text-muted-foreground"
                      >--</span>
                    </div>
                  </div>
                  <div>
                    <p class="mb-1 text-muted-foreground">
                      {{ $t('orchestration.downstreamNodes') }}
                    </p>
                    <div class="flex flex-wrap gap-1">
                      <button
                        v-for="task in downstreamTasks(selectedTask.id)"
                        :key="task.id"
                        class="rounded border border-primary/20 bg-primary/8 px-2 py-0.5 text-primary"
                        @click="selectTask(task.id)"
                      >
                        {{ compactTaskLabel(task.goal, task.id) }}
                      </button>
                      <span
                        v-if="downstreamTasks(selectedTask.id).length === 0"
                        class="text-muted-foreground"
                      >--</span>
                    </div>
                  </div>
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'outputs'"
                class="space-y-3"
              >
                <div
                  v-if="selectedTaskLatestResult"
                  class="space-y-2"
                >
                  <div class="flex items-center justify-between">
                    <p class="text-[11px] font-semibold">
                      {{ $t('orchestration.resultSummary') }}
                    </p>
                    <span
                      class="rounded border px-1.5 py-0.5 text-[10px]"
                      :class="statusMeta(selectedTaskLatestResult.status || '').chip"
                    >
                      {{ statusMeta(selectedTaskLatestResult.status || '').label }}
                    </span>
                  </div>
                  <div class="rounded-lg border border-border/70 bg-background px-3 py-2 text-[11px] leading-relaxed">
                    <p class="text-foreground">
                      {{ selectedTaskLatestResult.summary || '--' }}
                    </p>
                  </div>
                  <div
                    v-if="hasObjectValue(selectedTaskLatestResult.structured_output)"
                    class="space-y-1"
                  >
                    <p class="text-[11px] font-semibold">
                      {{ $t('orchestration.resultDetails') }}
                    </p>
                    <pre class="max-h-56 overflow-auto rounded-lg border border-border/70 bg-background px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">{{ formatJsonValue(selectedTaskLatestResult.structured_output) }}</pre>
                  </div>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-md px-1 py-1 text-left text-[11px] text-muted-foreground hover:bg-muted/40"
                    @click="outputRawOpen = !outputRawOpen"
                  >
                    <ChevronDown
                      class="size-3 shrink-0 transition-transform"
                      :class="outputRawOpen ? 'rotate-0' : '-rotate-90'"
                    />
                    <span>{{ $t('orchestration.technicalDetails') }}</span>
                  </button>
                  <pre
                    v-if="outputRawOpen"
                    class="max-h-96 overflow-auto rounded-lg border border-border/70 bg-background px-3 py-2 text-[11px] leading-relaxed text-muted-foreground"
                  >{{ formatJsonValue(selectedTaskLatestResult) }}</pre>
                </div>
                <div
                  v-else
                  class="rounded-lg border border-border/70 bg-background px-3 py-2 text-[11px] leading-relaxed text-muted-foreground"
                >
                  --
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'logs'"
                class="space-y-4"
              >
                <section class="space-y-2">
                  <div class="flex items-center justify-between">
                    <p class="text-[11px] font-semibold">
                      {{ $t('orchestration.execution') }}
                    </p>
                    <span class="text-[10px] text-muted-foreground tabular-nums">
                      {{ selectedTaskExecutionSpans.length }}
                    </span>
                  </div>
                  <div class="space-y-1.5">
                    <div
                      v-for="(span, spanIndex) in selectedTaskExecutionSpans"
                      :key="span.id"
                      class="rounded-lg border border-border/70 bg-background p-2 text-[11px]"
                    >
                      <div class="flex items-center justify-between gap-2">
                        <span class="truncate font-medium text-foreground">{{ spanTitle(span, spanIndex) }}</span>
                        <span
                          class="rounded border px-1 py-px text-[10px]"
                          :class="statusMeta(span.status || '').chip"
                        >{{ statusMeta(span.status || '').label }}</span>
                      </div>
                      <p class="mt-1 line-clamp-2 text-foreground">
                        {{ spanSummary(span) }}
                      </p>
                    </div>
                    <p
                      v-if="selectedTaskExecutionSpans.length === 0"
                      class="rounded border border-dashed border-border/60 bg-background/60 px-2 py-2 text-[11px] text-muted-foreground"
                    >
                      {{ $t('orchestration.noExecutionSpans') }}
                    </p>
                  </div>
                </section>

                <section
                  v-if="selectedTaskActions.length"
                  class="space-y-2"
                >
                  <div class="flex items-center justify-between">
                    <p class="text-[11px] font-semibold">
                      {{ $t('orchestration.toolCalls') }}
                    </p>
                    <span class="text-[10px] text-muted-foreground tabular-nums">
                      {{ selectedTaskActions.length }}
                    </span>
                  </div>
                  <div class="space-y-1">
                    <div
                      v-for="(action, actionIndex) in selectedTaskActions"
                      :key="logItemKey(action, actionIndex)"
                      class="rounded-md text-[11px]"
                    >
                      <button
                        type="button"
                        class="flex w-full items-center gap-2 rounded-md px-1 py-1 text-left hover:bg-muted/40"
                        @click="toggleLogItem(logItemKey(action, actionIndex))"
                      >
                        <ChevronDown
                          class="size-3 shrink-0 text-muted-foreground transition-transform"
                          :class="isLogItemExpanded(logItemKey(action, actionIndex)) ? 'rotate-0' : '-rotate-90'"
                        />
                        <span
                          class="size-1.5 shrink-0 rounded-full"
                          :class="statusMeta(action.status || '').dot"
                        />
                        <span class="min-w-0 flex-1 truncate">
                          <span class="font-medium text-foreground">{{ activityTitle(action) }}</span>
                          <span class="ml-1 text-muted-foreground">{{ compactResultSummary(action.summary || '') }}</span>
                        </span>
                      </button>
                      <pre
                        v-if="isLogItemExpanded(logItemKey(action, actionIndex))"
                        class="ml-8 mt-1 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/25 px-2 py-1.5 text-[11px] leading-relaxed text-muted-foreground"
                      >{{ actionDetailText(action) }}</pre>
                    </div>
                  </div>
                </section>
              </section>

              <section
                v-if="selectedInspectorTab === 'inputs'"
                class="space-y-2"
              >
                <p class="text-[11px] font-semibold">
                  {{ $t('orchestration.artifactsInputs') }}
                </p>
                <div class="rounded-lg border border-border/70 bg-background p-3 text-[11px] text-muted-foreground">
                  <div class="flex justify-between">
                    <span>{{ $t('orchestration.inputManifests') }}</span>
                    <span>{{ selectedTaskInputManifests.length }}</span>
                  </div>
                  <div class="mt-1 flex justify-between">
                    <span>{{ $t('orchestration.artifacts') }}</span>
                    <span>{{ selectedTaskArtifacts.length }}</span>
                  </div>
                  <div
                    v-if="selectedTaskCheckpoints.length > 0"
                    class="mt-2 border-t border-border/60 pt-2"
                  >
                    <p class="text-foreground">
                      {{ $t('orchestration.checkpoint') }}
                    </p>
                    <p class="mt-0.5 line-clamp-2">
                      {{ String(selectedTaskCheckpoints[0]?.question ?? selectedTask.waiting_checkpoint_id ?? '--') }}
                    </p>
                    <div
                      v-if="selectedOpenCheckpoint && selectedOpenCheckpoint.options?.length"
                      class="mt-2 flex flex-wrap gap-1.5"
                    >
                      <Button
                        v-for="option in selectedOpenCheckpoint.options"
                        :key="option.id"
                        size="sm"
                        variant="outline"
                        class="h-7 px-2 text-[11px]"
                        :disabled="resolvingCheckpointId === selectedOpenCheckpoint.id"
                        @click="resolveCheckpointOption(selectedOpenCheckpoint.id, option.id)"
                      >
                        {{ option.label || option.id }}
                      </Button>
                    </div>
                  </div>
                </div>
              </section>

              <section
                v-if="selectedInspectorTab === 'task'"
                class="space-y-2 text-[11px] text-muted-foreground"
              >
                <button
                  type="button"
                  class="flex w-full items-center gap-2 rounded-md px-1 py-1 text-left hover:bg-muted/40"
                  @click="taskTechnicalOpen = !taskTechnicalOpen"
                >
                  <ChevronDown
                    class="size-3 shrink-0 transition-transform"
                    :class="taskTechnicalOpen ? 'rotate-0' : '-rotate-90'"
                  />
                  <span>{{ $t('orchestration.technicalDetails') }}</span>
                </button>
                <div
                  v-if="taskTechnicalOpen"
                  class="space-y-1 rounded-lg bg-muted/20 px-3 py-2"
                >
                  <div class="flex justify-between gap-3">
                    <span>{{ $t('orchestration.createdAt') }}</span>
                    <span>{{ formatDate(selectedTask.created_at) }}</span>
                  </div>
                  <div class="flex justify-between gap-3">
                    <span>{{ $t('orchestration.updatedAt') }}</span>
                    <span>{{ formatDate(selectedTask.updated_at) }}</span>
                  </div>
                  <div class="flex justify-between gap-3">
                    <span>{{ $t('orchestration.taskId') }}</span>
                    <span class="font-mono">{{ shortId(selectedTask.id, 18) }}</span>
                  </div>
                  <div class="flex justify-between gap-3">
                    <span>{{ $t('orchestration.worker') }}</span>
                    <span class="truncate font-mono">{{ compactWorker(selectedTask.worker_profile) }}</span>
                  </div>
                  <div
                    v-if="selectedExecutionSpan"
                    class="flex justify-between gap-3"
                  >
                    <span>{{ $t('orchestration.heartbeat') }}</span>
                    <span>{{ formatDate((selectedExecutionSpan as RunInspectorExecutionSpan).last_heartbeat_at) }}</span>
                  </div>
                </div>
              </section>
            </div>

            <div
              v-else
              class="flex min-h-80 items-center justify-center px-6 text-center text-xs text-muted-foreground"
            >
              {{ $t('orchestration.noTaskSelected') }}
            </div>
          </ScrollArea>
        </aside>
      </div>

      <footer class="grid h-12 shrink-0 grid-cols-6 border-t border-border/70 bg-background text-[11px]">
        <div
          v-for="metric in footerMetrics"
          :key="metric.key"
          class="flex items-center gap-2 border-r border-border/60 px-4 last:border-r-0"
        >
          <component
            :is="metric.icon"
            class="size-3.5"
            :class="metric.className"
          />
          <span class="text-muted-foreground">{{ metric.label }}</span>
          <span class="font-semibold tabular-nums">{{ metric.value }}</span>
        </div>
      </footer>
    </template>
  </div>
</template>
