import { computed, type ComputedRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { RunFlowSpan, RunInspectorPayload, RunInspectorTask } from '../model'
import { compactTaskTitle, shortId } from '../model'

export type FlowLaneKind = 'planning' | 'attempt' | 'verification' | 'checkpoint' | 'system'

export interface FlowLaneNodeData {
  key: string
  kind: FlowLaneKind
  label: string
  subtitle: string
  count: number
}

export interface FlowSpanNodeData {
  span: RunFlowSpan
  isSelected: boolean
  isRelated: boolean
  hasSelection: boolean
  canRetry?: boolean
  isRetrying?: boolean
  onRetryTask?: (taskID: string) => void
  index: number
  total: number
  taskTitle: string
  taskStatus: string
  laneKey: string
  laneLabel: string
  width: number
  offsetLabel: string
  durationLabel: string
  scaleMode: 'time' | 'sequence'
}

export interface FlowGapNodeData {
  laneKey: string
  label: string
  width: number
  isGrouped: boolean
  groupLabel: string
}

export interface FlowSummary {
  steps: number
  planning: number
  attempts: number
  actions: number
}

export type FlowGraphNode = Node<FlowSpanNodeData> | Node<FlowLaneNodeData> | Node<FlowGapNodeData>

export interface FlowGraphResult {
  nodes: ComputedRef<FlowGraphNode[]>
  edges: ComputedRef<Edge[]>
  summary: ComputedRef<FlowSummary>
}

const LABEL_WIDTH = 184
const TRACK_PADDING_X = 28
const TRACK_WIDTH_MIN = 360
const TRACK_WIDTH_MAX = 1200
const TRACK_WIDTH_PER_SPAN = 120
const LANE_TRACK_WIDTH_MIN = 240
const LANE_MIN_OVERLAP_X = 80
const LANE_HEIGHT = 154
const LANE_SPAN_TOP = 44
const LANE_BOTTOM_PADDING = 18
const LANE_GAP = 10
const SPAN_HEIGHT = 92
const SPAN_WIDTH_MIN = 220
const SPAN_WIDTH_MAX = 420
const SPAN_ROW_GAP_X = 16
const UNIT_FALLBACK_DURATION = 1
const GAP_RATIO_THRESHOLD = 0.25
const GAP_MULTIPLE_THRESHOLD = 3
const GAP_WIDTH_MIN = 72
const GAP_WIDTH_MAX = 128
const GAP_GROUP_OVERLAP_THRESHOLD = 0.6

interface SpanRange {
  span: RunFlowSpan
  start: number
  end: number
}

interface LaneGapSegment {
  laneKey: string
  realStart: number
  realEnd: number
  duration: number
  localX: number
  visualX: number
  width: number
  label: string
  groupID: string
  isGrouped: boolean
}

interface LaneCompression {
  laneStart: number
  laneEnd: number
  laneDuration: number
  laneX: number
  gaps: LaneGapSegment[]
  mapValue: (value: number) => number
}

function safeStartSeq(span: RunFlowSpan): number {
  const value = Number(span.start_seq ?? 0)
  return Number.isFinite(value) ? value : 0
}

function safeEndSeq(span: RunFlowSpan): number {
  const start = safeStartSeq(span)
  const value = Number(span.end_seq ?? 0)
  if (!Number.isFinite(value) || value <= 0) return start + UNIT_FALLBACK_DURATION
  return Math.max(value, start + UNIT_FALLBACK_DURATION)
}

function safeStartTime(span: RunFlowSpan): number {
  const raw = span.started_at || span.finished_at
  if (!raw) return 0
  const t = new Date(raw).getTime()
  return Number.isFinite(t) ? t : 0
}

function safeEndTime(span: RunFlowSpan, now: number): number {
  const finished = span.finished_at ? new Date(span.finished_at).getTime() : 0
  if (Number.isFinite(finished) && finished > 0) return finished

  const start = safeStartTime(span)
  if (start <= 0) return 0

  const status = String(span.status ?? '').trim()
  if (['active', 'running', 'dispatching', 'verifying'].includes(status)) {
    return Math.max(now, start + 1000)
  }
  return start + 1000
}

function spanSortKey(span: RunFlowSpan): [number, number, string] {
  return [safeStartSeq(span), safeStartTime(span), String(span.id ?? '')]
}

function compareSpans(a: RunFlowSpan, b: RunFlowSpan): number {
  const ka = spanSortKey(a)
  const kb = spanSortKey(b)
  if (ka[0] !== kb[0]) return ka[0] - kb[0]
  if (ka[1] !== kb[1]) return ka[1] - kb[1]
  return ka[2].localeCompare(kb[2])
}

function nodeID(span: RunFlowSpan, fallbackIndex: number): string {
  const id = String(span.id ?? '').trim()
  return id || `span-${fallbackIndex}`
}

function resolveTaskTitle(taskID: string, tasks: RunInspectorTask[]): string {
  if (!taskID) return ''
  const task = tasks.find((item) => item.id === taskID)
  if (!task) return shortId(taskID)
  return compactTaskTitle(task.goal ?? '', task.id ?? '')
}

function resolveTaskStatus(taskID: string, tasks: RunInspectorTask[]): string {
  if (!taskID) return ''
  const task = tasks.find((item) => item.id === taskID)
  return String(task?.status ?? '').trim()
}

function laneKind(span: RunFlowSpan): FlowLaneKind {
  switch (span.kind) {
    case 'planning':
    case 'replanning':
      return 'planning'
    case 'attempt':
    case 'attempt_finalize':
      return 'attempt'
    case 'verification':
      return 'verification'
    case 'checkpoint':
    case 'checkpoint_resume':
      return 'checkpoint'
    default:
      return 'system'
  }
}

function laneKeyForSpan(span: RunFlowSpan): string {
  const kind = laneKind(span)
  const taskID = String(span.task_id ?? '').trim()
  if (kind === 'attempt' && taskID) return `task:${taskID}`
  return kind
}

function laneLabelForSpan(span: RunFlowSpan, tasks: RunInspectorTask[]): string {
  const kind = laneKind(span)
  const taskID = String(span.task_id ?? '').trim()
  if (kind === 'attempt' && taskID) return resolveTaskTitle(taskID, tasks)
  switch (kind) {
    case 'planning':
      return 'Planning'
    case 'attempt':
      return 'Attempt'
    case 'verification':
      return 'Verification'
    case 'checkpoint':
      return 'Checkpoint'
    default:
      return 'System'
  }
}

function laneSubtitleForSpan(span: RunFlowSpan, tasks: RunInspectorTask[]): string {
  const kind = laneKind(span)
  const taskID = String(span.task_id ?? '').trim()
  if (kind === 'attempt') return taskID ? shortId(taskID, 12) : ''
  if (kind === 'planning') return ''
  if (kind === 'verification') return ''
  if (kind === 'checkpoint') return ''
  return taskID ? resolveTaskTitle(taskID, tasks) : 'run span'
}

function laneRank(kind: FlowLaneKind): number {
  switch (kind) {
    case 'planning':
      return 0
    case 'attempt':
      return 1
    case 'verification':
      return 2
    case 'checkpoint':
      return 3
    default:
      return 4
  }
}

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return ''
  const seconds = Math.max(1, Math.round(ms / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  if (minutes < 60) return rest > 0 ? `${minutes}m ${rest}s` : `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const minuteRest = minutes % 60
  return minuteRest > 0 ? `${hours}h ${minuteRest}m` : `${hours}h`
}

function formatSeqRange(start: number, end: number): string {
  return start === end ? `#${start}` : `#${start}-${end}`
}

function clamp(min: number, max: number, value: number): number {
  return Math.min(max, Math.max(min, value))
}

function median(values: number[]): number {
  const sorted = values.filter((value) => Number.isFinite(value) && value > 0).sort((a, b) => a - b)
  if (sorted.length === 0) return 0
  const mid = Math.floor(sorted.length / 2)
  if (sorted.length % 2 === 1) return sorted[mid] ?? 0
  return ((sorted[mid - 1] ?? 0) + (sorted[mid] ?? 0)) / 2
}

function overlapDuration(aStart: number, aEnd: number, bStart: number, bEnd: number): number {
  return Math.max(0, Math.min(aEnd, bEnd) - Math.max(aStart, bStart))
}

function buildLaneCompression(
  laneKey: string,
  ranges: SpanRange[],
  domainStart: number,
  domainRange: number,
  trackWidth: number,
  mode: 'time' | 'sequence',
): LaneCompression {
  const ordered = [...ranges].sort((a, b) => {
    if (a.start !== b.start) return a.start - b.start
    return a.end - b.end
  })
  const laneStart = Math.min(...ordered.map((item) => item.start).filter((value) => value > 0))
  const normalizedLaneStart = Number.isFinite(laneStart) ? laneStart : domainStart
  let laneEnd = Math.max(...ordered.map((item) => item.end))
  if (!Number.isFinite(laneEnd) || laneEnd <= normalizedLaneStart) {
    laneEnd = normalizedLaneStart + UNIT_FALLBACK_DURATION
  }
  const laneDuration = Math.max(UNIT_FALLBACK_DURATION, laneEnd - normalizedLaneStart)
  const scale = trackWidth / Math.max(UNIT_FALLBACK_DURATION, domainRange)
  const laneX = ((normalizedLaneStart - domainStart) / Math.max(UNIT_FALLBACK_DURATION, domainRange)) * trackWidth

  if (mode !== 'time' || ordered.length < 2) {
    return {
      laneStart: normalizedLaneStart,
      laneEnd,
      laneDuration,
      laneX: Math.max(0, laneX),
      gaps: [],
      mapValue: (value: number) => Math.max(0, (value - normalizedLaneStart) * scale),
    }
  }

  const intervals: Array<{ start: number, end: number }> = []
  for (const item of ordered) {
    const start = Math.max(normalizedLaneStart, item.start)
    const end = Math.max(item.end, start + UNIT_FALLBACK_DURATION)
    intervals.push({ start, end })
  }

  const merged: Array<{ start: number, end: number }> = []
  for (const interval of intervals) {
    const last = merged[merged.length - 1]
    if (!last || interval.start > last.end) {
      merged.push({ ...interval })
      continue
    }
    last.end = Math.max(last.end, interval.end)
  }

  const gapCandidates: Array<{ start: number, end: number, duration: number }> = []
  for (let i = 1; i < merged.length; i += 1) {
    const previous = merged[i - 1]
    const next = merged[i]
    if (!previous || !next) continue
    const duration = next.start - previous.end
    if (duration > 0) {
      gapCandidates.push({ start: previous.end, end: next.start, duration })
    }
  }

  const typicalGap = median(gapCandidates.map((gap) => gap.duration))
  const collapsed = gapCandidates
    .filter((gap) => {
      return shouldCollapseGap(gap.duration, laneDuration, typicalGap)
    })
    .map((gap) => {
      const width = collapsedGapWidth(gap.duration, laneDuration, trackWidth)
      return {
        laneKey,
        realStart: gap.start,
        realEnd: gap.end,
        duration: gap.duration,
        localX: 0,
        visualX: 0,
        width,
        label: `+${formatDuration(gap.duration)} gap`,
        groupID: '',
        isGrouped: false,
      }
    })

  function mapValue(value: number): number {
    const raw = Math.max(0, (value - normalizedLaneStart) * scale)
    let adjustment = 0
    for (const gap of collapsed) {
      const originalWidth = gap.duration * scale
      const removedWidth = Math.max(0, originalWidth - gap.width)
      if (value >= gap.realEnd) {
        adjustment += removedWidth
        continue
      }
      if (value > gap.realStart) {
        const progress = (value - gap.realStart) / gap.duration
        const compressedProgress = gap.width * clamp(0, 1, progress)
        const originalProgress = (value - gap.realStart) * scale
        adjustment += Math.max(0, originalProgress - compressedProgress)
      }
    }
    return Math.max(0, raw - adjustment)
  }

  for (const gap of collapsed) {
    gap.localX = mapValue(gap.realStart)
    gap.visualX = gap.localX
  }

  return {
    laneStart: normalizedLaneStart,
    laneEnd,
    laneDuration,
    laneX: Math.max(0, laneX),
    gaps: collapsed,
    mapValue,
  }
}

function assignGapGroups(gaps: LaneGapSegment[]): void {
  let nextGroup = 1
  for (let i = 0; i < gaps.length; i += 1) {
    const gap = gaps[i]
    if (!gap) continue
    if (!gap.groupID) {
      gap.groupID = `gap-group-${nextGroup}`
      nextGroup += 1
    }
    for (let j = i + 1; j < gaps.length; j += 1) {
      const other = gaps[j]
      if (!other) continue
      const overlap = overlapDuration(gap.realStart, gap.realEnd, other.realStart, other.realEnd)
      const base = Math.min(gap.duration, other.duration)
      const ratio = base > 0 ? overlap / base : 0
      if (ratio >= GAP_GROUP_OVERLAP_THRESHOLD) {
        other.groupID = gap.groupID
      }
    }
  }

  const counts = new Map<string, number>()
  for (const gap of gaps) {
    counts.set(gap.groupID, (counts.get(gap.groupID) ?? 0) + 1)
  }
  for (const gap of gaps) {
    gap.isGrouped = (counts.get(gap.groupID) ?? 0) > 1
  }
}

function shouldCollapseGap(duration: number, laneDuration: number, typicalGap: number): boolean {
  if (laneDuration <= 0 || duration <= 0) return false
  const gapRatio = duration / laneDuration
  const gapMultiple = typicalGap > 0 ? duration / typicalGap : Infinity
  return gapRatio >= GAP_RATIO_THRESHOLD && gapMultiple >= GAP_MULTIPLE_THRESHOLD
}

function collapsedGapWidth(duration: number, laneDuration: number, trackWidth: number): number {
  const gapRatio = laneDuration > 0 ? duration / laneDuration : 0
  return clamp(GAP_WIDTH_MIN, GAP_WIDTH_MAX, trackWidth * Math.min(0.12, gapRatio * 0.35))
}

function alignGroupedGaps(gaps: LaneGapSegment[], lanes: Map<string, LaneCompression>): void {
  const byGroup = new Map<string, LaneGapSegment[]>()
  for (const gap of gaps) {
    const list = byGroup.get(gap.groupID) ?? []
    list.push(gap)
    byGroup.set(gap.groupID, list)
  }

  for (const group of byGroup.values()) {
    if (group.length <= 1) {
      const gap = group[0]
      if (gap) gap.visualX = gap.localX
      continue
    }

    const absolutePositions = group
      .map((gap) => {
        const lane = lanes.get(gap.laneKey)
        return (lane?.laneX ?? 0) + LABEL_WIDTH + TRACK_PADDING_X + gap.localX
      })
      .sort((a, b) => a - b)
    const mid = Math.floor(absolutePositions.length / 2)
    const groupX = absolutePositions.length % 2 === 1
      ? absolutePositions[mid] ?? 0
      : ((absolutePositions[mid - 1] ?? 0) + (absolutePositions[mid] ?? 0)) / 2

    for (const gap of group) {
      const lane = lanes.get(gap.laneKey)
      const laneTrackX = (lane?.laneX ?? 0) + LABEL_WIDTH + TRACK_PADDING_X
      gap.visualX = Math.max(0, groupX - laneTrackX)
    }
  }
}

function addTrailingLaneGaps(
  orderedLanes: Array<FlowLaneNodeData & { firstStart: number }>,
  compressions: Map<string, LaneCompression>,
  trackWidth: number,
): void {
  for (let index = 0; index < orderedLanes.length - 1; index += 1) {
    const lane = orderedLanes[index]
    const nextLane = orderedLanes[index + 1]
    if (!lane || !nextLane) continue

    const compression = compressions.get(lane.key)
    const nextCompression = compressions.get(nextLane.key)
    if (!compression || !nextCompression) continue

    const duration = nextCompression.laneStart - compression.laneEnd
    if (!shouldCollapseGap(duration, Math.max(compression.laneDuration, nextCompression.laneStart - compression.laneStart), 0)) {
      continue
    }

    const width = collapsedGapWidth(duration, Math.max(compression.laneDuration, nextCompression.laneStart - compression.laneStart), trackWidth)
    const localX = compression.mapValue(compression.laneEnd) + SPAN_ROW_GAP_X
    compression.gaps.push({
      laneKey: lane.key,
      realStart: compression.laneEnd,
      realEnd: nextCompression.laneStart,
      duration,
      localX,
      visualX: localX,
      width,
      label: `+${formatDuration(duration)} gap`,
      groupID: '',
      isGrouped: false,
    })
  }
}

function ensureLaneOverlaps(lanes: Array<FlowLaneNodeData & { firstStart: number }>, metrics: Map<string, { x: number, y: number, height: number, contentWidth: number }>): void {
  if (lanes.length <= 1) return

  const ordered = [...lanes]
    .map((lane) => ({ lane, metrics: metrics.get(lane.key) }))
    .filter((item): item is { lane: FlowLaneNodeData & { firstStart: number }, metrics: { x: number, y: number, height: number, contentWidth: number } } => Boolean(item.metrics))
    .sort((a, b) => a.metrics.x - b.metrics.x)

  for (let index = 1; index < ordered.length; index += 1) {
    const previous = ordered[index - 1]?.metrics
    const current = ordered[index]?.metrics
    if (!previous || !current) continue

    const previousRight = previous.x + previous.contentWidth
    const requiredRight = current.x + LANE_MIN_OVERLAP_X
    if (previousRight < requiredRight) {
      previous.contentWidth = requiredRight - previous.x
    }
  }
}

export function useFlowGraph(
  inspector: ComputedRef<RunInspectorPayload | null | undefined>,
  selectedTaskID: ComputedRef<string>,
): FlowGraphResult {
  const spans = computed<RunFlowSpan[]>(() => {
    const list = inspector.value?.flow_spans ?? []
    return [...list].sort(compareSpans)
  })

  const tasks = computed<RunInspectorTask[]>(() => inspector.value?.tasks ?? [])

  const scaleMode = computed<'time' | 'sequence'>(() => {
    const withTime = spans.value.filter((span) => safeStartTime(span) > 0)
    return withTime.length >= 2 ? 'time' : 'sequence'
  })

  const summary = computed<FlowSummary>(() => {
    let planning = 0
    let attempts = 0
    let actions = 0
    for (const span of spans.value) {
      if (span.kind === 'planning' || span.kind === 'replanning') planning += 1
      if (span.kind === 'attempt') attempts += 1
      const count = Number(span.action_count ?? 0)
      if (Number.isFinite(count)) actions += count
    }
    return { steps: spans.value.length, planning, attempts, actions }
  })

  const nodes = computed<FlowGraphNode[]>(() => {
    const total = spans.value.length
    const hasSelection = selectedTaskID.value.length > 0
    if (total === 0) return []

    const now = Date.now()
    const mode = scaleMode.value
    const spanRanges = spans.value.map((span) => {
      const start = mode === 'time' ? safeStartTime(span) : safeStartSeq(span)
      const end = mode === 'time' ? safeEndTime(span, now) : safeEndSeq(span)
      const normalizedStart = start > 0 ? start : 0
      const normalizedEnd = Math.max(end || 0, normalizedStart + UNIT_FALLBACK_DURATION)
      return { span, start: normalizedStart, end: normalizedEnd }
    })

    let domainStart = Math.min(...spanRanges.map((item) => item.start).filter((value) => value > 0))
    if (!Number.isFinite(domainStart)) domainStart = 0
    let domainEnd = Math.max(...spanRanges.map((item) => item.end))
    if (!Number.isFinite(domainEnd) || domainEnd <= domainStart) {
      domainEnd = domainStart + UNIT_FALLBACK_DURATION
    }

    const domainRange = Math.max(UNIT_FALLBACK_DURATION, domainEnd - domainStart)
    const trackWidth = Math.min(TRACK_WIDTH_MAX, Math.max(TRACK_WIDTH_MIN, total * TRACK_WIDTH_PER_SPAN))
    const spanIDToRange = new Map(spanRanges.map((item, index) => [nodeID(item.span, index), item]))

    const laneMap = new Map<string, FlowLaneNodeData & { firstStart: number }>()
    for (const item of spanRanges) {
      const key = laneKeyForSpan(item.span)
      const kind = laneKind(item.span)
      const existing = laneMap.get(key)
      if (existing) {
        existing.count += 1
        existing.firstStart = Math.min(existing.firstStart, item.start)
        continue
      }
      laneMap.set(key, {
        key,
        kind,
        label: laneLabelForSpan(item.span, tasks.value),
        subtitle: laneSubtitleForSpan(item.span, tasks.value),
        count: 1,
        firstStart: item.start,
      })
    }

    const lanes = [...laneMap.values()].sort((a, b) => {
      const rankDiff = laneRank(a.kind) - laneRank(b.kind)
      if (rankDiff !== 0) return rankDiff
      if (a.firstStart !== b.firstStart) return a.firstStart - b.firstStart
      return a.label.localeCompare(b.label)
    })
    const laneIndex = new Map(lanes.map((lane, index) => [lane.key, index]))

    const baseLaneWidth = LABEL_WIDTH + TRACK_PADDING_X * 2 + LANE_TRACK_WIDTH_MIN
    const rangesByLane = new Map<string, SpanRange[]>()
    for (const item of spanRanges) {
      const laneKey = laneKeyForSpan(item.span)
      const list = rangesByLane.get(laneKey) ?? []
      list.push(item)
      rangesByLane.set(laneKey, list)
    }

    const laneCompression = new Map<string, LaneCompression>()
    for (const lane of lanes) {
      laneCompression.set(
        lane.key,
        buildLaneCompression(lane.key, rangesByLane.get(lane.key) ?? [], domainStart, domainRange, trackWidth, mode),
      )
    }
    if (mode === 'time') {
      addTrailingLaneGaps(lanes, laneCompression, trackWidth)
    }

    const rowRightsByLane = new Map<string, number[]>()
    const laneMaxRight = new Map<string, number>()

    const spanLayouts = spans.value.map((span, index) => {
      const id = nodeID(span, index)
      const taskID = String(span.task_id ?? '').trim()
      const isSelected = hasSelection && taskID === selectedTaskID.value
      const laneKey = laneKeyForSpan(span)
      const range = spanIDToRange.get(id)
      const start = range?.start ?? domainStart
      const end = range?.end ?? start + UNIT_FALLBACK_DURATION
      const compression = laneCompression.get(laneKey)
      const laneX = compression?.laneX ?? 0
      const localStart = compression?.mapValue(start) ?? ((start - domainStart) / domainRange) * trackWidth
      const localEnd = compression?.mapValue(end) ?? ((end - domainStart) / domainRange) * trackWidth
      const rawWidth = Math.max(1, localEnd - localStart)
      const width = Math.min(SPAN_WIDTH_MAX, Math.max(SPAN_WIDTH_MIN, rawWidth))
      const x = laneX + LABEL_WIDTH + TRACK_PADDING_X + Math.max(0, localStart)
      const right = x + width
      const rowRights = rowRightsByLane.get(laneKey) ?? []
      let row = rowRights.findIndex((rowRight) => x >= rowRight + SPAN_ROW_GAP_X)
      if (row < 0) row = rowRights.length
      rowRights[row] = right
      rowRightsByLane.set(laneKey, rowRights)
      laneMaxRight.set(laneKey, Math.max(laneMaxRight.get(laneKey) ?? 0, right))

      return {
        id,
        span,
        index,
        taskID,
        isSelected,
        laneKey,
        x,
        row,
        width,
        start,
        end,
      }
    })

    const allGapSegments = [...laneCompression.values()].flatMap((compression) => compression.gaps)
    assignGapGroups(allGapSegments)
    alignGroupedGaps(allGapSegments, laneCompression)
    for (const gap of allGapSegments) {
      const compression = laneCompression.get(gap.laneKey)
      const trackOrigin = (compression?.laneX ?? 0) + LABEL_WIDTH + TRACK_PADDING_X
      let previousRight = 0
      for (const layout of spanLayouts) {
        if (layout.laneKey !== gap.laneKey || layout.end > gap.realStart) continue
        previousRight = Math.max(previousRight, layout.x + layout.width)
      }
      if (previousRight > 0) {
        gap.visualX = Math.max(gap.visualX, previousRight - trackOrigin + SPAN_ROW_GAP_X)
      }
    }

    const laneMetrics = new Map<string, { x: number, y: number, height: number, contentWidth: number }>()
    let cursorY = 0
    for (const lane of lanes) {
      const compression = laneCompression.get(lane.key)
      const laneX = compression?.laneX ?? 0
      const rowCount = Math.max(1, rowRightsByLane.get(lane.key)?.length ?? 1)
      const height = LANE_SPAN_TOP + rowCount * SPAN_HEIGHT + (rowCount - 1) * LANE_GAP + LANE_BOTTOM_PADDING
      let laneRight = laneMaxRight.get(lane.key) ?? (laneX + baseLaneWidth)
      for (const gap of compression?.gaps ?? []) {
        laneRight = Math.max(laneRight, laneX + LABEL_WIDTH + TRACK_PADDING_X + gap.visualX + gap.width)
      }
      const contentWidth = Math.max(baseLaneWidth, laneRight - laneX + TRACK_PADDING_X)
      laneMetrics.set(lane.key, { x: laneX, y: cursorY, height, contentWidth })
      cursorY += height + LANE_GAP
    }
    ensureLaneOverlaps(lanes, laneMetrics)

    const laneNodes = lanes.map<Node<FlowLaneNodeData>>((lane) => {
      const metrics = laneMetrics.get(lane.key) ?? { x: 0, y: 0, height: LANE_HEIGHT, contentWidth: baseLaneWidth }
      return {
        id: `lane:${lane.key}`,
        type: 'flowLane',
        position: { x: metrics.x, y: metrics.y },
        data: {
          key: lane.key,
          kind: lane.kind,
          label: lane.label,
          subtitle: lane.subtitle,
          count: lane.count,
        },
        draggable: false,
        selectable: false,
        connectable: false,
        width: metrics.contentWidth,
        height: metrics.height,
        zIndex: -10,
        style: { pointerEvents: 'none' },
      }
    })

    const gapNodes = allGapSegments.map<Node<FlowGapNodeData>>((gap, index) => {
      const metrics = laneMetrics.get(gap.laneKey) ?? { x: 0, y: 0, height: LANE_HEIGHT, contentWidth: baseLaneWidth }
      return {
        id: `gap:${gap.laneKey}:${index}:${Math.round(gap.realStart)}`,
        type: 'flowGap',
        position: {
          x: metrics.x + LABEL_WIDTH + TRACK_PADDING_X + gap.visualX,
          y: metrics.y + LANE_SPAN_TOP + (SPAN_HEIGHT - 22) / 2,
        },
        data: {
          laneKey: gap.laneKey,
          label: gap.label,
          width: gap.width,
          isGrouped: gap.isGrouped,
          groupLabel: gap.isGrouped ? gap.groupID.replace('gap-group-', 'group ') : '',
        },
        draggable: false,
        selectable: false,
        connectable: false,
        width: gap.width,
        height: 22,
        zIndex: 2,
        style: { pointerEvents: 'none' },
      }
    })

    const spanNodes = spanLayouts.map<Node<FlowSpanNodeData>>((layout) => {
      const metrics = laneMetrics.get(layout.laneKey) ?? { x: 0, y: 0, height: LANE_HEIGHT, contentWidth: baseLaneWidth }
      const y = metrics.y + LANE_SPAN_TOP + layout.row * (SPAN_HEIGHT + LANE_GAP)
      const lane = lanes[laneIndex.get(layout.laneKey) ?? 0]

      return {
        id: layout.id,
        type: 'flowSpan',
        position: {
          x: layout.x,
          y,
        },
        data: {
          span: layout.span,
          isSelected: layout.isSelected,
          isRelated: layout.isSelected,
          hasSelection,
          index: layout.index + 1,
          total,
          taskTitle: resolveTaskTitle(layout.taskID, tasks.value),
          taskStatus: laneKind(layout.span) === 'attempt' ? resolveTaskStatus(layout.taskID, tasks.value) : '',
          laneKey: layout.laneKey,
          laneLabel: lane?.label ?? '',
          width: layout.width,
          offsetLabel: mode === 'time'
            ? formatDuration(layout.start - domainStart)
            : formatSeqRange(Math.round(layout.start), Math.round(layout.end)),
          durationLabel: mode === 'time'
            ? formatDuration(layout.end - layout.start)
            : formatSeqRange(Math.round(layout.start), Math.round(layout.end)),
          scaleMode: mode,
        },
        draggable: false,
        selectable: false,
        connectable: false,
        width: layout.width,
        height: SPAN_HEIGHT,
        zIndex: 1,
      }
    })

    return [...laneNodes, ...gapNodes, ...spanNodes]
  })

  const edges = computed<Edge[]>(() => [])

  return { nodes, edges, summary }
}

export const FLOW_SPAN_NODE_DIMENSIONS = {
  labelWidth: LABEL_WIDTH,
  laneHeight: LANE_HEIGHT,
  spanHeight: SPAN_HEIGHT,
}
