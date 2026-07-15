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

  function responseKey(botId: string, sessionId: string, streamId: string) {
    return `${botId.trim()}\u0000${sessionId.trim()}\u0000${streamId.trim()}`
  }

  function findResponse(streamId: string, botId?: string, sessionId?: string) {
    const id = streamId.trim()
    if (!id) return undefined
    if (botId !== undefined && sessionId !== undefined) {
      return responses.get(responseKey(botId, sessionId, id))
    }
    const matches = [...responses.values()].filter(response => response.streamId === id
      && (botId === undefined || response.botId === botId.trim())
      && (sessionId === undefined || response.sessionId === sessionId.trim()))
    return matches.length === 1 ? matches[0] : undefined
  }

  function rememberTerminalResponse(response: ApprovalResponse) {
    terminalResponseIds.add(responseKey(response.botId, response.sessionId, response.streamId))
    if (terminalResponseIds.size <= terminalHistoryLimit) return
    const oldest = terminalResponseIds.values().next().value
    if (oldest) terminalResponseIds.delete(oldest)
  }

  function expireResponse(key: string) {
    const response = responses.get(key)
    if (!response) return
    const remaining = ttlMs - (now() - response.startedAt)
    if (remaining > 0) {
      response.cancelExpiry = scheduleExpiry(() => expireResponse(key), remaining)
      return
    }
    const expired = settleApprovalResponse(response.streamId, 'expired', response.botId, response.sessionId)
    if (expired) onExpired(expired)
  }

  function expireStaleResponses() {
    const currentTime = now()
    for (const [key, response] of responses) {
      if (currentTime - response.startedAt < ttlMs) continue
      expireResponse(key)
    }
  }

  function hasPendingApprovalResponse(approvalId: string, botId?: string, sessionId?: string): boolean {
    const id = approvalId.trim()
    if (!id) return false
    expireStaleResponses()
    for (const response of responses.values()) {
      if (
        response.approvalId === id
        && (botId === undefined || response.botId === botId.trim())
        && (sessionId === undefined || response.sessionId === sessionId.trim())
      ) return true
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
    const key = responseKey(botId, sessionId, streamId)
    if (responses.has(key) || hasPendingApprovalResponse(approvalId, botId, sessionId)) return false
    if (terminalResponseIds.has(key)) return false
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
    responses.set(key, response)
    response.cancelExpiry = scheduleExpiry(() => expireResponse(key), ttlMs)
    return true
  }

  function getApprovalResponse(streamId: string, botId?: string, sessionId?: string): ApprovalResponse | undefined {
    return findResponse(streamId, botId, sessionId)
  }

  function settleApprovalResponse(streamId: string, outcome: ApprovalResponseOutcome, botId?: string, sessionId?: string): ApprovalResponse | undefined {
    const response = findResponse(streamId, botId, sessionId)
    if (!response) return undefined
    responses.delete(responseKey(response.botId, response.sessionId, response.streamId))
    response.cancelExpiry?.()
    rememberTerminalResponse(response)
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

  function markApprovalResponseReplaySent(streamId: string, botId?: string, sessionId?: string): boolean {
    const response = findResponse(streamId, botId, sessionId)
    if (!response || !response.awaitingResync || response.replaySent) return false
    response.replaySent = true
    return true
  }

  function markApprovalResponseReplayFailed(streamId: string, botId?: string, sessionId?: string): boolean {
    const response = findResponse(streamId, botId, sessionId)
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
    for (const response of pending) settleApprovalResponse(response.streamId, 'canceled', response.botId, response.sessionId)
    return pending
  }

  function isTerminalApprovalResponse(streamId: string | undefined, botId?: string, sessionId?: string): boolean {
    const id = streamId?.trim()
    if (!id) return false
    if (botId !== undefined && sessionId !== undefined) {
      return terminalResponseIds.has(responseKey(botId, sessionId, id))
    }
    const bid = botId?.trim()
    const sid = sessionId?.trim()
    const matches = [...terminalResponseIds].filter((key) => {
      const [candidateBotId, candidateSessionId, candidateStreamId] = key.split('\u0000')
      return candidateStreamId === id
        && (bid === undefined || candidateBotId === bid)
        && (sid === undefined || candidateSessionId === sid)
    })
    return matches.length === 1
  }

  function resolveApprovalResponseIdentity(streamId: string, botId?: string, sessionId?: string) {
    const id = streamId.trim()
    if (!id) return undefined
    const bid = botId?.trim()
    const sid = sessionId?.trim()
    const keys = new Set<string>()
    for (const response of responses.values()) {
      if (
        response.streamId === id
        && (bid === undefined || response.botId === bid)
        && (sid === undefined || response.sessionId === sid)
      ) keys.add(responseKey(response.botId, response.sessionId, response.streamId))
    }
    for (const key of terminalResponseIds) {
      const [candidateBotId, candidateSessionId, candidateStreamId] = key.split('\u0000')
      if (
        candidateStreamId === id
        && (bid === undefined || candidateBotId === bid)
        && (sid === undefined || candidateSessionId === sid)
      ) keys.add(key)
    }
    if (keys.size !== 1) return undefined
    const [resolvedBotId, resolvedSessionId] = [...keys][0]!.split('\u0000')
    return { botId: resolvedBotId!, sessionId: resolvedSessionId! }
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
    resolveApprovalResponseIdentity,
    resetApprovalResponses,
  }
}
