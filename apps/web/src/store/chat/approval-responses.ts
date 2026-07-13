export type ApprovalResponseOutcome = 'succeeded' | 'failed' | 'canceled' | 'expired'

export interface ApprovalResponse {
  readonly streamId: string
  readonly approvalId: string
  readonly botId: string
  readonly sessionId: string
  readonly silent: boolean
  readonly decision?: 'approve' | 'reject'
  readonly shortId?: number
  awaitingResync: boolean
  replaySent: boolean
  replayFailed: boolean
}

export interface BeginApprovalResponseInput {
  streamId: string
  approvalId: string
  botId: string
  sessionId: string
  silent: boolean
  decision?: 'approve' | 'reject'
  shortId?: number
  rollback?: () => void
}

interface PendingApprovalResponse extends ApprovalResponse {
  startedAt: number
  cancelExpiry?: () => void
  rollback?: () => void
}

type ScheduleExpiry = (callback: () => void, delayMs: number) => () => void

interface ApprovalResponseTrackerDeps {
  rollbackApproval: (approvalId: string) => void
  now?: () => number
  ttlMs?: number
  terminalHistoryLimit?: number
  scheduleExpiry?: ScheduleExpiry
  onExpired?: (response: ApprovalResponse) => void
}

const DEFAULT_TTL_MS = 2 * 60 * 1000
const DEFAULT_TERMINAL_HISTORY_LIMIT = 512

const defaultScheduleExpiry: ScheduleExpiry = (callback, delayMs) => {
  const timer = setTimeout(callback, delayMs)
  return () => clearTimeout(timer)
}

export function createApprovalResponseTracker({
  rollbackApproval,
  now = Date.now,
  ttlMs = DEFAULT_TTL_MS,
  terminalHistoryLimit = DEFAULT_TERMINAL_HISTORY_LIMIT,
  scheduleExpiry = defaultScheduleExpiry,
  onExpired = () => {},
}: ApprovalResponseTrackerDeps) {
  const responses = new Map<string, PendingApprovalResponse>()
  const terminalResponseIds = new Set<string>()

  function rememberTerminalResponse(streamId: string) {
    terminalResponseIds.add(streamId)
    if (terminalResponseIds.size <= terminalHistoryLimit) return
    const oldest = terminalResponseIds.values().next().value
    if (oldest) terminalResponseIds.delete(oldest)
  }

  function expireResponse(streamId: string) {
    const response = responses.get(streamId)
    if (!response) return
    const remaining = ttlMs - (now() - response.startedAt)
    if (remaining > 0) {
      response.cancelExpiry = scheduleExpiry(() => expireResponse(streamId), remaining)
      return
    }
    if (response.awaitingResync) {
      response.cancelExpiry = undefined
      return
    }
    const expired = settleApprovalResponse(streamId, 'expired')
    if (expired) onExpired(expired)
  }

  function expireStaleResponses() {
    const currentTime = now()
    for (const [streamId, response] of responses) {
      if (currentTime - response.startedAt < ttlMs) continue
      expireResponse(streamId)
    }
  }

  function hasPendingApprovalResponse(approvalId: string): boolean {
    const id = approvalId.trim()
    if (!id) return false
    expireStaleResponses()
    for (const response of responses.values()) {
      if (response.approvalId === id) return true
    }
    return false
  }

  function beginApprovalResponse(input: BeginApprovalResponseInput): boolean {
    const streamId = input.streamId.trim()
    const approvalId = input.approvalId.trim()
    const botId = input.botId.trim()
    const sessionId = input.sessionId.trim()
    if (!streamId || !approvalId || !botId || !sessionId) return false
    expireStaleResponses()
    if (responses.has(streamId) || hasPendingApprovalResponse(approvalId)) return false
    if (terminalResponseIds.has(streamId)) return false
    const response: PendingApprovalResponse = {
      streamId,
      approvalId,
      botId,
      sessionId,
      silent: input.silent,
      decision: input.decision,
      shortId: input.shortId,
      awaitingResync: false,
      replaySent: false,
      replayFailed: false,
      startedAt: now(),
      rollback: input.rollback,
    }
    responses.set(streamId, response)
    response.cancelExpiry = scheduleExpiry(() => expireResponse(streamId), ttlMs)
    return true
  }

  function getApprovalResponse(streamId: string): ApprovalResponse | undefined {
    return responses.get(streamId.trim())
  }

  function settleApprovalResponse(streamId: string, outcome: ApprovalResponseOutcome): ApprovalResponse | undefined {
    const id = streamId.trim()
    const response = responses.get(id)
    if (!response) return undefined
    responses.delete(id)
    response.cancelExpiry?.()
    rememberTerminalResponse(id)
    if (outcome === 'failed' || outcome === 'expired') {
      if (response.rollback) response.rollback()
      else rollbackApproval(response.approvalId)
    }
    return response
  }

  function pendingApprovalResponses(): ApprovalResponse[] {
    return [...responses.values()]
  }

  function markPendingApprovalResponsesUncertain(botId: string) {
    const id = botId.trim()
    if (!id) return
    for (const response of responses.values()) {
      if (response.botId === id) {
        response.awaitingResync = true
        response.replaySent = false
        response.replayFailed = false
      }
    }
  }

  function markApprovalResponseReplaySent(streamId: string): boolean {
    const response = responses.get(streamId.trim())
    if (!response || !response.awaitingResync || response.replaySent) return false
    response.replaySent = true
    return true
  }

  function markApprovalResponseReplayFailed(streamId: string): boolean {
    const response = responses.get(streamId.trim())
    if (!response || !response.awaitingResync || !response.replaySent) return false
    response.replayFailed = true
    return true
  }

  function hasUncertainApprovalResponse(botId: string, sessionId: string): boolean {
    const bid = botId.trim()
    const sid = sessionId.trim()
    if (!bid || !sid) return false
    for (const response of responses.values()) {
      if (response.awaitingResync && response.botId === bid && response.sessionId === sid) return true
    }
    return false
  }

  function pendingApprovalResponsesForSession(botId: string, sessionId: string): ApprovalResponse[] {
    const bid = botId.trim()
    const sid = sessionId.trim()
    if (!bid || !sid) return []
    return pendingApprovalResponses().filter(response => response.botId === bid && response.sessionId === sid)
  }

  function discardAllApprovalResponses(): ApprovalResponse[] {
    const pending = pendingApprovalResponses()
    for (const response of pending) settleApprovalResponse(response.streamId, 'canceled')
    return pending
  }

  function isTerminalApprovalResponse(streamId: string | undefined): boolean {
    const id = streamId?.trim()
    return Boolean(id && terminalResponseIds.has(id))
  }

  function resetApprovalResponses() {
    for (const response of responses.values()) response.cancelExpiry?.()
    responses.clear()
    terminalResponseIds.clear()
  }

  return {
    hasPendingApprovalResponse,
    beginApprovalResponse,
    getApprovalResponse,
    settleApprovalResponse,
    pendingApprovalResponses,
    markPendingApprovalResponsesUncertain,
    markApprovalResponseReplaySent,
    markApprovalResponseReplayFailed,
    hasUncertainApprovalResponse,
    pendingApprovalResponsesForSession,
    discardAllApprovalResponses,
    isTerminalApprovalResponse,
    resetApprovalResponses,
  }
}
