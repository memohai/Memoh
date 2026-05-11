<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
  Bot,
  Boxes,
  CheckCircle2,
  ChevronDown,
  Copy,
  Database,
  FileOutput,
  GitMerge,
  LoaderCircle,
  PlayCircle,
  RefreshCw,
  ScanSearch,
  Search,
  ShieldCheck,
  Sparkles,
  Square,
  Workflow,
  Wrench,
  X,
  type LucideIcon,
} from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import {
  getOrchestrationRuns,
  postOrchestrationCheckpointsByCheckpointIdResolve,
  getOrchestrationRunsByRunIdInspector,
  postOrchestrationRunsByRunIdTasksByTaskIdCancel,
  postOrchestrationRunsByRunIdTasksByTaskIdRetry,
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
  type RunInspectorExecutionSpan,
  type RunInspectorTask,
  type RunListItem,
} from './model'
import { useRunEventStream } from './composables/use-run-event-stream'
import {
  buildTaskLevelMapWithRoot,
  inferTaskNodeKind,
  type TaskNodeKind,
} from './composables/use-dag-graph'
import { useOrchestrationMeta } from './composables/use-orchestration-meta'
import RunDag from './components/run-dag.vue'
import RunFlow from './components/run-flow.vue'

type NodeKind = TaskNodeKind
type InspectorTab = 'act' | 'config' | 'env' | 'task' | 'inputs' | 'outputs' | 'logs'
type RunViewMode = 'dag' | 'flow'

const { t } = useI18n()
const { statusMeta } = useOrchestrationMeta()
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
const runViewMode = computed<RunViewMode>(() => normalizeRunViewMode(route.query.view))
const outputRawOpen = ref(false)
const taskTechnicalOpen = ref(false)
const expandedLogItemIds = ref<string[]>([])
const botSelectOpen = ref(false)
const runSelectOpen = ref(false)
const inspectorOpen = ref(true)
const inspectorWidth = ref(340)
const stoppingTaskId = ref('')
const retryingTaskId = ref('')
const resolvingCheckpointId = ref('')
let inspectorSelectionFrame = 0
let taskScrollFrame = 0
let inspectorRefreshTimer = 0
let actRefreshTimer = 0
let bodySelectionLocked = false
let previousUserSelect = ''
let inspectorResizeStart: { clientX: number, width: number } | null = null

const minInspectorWidth = 280
const maxInspectorWidth = 560

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
    intent_status: run.intent_status,
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

const tasks = computed<RunInspectorTask[]>(() => {
  const value = inspector.value
  const items = value?.tasks ?? []
  if (!value || value.run.lifecycle_status !== 'failed') return items
  const terminalReason = String(value.run.terminal_reason ?? '').trim()
  return items.map((task) => {
    if (task.id !== value.run.root_task_id || task.status !== 'created') return task
    return {
      ...task,
      status: 'failed',
      terminal_reason: task.terminal_reason || terminalReason,
    }
  })
})
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

watch(inspector, (value) => {
  if (!value) return
  const routeTaskID = typeof route.query.task_id === 'string' ? route.query.task_id.trim() : ''
  const visibleTaskIDs = new Set(tasks.value.map((task) => task.id))
  if (routeTaskID && visibleTaskIDs.has(routeTaskID)) {
    setSelectedTask(routeTaskID, { immediateInspector: true })
    return
  }
  if (selectedTaskId.value && visibleTaskIDs.has(selectedTaskId.value)) {
    if (!inspectedTaskId.value || !visibleTaskIDs.has(inspectedTaskId.value)) {
      inspectedTaskId.value = selectedTaskId.value
    }
    return
  }
  setSelectedTask(value.run.root_task_id || tasks.value[0]?.id || value.tasks[0]?.id || '', { immediateInspector: true })
}, { immediate: true })

watch([selectedBotId, selectedRunId], ([botID, runID]) => {
  const nextQuery: Record<string, string> = {}
  if (botID) nextQuery.bot_id = botID
  if (runID) nextQuery.run_id = runID
  if (runViewMode.value !== 'dag') nextQuery.view = runViewMode.value

  const currentBotQuery = typeof route.query.bot_id === 'string' ? route.query.bot_id : ''
  const currentRunQuery = typeof route.query.run_id === 'string' ? route.query.run_id : ''
  const currentViewQuery = typeof route.query.view === 'string' ? route.query.view : ''
  if (
    currentBotQuery === (nextQuery.bot_id ?? '') &&
    currentRunQuery === (nextQuery.run_id ?? '') &&
    currentViewQuery === (nextQuery.view ?? '')
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

watch(selectedRunId, () => {
  selectedTaskId.value = ''
  inspectedTaskId.value = ''
  selectedInspectorTab.value = 'act'
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
})

onBeforeUnmount(() => {
  inspectorResizeStart = null
  restoreBodySelectionIfDragging()
  removeInspectorResizeListeners()
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
const isTaskRetryable = (task: RunInspectorTask) =>
  !!selectedRunId.value &&
  String(task.status ?? '') === 'failed'
const canRetryTaskCard = (task: RunInspectorTask) =>
  isTaskRetryable(task) &&
  !retryingTaskId.value
const retryableTaskIds = computed(() =>
  tasks.value
    .filter((task) => isTaskRetryable(task))
    .map((task) => String(task.id ?? '').trim())
    .filter(Boolean),
)
const canStopTaskCard = (task: RunInspectorTask) =>
  !!selectedRunId.value &&
  !stoppingTaskId.value &&
  ['ready', 'dispatching', 'running', 'verifying'].includes(String(task.status ?? '')) &&
  String(inspector.value?.run.lifecycle_status ?? '') === 'running'

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
const selectedTaskEnvManifest = computed<Record<string, unknown>>(() => {
  const manifest = selectedTaskInputManifests.value
    .slice()
    .reverse()
    .find((item) => hasObjectValue((item as Record<string, unknown>).captured_env_preconditions)) as Record<string, unknown> | undefined
  return recordValue(manifest?.captured_env_preconditions)
})
const selectedTaskEnvInfo = computed(() => {
  const manifest = selectedTaskEnvManifest.value
  const preconditions = recordValue(selectedTask.value?.env_preconditions)
  const actionPayload = selectedTaskActionRecords.value
    .map((item) => ({
      ...recordValue(item.input_payload),
      ...recordValue(item.output_payload),
    }))
    .find((payload) => hasObjectValue(payload)) ?? {}
  const kind = firstNonEmptyString(preconditions.kind, manifest.kind, actionPayload.kind)
  const name = firstNonEmptyString(preconditions.resource_name, manifest.resource_name, actionPayload.resource_name)
  return {
    hasEnv: preconditions.required === true || !!kind || !!name || hasObjectValue(manifest),
    kind,
    name,
  }
})

const taskLevelMap = computed(() =>
  buildTaskLevelMapWithRoot(tasks.value, dependencies.value, rootTaskId.value),
)
const maxLevel = computed(() => Math.max(0, ...Array.from(taskLevelMap.value.values())))

const filteredTasks = computed(() => {
  const q = taskSearchQuery.value.trim().toLowerCase()
  if (!q) return tasks.value
  return tasks.value.filter((task) => {
    const title = compactTaskTitle(task.goal ?? '', task.id ?? '').toLowerCase()
    return (
      title.includes(q) ||
      String(task.id ?? '').toLowerCase().includes(q) ||
      String(task.status ?? '').toLowerCase().includes(q)
    )
  })
})

const taskHasVerification = (taskID: string) =>
  verifications.value.some((item) => String(item.task_id ?? '') === taskID)

const selectedNodeKind = computed<NodeKind>(() => {
  const task = selectedTask.value
  if (!task || !task.id) return 'llm'
  const level = taskLevelMap.value.get(task.id) ?? 0
  return inferTaskNodeKind(task, level, maxLevel.value, rootTaskId.value, taskHasVerification)
})
const selectedNodeLevel = computed(() => {
  const task = selectedTask.value
  if (!task || !task.id) return 0
  return taskLevelMap.value.get(task.id) ?? 0
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

function normalizeRunViewMode(value: unknown): RunViewMode {
  return value === 'flow' ? 'flow' : 'dag'
}

function setRunViewMode(mode: RunViewMode) {
  if (runViewMode.value === mode) return
  const nextQuery = { ...route.query }
  if (mode === 'dag') {
    delete nextQuery.view
  } else {
    nextQuery.view = mode
  }
  void router.replace({ query: nextQuery })
}

function selectTaskFromList(task: RunInspectorTask) {
  if (!task.id) return
  setSelectedTask(task.id)
}

function selectTask(taskID: string) {
  setSelectedTask(taskID)
}

function selectTaskFromDag(taskID: string) {
  setSelectedTask(taskID, { immediateInspector: true })
  selectedInspectorTab.value = 'act'
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
  const ids = dependencies.value
    .filter((edge) => edge.successor_task_id === taskID)
    .map((edge) => edge.predecessor_task_id)
  return ids
    .map((id) => tasks.value.find((task) => task.id === id))
    .filter((task): task is RunInspectorTask => Boolean(task))
}

function downstreamTasks(taskID: string): RunInspectorTask[] {
  const ids = dependencies.value
    .filter((edge) => edge.predecessor_task_id === taskID)
    .map((edge) => edge.successor_task_id)
  return ids
    .map((id) => tasks.value.find((task) => task.id === id))
    .filter((task): task is RunInspectorTask => Boolean(task))
}

function kindMeta(kind: NodeKind): { label: string, icon: LucideIcon, color: string } {
  switch (kind) {
    case 'trigger':
      return { label: t('orchestration.nodeKindTrigger'), icon: PlayCircle, color: 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20' }
    case 'llm':
      return { label: t('orchestration.nodeKindLlm'), icon: Bot, color: 'border-border bg-background text-foreground' }
    case 'planner':
      return { label: t('orchestration.nodeKindPlanner'), icon: Sparkles, color: 'border-border bg-muted/45 text-foreground' }
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

function newIdempotencyKey(prefix: string) {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `${prefix}-${random}`
}

async function stopTask(task: RunInspectorTask) {
  const taskID = String(task.id ?? '').trim()
  if (!selectedRunId.value || !taskID || stoppingTaskId.value || !canStopTaskCard(task)) return
  stoppingTaskId.value = taskID
  try {
    await postOrchestrationRunsByRunIdTasksByTaskIdCancel({
      path: { run_id: selectedRunId.value, task_id: taskID },
      body: {
        reason: 'operator stopped task',
        idempotency_key: newIdempotencyKey('cancel-task'),
      },
      throwOnError: true,
    })
    toast.success(t('orchestration.taskStopRequested'))
    await refetchInspector()
    queryCache.invalidateQueries({ key: ['orchestration-runs'] })
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('orchestration.taskStopFailed')))
  } finally {
    stoppingTaskId.value = ''
  }
}

async function retryTask(task: RunInspectorTask) {
  const taskID = String(task.id ?? '').trim()
  if (!selectedRunId.value || !taskID || retryingTaskId.value || !canRetryTaskCard(task)) return
  retryingTaskId.value = taskID
  try {
    await postOrchestrationRunsByRunIdTasksByTaskIdRetry({
      path: { run_id: selectedRunId.value, task_id: taskID },
      body: {
        reason: 'operator retried task',
        idempotency_key: newIdempotencyKey('retry-task'),
      },
      throwOnError: true,
    })
    toast.success(t('orchestration.taskRetryStarted'))
    await refetchInspector()
    queryCache.invalidateQueries({ key: ['orchestration-runs'] })
  } catch (err) {
    toast.error(resolveApiErrorMessage(err, t('orchestration.taskRetryFailed')))
  } finally {
    retryingTaskId.value = ''
  }
}

async function retryTaskById(taskID: string) {
  const task = tasks.value.find((item) => String(item.id ?? '') === taskID)
  if (!task) return
  await retryTask(task)
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
  if (bodySelectionLocked && !inspectorResizeStart) {
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

</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-background text-foreground">
    <header class="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border/70 px-4">
      <div class="flex min-w-0 items-center gap-3">
        <div class="flex size-8 items-center justify-center rounded-lg border border-border bg-card text-foreground shadow-sm">
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
                :class="bot.id === selectedBotId ? 'bg-accent text-accent-foreground' : ''"
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
                :class="run.id === selectedRunId ? 'bg-accent text-accent-foreground' : ''"
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

              <template
                v-for="task in filteredTasks"
                :key="task.id"
              >
                <button
                  :id="`orchestration-task-${task.id}`"
                  type="button"
                  class="w-full rounded-lg px-2.5 py-2 text-left shadow-[0_0.6px_0.7px_hsl(var(--foreground)/0.04),0_1.8px_2.2px_-0.5px_hsl(var(--foreground)/0.05),0_5px_8px_-1px_hsl(var(--foreground)/0.06),0_14px_24px_-2px_hsl(var(--foreground)/0.07)] transition-colors hover:bg-muted/40 hover:shadow-[0_0.7px_0.8px_hsl(var(--foreground)/0.05),0_2.4px_3px_-0.5px_hsl(var(--foreground)/0.06),0_7px_11px_-1px_hsl(var(--foreground)/0.07),0_18px_32px_-2px_hsl(var(--foreground)/0.09)]"
                  :class="[
                    task.id === selectedTaskId ? 'border-primary/30 bg-primary/8 shadow-[0_1px_2px_hsl(var(--foreground)/0.06),0_4px_10px_-3px_hsl(var(--primary)/0.22),0_14px_28px_-10px_hsl(var(--primary)/0.28)]' : statusMeta(task.status).task,
                    rootHasChildren && task.id === rootTaskId && task.id !== selectedTaskId ? 'bg-muted/35' : '',
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
                <div
                  v-if="canStopTaskCard(task)"
                  class="-mt-1 flex justify-end gap-1 pr-1"
                >
                  <Button
                    v-if="canStopTaskCard(task)"
                    variant="ghost"
                    size="icon"
                    class="size-7 text-muted-foreground hover:text-rose-600"
                    :disabled="stoppingTaskId === task.id"
                    :title="$t('orchestration.stopTask')"
                    @click="stopTask(task)"
                  >
                    <LoaderCircle
                      v-if="stoppingTaskId === task.id"
                      class="size-3.5 animate-spin"
                    />
                    <Square
                      v-else
                      class="size-3.5"
                    />
                  </Button>
                </div>
              </template>
            </div>
          </ScrollArea>
        </aside>

        <main class="flex min-h-0 flex-col overflow-hidden">
          <div class="relative z-40 flex h-11 shrink-0 items-center justify-between border-b border-border/60 bg-background/80 px-3">
            <div class="flex items-center gap-2">
              <Workflow class="size-3.5 text-muted-foreground" />
              <p class="text-xs font-semibold">
                {{ runViewMode === 'dag' ? $t('orchestration.taskDag') : $t('orchestration.runFlow') }}
              </p>
            </div>
            <div
              class="flex rounded-md border border-border bg-background p-0.5 text-[11px] shadow-sm"
              @pointerdown.stop
              @mousedown.stop
            >
              <button
                type="button"
                class="inline-flex cursor-pointer items-center gap-1 rounded px-2.5 py-1 transition-colors"
                :class="runViewMode === 'dag' ? 'bg-card font-medium text-foreground shadow-[inset_0_0_0_1px_hsl(var(--foreground)/0.14),0_1px_2px_hsl(var(--foreground)/0.08),0_4px_10px_-4px_hsl(var(--foreground)/0.18)]' : 'text-muted-foreground hover:text-foreground'"
                :aria-pressed="runViewMode === 'dag'"
                @click="setRunViewMode('dag')"
              >
                <Workflow class="size-3" />
                {{ $t('orchestration.taskDag') }}
              </button>
              <button
                type="button"
                class="inline-flex cursor-pointer items-center gap-1 rounded px-2.5 py-1 transition-colors"
                :class="runViewMode === 'flow' ? 'bg-card font-medium text-foreground shadow-[inset_0_0_0_1px_hsl(var(--foreground)/0.14),0_1px_2px_hsl(var(--foreground)/0.08),0_4px_10px_-4px_hsl(var(--foreground)/0.18)]' : 'text-muted-foreground hover:text-foreground'"
                :aria-pressed="runViewMode === 'flow'"
                @click="setRunViewMode('flow')"
              >
                <GitMerge class="size-3" />
                {{ $t('orchestration.runFlow') }}
              </button>
            </div>
          </div>
          <RunDag
            v-if="runViewMode === 'dag'"
            :inspector="inspector"
            :selected-task-id="selectedTaskId"
            :inspector-open="inspectorOpen"
            :retryable-task-ids="retryableTaskIds"
            :retrying-task-id="retryingTaskId"
            @select-task="selectTaskFromDag"
            @open-inspector="inspectorOpen = true"
            @retry-task="retryTaskById"
          />

          <RunFlow
            v-else
            :inspector="inspector"
            :selected-task-id="selectedTaskId"
            :inspector-open="inspectorOpen"
            :retryable-task-ids="retryableTaskIds"
            :retrying-task-id="retryingTaskId"
            @select-task="selectTaskFromDag"
            @open-inspector="inspectorOpen = true"
            @retry-task="retryTaskById"
          />
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
              v-if="selectedTask"
              class="space-y-4 p-4"
              :class="inspectorSelectionPending ? 'opacity-70' : ''"
            >
              <div class="flex items-start gap-3">
                <span
                  class="flex size-10 items-center justify-center rounded-lg border"
                  :class="kindMeta(selectedNodeKind).color"
                >
                  <component
                    :is="kindMeta(selectedNodeKind).icon"
                    class="size-5"
                  />
                </span>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold">
                    {{ compactTaskTitle(selectedTask.goal, selectedTask.id) }}
                  </p>
                  <p class="text-[11px] text-muted-foreground">
                    {{ kindMeta(selectedNodeKind).label }} {{ $t('orchestration.node') }} / L{{ selectedNodeLevel }}
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
                    ? 'border-b border-foreground font-medium text-foreground'
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
                    <Search class="size-3.5 text-muted-foreground" />
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
                  <div class="rounded-lg border border-border/70 bg-background p-3 text-[11px]">
                    <div class="mb-3 flex items-center justify-between">
                      <p class="font-semibold">
                        {{ $t('orchestration.env') }}
                      </p>
                      <span class="rounded border border-border/70 px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        {{ envKindLabel(selectedTaskEnvInfo.kind) }}
                      </span>
                    </div>
                    <div class="space-y-1 text-muted-foreground">
                      <div class="flex justify-between gap-3">
                        <span>{{ $t('orchestration.envResourceName') }}</span>
                        <span class="truncate font-mono">{{ selectedTaskEnvInfo.name || '--' }}</span>
                      </div>
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
                        class="rounded border border-border bg-muted/35 px-2 py-0.5 text-foreground"
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
                        class="rounded border border-border bg-muted/35 px-2 py-0.5 text-foreground"
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
