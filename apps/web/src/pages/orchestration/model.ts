import type {
  OrchestrationRunInspector as RunInspectorPayload,
  OrchestrationRunExecutionSpan as RunInspectorExecutionSpan,
  OrchestrationTask as RunInspectorTask,
  OrchestrationTaskDependency as RunInspectorDependency,
  OrchestrationRunListItem as RunListItem,
} from '@memohai/sdk'

export interface BotItem {
  id?: string
  display_name?: string
}

export type {
  RunInspectorDependency,
  RunInspectorExecutionSpan,
  RunInspectorPayload,
  RunInspectorTask,
  RunListItem,
}

export function formatDate(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export function statusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'running':
    case 'dispatching':
    case 'verifying':
      return 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-300'
    case 'waiting_human':
      return 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'failed':
    case 'blocked':
    case 'cancelled':
      return 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300'
    default:
      return 'border-border bg-muted/70 text-muted-foreground'
  }
}

export function compactTaskTitle(goal: string, fallbackID: string): string {
  const normalized = goal
    .trim()
    .replace(/^use\s+[^ ]+\s+to\s+/i, '')
    .replace(/^then\s+/i, '')
    .replace(/\s+/g, ' ')

  const firstClause = normalized.split(/[.;\n]/, 1)[0]?.trim() || ''
  const candidate = firstClause || fallbackID
  return truncate(candidate, 72)
}

export function compactTaskLabel(goal: string, fallbackID: string): string {
  const title = compactTaskTitle(goal, fallbackID)
  const words = title.split(/\s+/).filter(Boolean)
  const compact = words.slice(0, 4).join(' ')
  return truncate(compact || fallbackID, 24)
}

export function compactWorker(workerProfile?: string): string {
  if (!workerProfile) return '--'
  const parts = workerProfile.split('.').filter(Boolean)
  if (parts.length <= 2) return workerProfile
  return parts.slice(-2).join('.')
}

export function compactResultSummary(value: unknown): string {
  const text = typeof value === 'string' ? value.trim() : ''
  if (!text) return '--'
  return truncate(text.replace(/\s+/g, ' '), 72)
}

export function executionKindLabel(kind: string): string {
  return kind === 'verification' ? 'verification' : 'attempt'
}

export function statusShortLabel(status: string): string {
  switch (status) {
    case 'dispatching':
      return 'dispatch'
    case 'running':
      return 'run'
    case 'verifying':
      return 'verify'
    case 'waiting_human':
      return 'wait'
    case 'completed':
      return 'done'
    case 'failed':
      return 'fail'
    case 'cancelled':
      return 'cancel'
    default:
      return status || '--'
  }
}

export function timelineLabel(type: string): string {
  const normalized = type.replace(/^run\.event\./, '')
  switch (normalized) {
    case 'created':
      return 'run created'
    case 'running':
      return 'run active'
    case 'task.created':
      return 'task created'
    case 'task.ready':
      return 'task ready'
    case 'task.dispatching':
      return 'task dispatching'
    case 'attempt.created':
      return 'attempt created'
    case 'attempt.claimed':
      return 'attempt claimed'
    case 'attempt.running':
      return 'attempt running'
    case 'attempt.completed':
      return 'attempt completed'
    case 'attempt.failed':
      return 'attempt failed'
    case 'verification.created':
      return 'verification created'
    case 'verification.completed':
      return 'verification completed'
    case 'checkpoint.created':
      return 'checkpoint opened'
    case 'checkpoint.resolved':
      return 'checkpoint resolved'
    case 'task.completed':
      return 'task completed'
    case 'task.failed':
      return 'task failed'
    default:
      return normalized.replaceAll('.', ' ')
  }
}

export function formatJsonValue(value: unknown): string {
  if (value == null) return '--'
  try {
    return JSON.stringify(value, null, 2)
  }
  catch {
    return '--'
  }
}

function truncate(value: string, maxLength: number): string {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength - 3)}...`
}

/** Compact id for dense tables (middle ellipsis). */
export function shortId(value: string, max = 10): string {
  const s = value.trim()
  if (s.length <= max) return s
  const head = Math.max(4, Math.floor(max / 2) - 1)
  const tail = max - head - 1
  return `${s.slice(0, head)}…${s.slice(-tail)}`
}
