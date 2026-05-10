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
import {
  getOrchestrationRuns,
  postOrchestrationCheckpointsByCheckpointIdResolve,
  getOrchestrationRunsByRunIdInspector,
  postOrchestrationRunsByRunIdCancel,
} from '@memohai/sdk'
import type { ToolCallBlock } from '@/store/chat-list'
import ToolCallInline from '@/pages/home/components/tool-call-inline.vue'
import { useClipboard } from '@/composables/useClipboard'
import { fetchBots } from '@/composables/api/useChat.chat-api'
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
import { useRunEventStream } from './composables/use-run-event-stream'

type NodeKind = 'trigger' | 'llm' | 'planner' | 'search' | 'tool' | 'memory' | 'merge' | 'verify' | 'output'
type InspectorTab = 'act' | 'config' | 'env' | 'task' | 'inputs' | 'outputs' | 'logs'

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

interface EnvSnapshotItem {
  id: string
  kind: string
  actionKind: string
  createdAt: string
}

const { t } = useI18n()
const { copyText, isSupported: clipboardSupported } = useClipboard()
const router = useRouter()
const route = useRoute()
const queryCache = useQueryCache()

const selectedRunId = ref('')
const selectedBotId = ref('')
const selectedTaskId = ref('')
const inspectedTaskId = ref('')
const botSearchQuery = ref('')
const runSearchQuery = ref('')
const taskSearchQuery = ref('')
const selectedInspectorTab = ref<InspectorTab>('act')
const outputRawOpen = ref(false)
const taskTechnicalOpen = ref(false)
const expandedLogItemIds = ref<string[]>([])
const botSelectOpen = ref(false)
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
let actRefreshTimer = 0
let inspectorResizeStart: { clientX: number, width: number } | null = null

const minCanvasZoom = 0.65
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

const { data: runPage } = useQuery({
  key: () => ['orchestration-runs'],
  query: async () => {
    const { data } = await getOrchestrationRuns({
      query: { limit: 100 },
      throwOnError: true,
    })
    return data as { items?: RunListItem[] }
  },
})

const { data: bots } = useQuery({
  key: () => ['orchestration-bots'],
  query: fetchBots,
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
const allRuns = computed(() => runPage.value?.items ?? [])
const runBotIDs = computed(() => new Set(allRuns.value.map((run) => run.bot_id).filter(Boolean)))
const runBotItems = computed(() => botItems.value.filter((bot) => bot.id && runBotIDs.value.has(bot.id)))
const selectedBotLabel = computed(() => {
  const bot = runBotItems.value.find((item) => item.id === selectedBotId.value)
  return bot?.display_name || bot?.id || t('orchestration.noRunsTitle')
})
const filteredBots = computed(() => {
  const q = botSearchQuery.value.trim().toLowerCase()
  if (!q) return runBotItems.value
  return runBotItems.value.filter((bot) => {
    const name = (bot.display_name ?? '').toLowerCase()
    const id = (bot.id ?? '').toLowerCase()
    return name.includes(q) || id.includes(q)
  })
})
const runs = computed(() => {
  const items = allRuns.value
  if (!selectedBotId.value) return []
  return items.filter((run) => run.bot_id === selectedBotId.value)
})
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
const rootTaskId = computed(() => inspector.value?.run.root_task_id ?? '')
const rootHasChildren = computed(() =>
  rootTaskId.value.length > 0 && dependencies.value.some((edge) => edge.predecessor_task_id === rootTaskId.value),
)
const canvasTasks = computed(() => tasks.value)
const canvasDependencies = computed(() => dependencies.value)

watch(runs, (items) => {
  const routeRunID = typeof route.query.run_id === 'string' ? route.query.run_id.trim() : ''
  if (routeRunID && items.some((item) => item.id === routeRunID)) {
    selectedRunId.value = routeRunID
    return
  }
  if (selectedRunId.value && items.some((item) => item.id === selectedRunId.value)) return
  selectedRunId.value = items[0]?.id ?? ''
}, { immediate: true })

watch(runBotItems, (items) => {
  const routeBotID = typeof route.query.bot_id === 'string' ? route.query.bot_id.trim() : ''
  if (routeBotID && items.some((item) => item.id === routeBotID)) {
    selectedBotId.value = routeBotID
    return
  }
  if (selectedBotId.value && items.some((item) => item.id === selectedBotId.value)) return
  selectedBotId.value = items[0]?.id ?? ''
}, { immediate: true })

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
  selectedInspectorTab.value = 'act'
  await nextTick()
  resetCanvasView()
})

// SSE-driven refresh. While the watch stream is open we skip the poll loop
// entirely; the timer below is just a safety net for when the bus is down,
// so it ticks slowly on purpose.
let streamRefetchTimer: number | null = null
function scheduleInspectorRefetch() {
  if (streamRefetchTimer !== null) return
  streamRefetchTimer = window.setTimeout(() => {
    streamRefetchTimer = null
    if (!selectedRunId.value) return
    void refetchInspector()
  }, 250)
}

const isStreamEnabled = computed(() => Boolean(selectedRunId.value) && isInspectorRunActive())

const { status: runEventStreamStatus } = useRunEventStream({
  runId: selectedRunId,
  enabled: isStreamEnabled,
  onEvent: () => {
    scheduleInspectorRefetch()
  },
})

onMounted(() => {
  setupCanvasViewport()
  inspectorRefreshTimer = window.setInterval(() => {
    if (!selectedRunId.value || !isInspectorRunActive()) return
    if (runEventStreamStatus.value === 'open') return
    void refetchInspector()
  }, 5000)
  actRefreshTimer = window.setInterval(() => {
    if (!selectedRunId.value || !isInspectorRunActive()) return
    if (selectedInspectorTab.value !== 'act') return
    void refetchInspector()
  }, 1000)
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
  if (actRefreshTimer) window.clearInterval(actRefreshTimer)
  if (streamRefetchTimer !== null) {
    window.clearTimeout(streamRefetchTimer)
    streamRefetchTimer = null
  }
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
const selectedTaskActions = computed(() =>
  selectedTaskActionRecords.value
    .filter((item) =>
      item.tool_name !== 'agent.thinking' &&
      item.tool_name !== 'agent.output',
    )
    .slice(-20),
)
const selectedTaskToolBlocks = computed<ToolCallBlock[]>(() =>
  selectedTaskActions.value.map((action, index) => actionRecordToToolBlock(action, index)),
)
const selectedTaskLatestResult = computed(() => selectedTaskResults.value[0] ?? null)
const selectedTaskEnvActions = computed(() =>
  selectedTaskActionRecords.value.filter((item) =>
    String(item.env_session_id ?? '').trim() !== '' ||
    String(item.env_binding_id ?? '').trim() !== '' ||
    String(item.before_env_snapshot_id ?? '').trim() !== '' ||
    String(item.after_env_snapshot_id ?? '').trim() !== '' ||
    String(item.action_kind ?? '').startsWith('env_'),
  ),
)
const selectedTaskEnvManifest = computed<Record<string, unknown>>(() => {
  const manifest = selectedTaskInputManifests.value
    .slice()
    .reverse()
    .find((item) => hasObjectValue((item as Record<string, unknown>).captured_env_preconditions)) as Record<string, unknown> | undefined
  return recordValue(manifest?.captured_env_preconditions)
})
const selectedTaskEnvInfo = computed(() => {
  const actions = selectedTaskEnvActions.value
  const manifest = selectedTaskEnvManifest.value
  const preconditions = recordValue(selectedTask.value?.env_preconditions)
  const firstPayload = actions.map((item) => envActionPayload(item)).find((payload) => Object.keys(payload).length > 0) ?? {}
  const firstAction = actions[0]
  const lastAction = actions[actions.length - 1]
  const snapshots = buildEnvSnapshots(actions, manifest)
  const sessionID = firstNonEmptyString(
    firstAction?.env_session_id,
    lastAction?.env_session_id,
    firstPayload.session_id,
    manifest.session_id,
  )
  const bindingID = firstNonEmptyString(
    firstAction?.env_binding_id,
    lastAction?.env_binding_id,
    firstPayload.binding_id,
    manifest.binding_id,
  )
  const beforeSnapshotID = firstNonEmptyString(
    manifest.before_snapshot_id,
    ...actions.map((item) => item.before_env_snapshot_id),
    ...actions.map((item) => envActionPayload(item).before_snapshot),
  )
  const afterSnapshotID = firstNonEmptyString(
    manifest.after_snapshot_id,
    ...actions.slice().reverse().map((item) => item.after_env_snapshot_id),
    ...actions.slice().reverse().map((item) => envActionPayload(item).after_snapshot),
  )
  return {
    hasEnv: actions.length > 0 || hasObjectValue(manifest) || preconditions.required === true || hasObjectValue(preconditions),
    sessionID,
    bindingID,
    kind: firstNonEmptyString(firstPayload.kind, manifest.kind, preconditions.kind),
    resourceName: firstNonEmptyString(firstPayload.resource_name, manifest.resource_name, preconditions.resource_name),
    mode: firstNonEmptyString(firstPayload.mode, manifest.mode, preconditions.mode),
    effectClass: firstNonEmptyString(firstAction?.effect_class, firstPayload.effect_class, manifest.effect_class, preconditions.effect_class),
    leaseEpoch: firstNonEmptyString(firstPayload.lease_epoch, manifest.lease_epoch),
    leaseToken: firstNonEmptyString(manifest.lease_token),
    beforeSnapshotID,
    afterSnapshotID,
    driftStatus: envDriftStatus(beforeSnapshotID, afterSnapshotID, snapshots.length),
    snapshots,
    actions,
  }
})

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

const inspectorTabs = computed(() => [
  { key: 'act' as const, label: t('orchestration.act') },
  { key: 'config' as const, label: t('orchestration.config') },
  { key: 'env' as const, label: t('orchestration.env') },
  { key: 'task' as const, label: t('orchestration.taskInfo') },
  { key: 'inputs' as const, label: t('orchestration.inputs') },
  { key: 'outputs' as const, label: t('orchestration.outputs') },
  { key: 'logs' as const, label: t('orchestration.logs') },
])

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

function selectBot(botID?: string) {
  selectedBotId.value = String(botID ?? '').trim()
  selectedRunId.value = ''
  botSelectOpen.value = false
  botSearchQuery.value = ''
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
    case 'created':
      return {
        label: t('orchestration.statusPending'),
        icon: Clock3,
        dot: 'bg-muted-foreground',
        chip: 'border-border bg-muted/70 text-muted-foreground',
        task: 'border-border bg-muted/30',
      }
    case 'idle':
      return {
        label: t('orchestration.statusIdle'),
        icon: Clock3,
        dot: 'bg-muted-foreground',
        chip: 'border-border bg-muted/70 text-muted-foreground',
        task: 'border-border bg-muted/30',
      }
    case 'active':
      return {
        label: t('orchestration.statusActive'),
        icon: LoaderCircle,
        dot: 'bg-sky-500',
        chip: 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300',
        task: 'border-sky-500/30 bg-sky-500/8',
      }
    case 'completed':
      return {
        label: t('orchestration.statusSuccess'),
        icon: CheckCircle2,
        dot: 'bg-emerald-500',
        chip: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
        task: 'bg-background',
      }
    case 'running':
      return {
        label: t('orchestration.statusRunning'),
        icon: LoaderCircle,
        dot: 'bg-sky-500',
        chip: 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300',
        task: 'border-sky-500/30 bg-sky-500/8',
      }
    case 'dispatching':
      return {
        label: t('orchestration.statusDispatching'),
        icon: LoaderCircle,
        dot: 'bg-sky-500',
        chip: 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300',
        task: 'border-sky-500/30 bg-sky-500/8',
      }
    case 'verifying':
      return {
        label: t('orchestration.statusVerifying'),
        icon: LoaderCircle,
        dot: 'bg-sky-500',
        chip: 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300',
        task: 'border-sky-500/30 bg-sky-500/8',
      }
    case 'waiting_human':
      return {
        label: t('orchestration.statusWaitingHuman'),
        icon: Clock3,
        dot: 'bg-amber-500',
        chip: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        task: 'border-amber-500/30 bg-amber-500/8',
      }
    case 'failed':
      return {
        label: t('orchestration.statusFailed'),
        icon: AlertCircle,
        dot: 'bg-rose-500',
        chip: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300',
        task: 'border-rose-500/30 bg-rose-500/8',
      }
    case 'blocked':
      return {
        label: t('orchestration.statusBlocked'),
        icon: AlertCircle,
        dot: 'bg-rose-500',
        chip: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300',
        task: 'border-rose-500/30 bg-rose-500/8',
      }
    case 'cancelled':
      return {
        label: t('orchestration.statusCancelled'),
        icon: AlertCircle,
        dot: 'bg-rose-500',
        chip: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300',
        task: 'border-rose-500/30 bg-rose-500/8',
      }
    default:
      return {
        label: status ? status.replaceAll('_', ' ') : t('orchestration.statusPending'),
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

function actionRecordToToolBlock(action: Record<string, unknown>, index: number): ToolCallBlock {
  const status = String(action.status || '').trim()
  const done = ['completed', 'failed', 'cancelled'].includes(status)
  const toolName = String(action.tool_name || action.action_kind || 'tool').trim() || 'tool'
  const input = normalizeActionPayload(action.input_payload)
  const output = normalizeActionPayload(action.output_payload)
  const error = normalizeActionPayload(action.error_payload)
  const result = done
    ? actionToolResult(status, output, error, action.summary)
    : (output == null ? null : actionToolResult(status, output, error, action.summary))

  return {
    type: 'tool',
    id: index,
    name: toolName,
    toolCallId: String(action.tool_call_id || action.id || `tool-${index}`),
    tool_call_id: String(action.tool_call_id || action.id || `tool-${index}`),
    toolName,
    input,
    output: result,
    result,
    done,
    running: !done,
  }
}

function normalizeActionPayload(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed) return ''
  if (
    (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'))
  ) {
    try {
      return JSON.parse(trimmed)
    }
    catch {
      return value
    }
  }
  return value
}

function actionToolResult(status: string, output: unknown, error: unknown, summary: unknown): unknown {
  if (status === 'failed' || status === 'cancelled' || error != null) {
    return {
      isError: true,
      structuredContent: error ?? output ?? summary ?? null,
    }
  }
  if (output && typeof output === 'object') return output
  if (output != null) {
    return {
      structuredContent: output,
    }
  }
  if (summary) {
    return {
      structuredContent: summary,
    }
  }
  return null
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

function recordValue(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return value as Record<string, unknown>
}

function firstNonEmptyString(...values: unknown[]) {
  for (const value of values) {
    const text = String(value ?? '').trim()
    if (text) return text
  }
  return ''
}

function envActionPayload(action: Record<string, unknown>) {
  const output = recordValue(action.output_payload)
  const input = recordValue(action.input_payload)
  return {
    ...input,
    ...output,
  }
}

function buildEnvSnapshots(actions: Record<string, unknown>[], manifest: Record<string, unknown>) {
  const items: EnvSnapshotItem[] = []
  const seen = new Set<string>()
  const push = (id: unknown, kind: string, action: Record<string, unknown> | null) => {
    const snapshotID = String(id ?? '').trim()
    if (!snapshotID || seen.has(`${kind}:${snapshotID}`)) return
    seen.add(`${kind}:${snapshotID}`)
    items.push({
      id: snapshotID,
      kind,
      actionKind: String(action?.action_kind ?? action?.tool_name ?? '').trim(),
      createdAt: String(action?.created_at ?? action?.finished_at ?? '').trim(),
    })
  }

  push(manifest.before_snapshot_id, 'pre_action', null)
  push(manifest.after_snapshot_id, 'post_action', null)
  for (const action of actions) {
    const payload = envActionPayload(action)
    push(action.before_env_snapshot_id ?? payload.before_snapshot, 'pre_action', action)
    push(action.after_env_snapshot_id ?? payload.after_snapshot, 'post_action', action)
  }
  return items
}

function envDriftStatus(beforeSnapshotID: string, afterSnapshotID: string, snapshotCount: number) {
  if (beforeSnapshotID && afterSnapshotID) {
    return beforeSnapshotID === afterSnapshotID ? 'unchanged' : 'changed'
  }
  return snapshotCount > 0 ? 'unknown' : 'not_applicable'
}

function envDriftLabel(status: string) {
  switch (status) {
    case 'changed':
      return t('orchestration.envChanged')
    case 'unchanged':
      return t('orchestration.envUnchanged')
    case 'unknown':
      return t('orchestration.envChangeUnknown')
    default:
      return t('orchestration.envNotUsed')
  }
}

function envDriftClass(status: string) {
  switch (status) {
    case 'changed':
      return 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'unchanged':
      return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'unknown':
      return 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300'
    default:
      return 'border-border bg-muted/70 text-muted-foreground'
  }
}

function envSnapshotKindLabel(kind: string) {
  switch (kind) {
    case 'pre_action':
      return t('orchestration.envSnapshotBefore')
    case 'post_action':
      return t('orchestration.envSnapshotAfter')
    case 'periodic':
      return t('orchestration.envSnapshotPeriodic')
    default:
      return kind.replaceAll('_', ' ') || '--'
  }
}

function envKindLabel(kind: string) {
  switch (kind) {
    case 'container':
      return t('orchestration.envKindContainer')
    case 'browser':
      return t('orchestration.envKindBrowser')
    default:
      return kind || '--'
  }
}

function envActionKindLabel(actionKind: string) {
  switch (actionKind) {
    case 'env_acquire':
      return t('orchestration.envActionReserved')
    case 'env_release':
      return t('orchestration.envActionReleased')
    case 'env_hold':
      return t('orchestration.envActionHeld')
    case 'env_resume':
      return t('orchestration.envActionResumed')
    default:
      return actionKind.replaceAll('_', ' ') || '--'
  }
}

function envModeLabel(mode: string) {
  switch (mode) {
    case 'read':
    case 'readonly':
      return t('orchestration.envModeRead')
    case 'write':
    case 'readwrite':
      return t('orchestration.envModeWrite')
    default:
      return mode || '--'
  }
}

function envEffectLabel(effectClass: string) {
  switch (effectClass) {
    case 'env_local_read':
      return t('orchestration.envEffectLocalRead')
    case 'env_local_mutation':
      return t('orchestration.envEffectLocalChange')
    case 'external_read':
      return t('orchestration.envEffectExternalRead')
    case 'external_write':
      return t('orchestration.envEffectExternalWrite')
    case 'external_irreversible':
      return t('orchestration.envEffectExternalIrreversible')
    default:
      return effectClass.replaceAll('_', ' ') || '--'
  }
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
    queryCache.invalidateQueries({ key: ['orchestration-runs'] })
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
    toast.success(t('orchestration.checkpointResolved'))
    await refetchInspector()
    queryCache.invalidateQueries({ key: ['orchestration-runs'] })
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('orchestration.checkpointResolveFailed')))
  } finally {
    resolvingCheckpointId.value = ''
  }
}

function refreshInspector() {
  queryCache.invalidateQueries({ key: ['orchestration-runs'] })
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
    keepGraphPositionOnViewportResize()
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

function keepGraphPositionOnViewportResize() {
  const previousWidth = viewportWidth.value
  const previousHeight = viewportHeight.value
  if (previousWidth <= 0 || previousHeight <= 0) {
    updateViewportSize()
    constrainView()
    return
  }

  const previousGraphOriginX = graphOriginX()
  const previousGraphOriginY = graphOriginY()
  const previousViewX = currentViewX
  const previousViewY = currentViewY
  updateViewportSize()
  setViewPosition(
    previousViewX + graphOriginX() - previousGraphOriginX,
    previousViewY + graphOriginY() - previousGraphOriginY,
  )
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
  const translateX = Math.round(-x * zoom)
  const translateY = Math.round(-y * zoom)
  return `translate3d(${translateX}px, ${translateY}px, 0) scale(${zoom})`
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
        <Popover
          v-if="runBotItems.length > 0"
          v-model:open="botSelectOpen"
        >
          <PopoverTrigger as-child>
            <Button
              variant="outline"
              class="ml-2 h-8 w-48 justify-between gap-2 px-3 text-xs"
            >
              <span class="truncate">{{ selectedBotLabel }}</span>
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
                v-model="botSearchQuery"
                :placeholder="$t('orchestration.searchBotsPlaceholder')"
                class="h-8 pl-7 text-xs"
              />
            </div>
            <div class="mt-2 max-h-72 space-y-1 overflow-y-auto">
              <button
                v-for="bot in filteredBots"
                :key="bot.id"
                type="button"
                class="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-xs hover:bg-muted/60"
                :class="bot.id === selectedBotId ? 'bg-primary/8 text-primary' : ''"
                @click="selectBot(bot.id)"
              >
                <CheckCircle2
                  class="size-3.5 shrink-0"
                  :class="bot.id === selectedBotId ? 'opacity-100' : 'opacity-0'"
                />
                <span class="min-w-0 flex-1 truncate">{{ bot.display_name || bot.id }}</span>
                <span class="font-mono text-[10px] text-muted-foreground">{{ bot.id ? shortId(bot.id, 8) : '--' }}</span>
              </button>
              <div
                v-if="filteredBots.length === 0"
                class="px-2 py-6 text-center text-xs text-muted-foreground"
              >
                {{ $t('orchestration.noBotsTitle') }}
              </div>
            </div>
          </PopoverContent>
        </Popover>
        <Popover
          v-if="runsWithSelected.length > 0"
          v-model:open="runSelectOpen"
        >
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
          v-if="inspector"
          class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px]"
          :class="statusMeta(inspector.run.lifecycle_status).chip"
        >
          <span
            class="size-1.5 rounded-full"
            :class="statusMeta(inspector.run.lifecycle_status).dot"
          />
          {{ statusMeta(inspector.run.lifecycle_status).label }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <Button
          v-if="inspector"
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

    <Empty
      v-if="botItems.length === 0"
      class="flex min-h-0 flex-1 items-center justify-center"
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
      v-else-if="runs.length === 0"
      class="flex min-h-0 flex-1 items-center justify-center"
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
      class="flex min-h-0 flex-1 items-center justify-center"
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
      class="flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground"
    >
      {{ inspectorStatus === 'loading' ? $t('orchestration.loadingInspector') : '--' }}
    </div>

    <template v-else>
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
                    <p class="line-clamp-2 text-xs font-medium leading-normal">
                      {{ compactTaskTitle(task.goal, task.id) }}
                    </p>
                    <div class="mt-1 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
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
                      class="mt-1 text-[11px] text-muted-foreground"
                    >
                      {{ $t('orchestration.rootTaskEntryHint') }}
                    </p>
                  </div>
                </div>
              </button>
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
                    <span class="block truncate text-xs font-medium">{{ node.title }}</span>
                    <span class="mt-0.5 block truncate text-[11px] text-muted-foreground">{{ node.subtitle }}</span>
                  </span>
                </div>
                <div class="mt-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
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
                v-if="selectedInspectorTab === 'act'"
                class="space-y-2"
              >
                <div class="max-h-[500px] overflow-y-auto pr-1">
                  <div
                    v-if="selectedTaskToolBlocks.length === 0"
                    class="text-[11px] text-muted-foreground"
                  >
                    {{ $t('orchestration.noAct') }}
                  </div>
                  <div
                    v-else
                    class="orchestration-act-list space-y-1.5"
                    :class="inspectorWidth < 360 ? 'orchestration-act-list-compact' : ''"
                  >
                    <ToolCallInline
                      v-for="block in selectedTaskToolBlocks"
                      :key="block.toolCallId"
                      :block="block"
                    />
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
                v-if="selectedInspectorTab === 'env'"
                class="space-y-3"
              >
                <div
                  v-if="!selectedTaskEnvInfo.hasEnv"
                  class="rounded-lg border border-dashed border-border/70 bg-background/60 px-3 py-6 text-center text-[11px] text-muted-foreground"
                >
                  {{ $t('orchestration.noEnv') }}
                </div>

                <template v-else>
                  <div class="grid grid-cols-2 gap-2 text-[11px]">
                    <div class="space-y-1">
                      <p class="text-muted-foreground">
                        {{ $t('orchestration.envSession') }}
                      </p>
                      <div class="h-8 truncate rounded-md border border-border/70 bg-background px-2 py-1.5 font-mono text-[10px]">
                        {{ selectedTaskEnvInfo.sessionID ? shortId(selectedTaskEnvInfo.sessionID, 18) : '--' }}
                      </div>
                    </div>
                    <div class="space-y-1">
                      <p class="text-muted-foreground">
                        {{ $t('orchestration.envBinding') }}
                      </p>
                      <div class="h-8 truncate rounded-md border border-border/70 bg-background px-2 py-1.5 font-mono text-[10px]">
                        {{ selectedTaskEnvInfo.bindingID ? shortId(selectedTaskEnvInfo.bindingID, 18) : '--' }}
                      </div>
                    </div>
                    <div class="space-y-1">
                      <p class="text-muted-foreground">
                        {{ $t('orchestration.envLease') }}
                      </p>
                      <div class="h-8 truncate rounded-md border border-border/70 bg-background px-2 py-1.5 font-mono text-[10px]">
                        {{ selectedTaskEnvInfo.leaseEpoch ? $t('orchestration.envLeaseEpoch', { epoch: selectedTaskEnvInfo.leaseEpoch }) : '--' }}
                      </div>
                    </div>
                    <div class="space-y-1">
                      <p class="text-muted-foreground">
                        {{ $t('orchestration.envDriftStatus') }}
                      </p>
                      <div
                        class="h-8 rounded-md border px-2 py-1.5 text-[10px]"
                        :class="envDriftClass(selectedTaskEnvInfo.driftStatus)"
                      >
                        {{ envDriftLabel(selectedTaskEnvInfo.driftStatus) }}
                      </div>
                    </div>
                  </div>

                  <div class="rounded-lg border border-border/70 bg-background p-3 text-[11px]">
                    <div class="mb-2 flex items-center justify-between">
                      <p class="font-semibold">
                        {{ $t('orchestration.envBindingDetails') }}
                      </p>
                      <span class="rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        {{ envKindLabel(selectedTaskEnvInfo.kind) }}
                      </span>
                    </div>
                    <div class="space-y-1 text-muted-foreground">
                      <div class="flex justify-between gap-3">
                        <span>{{ $t('orchestration.envResource') }}</span>
                        <span class="truncate font-mono">{{ selectedTaskEnvInfo.resourceName || '--' }}</span>
                      </div>
                      <div class="flex justify-between gap-3">
                        <span>{{ $t('orchestration.envMode') }}</span>
                        <span class="truncate">{{ envModeLabel(selectedTaskEnvInfo.mode) }}</span>
                      </div>
                      <div class="flex justify-between gap-3">
                        <span>{{ $t('orchestration.envEffectClass') }}</span>
                        <span class="truncate">{{ envEffectLabel(selectedTaskEnvInfo.effectClass) }}</span>
                      </div>
                      <div class="flex justify-between gap-3">
                        <span>{{ $t('orchestration.envLeaseToken') }}</span>
                        <span class="truncate font-mono">{{ selectedTaskEnvInfo.leaseToken ? shortId(selectedTaskEnvInfo.leaseToken, 18) : '--' }}</span>
                      </div>
                    </div>
                  </div>

                  <div class="space-y-2">
                    <div class="flex items-center justify-between">
                      <p class="text-[11px] font-semibold">
                        {{ $t('orchestration.envSnapshots') }}
                      </p>
                      <span class="text-[10px] text-muted-foreground tabular-nums">
                        {{ selectedTaskEnvInfo.snapshots.length }}
                      </span>
                    </div>
                    <div class="space-y-1.5">
                      <div
                        v-for="snapshot in selectedTaskEnvInfo.snapshots"
                        :key="`${snapshot.kind}:${snapshot.id}`"
                        class="rounded-lg border border-border/70 bg-background p-2 text-[11px]"
                      >
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-mono text-[10px]">{{ shortId(snapshot.id, 18) }}</span>
                          <span class="rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground">
                            {{ envSnapshotKindLabel(snapshot.kind) }}
                          </span>
                        </div>
                        <div class="mt-1 flex justify-between gap-3 text-[10px] text-muted-foreground">
                          <span class="truncate">{{ envActionKindLabel(snapshot.actionKind) }}</span>
                          <span>{{ formatDate(snapshot.createdAt) }}</span>
                        </div>
                      </div>
                      <p
                        v-if="selectedTaskEnvInfo.snapshots.length === 0"
                        class="rounded border border-dashed border-border/60 bg-background/60 px-2 py-2 text-[11px] text-muted-foreground"
                      >
                        {{ $t('orchestration.noEnvSnapshots') }}
                      </p>
                    </div>
                  </div>
                </template>
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
    </template>
  </div>
</template>

<style scoped>
.orchestration-act-list :deep(.text-sm) {
  font-size: 0.75rem;
  line-height: 1.25rem;
}

.orchestration-act-list-compact :deep(.ml-5) {
  margin-left: 0.75rem;
}

.orchestration-act-list-compact :deep(pre) {
  font-size: 0.6875rem;
}
</style>
