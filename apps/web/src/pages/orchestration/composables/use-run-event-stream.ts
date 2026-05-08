import { onBeforeUnmount, ref, watch, type Ref } from 'vue'

export interface RunEventStreamEvent {
  type: string
  seq?: number
  run_id?: string
  task_id?: string
  attempt_id?: string
  payload?: Record<string, unknown>
  [key: string]: unknown
}

export type RunEventStreamStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error'

interface UseRunEventStreamOptions {
  runId: Ref<string>
  // Whether the stream should be open. Re-checked when runId changes and
  // on every event, so callers can drop it as soon as the run terminates.
  enabled: Ref<boolean>
  // Called once per decoded event. Returning a Promise is fine; we do not
  // await it, so handlers must be safe to fire-and-forget.
  onEvent: (event: RunEventStreamEvent) => void | Promise<void>
  // Override for the API base. Defaults to the /api proxy.
  baseUrl?: string
}

const DEFAULT_BASE_URL = '/api'

// Reconnect backoff in ms. After the last value we keep retrying at that
// interval until enabled flips off. Numbers are intentionally relaxed: the
// server still has every event in Postgres, so a delayed reconnect just
// triggers one backfill page.
const RECONNECT_BACKOFF_MS = [500, 1000, 2000, 4000, 8000]

/**
 * Subscribes to /orchestration/runs/{runId}/watch over fetch + ReadableStream.
 * EventSource is not used because it cannot send an Authorization header.
 * Each decoded event goes to onEvent; status is exposed so the host page can
 * show a connection indicator.
 *
 * The stream closes itself when:
 *   - runId becomes empty
 *   - enabled flips to false
 *   - the component unmounts
 *   - the server closes the response
 */
export function useRunEventStream(options: UseRunEventStreamOptions) {
  const { runId, enabled, onEvent } = options
  const baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/$/, '')

  const status = ref<RunEventStreamStatus>('idle')
  const lastEventAt = ref<Date | null>(null)
  const lastError = ref<unknown>(null)

  let controller: AbortController | null = null
  let reconnectTimer: number | null = null
  let reconnectAttempt = 0
  let lastSeq = 0
  let activeRunId = ''

  function clearReconnect() {
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function close() {
    clearReconnect()
    if (controller) {
      controller.abort()
      controller = null
    }
    status.value = 'closed'
  }

  function scheduleReconnect() {
    if (!enabled.value || !activeRunId) return
    const delay = RECONNECT_BACKOFF_MS[Math.min(reconnectAttempt, RECONNECT_BACKOFF_MS.length - 1)]
    reconnectAttempt += 1
    reconnectTimer = window.setTimeout(() => {
      reconnectTimer = null
      void open()
    }, delay)
  }

  async function open() {
    if (!enabled.value || !activeRunId) {
      close()
      return
    }
    clearReconnect()
    if (controller) controller.abort()
    const localController = new AbortController()
    controller = localController

    const url = `${baseUrl}/orchestration/runs/${encodeURIComponent(activeRunId)}/watch${
      lastSeq > 0 ? `?after_seq=${lastSeq}` : ''
    }`
    const headers: HeadersInit = {
      Accept: 'text/event-stream',
    }
    const token = localStorage.getItem('token')
    if (token) headers.Authorization = `Bearer ${token}`

    status.value = 'connecting'
    let response: Response
    try {
      response = await fetch(url, {
        method: 'GET',
        headers,
        signal: localController.signal,
      })
    } catch (err) {
      if (localController.signal.aborted) return
      lastError.value = err
      status.value = 'error'
      scheduleReconnect()
      return
    }

    if (!response.ok || !response.body) {
      lastError.value = new Error(`watch stream returned ${response.status}`)
      status.value = 'error'
      scheduleReconnect()
      return
    }

    status.value = 'open'
    reconnectAttempt = 0
    const reader = response.body.pipeThrough(new TextDecoderStream()).getReader()
    let buffer = ''

    try {
      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buffer += value
        const events = buffer.split('\n\n')
        buffer = events.pop() ?? ''
        for (const block of events) {
          dispatchBlock(block)
        }
      }
    } catch (err) {
      if (!localController.signal.aborted) {
        lastError.value = err
        status.value = 'error'
        scheduleReconnect()
      }
      return
    }

    if (!localController.signal.aborted) {
      // Server closed cleanly. Reconnect so the live tail stays open until
      // the host disables it.
      status.value = 'closed'
      scheduleReconnect()
    }
  }

  function dispatchBlock(block: string) {
    const trimmed = block.trim()
    if (!trimmed) return
    let payload: string | null = null
    for (const line of trimmed.split('\n')) {
      if (line.startsWith('data:')) {
        payload = line.slice(5).trim()
        break
      }
    }
    if (!payload) return
    let parsed: RunEventStreamEvent
    try {
      parsed = JSON.parse(payload) as RunEventStreamEvent
    } catch {
      return
    }
    if (parsed.type === 'ping') return
    if (typeof parsed.seq === 'number' && parsed.seq > lastSeq) {
      lastSeq = parsed.seq
    }
    lastEventAt.value = new Date()
    void onEvent(parsed)
  }

  watch(
    [runId, enabled],
    ([newRunId, newEnabled]) => {
      const trimmed = (newRunId ?? '').trim()
      if (trimmed !== activeRunId) {
        // New run: reset the cursor so the next stream backfills from the
        // start of its own history.
        lastSeq = 0
        reconnectAttempt = 0
      }
      activeRunId = trimmed
      if (!trimmed || !newEnabled) {
        close()
        status.value = 'idle'
        return
      }
      void open()
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    close()
  })

  return {
    status,
    lastEventAt,
    lastError,
  }
}
