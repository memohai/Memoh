import { reactive } from 'vue'

// Our own toast engine. Replaces vue-sonner: we only ever need a fixed-corner,
// always-expanded column of string messages, so we skip Sonner's JS height
// measurement + transform-offset stacking entirely (that machinery was the source
// of the close-button hover "twitch"). Layout is a plain CSS flex column; this
// module is just the imperative, call-from-anywhere data layer behind <Toaster>.

export type ToastVariant = 'message' | 'success' | 'error' | 'info' | 'warning'

export interface ToastAction {
  label: string
  onClick: () => void
}

export interface ToastOptions {
  /** Secondary line under the title. */
  description?: string
  /** Auto-dismiss delay in ms. `0`/`Infinity` keeps it until dismissed. */
  duration?: number
  /** Optional trailing action button. */
  action?: ToastAction
  /** Stable id — re-using one updates the existing toast in place. */
  id?: string | number
}

/** Render-facing record. Timer bookkeeping is kept OUT of here (see `timers`) so
 *  pausing/resuming on hover never churns reactivity or re-renders the list. */
export interface ToastRecord {
  id: string
  variant: ToastVariant
  title: string
  description?: string
  action?: ToastAction
  /** True when `title` is a synthesized variant heading (the long-blob auto-shape
   *  below), NOT caller copy. `<Toaster>` swaps it for a localized label when one
   *  is supplied via the `headings` prop; the English `title` here is the fallback. */
  autoHeading?: boolean
}

interface TimerEntry {
  duration: number
  remaining: number
  startedAt: number
  handle: ReturnType<typeof setTimeout> | undefined
}

const DEFAULT_DURATION = 4000

/** Concise, variant-derived heading used when a caller fires a long,
 *  headline-unfriendly blob — a raw backend error, a file path, a URL, a
 *  connection string — as the message with NO description. Rather than render
 *  that wall of text as a giant bold title (the "ugly overflow" bug), we surface
 *  it as gray BODY copy under this heading, matching the title→description shape
 *  of a well-formed rich toast (e.g. "Upload failed" + detail).
 *
 *  This is a safety net for the many `toast.error(rawBackendError)` call sites.
 *  Callers that can do better should pass their own short, localized title plus
 *  the detail in `description` — that path is always respected verbatim. */
const VARIANT_HEADLINE: Record<ToastVariant, string> = {
  message: 'Notice',
  success: 'Success',
  error: 'Error',
  warning: 'Warning',
  info: 'Info',
}

/** A message reads as a HEADLINE only when it is short AND free of long
 *  unbreakable tokens. A long sentence, or any 32+ char unbroken run (path /
 *  URL / token / hash), is body copy that belongs in the description instead. */
function isHeadline(message: string): boolean {
  return message.length <= 56 && !/\S{32,}/.test(message)
}

// Single source of truth: a module-level reactive list rendered by <Toaster>.
export const toasts = reactive<ToastRecord[]>([])

// Non-reactive timer state, keyed by id.
const timers = new Map<string, TimerEntry>()

let seq = 0
function uid(): string {
  seq += 1
  return `toast-${Date.now().toString(36)}-${seq.toString(36)}`
}

function clearTimer(entry: TimerEntry | undefined) {
  if (entry?.handle) {
    clearTimeout(entry.handle)
    entry.handle = undefined
  }
}

function startTimer(id: string) {
  const entry = timers.get(id)
  if (!entry || entry.duration <= 0 || !Number.isFinite(entry.duration)) return
  clearTimer(entry)
  entry.startedAt = Date.now()
  entry.handle = setTimeout(() => dismiss(id), entry.remaining)
}

/** Remove a toast (and its timer). No id → remove all. */
export function dismiss(id?: string | number) {
  if (id == null) {
    for (const entry of timers.values()) clearTimer(entry)
    timers.clear()
    toasts.splice(0, toasts.length)
    return
  }
  const key = String(id)
  clearTimer(timers.get(key))
  timers.delete(key)
  const index = toasts.findIndex(t => t.id === key)
  if (index !== -1) toasts.splice(index, 1)
}

/** Freeze every running timer (hover / tab hidden), banking remaining time. */
export function pauseAll() {
  const now = Date.now()
  for (const entry of timers.values()) {
    if (entry.handle) {
      clearTimeout(entry.handle)
      entry.handle = undefined
      entry.remaining = Math.max(0, entry.remaining - (now - entry.startedAt))
    }
  }
}

/** Resume every paused timer from its banked remaining time. */
export function resumeAll() {
  for (const id of timers.keys()) {
    const entry = timers.get(id)
    if (entry && entry.duration > 0 && Number.isFinite(entry.duration) && !entry.handle) {
      startTimer(id)
    }
  }
}

function create(variant: ToastVariant, message: string, options?: ToastOptions): string {
  const id = options?.id != null ? String(options.id) : uid()
  const duration = options?.duration ?? DEFAULT_DURATION

  // Respect an explicit title+description as-is; only auto-shape a long,
  // titleless blob into heading + gray description.
  let title = message
  let description = options?.description
  let autoHeading = false
  if (description == null && !isHeadline(message)) {
    title = VARIANT_HEADLINE[variant]
    description = message
    autoHeading = true
  }

  const record: ToastRecord = {
    id,
    variant,
    title,
    description,
    action: options?.action,
    autoHeading,
  }

  const existing = toasts.find(t => t.id === id)
  if (existing) Object.assign(existing, record)
  else toasts.push(record)

  timers.set(id, { duration, remaining: duration, startedAt: 0, handle: undefined })
  startTimer(id)
  return id
}

type ToastFn = (message: string, options?: ToastOptions) => string

export interface ToastApi extends ToastFn {
  message: ToastFn
  success: ToastFn
  error: ToastFn
  info: ToastFn
  warning: ToastFn
  dismiss: (id?: string | number) => void
}

// Callable + namespaced, mirroring the `toast()` / `toast.success()` surface the
// app already uses (231 call sites) so nothing downstream changes.
export const toast: ToastApi = Object.assign(
  ((message, options) => create('message', message, options)) as ToastFn,
  {
    message: (message, options) => create('message', message, options),
    success: (message, options) => create('success', message, options),
    error: (message, options) => create('error', message, options),
    info: (message, options) => create('info', message, options),
    warning: (message, options) => create('warning', message, options),
    dismiss,
  } satisfies Omit<ToastApi, keyof ToastFn>,
)
