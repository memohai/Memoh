import type {
  CommandEventResponse,
  UIRuntimeStateEvent,
  UIStreamEvent,
  WSClientMessage,
} from '@/composables/api/useChat'

export interface RuntimeSubscriptionRef {
  botId: string
  sessionId: string
}

export type RuntimeSubscriptionApplyResult = {
  kind: 'applied' | 'ignored' | 'resync'
}

export interface RuntimeSubscriptionTransport {
  ensureConnected: (botId: string) => boolean
  send: (botId: string, message: WSClientMessage) => boolean
}

export interface RuntimeSubscriptionCallbacks {
  awaitCheckpoint: (ref: RuntimeSubscriptionRef) => void
  release: (ref: RuntimeSubscriptionRef) => void
  apply: (
    ref: RuntimeSubscriptionRef,
    event: Exclude<UIRuntimeStateEvent, { type: 'runtime_dropped' }>,
  ) => RuntimeSubscriptionApplyResult | Promise<RuntimeSubscriptionApplyResult>
}

export interface RuntimeSubscriptionDependencies {
  transport: RuntimeSubscriptionTransport
  runtime: RuntimeSubscriptionCallbacks
  createInvocationId: () => string
  retryDelay?: (attempt: number, ref: RuntimeSubscriptionRef) => number
  onError?: (ref: RuntimeSubscriptionRef, error: unknown) => void
}

export type RuntimeSubscriptionPhase = 'idle' | 'subscribing' | 'subscribed' | 'retry_wait'

interface RuntimeSubscriptionEntry {
  ref: RuntimeSubscriptionRef
  phase: RuntimeSubscriptionPhase
  awaitingCheckpoint: boolean
  resyncPending: boolean
  eventGeneration: number
  commandGeneration: number
  currentInvocationId: string
  wireActive: boolean
  retryAttempt: number
  retryTimer: ReturnType<typeof setTimeout> | null
}

interface PendingSubscriptionCommand {
  key: string
  action: 'subscribe' | 'unsubscribe'
  generation: number
}

function normalizeRef(ref: RuntimeSubscriptionRef): RuntimeSubscriptionRef | null {
  const botId = ref.botId.trim()
  const sessionId = ref.sessionId.trim()
  return botId && sessionId ? { botId, sessionId } : null
}

function subscriptionKey(ref: RuntimeSubscriptionRef) {
  return `${ref.botId}\u0000${ref.sessionId}`
}

function commandKey(botId: string, sessionId: string, invocationId: string) {
  return `${botId}\u0000${sessionId}\u0000${invocationId}`
}

function defaultRetryDelay(attempt: number) {
  const base = Math.min(1000 * 2 ** Math.min(Math.max(attempt - 1, 0), 5), 30_000)
  const jitterWindow = Math.max(1, Math.floor(base / 5))
  return base - jitterWindow + Math.floor(Math.random() * (jitterWindow + 1))
}

function isRuntimeStateEvent(event: UIStreamEvent): event is UIRuntimeStateEvent {
  return event.type === 'runtime_snapshot'
    || event.type === 'runtime_delta'
    || event.type === 'runtime_dropped'
}

function isCommandEvent(event: UIStreamEvent): event is CommandEventResponse {
  return event.type === 'command_result' || event.type === 'command_error'
}

function isPromiseLike<T>(value: T | Promise<T>): value is Promise<T> {
  return typeof (value as Promise<T>)?.then === 'function'
}

// Owns only runtime subscription intent and recovery. Wire parsing stays in the
// transport, while reducer/projection effects stay behind the runtime callbacks.
export function createRuntimeSubscriptionController({
  transport,
  runtime,
  createInvocationId,
  retryDelay = defaultRetryDelay,
  onError,
}: RuntimeSubscriptionDependencies) {
  const entries = new Map<string, RuntimeSubscriptionEntry>()
  const pendingCommands = new Map<string, PendingSubscriptionCommand>()
  let disposed = false

  function reportError(ref: RuntimeSubscriptionRef, error: unknown) {
    onError?.(ref, error)
  }

  function cancelRetry(entry: RuntimeSubscriptionEntry) {
    if (entry.retryTimer) clearTimeout(entry.retryTimer)
    entry.retryTimer = null
  }

  function pendingCommand(entry: RuntimeSubscriptionEntry) {
    if (!entry.currentInvocationId) return undefined
    return pendingCommands.get(commandKey(entry.ref.botId, entry.ref.sessionId, entry.currentInvocationId))
  }

  function supersedeCurrentCommand(entry: RuntimeSubscriptionEntry) {
    const pending = pendingCommand(entry)
    if (pending) pending.generation = -1
    entry.currentInvocationId = ''
  }

  function beginCheckpoint(entry: RuntimeSubscriptionEntry, invalidate = false) {
    if (entry.awaitingCheckpoint && (!invalidate || entry.resyncPending)) return false
    entry.awaitingCheckpoint = true
    entry.resyncPending = invalidate
    entry.eventGeneration += 1
    try {
      runtime.awaitCheckpoint(entry.ref)
    } catch (error) {
      reportError(entry.ref, error)
    }
    return true
  }

  function scheduleRetry(entry: RuntimeSubscriptionEntry) {
    if (disposed || entries.get(subscriptionKey(entry.ref)) !== entry || entry.retryTimer) return
    entry.phase = 'retry_wait'
    entry.retryAttempt += 1
    const delay = Math.max(0, retryDelay(entry.retryAttempt, entry.ref))
    entry.retryTimer = setTimeout(() => {
      entry.retryTimer = null
      if (disposed || entries.get(subscriptionKey(entry.ref)) !== entry) return
      subscribe(entry, true)
    }, delay)
  }

  function subscribe(entry: RuntimeSubscriptionEntry, force = false) {
    if (disposed || entries.get(subscriptionKey(entry.ref)) !== entry) return
    if (!force && (entry.phase === 'subscribing' || entry.phase === 'subscribed')) return
    beginCheckpoint(entry)
    if (force) cancelRetry(entry)

    let connected = false
    try {
      connected = transport.ensureConnected(entry.ref.botId)
    } catch (error) {
      reportError(entry.ref, error)
      scheduleRetry(entry)
      return
    }
    if (!connected) {
      entry.phase = 'idle'
      return
    }

    supersedeCurrentCommand(entry)
    const invocationId = createInvocationId().trim()
    if (!invocationId) {
      reportError(entry.ref, new Error('runtime subscription invocation id is required'))
      scheduleRetry(entry)
      return
    }
    entry.commandGeneration += 1
    entry.currentInvocationId = invocationId
    entry.phase = 'subscribing'
    const pendingKey = commandKey(entry.ref.botId, entry.ref.sessionId, invocationId)
    pendingCommands.set(pendingKey, {
      key: subscriptionKey(entry.ref),
      action: 'subscribe',
      generation: entry.commandGeneration,
    })

    try {
      if (transport.send(entry.ref.botId, {
        type: 'runtime_subscribe',
        invocation_id: invocationId,
        stream_id: invocationId,
        session_id: entry.ref.sessionId,
      })) {
        entry.wireActive = true
        return
      }
      throw new Error('WebSocket is not connected')
    } catch (error) {
      pendingCommands.delete(pendingKey)
      entry.currentInvocationId = ''
      reportError(entry.ref, error)
      scheduleRetry(entry)
    }
  }

  function markRecovered(entry: RuntimeSubscriptionEntry) {
    entry.awaitingCheckpoint = false
    entry.resyncPending = false
    entry.phase = 'subscribed'
    entry.wireActive = true
    entry.retryAttempt = 0
    cancelRetry(entry)
    // The server sends the checkpoint before the subscribe command result. Once
    // accepted, that checkpoint is stronger evidence than a delayed reply.
    supersedeCurrentCommand(entry)
  }

  function requestCheckpoint(ref: RuntimeSubscriptionRef, invalidate = false) {
    const normalized = normalizeRef(ref)
    if (!normalized) return
    const entry = entries.get(subscriptionKey(normalized))
    if (!entry || !beginCheckpoint(entry, invalidate)) return
    subscribe(entry, true)
  }

  function finishRuntimeApply(
    entry: RuntimeSubscriptionEntry,
    eventGeneration: number,
    result: RuntimeSubscriptionApplyResult,
  ) {
    if (
      disposed
      || entries.get(subscriptionKey(entry.ref)) !== entry
      || entry.eventGeneration !== eventGeneration
    ) return
    if (result.kind === 'resync') {
      requestCheckpoint(entry.ref, true)
      return
    }
    if (result.kind === 'applied') markRecovered(entry)
  }

  function failRuntimeApply(entry: RuntimeSubscriptionEntry, eventGeneration: number, error: unknown) {
    if (
      disposed
      || entries.get(subscriptionKey(entry.ref)) !== entry
      || entry.eventGeneration !== eventGeneration
    ) return
    reportError(entry.ref, error)
    requestCheckpoint(entry.ref, true)
  }

  function applyRuntimeEvent(
    entry: RuntimeSubscriptionEntry,
    event: Exclude<UIRuntimeStateEvent, { type: 'runtime_dropped' }>,
  ) {
    const eventGeneration = entry.eventGeneration
    try {
      const result = runtime.apply(entry.ref, event)
      if (isPromiseLike(result)) {
        void result.then(
          reduction => finishRuntimeApply(entry, eventGeneration, reduction),
          error => failRuntimeApply(entry, eventGeneration, error),
        )
        return
      }
      finishRuntimeApply(entry, eventGeneration, result)
    } catch (error) {
      failRuntimeApply(entry, eventGeneration, error)
    }
  }

  function handleRuntimeEvent(sourceBotId: string, event: UIRuntimeStateEvent) {
    const source = sourceBotId.trim()
    const declaredBotId = (event.bot_id ?? '').trim()
    const sessionId = (event.session_id ?? '').trim()
    const botId = source || declaredBotId
    if (!botId || !sessionId) return

    const ref = { botId, sessionId }
    const entry = entries.get(subscriptionKey(ref))
    if (!entry) return
    if (source && declaredBotId && source !== declaredBotId) {
      requestCheckpoint(ref, true)
      return
    }
    if (event.type === 'runtime_dropped') {
      requestCheckpoint(ref)
      return
    }
    applyRuntimeEvent(entry, event)
  }

  function handleCommandEvent(sourceBotId: string, event: CommandEventResponse) {
    const botId = sourceBotId.trim()
    const invocationId = event.invocation_id?.trim() ?? ''
    if (!botId || !invocationId) return false
    const declaredSessionId = event.session_id?.trim() ?? ''
    let pendingKey = declaredSessionId
      ? commandKey(botId, declaredSessionId, invocationId)
      : ''
    if (!pendingKey) {
      const prefix = `${botId}\u0000`
      const suffix = `\u0000${invocationId}`
      const matches = [...pendingCommands.keys()].filter(key => key.startsWith(prefix) && key.endsWith(suffix))
      if (matches.length !== 1) return false
      pendingKey = matches[0]!
    }
    const pending = pendingCommands.get(pendingKey)
    if (!pending) return false

    const expectedAction = pending.action === 'subscribe' ? 'runtime_subscribe' : 'runtime_unsubscribe'
    if (event.action_id?.trim() && event.action_id !== expectedAction) return false
    pendingCommands.delete(pendingKey)
    if (pending.action === 'unsubscribe') return true

    const entry = entries.get(pending.key)
    if (
      !entry
      || pending.generation !== entry.commandGeneration
      || entry.currentInvocationId !== invocationId
    ) return true
    entry.currentInvocationId = ''
    if (event.type === 'command_error') {
      entry.wireActive = false
      reportError(entry.ref, new Error(event.error?.message || 'runtime subscription failed'))
      scheduleRetry(entry)
      return true
    }
    entry.phase = entry.awaitingCheckpoint ? 'subscribing' : 'subscribed'
    return true
  }

  function handleEvent(sourceBotId: string, event: UIStreamEvent) {
    if (isRuntimeStateEvent(event)) {
      handleRuntimeEvent(sourceBotId, event)
      return true
    }
    if (isCommandEvent(event)) return handleCommandEvent(sourceBotId, event)
    return false
  }

  function sendUnsubscribe(entry: RuntimeSubscriptionEntry) {
    if (!entry.wireActive) return
    const invocationId = createInvocationId().trim()
    if (!invocationId) return
    const pendingKey = commandKey(entry.ref.botId, entry.ref.sessionId, invocationId)
    pendingCommands.set(pendingKey, {
      key: subscriptionKey(entry.ref),
      action: 'unsubscribe',
      generation: entry.commandGeneration,
    })
    try {
      if (transport.send(entry.ref.botId, {
        type: 'runtime_unsubscribe',
        invocation_id: invocationId,
        stream_id: invocationId,
        session_id: entry.ref.sessionId,
      })) return
    } catch {
      // Local ownership has already ended; reconnect drops the server-side sub.
    }
    pendingCommands.delete(pendingKey)
  }

  function releaseEntry(entry: RuntimeSubscriptionEntry, notifyServer: boolean) {
    const key = subscriptionKey(entry.ref)
    if (entries.get(key) !== entry) return
    entries.delete(key)
    entry.eventGeneration += 1
    cancelRetry(entry)
    supersedeCurrentCommand(entry)
    if (notifyServer) sendUnsubscribe(entry)
    runtime.release(entry.ref)
  }

  function reconcile(desiredRefs: Iterable<RuntimeSubscriptionRef>) {
    if (disposed) return
    const desired = new Map<string, RuntimeSubscriptionRef>()
    for (const ref of desiredRefs) {
      const normalized = normalizeRef(ref)
      if (normalized) desired.set(subscriptionKey(normalized), normalized)
    }

    for (const entry of [...entries.values()]) {
      if (!desired.has(subscriptionKey(entry.ref))) releaseEntry(entry, true)
    }
    for (const [key, ref] of desired) {
      let entry = entries.get(key)
      if (!entry) {
        entry = {
          ref,
          phase: 'idle',
          awaitingCheckpoint: false,
          resyncPending: false,
          eventGeneration: 0,
          commandGeneration: 0,
          currentInvocationId: '',
          wireActive: false,
          retryAttempt: 0,
          retryTimer: null,
        }
        entries.set(key, entry)
      }
      if (entry.phase === 'idle') subscribe(entry)
    }
  }

  function remove(ref: RuntimeSubscriptionRef) {
    const normalized = normalizeRef(ref)
    if (!normalized) return
    const entry = entries.get(subscriptionKey(normalized))
    if (entry) releaseEntry(entry, true)
  }

  function clearPendingCommandsForBot(botId: string) {
    const prefix = `${botId}\u0000`
    for (const key of pendingCommands.keys()) {
      if (key.startsWith(prefix)) pendingCommands.delete(key)
    }
  }

  function handleTransportOpen(botId: string) {
    const bid = botId.trim()
    if (disposed || !bid) return
    clearPendingCommandsForBot(bid)
    for (const entry of entries.values()) {
      if (entry.ref.botId !== bid) continue
      cancelRetry(entry)
      entry.retryAttempt = 0
      entry.currentInvocationId = ''
      entry.wireActive = false
      entry.phase = 'idle'
      entry.resyncPending = false
      beginCheckpoint(entry)
      subscribe(entry, true)
    }
  }

  function handleTransportClose(botId: string) {
    const bid = botId.trim()
    if (disposed || !bid) return
    clearPendingCommandsForBot(bid)
    for (const entry of entries.values()) {
      if (entry.ref.botId !== bid) continue
      cancelRetry(entry)
      entry.currentInvocationId = ''
      entry.wireActive = false
      entry.phase = 'idle'
      beginCheckpoint(entry, true)
    }
  }

  function reset() {
    for (const entry of [...entries.values()]) releaseEntry(entry, false)
    pendingCommands.clear()
  }

  function dispose() {
    if (disposed) return
    reset()
    disposed = true
  }

  return {
    reconcile,
    remove,
    requestCheckpoint,
    handleTransportOpen,
    handleTransportClose,
    handleEvent,
    reset,
    dispose,
  }
}
