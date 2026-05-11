import { computed, type ComputedRef } from 'vue'
import dagre from 'dagre'
import { MarkerType, type Edge, type Node } from '@vue-flow/core'
import type { RunInspectorDependency, RunInspectorPayload, RunInspectorTask } from '../model'

export type TaskNodeKind =
  | 'trigger'
  | 'llm'
  | 'planner'
  | 'search'
  | 'tool'
  | 'memory'
  | 'merge'
  | 'verify'
  | 'output'

export interface TaskFlowNodeData {
  task: RunInspectorTask
  isRoot: boolean
  isSelected: boolean
  isRelated: boolean
  hasSelection: boolean
  canRetry?: boolean
  isRetrying?: boolean
  onRetryTask?: (taskID: string) => void
  level: number
  kind: TaskNodeKind
  maxLevel: number
}

export interface LaneNodeData {
  level: number
  count: number
  isRootLane: boolean
}

const NODE_WIDTH = 208
const NODE_HEIGHT = 80
const RANK_SEP = 120
const NODE_SEP = 32
const LANE_PAD_X = 28
const LANE_PAD_Y_TOP = 76
const LANE_PAD_Y_BOTTOM = 32

const EDGE_COLOR_DEFAULT = '#9ca3af'
const EDGE_COLOR_ACTIVE = '#6366f1'

type DagRelation = Pick<RunInspectorDependency, 'id' | 'predecessor_task_id' | 'successor_task_id'> & {
  structural?: boolean
}

function relationKey(pred?: string, succ?: string): string {
  return `${pred ?? ''}->${succ ?? ''}`
}

function buildDagRelations(tasks: RunInspectorTask[], dependencies: RunInspectorDependency[]): DagRelation[] {
  const taskIDs = new Set(tasks.map((task) => task.id).filter(Boolean) as string[])
  const seen = new Set<string>()
  const relations: DagRelation[] = []

  for (const dependency of dependencies) {
    const pred = dependency.predecessor_task_id
    const succ = dependency.successor_task_id
    if (!pred || !succ || !taskIDs.has(pred) || !taskIDs.has(succ)) continue
    seen.add(relationKey(pred, succ))
    relations.push(dependency)
  }

  for (const task of tasks) {
    const succ = task.id
    const pred = task.decomposed_from_task_id
    if (!pred || !succ || !taskIDs.has(pred) || !taskIDs.has(succ)) continue
    const key = relationKey(pred, succ)
    if (seen.has(key)) continue
    seen.add(key)
    relations.push({
      id: `decompose:${pred}:${succ}`,
      predecessor_task_id: pred,
      successor_task_id: succ,
      structural: true,
    })
  }

  return relations
}

export function buildTaskLevels(
  taskList: RunInspectorTask[],
  edges: DagRelation[],
): Map<string, number> {
  const byID = new Map<string, RunInspectorTask>()
  for (const task of taskList) {
    if (task.id) byID.set(task.id, task)
  }
  const incoming = new Map<string, number>()
  const outgoing = new Map<string, string[]>()

  for (const task of taskList) {
    if (!task.id) continue
    incoming.set(task.id, 0)
    outgoing.set(task.id, [])
  }

  for (const edge of edges) {
    const pred = edge.predecessor_task_id
    const succ = edge.successor_task_id
    if (!pred || !succ || !byID.has(pred) || !byID.has(succ)) continue
    incoming.set(succ, (incoming.get(succ) ?? 0) + 1)
    outgoing.get(pred)?.push(succ)
  }

  const queue = taskList
    .filter((task) => task.id && (incoming.get(task.id) ?? 0) === 0)
    .map((task) => task.id as string)
  const levels = new Map<string, number>()
  for (const id of queue) levels.set(id, 0)

  while (queue.length > 0) {
    const currentID = queue.shift() as string
    const currentLevel = levels.get(currentID) ?? 0
    for (const nextID of outgoing.get(currentID) ?? []) {
      levels.set(nextID, Math.max(levels.get(nextID) ?? 0, currentLevel + 1))
      incoming.set(nextID, (incoming.get(nextID) ?? 0) - 1)
      if ((incoming.get(nextID) ?? 0) <= 0) queue.push(nextID)
    }
  }

  for (const task of taskList) {
    if (task.id && !levels.has(task.id)) levels.set(task.id, 0)
  }

  return levels
}

export function buildTaskLevelMapWithRoot(
  tasks: RunInspectorTask[],
  dependencies: DagRelation[],
  rootID: string,
): Map<string, number> {
  if (!rootID || tasks.length <= 1) return buildTaskLevels(tasks, dependencies)

  const rootTask = tasks.find((task) => task.id === rootID)
  if (!rootTask) return buildTaskLevels(tasks, dependencies)

  const childTasks = tasks.filter((task) => task.id && task.id !== rootID)
  const childEdges = dependencies.filter((edge) =>
    edge.predecessor_task_id !== rootID && edge.successor_task_id !== rootID,
  )
  const childLevels = buildTaskLevels(childTasks, childEdges)
  const levels = new Map<string, number>([[rootID, 0]])
  for (const task of childTasks) {
    if (!task.id) continue
    levels.set(task.id, (childLevels.get(task.id) ?? 0) + 1)
  }
  return levels
}

export function inferTaskNodeKind(
  task: RunInspectorTask,
  level: number,
  maxLevel: number,
  rootID: string,
  hasVerification: (taskID: string) => boolean,
): TaskNodeKind {
  const text = `${task.goal ?? ''} ${task.worker_profile ?? ''}`.toLowerCase()
  if ((task.id && rootID === task.id) || level === 0) return 'trigger'
  if (task.status === 'verifying' || (task.id && hasVerification(task.id))) return 'verify'
  if (text.includes('search') || text.includes('web')) return 'search'
  if (text.includes('memory') || text.includes('blackboard')) return 'memory'
  if (text.includes('merge') || text.includes('aggregate') || text.includes('combine')) return 'merge'
  if (level === maxLevel || text.includes('output') || text.includes('deliver') || text.includes('final')) return 'output'
  if (text.includes('plan') || text.includes('decompose')) return 'planner'
  if (text.includes('tool') || text.includes('api') || text.includes('exec')) return 'tool'
  return 'llm'
}

export interface DagGraphResult {
  taskNodes: ComputedRef<Node<TaskFlowNodeData>[]>
  laneNodes: ComputedRef<Node<LaneNodeData>[]>
  nodes: ComputedRef<Node[]>
  edges: ComputedRef<Edge[]>
  levelMap: ComputedRef<Map<string, number>>
  maxLevel: ComputedRef<number>
  kindByTaskID: ComputedRef<Map<string, TaskNodeKind>>
  selectedRelated: ComputedRef<Set<string>>
}

export function useDagGraph(
  inspector: ComputedRef<RunInspectorPayload | null | undefined>,
  selectedTaskID: ComputedRef<string>,
): DagGraphResult {
  const tasks = computed<RunInspectorTask[]>(() => inspector.value?.tasks ?? [])
  const deps = computed<RunInspectorDependency[]>(() => inspector.value?.dependencies ?? [])
  const verifications = computed(() => inspector.value?.verifications ?? [])
  const rootID = computed(() => String(inspector.value?.run.root_task_id ?? ''))
  const relations = computed(() => buildDagRelations(tasks.value, deps.value))

  const levelMap = computed(() => buildTaskLevelMapWithRoot(tasks.value, relations.value, rootID.value))

  const maxLevel = computed(() => {
    let max = 0
    for (const value of levelMap.value.values()) {
      if (value > max) max = value
    }
    return max
  })

  const taskHasVerification = (taskID: string) =>
    verifications.value.some((item) => String(item.task_id ?? '') === taskID)

  const kindByTaskID = computed(() => {
    const map = new Map<string, TaskNodeKind>()
    for (const task of tasks.value) {
      if (!task.id) continue
      const level = levelMap.value.get(task.id) ?? 0
      map.set(task.id, inferTaskNodeKind(task, level, maxLevel.value, rootID.value, taskHasVerification))
    }
    return map
  })

  const selectedRelated = computed(() => {
    const related = new Set<string>()
    const id = selectedTaskID.value
    if (!id) return related
    related.add(id)
    for (const edge of relations.value) {
      if (edge.predecessor_task_id === id && edge.successor_task_id) {
        related.add(edge.successor_task_id)
      }
      if (edge.successor_task_id === id && edge.predecessor_task_id) {
        related.add(edge.predecessor_task_id)
      }
    }
    return related
  })

  const positions = computed(() => {
    const graph = new dagre.graphlib.Graph()
    graph.setGraph({ rankdir: 'LR', ranksep: RANK_SEP, nodesep: NODE_SEP, marginx: 32, marginy: 48 })
    graph.setDefaultEdgeLabel(() => ({}))

    for (const task of tasks.value) {
      if (!task.id) continue
      graph.setNode(task.id, { width: NODE_WIDTH, height: NODE_HEIGHT })
    }
    for (const edge of relations.value) {
      const pred = edge.predecessor_task_id
      const succ = edge.successor_task_id
      if (!pred || !succ) continue
      if (!graph.hasNode(pred) || !graph.hasNode(succ)) continue
      graph.setEdge(pred, succ)
    }
    dagre.layout(graph)

    const result = new Map<string, { x: number, y: number }>()
    for (const task of tasks.value) {
      if (!task.id) continue
      const node = graph.node(task.id) as { x?: number, y?: number } | undefined
      if (!node || typeof node.x !== 'number' || typeof node.y !== 'number') continue
      result.set(task.id, { x: node.x - NODE_WIDTH / 2, y: node.y - NODE_HEIGHT / 2 })
    }
    return result
  })

  const taskNodes = computed<Node<TaskFlowNodeData>[]>(() =>
    tasks.value
      .filter((task): task is RunInspectorTask & { id: string } => Boolean(task.id))
      .map<Node<TaskFlowNodeData>>((task) => {
        const level = levelMap.value.get(task.id) ?? 0
        const kind = kindByTaskID.value.get(task.id) ?? 'llm'
        const isRoot = rootID.value === task.id
        const isSelected = selectedTaskID.value === task.id
        const hasSelection = selectedTaskID.value.length > 0
        const isRelated = selectedRelated.value.has(task.id)

        return {
          id: task.id,
          type: 'taskFlow',
          position: positions.value.get(task.id) ?? { x: 0, y: 0 },
          data: {
            task,
            isRoot,
            isSelected,
            isRelated,
            hasSelection,
            level,
            kind,
            maxLevel: maxLevel.value,
          },
          draggable: false,
          selectable: false,
          connectable: false,
          width: NODE_WIDTH,
          height: NODE_HEIGHT,
          zIndex: 1,
        }
      }),
  )

  const laneNodes = computed<Node<LaneNodeData>[]>(() => {
    const byLevel = new Map<number, { x: number, top: number, bottom: number, count: number }>()
    for (const task of tasks.value) {
      if (!task.id) continue
      const pos = positions.value.get(task.id)
      if (!pos) continue
      const lvl = levelMap.value.get(task.id) ?? 0
      const top = pos.y
      const bottom = pos.y + NODE_HEIGHT
      const entry = byLevel.get(lvl)
      if (entry) {
        entry.x = Math.min(entry.x, pos.x)
        entry.top = Math.min(entry.top, top)
        entry.bottom = Math.max(entry.bottom, bottom)
        entry.count += 1
      } else {
        byLevel.set(lvl, { x: pos.x, top, bottom, count: 1 })
      }
    }
    const sorted = Array.from(byLevel.entries()).sort((a, b) => a[0] - b[0])
    if (sorted.length === 0) return []

    let globalTop = Infinity
    let globalBottom = -Infinity
    for (const [, e] of sorted) {
      globalTop = Math.min(globalTop, e.top)
      globalBottom = Math.max(globalBottom, e.bottom)
    }

    return sorted.map(([level, e]) => {
      const isRootLane = level === 0 && e.count === 1 && tasks.value.some((task) => task.id === rootID.value)
      const x = e.x - LANE_PAD_X
      const width = NODE_WIDTH + LANE_PAD_X * 2
      const top = globalTop - LANE_PAD_Y_TOP
      const height = (globalBottom - globalTop) + LANE_PAD_Y_TOP + LANE_PAD_Y_BOTTOM
      return {
        id: `lane-${level}`,
        type: 'lane',
        position: { x, y: top },
        data: { level, count: e.count, isRootLane },
        draggable: false,
        selectable: false,
        connectable: false,
        focusable: false,
        deletable: false,
        width,
        height,
        zIndex: -10,
        style: { width: `${width}px`, height: `${height}px`, pointerEvents: 'none' as const },
      }
    })
  })

  const nodes = computed<Node[]>(() => [...laneNodes.value, ...taskNodes.value])

  const edges = computed<Edge[]>(() =>
    relations.value
      .filter((edge) => edge.predecessor_task_id && edge.successor_task_id)
      .map<Edge>((edge) => {
        const id = edge.id || `e-${edge.predecessor_task_id}-${edge.successor_task_id}`
        const isActive = Boolean(
          selectedTaskID.value && (
            edge.predecessor_task_id === selectedTaskID.value ||
            edge.successor_task_id === selectedTaskID.value
          ),
        )
        return {
          id: String(id),
          source: edge.predecessor_task_id as string,
          target: edge.successor_task_id as string,
          type: 'smoothstep',
          updatable: false,
          selectable: false,
          focusable: false,
          class: [
            isActive ? 'memoh-edge-active' : 'memoh-edge-default',
            edge.structural ? 'memoh-edge-structural' : '',
          ].filter(Boolean).join(' '),
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: isActive ? EDGE_COLOR_ACTIVE : EDGE_COLOR_DEFAULT,
            width: 14,
            height: 14,
          },
          style: {
            strokeWidth: isActive ? 1.8 : 1.25,
          },
        }
      }),
  )

  return { taskNodes, laneNodes, nodes, edges, levelMap, maxLevel, kindByTaskID, selectedRelated }
}

export const TASK_FLOW_NODE_DIMENSIONS = {
  width: NODE_WIDTH,
  height: NODE_HEIGHT,
}
